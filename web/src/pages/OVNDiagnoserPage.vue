<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">OVN-Kube Diagnoser</div>
        <h1 class="dasm-title">Is OVN becoming unhealthy?</h1>
        <p class="dasm-subtitle">
          Active cluster interrogation (nodes, ovnkube pods, DB containers, metrics, logs, events) —
          not a kube-burner dashboard. Capture a baseline before load, then sample during Execute.
        </p>
      </div>
    </div>

    <div class="row items-center q-gutter-sm q-mb-md">
      <q-btn outline color="primary" label="Capture baseline" :loading="busy" @click="captureBaseline" />
      <q-btn unelevated color="primary" icon="refresh" label="Sample now" :loading="busy" @click="sample" />
      <q-btn flat color="primary" icon="history" label="Reload latest" :loading="busy" @click="loadLatest" />
      <q-badge v-if="snap?.overallState" :color="stateColor" text-color="white">{{ snap.overallState }}</q-badge>
      <span v-if="snap?.baselineAt" class="text-caption text-grey-7">baseline {{ fmt(snap.baselineAt) }}</span>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

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
      <div v-if="snap.why" class="q-mt-md"><strong>Why?</strong> {{ snap.why }}</div>
    </div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-lg-7">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Per-node health</div>
          <table class="ovn-table">
            <thead>
              <tr>
                <th>Node</th>
                <th>State</th>
                <th>Ready</th>
                <th>Annots</th>
                <th>DB</th>
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
            <li v-for="r in (nodeDetail.ovnKube?.resources || [])" :key="r.container">
              {{ r.container }}: {{ Number(r.cpuCores || 0).toFixed(3) }}c · {{ Number(r.memoryMiB || 0).toFixed(0) }}Mi
            </li>
            <li v-for="f in (nodeDetail.findings || [])" :key="f.id">
              <q-badge dense :color="sevColor(f.severity)" text-color="white">{{ f.ruleId }}</q-badge>
              {{ f.summary }}
            </li>
          </ul>
        </div>
        <div class="dasm-panel q-mb-md">
          <div class="dasm-stat-label q-mb-sm">Findings</div>
          <ul class="detail-list">
            <li v-for="f in findings" :key="f.id">
              <q-badge dense :color="sevColor(f.severity)" text-color="white">{{ f.ruleId }}</q-badge>
              {{ f.summary }}
              <span class="text-grey-7" v-if="f.node"> · {{ f.node }}</span>
            </li>
            <li v-if="!findings.length" class="text-grey-7">No warning+ findings in latest sample.</li>
          </ul>
        </div>
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Timeline</div>
          <ul class="detail-list">
            <li v-for="(t, i) in (snap?.timeline || []).slice(0, 20)" :key="i">
              <span class="text-mono">{{ fmt(t.at) }}</span> · {{ t.summary }}
            </li>
          </ul>
        </div>
      </div>
    </div>
  </q-page>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { baselineOVNDiag, getOVNDiag, sampleOVNDiag } from 'src/services/api'

const busy = ref(false)
const error = ref('')
const snap = ref(null)
const selectedNode = ref('')

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
  (snap.value?.findings || []).filter((f) => ['WARNING', 'ERROR', 'CRITICAL', 'NOTICE'].includes(f.severity)).slice(0, 40),
)
const nodeDetail = computed(() => (snap.value?.nodes || []).find((n) => n.nodeName === selectedNode.value) || null)

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
function dbLabel(d) {
  if (!d?.present) return '—'
  if (d.nbdbReady && d.sbdbReady && d.northdReady) return 'ok'
  return 'warn'
}
function cpuSum(resources) {
  if (!resources?.length) return '—'
  const s = resources.reduce((a, r) => a + (r.cpuCores || 0), 0)
  return s.toFixed(2) + 'c'
}

async function loadLatest() {
  busy.value = true
  error.value = ''
  try {
    const data = await getOVNDiag()
    snap.value = data.snapshot
    if (!selectedNode.value && snap.value?.nodes?.[0]) selectedNode.value = snap.value.nodes[0].nodeName
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    busy.value = false
  }
}
async function sample() {
  busy.value = true
  error.value = ''
  try {
    const data = await sampleOVNDiag({})
    snap.value = data.snapshot
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    busy.value = false
  }
}
async function captureBaseline() {
  busy.value = true
  error.value = ''
  try {
    const data = await baselineOVNDiag()
    snap.value = data.snapshot
  } catch (e) {
    error.value = e.response?.data?.error || e.message
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
.detail-list li { margin-bottom: 0.3rem; }
.text-mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>
