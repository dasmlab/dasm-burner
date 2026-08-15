<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">ETCD / Control-plane Diagnoser</div>
        <h1 class="dasm-title">API flex → etcd dies → leftover RSS</h1>
        <p class="dasm-subtitle">
          Density cascade on this lab: kube-apiserver working set climbs first,
          then etcd timeouts, then masters/OVN. After cleanup, API RSS does not
          return to baseline until the static pod restarts. Capture baseline
          before a run; samples during Execute fill the graph.
        </p>
      </div>
    </div>

    <div class="row items-center q-gutter-sm q-mb-sm">
      <q-btn outline color="primary" label="Capture baseline" :loading="busy" :disable="!canAdmin" @click="captureBaseline" />
      <q-btn unelevated color="primary" icon="refresh" label="Sample now" :loading="busy" :disable="!canAdmin" @click="sample" />
      <q-btn flat color="primary" icon="history" label="Reload latest" :loading="busy" @click="loadLatest" />
      <q-badge v-if="snap?.cascade" :color="cascadeColor" text-color="white">{{ snap.cascade }}</q-badge>
      <q-badge v-if="snap?.overallState" :color="stateColor" text-color="white">{{ snap.overallState }}</q-badge>
    </div>
    <p class="text-caption text-grey-7 q-mb-md">
      Off the HTTP path. PVC: <code>etcddiag/series.json</code> + snapshots.
      Workers typically maxPods={{ snap?.maxPodsTypical || 1000 }} · IPs are not the cliff (/22).
    </p>

    <div v-if="notice" class="dasm-panel q-mb-md text-positive">{{ notice }}</div>
    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <q-expansion-item class="dasm-panel q-mb-md iso-exp" dense switch-toggle-side expand-separator label="How to reproduce">
      <ol class="detail-list q-mt-sm">
        <li>Baseline on an empty cluster (or accept leftover RSS as the warmed start).</li>
        <li>Execute density (2R:2S:3P, wave-capped). ETCD + OVN samples on.</li>
        <li>Watch the graph: API RSS climbs while etcd Ready stays 3/3 — that is <code>api_flex</code>.</li>
        <li><code>etcd_flex</code> / <code>collapse</code> is when etcd or masters drop. Abort/settle should fire.</li>
        <li>Cleanup. If API RSS stays high with namespaces gone, that is <code>leftover</code> — restart kube-apiserver only for a cold baseline.</li>
      </ol>
    </q-expansion-item>

    <div v-if="chart.points.length" class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-xs">Working set vs burn pods</div>
      <p class="text-caption q-mb-sm">
        <strong>Working set / RSS</strong> is the RAM the process is actually sitting on right now
        (<code>container_memory_working_set_bytes</code> via metrics.k8s.io) — not etcd DB size, not Go’s idea of “free.”
        kube-apiserver allocates watch-cache under LIST/WATCH; Go keeps that heap. Deletes remove objects;
        they do not give the pages back to the node. That is why the red line can stay high after the brown line drops.
      </p>
      <svg class="rss-chart" viewBox="0 0 640 220" preserveAspectRatio="xMidYMid meet">
        <text x="8" y="16" fill="#6f7f8d" font-size="11">Mi (RSS)</text>
        <line v-if="chart.baseY" x1="10" :y1="chart.baseY" x2="630" :y2="chart.baseY" stroke="#c0392b" stroke-width="1" stroke-dasharray="3 4" opacity="0.45" />
        <polyline fill="none" stroke="#c0392b" stroke-width="2.4" :points="chart.api" />
        <polyline fill="none" stroke="#2980b9" stroke-width="2" :points="chart.etcd" />
        <polyline fill="none" stroke="#27ae60" stroke-width="2" :points="chart.ovn" />
        <polyline fill="none" stroke="#8e6b3a" stroke-width="1.6" stroke-dasharray="4 3" :points="chart.pods" />
      </svg>
      <div class="row q-gutter-md text-caption">
        <span><span class="swatch swatch-api" /> kube-apiserver RSS (the ratchet)</span>
        <span><span class="swatch swatch-etcd" /> etcd RSS</span>
        <span><span class="swatch swatch-ovn" /> ovnkube-controller RSS</span>
        <span><span class="swatch swatch-pods" /> burn pods (scaled to the same height)</span>
      </div>
      <p class="text-caption text-grey-7 q-mt-xs">
        {{ chart.first }} → {{ chart.last }} · {{ chart.points.length }} samples
        <span v-if="snap?.baselineApiserverRssMi"> · baseline API {{ Math.round(snap.baselineApiserverRssMi) }} Mi (dashed red)</span>
      </p>
    </div>
    <div v-else class="dasm-panel q-mb-md text-caption text-grey-7">
      No timeseries yet. Capture baseline, then Sample now (or run Execute with ETCD samples on).
    </div>

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">Cascade — live, not a poster</div>
      <p class="text-caption q-mb-sm">
        Recomputed on every sample from Ready, MemoryPressure, RSS vs baseline, and whether burn namespaces are gone.
        Current stage is filled. Collapse is red. Stages this series already visited stay tinted.
      </p>
      <div class="row q-col-gutter-sm">
        <div v-for="st in stages" :key="st.id" class="col-12 col-sm">
          <div class="stage" :class="stageClass(st.id)">
            <div class="text-weight-medium">{{ st.name }}</div>
            <div class="text-caption">{{ st.see }}</div>
          </div>
        </div>
      </div>
      <p class="text-caption q-mt-sm" v-if="snap?.cascadeWhy">
        <strong>Now:</strong> {{ snap.cascade }} — {{ snap.cascadeWhy }}
      </p>
      <p class="text-caption q-mt-sm">
        North star is <router-link :to="{ name: 'isolation' }">Isolated wave mode</router-link>
        · leftover RSS:
        <router-link :to="{ name: 'investigations', params: { id: 'watch-cache-shrink-without-full' } }">Investigations</router-link>
        ·
        <router-link :to="{ name: 'source-map' }">watch_cache.go on the source map</router-link>.
      </p>
    </div>

    <div v-if="snap" class="dasm-panel q-mb-md">
      <div class="row q-col-gutter-md">
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">Masters</div>
          <div class="text-h6">{{ snap.mastersReady }}/{{ snap.mastersTotal }}</div>
        </div>
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">etcd</div>
          <div class="text-h6">{{ snap.etcdReady }}/{{ snap.etcdTotal }}</div>
        </div>
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">API</div>
          <div class="text-h6">{{ snap.apiserverReady }}/{{ snap.apiserverTotal }}</div>
        </div>
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">API RSS</div>
          <div class="text-h6">{{ rss(snap.apiserverRssMi) }}</div>
        </div>
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">Burn pods</div>
          <div class="text-h6">{{ snap.workloadPods }}</div>
        </div>
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">Metrics</div>
          <div class="text-h6">{{ snap.metricsOk ? 'ok' : 'off' }}</div>
        </div>
      </div>
    </div>

    <div v-if="whyLines.length" class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-xs">Why</div>
      <ul class="detail-list">
        <li v-for="(line, i) in whyLines" :key="i">{{ line }}</li>
      </ul>
    </div>

    <div class="dasm-panel q-mb-md" v-if="(snap?.masters || []).length">
      <div class="dasm-stat-label q-mb-sm">Masters</div>
      <q-markup-table flat dense class="etcd-table">
        <thead>
          <tr>
            <th>Node</th>
            <th>Ready</th>
            <th>MemPressure</th>
            <th>API Mi</th>
            <th>etcd Mi</th>
            <th>OVN Mi</th>
            <th>etcd rst</th>
            <th>API rst</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in snap.masters" :key="m.name">
            <td>{{ shortNode(m.name) }}</td>
            <td><q-badge :color="m.ready ? 'positive' : 'negative'">{{ m.ready ? 'yes' : 'no' }}</q-badge></td>
            <td><q-badge :color="m.memoryPressure ? 'warning' : 'grey-6'">{{ m.memoryPressure ? 'yes' : 'no' }}</q-badge></td>
            <td>{{ rss(m.apiserverRssMi) }}</td>
            <td>{{ rss(m.etcdRssMi) }}</td>
            <td>{{ rss(m.ovnRssMi) }}</td>
            <td>{{ m.etcdRestarts }}</td>
            <td>{{ m.apiserverRestarts }}</td>
          </tr>
        </tbody>
      </q-markup-table>
    </div>

    <div class="dasm-panel q-mb-md" v-if="findings.length">
      <div class="dasm-stat-label q-mb-sm">Findings</div>
      <div v-for="f in findings" :key="f.id" class="q-mb-sm">
        <q-badge :color="sevColor(f.severity)" class="q-mr-sm">{{ f.severity }}</q-badge>
        <strong>{{ f.ruleId }}</strong> — {{ f.summary }}
        <div class="text-caption text-grey-7" v-if="f.why">{{ f.why }}</div>
      </div>
    </div>

    <div class="dasm-panel" v-if="samples.length">
      <div class="dasm-stat-label q-mb-sm">History</div>
      <q-list dense>
        <q-item v-for="s in samples" :key="s.id" clickable @click="openSample(s.id)">
          <q-item-section>
            <q-item-label>
              {{ s.cascade || s.overallState }} · API {{ rss(s.apiserverRssMi) }} · pods {{ s.workloadPods }} · etcd {{ s.etcdReady }}/{{ s.etcdTotal }}
            </q-item-label>
            <q-item-label caption>{{ fmt(s.generatedAt) }} · {{ s.kind }} · {{ s.id }}</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </div>
  </q-page>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import {
  baselineEtcdDiag,
  getEtcdDiag,
  getEtcdDiagSnapshot,
  listEtcdDiagHistory,
  sampleEtcdDiag,
} from 'src/services/api'
import { useAuth } from 'src/services/auth'
import { useCluster } from 'src/services/cluster'

