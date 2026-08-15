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

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">Reproduce this</div>
      <ol class="detail-list">
        <li>Baseline on an empty cluster (or accept leftover RSS as the warmed start).</li>
        <li>Execute density (2R:2S:3P, wave-capped). ETCD + OVN samples on.</li>
        <li>Watch the graph: API RSS climbs while etcd Ready stays 3/3 — that is <code>api_flex</code>.</li>
        <li><code>etcd_flex</code> / <code>collapse</code> is when etcd or masters drop. Abort/settle should fire.</li>
        <li>Cleanup. If API RSS stays high with pods≈0, that is <code>leftover</code> — restart kube-apiserver only for a cold baseline.</li>
      </ol>
    </div>

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">Cascade</div>
      <div class="row q-col-gutter-sm">
        <div v-for="st in stages" :key="st.id" class="col-12 col-sm">
          <div class="stage" :class="{ 'is-now': snap?.cascade === st.id }">
            <div class="text-weight-medium">{{ st.name }}</div>
            <div class="text-caption text-grey-7">{{ st.see }}</div>
          </div>
        </div>
      </div>
      <p class="text-caption q-mt-sm" v-if="snap?.cascadeWhy">{{ snap.cascadeWhy }}</p>
      <p class="text-caption q-mt-sm">
        North star is <router-link :to="{ name: 'isolation' }">Isolated wave mode</router-link>
        · leftover RSS:
        <router-link :to="{ name: 'investigations', params: { id: 'watch-cache-shrink-without-full' } }">Investigations</router-link>
        ·
        <router-link :to="{ name: 'source-map' }">watch_cache.go on the source map</router-link>.
      </p>
    </div>

    <div v-if="chart.points.length" class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-xs">Control-plane RSS vs workload pods</div>
      <p class="text-caption text-grey-7 q-mb-sm">
        Sum of kube-apiserver / etcd / ovnkube-controller working set on masters (Mi).
        Pods are managed burn pods only. Source: metrics.k8s.io each sample.
      </p>
      <svg class="rss-chart" viewBox="0 0 640 220" preserveAspectRatio="none">
        <polyline fill="none" stroke="#c0392b" stroke-width="2" :points="chart.api" />
        <polyline fill="none" stroke="#2980b9" stroke-width="2" :points="chart.etcd" />
        <polyline fill="none" stroke="#27ae60" stroke-width="2" :points="chart.ovn" />
        <polyline fill="none" stroke="#8e6b3a" stroke-width="1.5" stroke-dasharray="4 3" :points="chart.pods" />
      </svg>
      <div class="row q-gutter-md text-caption">
        <span><span class="swatch swatch-api" /> kube-apiserver RSS</span>
        <span><span class="swatch swatch-etcd" /> etcd RSS</span>
        <span><span class="swatch swatch-ovn" /> ovnkube-controller RSS</span>
        <span><span class="swatch swatch-pods" /> workload pods (scaled)</span>
      </div>
      <p class="text-caption text-grey-7 q-mt-xs">
        {{ chart.first }} → {{ chart.last }} · {{ chart.points.length }} samples
        <span v-if="snap?.baselineApiserverRssMi"> · baseline API {{ Math.round(snap.baselineApiserverRssMi) }} Mi</span>
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

const chart = computed(() => {
  const pts = (series.value || []).slice().sort((a, b) => new Date(a.at) - new Date(b.at))
  const w = 640
  const h = 200
  const pad = 10
  if (!pts.length) return { points: [], api: '', etcd: '', ovn: '', pods: '', first: '', last: '' }
  const maxRss = Math.max(1, ...pts.map((p) => Math.max(p.apiserverRssMi || 0, p.etcdRssMi || 0, p.ovnRssMi || 0)))
  const maxPods = Math.max(1, ...pts.map((p) => p.workloadPods || 0))
  const x = (i) => pad + (i * (w - 2 * pad)) / Math.max(1, pts.length - 1)
  const yRss = (v) => h - pad - ((v || 0) / maxRss) * (h - 2 * pad)
  const yPods = (v) => h - pad - ((v || 0) / maxPods) * (h - 2 * pad)
  const line = (fn) => pts.map((p, i) => `${x(i).toFixed(1)},${fn(p).toFixed(1)}`).join(' ')
  return {
    points: pts,
    api: line((p) => yRss(p.apiserverRssMi)),
    etcd: line((p) => yRss(p.etcdRssMi)),
    ovn: line((p) => yRss(p.ovnRssMi)),
    pods: line((p) => yPods(p.workloadPods)),
    first: fmt(pts[0].at),
    last: fmt(pts[pts.length - 1].at),
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
.rss-chart { width: 100%; height: 220px; background: #f4f7fa; display: block; }
.swatch { display: inline-block; width: 10px; height: 10px; margin-right: 4px; vertical-align: middle; }
.swatch-api { background: #c0392b; }
.swatch-etcd { background: #2980b9; }
.swatch-ovn { background: #27ae60; }
.swatch-pods { background: #8e6b3a; }
.stage { border: 1px solid rgba(28, 52, 73, 0.12); padding: 0.6rem 0.7rem; min-height: 4.2rem; }
.stage.is-now { border-color: #1c3449; background: #eef3f8; }
</style>
