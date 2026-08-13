<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">Narrative</div>
        <h1 class="dasm-title">OVN / API report</h1>
        <p class="dasm-subtitle">
          Merges cluster health with kube-burner collected metrics from the last apply.
        </p>
      </div>
    </div>

    <q-btn class="q-mb-md" flat dense color="primary" icon="refresh" label="Reload" :loading="loading" @click="load" />
    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="dasm-panel q-mb-lg" style="white-space: pre-wrap; font-family: ui-monospace, monospace; font-size: 0.88rem;">
      {{ doc.narrative || 'No report yet. Run apply --measure then dasm-burner report.' }}
    </div>

    <div v-if="metrics.length" class="dasm-grid dasm-grid--2">
      <div class="dasm-panel" v-for="m in metrics" :key="m.metric">
        <div class="dasm-stat-label">{{ m.metric }}</div>
        <div class="dasm-stat">{{ Number(m.last || 0).toPrecision(4) }}</div>
        <div class="text-caption text-grey-7">max {{ Number(m.max || 0).toPrecision(4) }} · n={{ m.count }}</div>
      </div>
    </div>
  </q-page>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getReport } from 'src/services/api'

const loading = ref(false)
const error = ref('')
const doc = ref({})
const metrics = computed(() => Object.values(doc.value.metrics || {}))

async function load() {
  loading.value = true
  error.value = ''
  try {
    doc.value = await getReport()
  } catch (e) {
    error.value = e.response?.data?.error || e.message || 'failed to load report'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