const auth = useAuth()
const canAdmin = computed(() => auth.isAdmin.value)
const cluster = useCluster()
const busy = ref(false)
const error = ref('')
const notice = ref('')
const snap = ref(null)
const samples = ref([])
const series = ref([])
const model = ref(null)

const stages = computed(() => model.value?.stages || [
  { id: 'idle', name: 'Idle', see: 'Ready 3/3, RSS near baseline' },
  { id: 'api_flex', name: 'API flex', see: 'kube-apiserver RSS climbs' },
  { id: 'etcd_flex', name: 'etcd flex', see: 'etcd timeouts / MemoryPressure' },
  { id: 'collapse', name: 'Collapse', see: 'master NotReady / OVN' },
  { id: 'leftover', name: 'Leftover RSS', see: 'pods gone, API RSS still fat' },
])

const stateColor = computed(() => {
  switch (snap.value?.overallState) {
    case 'HEALTHY': return 'positive'
    case 'WARNING': return 'warning'
    case 'CRITICAL':
    case 'FAILED': return 'negative'
    default: return 'grey-6'
  }
})
const cascadeColor = computed(() => {
  switch (snap.value?.cascade) {
    case 'idle': return 'positive'
    case 'api_flex': return 'warning'
    case 'leftover': return 'warning'
    case 'etcd_flex':
    case 'collapse': return 'negative'
    default: return 'grey-6'
  }
})
const findings = computed(() => (snap.value?.findings || []).slice(0, 80))
const whyLines = computed(() => snap.value?.whyLines?.length ? snap.value.whyLines : [])
const seenStages = computed(() => new Set((series.value || []).map((p) => p.cascade).filter(Boolean)))

