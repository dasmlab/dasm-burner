<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">OVN-Kube Diagnoser</div>
        <h1 class="dasm-title">Is OVN becoming unhealthy?</h1>
        <p class="dasm-subtitle">
          Active cluster interrogation (nodes, ovnkube pods, DB, dataplane, metrics, logs, events) —
          not a kube-burner dashboard. Capture a baseline before load, then sample during Execute.
        </p>
      </div>
    </div>

    <div class="row items-center q-gutter-sm q-mb-sm">
      <q-btn outline color="primary" label="Capture baseline" :loading="busy" :disable="!canAdmin" @click="captureBaseline" />
      <q-btn unelevated color="primary" icon="refresh" label="Sample now" :loading="busy" :disable="!canAdmin" @click="sample" />
      <q-btn flat color="primary" icon="history" label="Reload latest" :loading="busy" @click="loadLatest" />
      <q-badge v-if="snap?.overallState" :color="stateColor" text-color="white">{{ snap.overallState }}</q-badge>
      <span v-if="snap?.baselineAt" class="text-caption text-grey-7">baseline {{ fmt(snap.baselineAt) }}</span>
    </div>
    <p class="text-caption text-grey-7 q-mb-md">
      Sample / baseline can take 30–90s (nodes + OVN pods + events + log tails). A short axios timeout used to look like a Network Error — wait for the button spinner.
    </p>

    <div v-if="notice" class="dasm-panel q-mb-md text-positive">{{ notice }}</div>
    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-xs">What baseline captures</div>
      <ul class="detail-list">
        <li><strong>What:</strong> watermarks for ovnkube-node restart counts, per-node Ready, and (when metrics API exists) CPU/mem per container — used later for Δ.</li>
        <li><strong>Where:</strong> in-memory on this pod + immutable snapshot on the data PVC under <code>ovndiag/&lt;id&gt;/snapshot.json</code>.</li>
        <li><strong>Cluster:</strong> {{ clusterLabel }} · baseline {{ baselineLabel }}</li>
      </ul>
    </div>

    <div v-if="snap" class="dasm-panel q-mb-md">
      <div class="row q-col-gutter-md">
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
          <div class="dasm-stat-label">Capabilities</div>
          <div class="text-caption text-mono">{{ capsLabel }}</div>
        </div>
      </div>
      <div v-if="whyLines.length" class="q-mt-md">
        <div class="dasm-stat-label q-mb-xs">Why?</div>
        <ul class="why-list">
          <li v-for="(line, i) in whyLines" :key="i">{{ line }}</li>
        </ul>
      </div>
    </div>

    <div v-if="snap" class="dasm-panel q-mb-md">
      <div class="row items-center justify-between q-mb-sm">
        <div class="dasm-stat-label">Findings</div>
        <span class="text-caption text-grey-7">{{ findings.length }} warning+ in this sample</span>
      </div>
      <div v-if="!findings.length" class="text-caption text-grey-7">No warning+ findings in selected sample.</div>
      <div v-else class="finding-grid">
        <div v-for="f in findings" :key="f.id" class="finding-card">
          <div class="finding-head">
            <q-badge dense :color="sevColor(f.severity)" text-color="white">{{ f.ruleId }}</q-badge>
            <strong>{{ ruleTitle(f.ruleId) }}</strong>
            <span class="text-mono text-grey-7" v-if="f.node">{{ f.node }}</span>
          </div>
          <div class="finding-summary">{{ f.summary }}</div>
          <div class="text-caption text-grey-7" v-if="ruleAbout(f.ruleId)">{{ ruleAbout(f.ruleId) }}</div>
          <div class="text-caption" v-if="f.why">{{ f.why }}</div>
        </div>
      </div>
    </div>

    <div class="dasm-panel q-mb-md">
      <div class="row items-center justify-between q-mb-sm">
        <div class="dasm-stat-label">Sample history</div>
        <span class="text-caption text-grey-7">{{ samples.length }} stored · click a row to inspect</span>
      </div>
      <table class="ovn-table">
        <thead>
          <tr>
            <th>When</th>
            <th>Kind</th>
            <th>State</th>
            <th>Nodes</th>
            <th>Findings</th>
            <th>Run</th>
            <th>Id</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in samples"
            :key="row.id"
            :class="{ 'is-sel': row.id === selectedSampleId }"
            @click="openSample(row.id)"
          >
            <td class="text-mono">{{ fmt(row.generatedAt) }}</td>
            <td>{{ row.kind }}</td>
            <td>{{ row.overallState }}</td>
            <td>{{ row.nodeCount }}</td>
            <td>{{ row.findingCount }}</td>
            <td class="text-mono">{{ row.runId || '—' }}</td>
            <td class="text-mono">{{ shortId(row.id) }}</td>
          </tr>
          <tr v-if="!samples.length">
            <td colspan="7" class="text-grey-7">No samples yet — Capture baseline or Sample now.</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-lg-7">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Per-node health (selected sample)</div>
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
                <th>CPU (Σ)</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="n in (snap?.nodes || [])"
                :key="n.nodeName"
                :class="{ 'is-hot': n.overallState !== 'HEALTHY', 'is-sel': n.nodeName === selectedNode }"
                @click="selectedNode = n.nodeName"
              >
                <td class="text-mono">{{ n.nodeName }}</td>
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
      <div class="col-12 col-lg-5">
        <div class="dasm-panel q-mb-md" v-if="nodeDetail">
          <div class="dasm-stat-label q-mb-sm">{{ nodeDetail.nodeName }}</div>
          <ul class="detail-list">
            <li>Ready={{ nodeDetail.node?.ready }} MemoryPressure={{ nodeDetail.node?.memoryPressure }}</li>
            <li>Annots OK={{ nodeDetail.network?.annotationsOK }} zone={{ nodeDetail.network?.zone || '—' }}</li>
            <li>DB nbdb={{ nodeDetail.database?.nbdbReady }} sbdb={{ nodeDetail.database?.sbdbReady }} northd={{ nodeDetail.database?.northdReady }}</li>
            <li>DP ovs={{ nodeDetail.dataplane?.ovsReady }} gw={{ nodeDetail.dataplane?.gatewayOK }} sandbox={{ nodeDetail.dataplane?.sandboxFailures ?? 0 }} pendingNoIP={{ nodeDetail.dataplane?.pendingNoIP ?? 0 }}</li>
            <li v-for="r in (nodeDetail.ovnKube?.resources || [])" :key="r.container">
              {{ r.container }}: {{ Number(r.cpuCores || 0).toFixed(3) }}c · {{ Number(r.memoryMiB || 0).toFixed(0) }}Mi
            </li>
          </ul>
        </div>
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Events in this sample</div>
          <ul class="detail-list">
            <li v-for="(t, i) in (snap?.timeline || []).slice(0, 20)" :key="i">
              <span class="text-mono">{{ fmt(t.at) }}</span> · {{ t.summary }}
            </li>
            <li v-if="!(snap?.timeline || []).length" class="text-grey-7">No timeline events in this snapshot.</li>
          </ul>
        </div>
      </div>
    </div>
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
import { splitWhy } from 'src/utils/metrics'
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
const whyLines = computed(() => {
  if (snap.value?.whyLines?.length) return snap.value.whyLines
  return splitWhy(snap.value?.why)
})
const nodeDetail = computed(() => (snap.value?.nodes || []).find((n) => n.nodeName === selectedNode.value) || null)
const clusterLabel = computed(() => cluster.currentLabel?.value || snap.value?.cluster || '—')
const baselineLabel = computed(() => {
  if (baselineMeta.value?.at) return fmt(baselineMeta.value.at)
  if (snap.value?.baselineAt) return fmt(snap.value.baselineAt)
  return 'not captured yet'
})

