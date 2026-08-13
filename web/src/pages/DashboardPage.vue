<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">OpenShift density</div>
        <h1 class="dasm-title">Control plane around kube-burner</h1>
        <p class="dasm-subtitle">
          Observational UI. Apply still requires the CLI safety flags.
          The canvas is a compact template (Namespace × N), not 2,500 boxes.
          CLI default mix remains 2,500 namespaces.
        </p>
      </div>
    </div>

    <div class="row items-center justify-between q-mb-md">
      <div class="text-subtitle1 text-weight-medium">Cluster health</div>
      <q-btn flat dense color="primary" icon="refresh" label="Refresh" :loading="loading" @click="load" />
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="dasm-grid dasm-grid--4 q-mb-lg">
      <div class="dasm-panel">
        <div class="dasm-stat-label">Abort gate</div>
        <div class="dasm-stat">{{ health.level || '—' }}</div>
        <div class="text-caption text-grey-7">{{ health.reason || 'no abort' }}</div>
      </div>
      <div class="dasm-panel">
        <div class="dasm-stat-label">Nodes Ready</div>
        <div class="dasm-stat">{{ h.nodesReady ?? '—' }} / {{ (h.nodesReady || 0) + (h.nodesNotReady || 0) }}</div>
      </div>
      <div class="dasm-panel">
        <div class="dasm-stat-label">OVN Ready</div>
        <div class="dasm-stat">{{ h.ovnReady ?? '—' }} / {{ h.ovnPods ?? '—' }}</div>
        <div class="text-caption text-grey-7">restarts {{ h.ovnRestarts ?? 0 }}</div>
      </div>
      <div class="dasm-panel">
        <div class="dasm-stat-label">OOMKilled</div>
        <div class="dasm-stat">{{ h.oomKilled ?? '—' }}</div>
      </div>
    </div>

    <div class="text-subtitle1 text-weight-medium q-mb-md">Intended mix</div>
    <div class="dasm-grid dasm-grid--4">
      <div class="dasm-panel" v-for="row in counts" :key="row.label">
        <div class="dasm-stat-label">{{ row.label }}</div>
        <div class="dasm-stat">{{ row.value }}</div>
      </div>
    </div>

    <div v-if="status" class="dasm-panel q-mt-lg">
      <div class="dasm-stat-label">Live convergence</div>
      <div class="dasm-stat">{{ Number(status.convergence?.overallPercent || 0).toFixed(1) }}%</div>
      <div class="text-caption text-grey-7">run {{ status.runId }}</div>
    </div>
  </q-page>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getHealth, getPlan, getStatus } from 'src/services/api'

const loading = ref(false)
const error = ref('')
const health = ref({})
const plan = ref(null)
const status = ref(null)

const h = computed(() => health.value.health || {})
const counts = computed(() => {
  const c = plan.value?.counts || {}
  return [
    { label: 'Namespaces', value: fmt(c.namespaces) },
    { label: 'Routes / Services', value: fmt(c.routes) },
    { label: 'Deployments', value: fmt(c.deployments) },
    { label: 'Pods', value: fmt(c.pods) },
  ]
})

function fmt(n) {
  if (n == null) return '—'
  return Number(n).toLocaleString()
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [p, hl, st] = await Promise.all([
      getPlan().catch((e) => { throw e }),
      getHealth(),
      getStatus().catch(() => null),
    ])
    plan.value = p
    health.value = hl
    status.value = st
  } catch (e) {
    error.value = e.response?.data?.error || e.message || 'failed to load'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
