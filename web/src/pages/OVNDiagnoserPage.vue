<template>
  <q-page padding>
    <div class="dasm-shell q-mb-md">
      <div class="dasm-shell__content">
        <div class="dasm-caps">OVN-Kube Diagnoser</div>
        <h1 class="dasm-title">Is OVN becoming unhealthy?</h1>
        <p class="dasm-subtitle">
          {{ clusterLabel }} · baseline {{ baselineLabel }}.
          Capture before load, sample during Execute. Watermarks: ovnkube-node restarts, Ready, CPU/mem.
          Snapshots on PVC <code>ovndiag/&lt;id&gt;/snapshot.json</code>.
        </p>
      </div>
    </div>

    <div class="row items-center q-gutter-sm q-mb-sm">
      <q-btn outline color="primary" label="Capture baseline" :loading="busy" :disable="!canAdmin" @click="captureBaseline" />
      <q-btn unelevated color="primary" icon="refresh" label="Sample now" :loading="busy" :disable="!canAdmin" @click="sample" />
      <q-btn flat color="primary" icon="history" label="Reload latest" :loading="busy" @click="loadLatest" />
      <q-badge v-if="snap?.overallState" :color="stateColor" text-color="white">{{ snap.overallState }}</q-badge>
    </div>
    <p class="text-caption text-grey-7 q-mb-md">
      Off the HTTP path (30–90s). Look at grouped findings, then the node table. History is the strip — it does not grow the page.
    </p>

    <div v-if="notice" class="dasm-panel q-mb-md text-positive">{{ notice }}</div>
    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div v-if="snap" class="dasm-panel q-mb-md">
      <div class="row q-col-gutter-md items-end">
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">Healthy</div>
          <div class="text-h6">{{ snap.healthyCount }}</div>
        </div>
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">Warning</div>
          <div class="text-h6">{{ snap.warningCount }}</div>
        </div>
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">Critical</div>
          <div class="text-h6">{{ snap.criticalCount }}</div>
        </div>
        <div class="col-12 col-sm-6">
          <div class="dasm-stat-label">Look here</div>
          <div>{{ lookAt }}</div>
        </div>
      </div>
    </div>

    <q-expansion-item
      v-if="snap"
      class="dasm-panel q-mb-md iso-exp"
      dense
      switch-toggle-side
      expand-separator
      default-opened
      :label="`Findings · ${groupedFindings.length} rule(s) · ${findings.length} hit(s)`"
    >
      <div v-if="!groupedFindings.length" class="text-caption text-grey-7 q-mt-sm">No warning+ findings in this sample.</div>
      <div v-else class="finding-list q-mt-sm">
        <div v-for="g in groupedFindings" :key="g.ruleId" class="finding-card">
          <div class="finding-head">
            <q-badge dense :color="sevColor(g.severity)" text-color="white">{{ g.ruleId }}</q-badge>
            <strong>{{ g.title }}</strong>
            <span class="text-caption text-grey-7">{{ g.systems.length }} system{{ g.systems.length === 1 ? '' : 's' }} reporting</span>
          </div>
          <p v-if="g.hint" class="text-caption q-mb-xs">{{ g.hint }}</p>
          <div class="sys-row">
            <q-chip
              v-for="s in g.systems"
              :key="s.node"
              dense
              square
              outline
              color="primary"
              class="sys-chip"
              :title="s.summary"
              @click="selectNode(s.node)"
            >{{ shortNode(s.node) }}<span v-if="s.hint" class="sys-hint"> {{ s.hint }}</span></q-chip>
          </div>
          <p class="text-caption text-grey-7 q-mb-none q-mt-xs">{{ g.about }}</p>
        </div>
      </div>
    </q-expansion-item>

    <q-expansion-item
      class="dasm-panel q-mb-md iso-exp"
      dense
      switch-toggle-side
      expand-separator
      default-opened
      label="Per-node health"
    >
    <div class="row q-col-gutter-md q-mt-sm">
      <div class="col-12 col-lg-8">
        <div>
          <div class="table-scroll">
            <table class="ovn-table">
              <thead>
                <tr>
                  <th>Node</th>
                  <th>State</th>
                  <th>Ready</th>
                  <th>Annots</th>
                  <th>DB</th>
                  <th>DP</th>
                  <th>OVN pod</th>
                  <th>Δ rst</th>
                  <th>CPU</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="n in (snap?.nodes || [])"
                  :key="n.nodeName"
                  :class="{ 'is-hot': n.overallState !== 'HEALTHY', 'is-sel': n.nodeName === selectedNode }"
                  @click="selectedNode = n.nodeName"
                >
                  <td class="text-mono">{{ shortNode(n.nodeName) }}</td>
                  <td>{{ n.overallState }}</td>
                  <td>{{ n.node?.ready ? 'yes' : 'no' }}</td>
                  <td>{{ n.network?.annotationsOK === false ? 'drift' : 'ok' }}</td>
                  <td>{{ dbLabel(n.database) }}</td>
                  <td>{{ dpLabel(n.dataplane) }}</td>
                  <td class="text-mono">{{ shortPod(n.ovnKube?.podName) }}</td>
                  <td><strong>{{ n.ovnKube?.restartsDelta ?? 0 }}</strong></td>
                  <td>{{ cpuSum(n.ovnKube?.resources) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
      <div class="col-12 col-lg-4">
        <div v-if="nodeDetail" class="q-mb-md">
          <div class="dasm-stat-label q-mb-xs">{{ shortNode(nodeDetail.nodeName) }}</div>
          <ul class="detail-list">
            <li>Ready={{ nodeDetail.node?.ready }} · MemPressure={{ nodeDetail.node?.memoryPressure }}</li>
            <li>Annots={{ nodeDetail.network?.annotationsOK }} · zone={{ nodeDetail.network?.zone || '—' }}</li>
            <li>DB nb={{ nodeDetail.database?.nbdbReady }} sb={{ nodeDetail.database?.sbdbReady }} northd={{ nodeDetail.database?.northdReady }}</li>
            <li>DP ovs={{ nodeDetail.dataplane?.ovsReady }} gw={{ nodeDetail.dataplane?.gatewayOK }}</li>
          </ul>
        </div>
        <div v-if="groupedEvents.length">
          <div class="dasm-stat-label q-mb-xs">Events</div>
          <div v-for="e in groupedEvents" :key="e.key" class="ev-line">
            <span class="text-caption">{{ e.label }}</span>
            <span class="text-caption text-grey-7"> · {{ e.count }} node{{ e.count === 1 ? '' : 's' }}</span>
          </div>
        </div>
      </div>
    </div>
    </q-expansion-item>

    <q-expansion-item
      class="dasm-panel q-mb-md iso-exp"
      dense
      switch-toggle-side
      expand-separator
      :label="`Samples · ${samples.length} stored`"
      default-opened
    >
      <p class="text-caption text-grey-7 q-mt-sm q-mb-sm">
        Horizontal strip — does not grow the page. Click a tick to load that snapshot.
      </p>
      <div v-if="!samples.length" class="text-caption text-grey-7">None yet — Capture baseline or Sample now.</div>
      <div v-else class="sample-strip" role="list">
        <button
          v-for="row in recentSamples"
          :key="row.id"
          type="button"
          class="sample-pill"
          :class="pillClass(row)"
          role="listitem"
          @click="openSample(row.id)"
        >
          <span class="sample-pill__dot" />
          <span class="sample-pill__state">{{ row.overallState || row.kind }}</span>
          <span class="sample-pill__when">{{ fmtShort(row.generatedAt) }}</span>
          <span class="sample-pill__meta">{{ row.findingCount || 0 }} hit · {{ row.kind }}</span>
        </button>
      </div>
      <q-select
        v-if="samples.length > recentSamples.length"
        class="q-mt-sm"
        dense
        outlined
        emit-value
        map-options
        :model-value="selectedSampleId"
        :options="sampleOptions"
        label="Older samples"
        @update:model-value="openSample"
      />
    </q-expansion-item>

    <q-expansion-item class="dasm-panel q-mb-md iso-exp" dense switch-toggle-side expand-separator label="Capabilities &amp; how this works">
      <p class="text-caption q-mt-sm">{{ capsLabel }}</p>
      <p class="text-caption text-grey-7">
        Watermarks: ovnkube-node restarts, Ready, CPU/mem when metrics exist.
        Sample / baseline run on a single-slot worker, not the HTTP handler.
      </p>
    </q-expansion-item>
  </q-page>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import {
  baselineOVNDiag,
  getOVNDiag,
  getOVNDiagSnapshot,
  listOVNDiagHistory,
  sampleOVNDiag,
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
const rules = ref({})
const baselineMeta = ref(null)
const selectedNode = ref('')
const selectedSampleId = ref('')
const RECENT = 10

const stateColor = computed(() => {
  switch (snap.value?.overallState) {
    case 'HEALTHY': return 'positive'
    case 'WARNING':
    case 'DEGRADED': return 'warning'
    case 'CRITICAL':
    case 'FAILED': return 'negative'
    default: return 'grey-6'
  }
})
const capsLabel = computed(() => {
  const c = snap.value?.capabilities || {}
  return Object.keys(c).filter((k) => c[k]).join(', ') || '—'
})
const findings = computed(() =>
  (snap.value?.findings || []).filter((f) => ['WARNING', 'ERROR', 'CRITICAL', 'NOTICE'].includes(f.severity)).slice(0, 80),
)
const groupedFindings = computed(() => {
  const map = new Map()
  for (const f of findings.value) {
    const id = f.ruleId || 'unknown'
    if (!map.has(id)) {
      map.set(id, {
        ruleId: id,
        title: ruleTitle(id),
        about: ruleAbout(id),
        severity: f.severity,
        systems: [],
        nodes: new Map(),
      })
    }
    const g = map.get(id)
    if (worse(f.severity, g.severity)) g.severity = f.severity
    const node = f.node || 'cluster'
    const hint = classHint(f.summary)
    const prev = g.nodes.get(node)
    if (!prev) {
      const sys = { node, summary: f.summary || '', hint }
      g.nodes.set(node, sys)
      g.systems.push(sys)
    } else if (hint && prev.hint !== hint) {
      prev.hint = prev.hint ? `${prev.hint}, ${hint}` : hint
      prev.summary = `${prev.summary}; ${f.summary}`
    }
  }
  return [...map.values()].map((g) => {
    const hints = [...new Set(g.systems.map((s) => s.hint).filter(Boolean))]
    return {
      ruleId: g.ruleId,
      title: g.title,
      about: g.about,
      severity: g.severity,
      systems: g.systems,
      hint: hints.length ? hints.join(' · ') : stripHost(g.systems[0]?.summary || ''),
    }
  }).sort((a, b) => b.systems.length - a.systems.length)
})
const lookAt = computed(() => {
  const g = groupedFindings.value
  if (!g.length) return 'Nothing in warning+. The node table is the picture.'
  const top = g[0]
  return `${top.ruleId} · ${top.title} on ${top.systems.length} system${top.systems.length === 1 ? '' : 's'}. Click a chip to jump to that node.`
})
const groupedEvents = computed(() => {
  const map = new Map()
  for (const t of (snap.value?.timeline || []).slice(0, 40)) {
    const label = (t.summary || '').replace(/\s+on\s+\S+$/, '') || 'event'
    const cur = map.get(label) || { key: label, label, count: 0 }
    cur.count += 1
    map.set(label, cur)
  }
  return [...map.values()].slice(0, 8)
})
const nodeDetail = computed(() => (snap.value?.nodes || []).find((n) => n.nodeName === selectedNode.value) || null)
const clusterLabel = computed(() => cluster.currentLabel.value || snap.value?.cluster || '—')
const baselineLabel = computed(() => {
  if (baselineMeta.value?.at) return fmt(baselineMeta.value.at)
  if (snap.value?.baselineAt) return fmt(snap.value.baselineAt)
  return 'not captured yet'
})
const recentSamples = computed(() => (samples.value || []).slice(0, RECENT))
const sampleOptions = computed(() =>
  (samples.value || []).map((s) => ({
    label: `${fmtShort(s.generatedAt)} · ${s.overallState || s.kind} · ${s.findingCount || 0} hits`,
    value: s.id,
  })),
)

function worse(a, b) {
  const rank = { CRITICAL: 4, ERROR: 3, WARNING: 2, NOTICE: 1 }
  return (rank[a] || 0) > (rank[b] || 0)
}
function classHint(summary) {
  if (!summary) return ''
  const m = summary.match(/class\s+(\w+)\s*[x×>]?\s*(\d+)/i)
    || summary.match(/\b(ERROR|TIMEOUT|IPTABLES|WARN|WARNING)\b[^\d]*(\d+)/i)
  return m ? `${m[1]}×${m[2]}` : ''
}
function stripHost(summary) {
  return (summary || '').replace(/\s+on\s+\S+$/, '')
}
function pillClass(row) {
  const st = (row.overallState || '').toLowerCase()
  return {
    'is-sel': row.id === selectedSampleId.value,
    'is-healthy': st === 'healthy',
    'is-warning': st === 'warning' || st === 'degraded',
    'is-critical': st === 'critical' || st === 'failed',
  }
}
function sevColor(sev) {
  if (sev === 'CRITICAL' || sev === 'ERROR') return 'negative'
  if (sev === 'WARNING') return 'warning'
  return 'grey-7'
}
function fmt(at) {
  try { return new Date(at).toLocaleString() } catch { return '' }
}
function fmtShort(at) {
  try {
    const d = new Date(at)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } catch { return '' }
}
function shortPod(n) {
  if (!n) return '—'
  return n.length > 22 ? n.slice(0, 20) + '…' : n
}
function shortNode(n) {
  if (!n) return '—'
  const i = n.lastIndexOf('-')
  if (n.includes('master-')) return n.slice(n.indexOf('master-'))
  if (n.includes('infra-')) return n.slice(n.indexOf('infra-'))
  if (n.includes('worker-')) return n.slice(n.indexOf('worker-'))
  return i > 0 ? n.slice(i + 1) : n
}
function selectNode(node) {
  if (node && node !== 'cluster') selectedNode.value = node
}
function dbLabel(d) {
  if (!d?.present) return '—'
  if (d.nbdbReady && d.sbdbReady && d.northdReady) return 'ok'
  return 'warn'
}
function dpLabel(d) {
  if (!d?.present) return '—'
  if (d.ovsReady && d.gatewayOK && !(d.sandboxFailures > 0) && !(d.pendingNoIP > 0)) return 'ok'
  return 'warn'
}
function cpuSum(resources) {
  if (!resources?.length) return '—'
  const s = resources.reduce((a, r) => a + (r.cpuCores || 0), 0)
  return s.toFixed(2) + 'c'
}
function ruleTitle(id) {
  return rules.value?.[id]?.title || id
}
function ruleAbout(id) {
  return rules.value?.[id]?.about || ''
}
function applyPayload(data, { preferId } = {}) {
  if (data?.snapshot) snap.value = data.snapshot
  if (data?.samples) samples.value = data.samples
  if (data?.rules) rules.value = data.rules
  if (data?.baseline) baselineMeta.value = data.baseline
  if (preferId) selectedSampleId.value = preferId
  else if (data?.samples?.[0]?.id) selectedSampleId.value = data.samples[0].id
  if (!selectedNode.value && snap.value?.nodes?.[0]) selectedNode.value = snap.value.nodes[0].nodeName
}
function errText(e) {
  if (e.code === 'ECONNABORTED' || /timeout/i.test(e.message || '')) {
    return 'Request timed out waiting for OVN sample (cluster interrogation can take >30s). Try again — timeout is now 120s.'
  }
  if (e.message === 'Network Error') {
    return 'Network Error — often a route/proxy cut during a long sample. Wait and Sample now again; if it persists, check the preview pod logs.'
  }
  return e.response?.data?.error || e.message
}

async function loadLatest() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const data = await getOVNDiag()
    applyPayload(data)
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}
async function refreshHistory() {
  try {
    const data = await listOVNDiagHistory()
    if (data?.samples) samples.value = data.samples
    if (data?.rules) rules.value = data.rules
  } catch {
    /* non-fatal */
  }
}
async function openSample(id) {
  if (!id) return
  busy.value = true
  error.value = ''
  try {
    const data = await getOVNDiagSnapshot(id)
    applyPayload(data, { preferId: id })
    notice.value = `Loaded sample ${id}`
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}
async function sample() {
  busy.value = true
  error.value = ''
  notice.value = 'Queued on OVN worker (off the web path)…'
  const before = samples.value?.[0]?.id || snap.value?.generatedAt
  try {
    await sampleOVNDiag({})
    const deadline = Date.now() + 120000
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 2000))
      const data = await getOVNDiag()
      applyPayload(data)
      const now = data.samples?.[0]?.id || data.snapshot?.generatedAt
      if (now && now !== before) {
        notice.value = `Sample stored${data.snapshotId || data.samples?.[0]?.id ? ` · ${data.snapshotId || data.samples[0].id}` : ''}`
        await refreshHistory()
        return
      }
    }
    notice.value = 'Sample still running — use Reload latest in a moment.'
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
  notice.value = 'Baseline queued on OVN worker…'
  const before = samples.value?.[0]?.id || snap.value?.generatedAt
  try {
    await baselineOVNDiag()
    const deadline = Date.now() + 120000
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 2000))
      const data = await getOVNDiag()
      applyPayload(data)
      const now = data.samples?.[0]?.id || data.snapshot?.generatedAt
      if (now && now !== before) {
        notice.value = `Baseline captured · ${data.snapshotId || data.samples?.[0]?.id || 'snapshot'} on PVC.`
        await refreshHistory()
        return
      }
    }
    notice.value = 'Baseline still running — use Reload latest in a moment.'
  } catch (e) {
    notice.value = ''
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

onMounted(loadLatest)
</script>

<style scoped>
.ovn-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.8rem;
}
.ovn-table th, .ovn-table td {
  text-align: left;
  padding: 0.35rem 0.4rem;
  border-bottom: 1px solid #d9e2ea;
  white-space: nowrap;
}
.ovn-table tr.is-hot td { background: #fff4f0; }
.ovn-table tr.is-sel td { outline: 1px solid #2f8f7d; }
.ovn-table tr { cursor: pointer; }
.table-scroll { overflow: auto; max-height: 28rem; }
.detail-list {
  margin: 0;
  padding-left: 1rem;
  font-size: 0.85rem;
}
.detail-list li { margin-bottom: 0.35rem; }
.finding-list { display: flex; flex-direction: column; gap: 0.65rem; }
.finding-card {
  border: 1px solid #d9e2ea;
  border-radius: 10px;
  background: #f7fafc;
  padding: 0.65rem 0.8rem;
}
.finding-head {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
  align-items: center;
  margin-bottom: 0.35rem;
}
.sys-row { display: flex; flex-wrap: wrap; gap: 0.25rem; }
.sys-chip { font-size: 0.75rem; }
.sys-hint { color: #6f7f8d; font-weight: 500; }
.ev-line { padding: 0.15rem 0; }
.sample-strip {
  display: flex;
  gap: 0;
  overflow-x: auto;
  padding: 0.15rem 0 0.5rem;
}
.sample-pill {
  position: relative;
  flex: 0 0 auto;
  min-width: 7.2rem;
  text-align: left;
  border: 1px solid var(--dasm-border-soft);
  border-radius: 10px;
  background: #fff;
  padding: 0.45rem 0.55rem 0.45rem 0.7rem;
  margin-right: 0.7rem;
  cursor: pointer;
}
.sample-pill::after {
  content: '';
  position: absolute;
  top: 50%;
  right: -0.7rem;
  width: 0.7rem;
  height: 1px;
  background: #c5d0d8;
}
.sample-pill:last-child { margin-right: 0; }
.sample-pill:last-child::after { display: none; }
.sample-pill__dot {
  position: absolute;
  left: 0.28rem;
  top: 0.55rem;
  width: 0.45rem;
  height: 0.45rem;
  border-radius: 50%;
  background: #9aa8b3;
}
.sample-pill.is-healthy .sample-pill__dot { background: #2f8f7d; }
.sample-pill.is-warning .sample-pill__dot { background: #c48a2a; }
.sample-pill.is-critical .sample-pill__dot { background: #c0392b; }
.sample-pill.is-sel { border-color: var(--dasm-border-strong); background: rgba(63, 122, 107, 0.08); }
.sample-pill__state { display: block; font-size: 0.72rem; font-weight: 700; letter-spacing: 0.04em; }
.sample-pill__when { display: block; font-size: 0.9rem; }
.sample-pill__meta { display: block; font-size: 0.72rem; color: #6f7f8d; }
.iso-exp :deep(.q-item) { padding-left: 0; min-height: 2.2rem; }
.iso-exp :deep(.q-item__label) {
  font-size: 0.78rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #6f7f8d;
  font-weight: 600;
}
.text-mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>
