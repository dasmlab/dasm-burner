<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">ETCD / Control-plane Diagnoser</div>
        <h1 class="dasm-title">Is etcd becoming unhealthy?</h1>
        <p class="dasm-subtitle">
          Masters Ready, etcd members, kube-apiserver static pods, MemoryPressure —
          the cliff before <code>etcdserver: request timed out</code>. Pairs with OVN Diagnoser and etcd-triage.
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
      Samples run on a single-slot ETCD worker (off the HTTP path). POST returns immediately; reload from PVC until the snapshot appears.
    </p>

    <div v-if="notice" class="dasm-panel q-mb-md text-positive">{{ notice }}</div>
    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

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
          <div class="dasm-stat-label">MemPressure</div>
          <div class="text-h6">{{ snap.mastersMemoryPressure }}</div>
        </div>
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">Findings</div>
          <div class="text-h6">{{ snap.findingCount }}</div>
        </div>
        <div class="col-4 col-sm-2">
          <div class="dasm-stat-label">Cluster</div>
          <div class="text-body1">{{ snap.cluster || clusterLabel }}</div>
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
            <th>etcd</th>
            <th>restarts</th>
            <th>apiserver</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in snap.masters" :key="m.name">
            <td>{{ m.name }}</td>
            <td><q-badge :color="m.ready ? 'positive' : 'negative'">{{ m.ready ? 'yes' : 'no' }}</q-badge></td>
            <td><q-badge :color="m.memoryPressure ? 'warning' : 'grey-6'">{{ m.memoryPressure ? 'yes' : 'no' }}</q-badge></td>
            <td>{{ m.etcdReady ? 'ok' : (m.etcdPod || '—') }}</td>
            <td>{{ m.etcdRestarts }}</td>
            <td>{{ m.apiserverReady ? 'ok' : (m.apiserverPod || '—') }}</td>
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
            <q-item-label>{{ s.id }} · {{ s.overallState }} · masters {{ s.mastersReady }}/{{ s.mastersTotal }} · etcd {{ s.etcdReady }}/{{ s.etcdTotal }}</q-item-label>
            <q-item-label caption>{{ fmt(s.generatedAt) }} · findings {{ s.findingCount }}</q-item-label>
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
const rules = ref({})

const stateColor = computed(() => {
  switch (snap.value?.overallState) {
    case 'HEALTHY': return 'positive'
    case 'WARNING': return 'warning'
    case 'CRITICAL':
    case 'FAILED': return 'negative'
    default: return 'grey-6'
  }
})
const clusterLabel = computed(() => cluster.currentLabel?.value || snap.value?.cluster || '—')
const findings = computed(() => (snap.value?.findings || []).slice(0, 80))
const whyLines = computed(() => snap.value?.whyLines?.length ? snap.value.whyLines : [])

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
  if (data?.rules) rules.value = data.rules
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
</script>

<style scoped>
.etcd-table { width: 100%; }
.detail-list { margin: 0; padding-left: 1.1rem; }
</style>
