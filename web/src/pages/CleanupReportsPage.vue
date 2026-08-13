<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">Cleanup reports</div>
        <h1 class="dasm-title">How long did cleanup take?</h1>
        <p class="dasm-subtitle">
          Immutable records of each cleanup job: start / end / duration, targeted object totals,
          namespaces removed, and the full CLEANUP log.
        </p>
      </div>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-md-4">
        <div class="dasm-panel">
          <div class="row items-center justify-between q-mb-sm">
            <div class="dasm-stat-label">Jobs</div>
            <q-btn flat dense icon="refresh" :loading="loading" @click="loadList" />
          </div>
          <div v-if="!list.length" class="text-caption text-grey-7">
            No cleanup reports yet. Run Clean last / template / all on Execute.
          </div>
          <div
            v-for="r in list"
            :key="r.id"
            class="clean-row"
            :class="{ 'is-active': r.id === selectedId }"
            @click="select(r.id)"
          >
            <div class="row items-center justify-between">
              <q-badge :color="statusColor(r.status)" text-color="white">{{ r.status }}</q-badge>
              <span class="text-caption text-mono">{{ r.duration || '—' }}</span>
            </div>
            <div class="text-body2 q-mt-xs">
              {{ r.scope }}<span v-if="r.template"> · {{ r.template }}</span>
            </div>
            <div class="text-caption text-grey-7">
              {{ fmtTime(r.finished) }} · {{ r.deletedNamespaces ?? 0 }} NS
              <span v-if="r.dryRun"> · dry-run</span>
            </div>
          </div>
        </div>
      </div>

      <div class="col-12 col-md-8">
        <div v-if="!selected" class="dasm-panel text-caption text-grey-7">
          Select a cleanup job to inspect duration and logs.
        </div>
        <div v-else class="dasm-panel">
          <div class="row items-center q-gutter-sm q-mb-md">
            <q-badge :color="statusColor(selected.status)" text-color="white">{{ selected.status }}</q-badge>
            <span class="text-mono text-weight-bold">{{ selected.id }}</span>
            <q-badge v-if="selected.dryRun" outline color="grey-7">dry-run</q-badge>
          </div>

          <div class="row q-col-gutter-md q-mb-lg">
            <div class="col-6 col-sm-3">
              <div class="dasm-stat-label">Started</div>
              <div>{{ fmtTime(selected.started) }}</div>
            </div>
            <div class="col-6 col-sm-3">
              <div class="dasm-stat-label">Ended</div>
              <div>{{ fmtTime(selected.finished) }}</div>
            </div>
            <div class="col-6 col-sm-3">
              <div class="dasm-stat-label">Duration</div>
              <div class="text-weight-bold">{{ selected.duration }}</div>
              <div class="text-caption text-grey-7">{{ selected.durationMs }} ms</div>
            </div>
            <div class="col-6 col-sm-3">
              <div class="dasm-stat-label">Cluster</div>
              <div>{{ selected.cluster || '—' }}</div>
            </div>
          </div>

          <div class="dasm-stat-label q-mb-sm">Targeted objects (pre-delete sample)</div>
          <div class="row q-col-gutter-sm q-mb-lg">
            <div class="col-4 col-sm-2" v-for="(v, k) in totals" :key="k">
              <div class="metric-tile">
                <div class="dasm-stat-label">{{ k }}</div>
                <div class="text-h6">{{ v }}</div>
              </div>
            </div>
          </div>

          <div class="row q-col-gutter-md q-mb-lg">
            <div class="col-6 col-sm-3">
              <div class="dasm-stat-label">Namespaces removed</div>
              <div class="text-h6">{{ selected.deletedNamespaces }}</div>
            </div>
            <div class="col-6 col-sm-3">
              <div class="dasm-stat-label">Remaining</div>
              <div class="text-h6">{{ selected.remainingNamespaces }}</div>
            </div>
            <div class="col-12 col-sm-6">
              <div class="dasm-stat-label">Scope / template</div>
              <div>{{ selected.scope }} · {{ selected.template || '—' }}</div>
              <div class="text-caption text-mono text-grey-7" v-if="selected.runIds?.length">
                runs: {{ selected.runIds.join(', ') }}
              </div>
            </div>
          </div>

          <div v-if="selected.error" class="text-negative q-mb-md">{{ selected.error }}</div>

          <div class="dasm-stat-label q-mb-sm">Cleanup log</div>
          <div class="log-canvas">
            <div v-for="(line, i) in (selected.logs || [])" :key="i" class="log-line" :class="`lv-${line.level}`">
              <span class="log-ts">{{ fmtTime(line.at) }}</span>
              <span>{{ line.message }}</span>
            </div>
            <div v-if="!(selected.logs || []).length" class="text-caption text-grey-7">No log lines captured.</div>
          </div>

          <div v-if="selected.namespaces?.length" class="q-mt-md">
            <div class="dasm-stat-label q-mb-sm">Namespaces ({{ selected.namespaces.length }})</div>
            <div class="ns-list text-mono text-caption">
              <div v-for="n in selected.namespaces.slice(0, 200)" :key="n">{{ n }}</div>
              <div v-if="selected.namespaces.length > 200" class="text-grey-7">
                … +{{ selected.namespaces.length - 200 }} more
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </q-page>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getCleanupReport, listCleanupReports } from 'src/services/api'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const error = ref('')
const list = ref([])
const selectedId = ref('')
const selected = ref(null)