function sevColor(sev) {
  if (sev === 'CRITICAL' || sev === 'ERROR') return 'negative'
  if (sev === 'WARNING') return 'warning'
  return 'grey-7'
}
function fmt(at) {
  try { return new Date(at).toLocaleString() } catch { return '' }
}
function shortPod(n) {
  if (!n) return '—'
  return n.length > 22 ? n.slice(0, 20) + '…' : n
}
function shortId(id) {
  if (!id) return '—'
  return id.length > 28 ? id.slice(0, 26) + '…' : id
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
  else if (data?.snapshotId) selectedSampleId.value = data.snapshotId
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
    notice.value = `Loaded sample ${shortId(id)}`
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}
async function sample() {
  busy.value = true
  error.value = ''
  notice.value = 'Sampling cluster…'
  try {
    const data = await sampleOVNDiag({})
    applyPayload(data)
    notice.value = `Sample stored${data.snapshotId ? ` · ${data.snapshotId}` : ''}. History table updated.`
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
  notice.value = 'Capturing baseline watermarks…'
  try {
    const data = await baselineOVNDiag()
    applyPayload(data)
    const c = data.captured || {}
    notice.value = [
      'Baseline captured.',
      `Nodes=${c.nodes ?? '—'}`,
      `ovnkube pods=${c.ovnkubePods ?? '—'}`,
      `at ${fmt(c.at || data.baselineAt)}`,
      `stored as ${data.snapshotId || 'snapshot'} on PVC.`,
    ].join(' ')
    await refreshHistory()
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
}
.ovn-table tr.is-hot td { background: #fff4f0; }
.ovn-table tr.is-sel td { outline: 1px solid #2f8f7d; }
.ovn-table tr { cursor: pointer; }
.detail-list {
  margin: 0;
  padding-left: 1rem;
  font-size: 0.85rem;
}
.detail-list li { margin-bottom: 0.45rem; }
.why-list {
  margin: 0;
  padding-left: 1.15rem;
  font-size: 0.9rem;
  line-height: 1.45;
  color: #1d2b36;
}
.why-list li { margin-bottom: 0.4rem; }
.finding-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0.75rem;
}
@media (min-width: 900px) {
  .finding-grid { grid-template-columns: 1fr 1fr; }
}
.finding-card {
  border: 1px solid #d9e2ea;
  border-radius: 10px;
  background: #f7fafc;
  padding: 0.75rem 0.85rem;
}
.finding-head {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
  align-items: center;
  margin-bottom: 0.35rem;
}
.finding-summary { font-size: 0.9rem; margin-bottom: 0.25rem; }
.text-mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>