function stageClass(id) {
  const now = snap.value?.cascade === id
  const seen = seenStages.value.has(id)
  return {
    [`is-${id}`]: true,
    'is-now': now,
    'is-seen': seen && !now,
  }
}

const chart = computed(() => {
  const pts = (series.value || []).slice().sort((a, b) => new Date(a.at) - new Date(b.at))
  const w = 640
  const h = 200
  const pad = 10
  if (!pts.length) return { points: [], api: '', etcd: '', ovn: '', pods: '', first: '', last: '', baseY: 0 }
  const maxRss = Math.max(1, ...pts.map((p) => Math.max(p.apiserverRssMi || 0, p.etcdRssMi || 0, p.ovnRssMi || 0)), snap.value?.baselineApiserverRssMi || 0)
  const maxPods = Math.max(1, ...pts.map((p) => p.workloadPods || 0))
  const x = (i) => pad + (i * (w - 2 * pad)) / Math.max(1, pts.length - 1)
  const yRss = (v) => h - pad - ((v || 0) / maxRss) * (h - 2 * pad)
  const yPods = (v) => h - pad - ((v || 0) / maxPods) * (h - 2 * pad)
  const line = (fn) => pts.map((p, i) => `${x(i).toFixed(1)},${fn(p).toFixed(1)}`).join(' ')
  const base = snap.value?.baselineApiserverRssMi || 0
  return {
    points: pts,
    api: line((p) => yRss(p.apiserverRssMi)),
    etcd: line((p) => yRss(p.etcdRssMi)),
    ovn: line((p) => yRss(p.ovnRssMi)),
    pods: line((p) => yPods(p.workloadPods)),
    first: fmt(pts[0].at),
    last: fmt(pts[pts.length - 1].at),
    baseY: base ? yRss(base) : 0,
  }
})