const totals = computed(() => {
  const t = selected.value?.targeted || {}
  return {
    NS: t.namespaces ?? 0,
    Services: t.services ?? 0,
    Routes: t.routes ?? 0,
    Deploys: t.deployments ?? 0,
    Pods: t.pods ?? 0,
  }
})

function statusColor(st) {
  if (st === 'passed') return 'positive'
  if (st === 'partial') return 'warning'
  if (st === 'failed') return 'negative'
  return 'grey-6'
}

function fmtTime(at) {
  try {
    return new Date(at).toLocaleString()
  } catch {
    return ''
  }
}

async function loadList() {
  loading.value = true
  error.value = ''
  try {
    const data = await listCleanupReports()
    list.value = data.reports || []
    const want = route.query.id || list.value[0]?.id
    if (want) await select(want)
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

async function select(id) {
  selectedId.value = id
  error.value = ''
  try {
    const data = await getCleanupReport(id)
    selected.value = data.report
    if (route.query.id !== id) {
      router.replace({ name: 'cleanup-reports', query: { id } })
    }
  } catch (e) {
    error.value = e.response?.data?.error || e.message
    selected.value = null
  }
}

watch(() => route.query.id, (id) => {
  if (id && id !== selectedId.value) select(id)
})

onMounted(loadList)
</script>

<style scoped>
.clean-row {
  padding: 0.65rem 0.7rem;
  border-radius: 10px;
  border: 1px solid var(--dasm-border-soft);
  margin-bottom: 0.45rem;
  cursor: pointer;
  background: #f4f7fa;
}
.clean-row.is-active {
  border-color: var(--q-primary);
  background: #eef5fb;
}
.metric-tile {
  padding: 0.5rem 0.55rem;
  border-radius: 8px;
  background: #f4f7fa;
  border: 1px solid var(--dasm-border-soft);
}
.log-canvas {
  max-height: 360px;
  overflow: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.78rem;
  background: #0f1720;
  color: #d7e0ea;
  border-radius: 10px;
  padding: 0.75rem;
}
.log-line { margin-bottom: 0.2rem; }
.log-ts { color: #7f93a8; margin-right: 0.55rem; }
.lv-error { color: #ff8f8f; }
.lv-warn { color: #ffd27a; }
.ns-list {
  max-height: 180px;
  overflow: auto;
  padding: 0.5rem;
  border-radius: 8px;
  background: #f4f7fa;
}
</style>