function rss(v) {
  if (!v) return '—'
  return `${Math.round(v)} Mi`
}
function shortNode(n) {
  if (!n) return '—'
  const i = n.lastIndexOf('-master-')
  return i >= 0 ? n.slice(i + 1) : n
}
function sevColor(sev) {
  if (sev === 'CRITICAL' || sev === 'ERROR') return 'negative'
  if (sev === 'WARNING') return 'warning'
  return 'grey-7'
}
function fmt(at) {
  try { return new Date(at).toLocaleString() } catch { return '' }
}
function applyPayload(data) {
  if (data?.snapshot) snap.value = data.snapshot
  if (data?.samples) samples.value = data.samples
  if (data?.series) series.value = data.series
  if (data?.model) model.value = data.model
}
function errText(e) {
  return e.response?.data?.error || e.message
}

async function loadLatest() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    applyPayload(await getEtcdDiag())
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}
async function openSample(id) {
  if (!id) return
  busy.value = true
  try {
    applyPayload(await getEtcdDiagSnapshot(id))
    notice.value = `Loaded ${id}`
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}
async function waitForSnap(before) {
  const deadline = Date.now() + 90000
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 2000))
    const data = await getEtcdDiag()
    applyPayload(data)
    const now = data.snapshot?.generatedAt || data.samples?.[0]?.id
    if (now && now !== before) return true
  }
  return false
}
async function sample() {
  busy.value = true
  error.value = ''
  notice.value = 'Queued on ETCD worker…'
  const before = snap.value?.generatedAt || samples.value?.[0]?.id
  try {
    await sampleEtcdDiag({})
    notice.value = (await waitForSnap(before)) ? 'Sample stored on PVC.' : 'Still running — Reload latest shortly.'
    const hist = await listEtcdDiagHistory().catch(() => null)
    if (hist?.samples) samples.value = hist.samples
    if (hist?.series) series.value = hist.series
  } catch (e) {
    notice.value = ''
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}
async function captureBaseline() {
  busy.value = true
  error.value = ''
  notice.value = 'Baseline queued…'
  const before = snap.value?.generatedAt || samples.value?.[0]?.id
  try {
    await baselineEtcdDiag()
    notice.value = (await waitForSnap(before)) ? 'Baseline captured on PVC.' : 'Still running — Reload latest shortly.'
  } catch (e) {
    notice.value = ''
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

onMounted(loadLatest)
void cluster
</script>

<style scoped>
.etcd-table { width: 100%; }
.detail-list { margin: 0; padding-left: 1.1rem; }
.iso-exp :deep(.q-item) { padding-left: 0; min-height: 2.2rem; }
.iso-exp :deep(.q-item__label) {
  font-size: 0.78rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #6f7f8d;
  font-weight: 600;
}
.rss-chart { width: 100%; max-height: 240px; background: #f4f7fa; display: block; border-radius: 8px; }
.swatch { display: inline-block; width: 10px; height: 10px; margin-right: 4px; vertical-align: middle; }
.swatch-api { background: #c0392b; }
.swatch-etcd { background: #2980b9; }
.swatch-ovn { background: #27ae60; }
.swatch-pods { background: #8e6b3a; }
.stage { border: 1px solid rgba(28, 52, 73, 0.12); padding: 0.6rem 0.7rem; min-height: 4.2rem; border-radius: 8px; background: #fff; }
.stage.is-now { color: #fff; }
.stage.is-now.is-idle { background: #1f6f62; border-color: #1f6f62; }
.stage.is-now.is-api_flex { background: #c47b12; border-color: #c47b12; }
.stage.is-now.is-etcd_flex { background: #c0392b; border-color: #c0392b; }
.stage.is-now.is-collapse { background: #8b1e1e; border-color: #8b1e1e; }
.stage.is-now.is-leftover { background: #6b4c9a; border-color: #6b4c9a; }
.stage.is-seen { background: #eef3f8; }
.stage.is-now .text-caption { color: rgba(255,255,255,0.85); }
</style>
