<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">Narrative</div>
        <h1 class="dasm-title">OVN / API report</h1>
        <p class="dasm-subtitle">
          Immutable end-of-run snapshots with OVN node deltas, kube-burner metrics, and job summary.
        </p>
      </div>
    </div>

    <div class="row items-center q-gutter-sm q-mb-md">
      <q-btn flat dense color="primary" icon="refresh" label="Reload" :loading="loading" @click="load" />
      <q-chip v-if="selected?.immutable" dense square color="positive" text-color="white" icon="lock">
        immutable
      </q-chip>
      <span v-if="list.length" class="text-caption text-grey-7">{{ list.length }} snapshot(s)</span>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-md-4">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Snapshots</div>
          <div v-if="!list.length" class="text-caption text-grey-7">
            No snapshots yet. Finish an Execute run to freeze one.
          </div>
          <div class="snap-list">
            <button
              v-for="r in list"
              :key="r.snapshotId"
              type="button"
              class="snap-row"
              :class="{ 'is-active': r.snapshotId === selectedId }"
              @click="select(r.snapshotId)"
            >
              <div class="snap-title">
                <span class="text-mono">{{ r.prefix || `kb-${r.runId}` }}</span>
                <q-badge v-if="r.dryRun" outline color="grey">dry-run</q-badge>
                <q-badge :color="statusColor(r.status)" text-color="white">{{ r.status || '—' }}</q-badge>
              </div>
              <div class="snap-meta">
                {{ fmt(r.finished || r.generatedAt) }}
                <span v-if="r.duration"> · {{ r.duration }}</span>
                <span v-if="r.template"> · {{ r.template }}</span>
              </div>
              <div class="snap-close text-caption">{{ r.closeHeadline || r.openHeadline }}</div>
            </button>
          </div>
        </div>
      </div>

      <div class="col-12 col-md-8">
        <div v-if="!selected" class="dasm-panel text-caption text-grey-7">
          Select a snapshot to view Open / Close summaries.
        </div>
        <template v-else>
          <div class="dasm-panel q-mb-md">
            <div class="row items-start justify-between">
              <div>
                <div class="dasm-stat-label">Snapshot</div>
                <div class="text-mono text-weight-bold">{{ selected.snapshotId }}</div>
                <div class="text-caption text-grey-7 q-mt-xs">
                  run <span class="text-mono">{{ selected.runId }}</span>
                  <span v-if="selected.prefix"> · {{ selected.prefix }}</span>
                  <span v-if="selected.cluster"> · {{ selected.cluster }}</span>
                  <span v-if="selected.template"> · {{ selected.template }}</span>
                </div>
              </div>
              <div class="text-right">
                <div class="dasm-stat" v-if="conv != null">{{ Number(conv).toFixed(1) }}%</div>
                <div class="dasm-stat-label">convergence</div>
              </div>
            </div>

            <div class="row q-col-gutter-md q-mt-md timing-row">
              <div class="col-6 col-sm-3">
                <div class="dasm-stat-label">Started</div>
                <div>{{ fmt(selected.started) || '—' }}</div>
              </div>
              <div class="col-6 col-sm-3">
                <div class="dasm-stat-label">Ended</div>
                <div>{{ fmt(selected.finished) || '—' }}</div>
              </div>
              <div class="col-6 col-sm-3">
                <div class="dasm-stat-label">Wall duration</div>
                <div class="text-weight-bold">{{ selected.duration || '—' }}</div>
                <div class="text-caption text-grey-7" v-if="selected.durationMs">{{ selected.durationMs }} ms</div>
              </div>
              <div class="col-6 col-sm-3">
                <div class="dasm-stat-label">Apply / batches</div>
                <div>{{ selected.applyDuration || '—' }}</div>
                <div class="text-caption text-grey-7">
                  {{ selected.batchCount || 0 }} batches
                  <span v-if="selected.mode"> · {{ selected.mode }}</span>
                </div>
              </div>
            </div>
          </div>

          <q-expansion-item
            class="dasm-panel q-mb-md box-exp"
            expand-separator
            default-opened
            icon="play_circle"
            :label="selected.open?.title || 'Open'"
            :caption="selected.open?.headline"
          >
            <div class="q-pa-md">
              <ul class="box-list">
                <li v-for="(h, i) in (selected.open?.highlights || [])" :key="'o'+i">{{ h }}</li>
              </ul>
              <div v-if="selected.open?.health" class="text-caption text-grey-7 q-mt-sm">
                Sampled {{ fmt(selected.open.at || selected.open.health.sampledAt) }}
              </div>
            </div>
          </q-expansion-item>

          <q-expansion-item
            class="dasm-panel q-mb-md box-exp"
            expand-separator
            default-opened
            icon="stop_circle"
            :label="selected.close?.title || 'Close'"
            :caption="selected.close?.headline"
          >
            <div class="q-pa-md">
              <ul class="box-list">
                <li v-for="(h, i) in (selected.close?.highlights || [])" :key="'c'+i">{{ h }}</li>
              </ul>
              <div v-if="selected.close?.convergence" class="text-caption q-mt-sm">
                Convergence overall
                <strong>{{ Number(selected.close.convergence.overallPercent || selected.close.convergence.overall || 0).toFixed(1) }}%</strong>
              </div>
              <div v-if="selected.close?.health" class="text-caption text-grey-7 q-mt-sm">
                Sampled {{ fmt(selected.close.at || selected.close.health.sampledAt) }}
              </div>
            </div>
          </q-expansion-item>

          <q-expansion-item
            v-if="ovnRows.length"
            class="dasm-panel q-mb-md box-exp"
            expand-separator
            default-opened
            icon="hub"
            label="OVN pods by node"
            :caption="`Δ restarts during run: ${ovnDeltaTotal}`"
          >
            <div class="q-pa-md">
              <table class="ovn-table">
                <thead>
                  <tr>
                    <th>Pod</th>
                    <th>Node</th>
                    <th>Ready</th>
                    <th>Restarts</th>
                    <th>Δ</th>
                    <th>Phase</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="p in ovnRows" :key="p.name" :class="{ 'is-hot': p.restartsDelta > 0 || !p.ready }">
                    <td class="text-mono">{{ p.name }}</td>
                    <td class="text-mono">{{ p.node || '—' }}</td>
                    <td>{{ p.ready ? 'yes' : 'no' }}</td>
                    <td>{{ p.restarts }}</td>
                    <td><strong>{{ p.restartsDelta || 0 }}</strong></td>
                    <td>{{ p.phase || '—' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </q-expansion-item>

          <q-expansion-item
            v-if="selected.jobSummary"
            class="dasm-panel q-mb-md box-exp"
            expand-separator
            default-opened
            icon="summarize"
            label="Job summary"
            caption="kube-burner indexer document"
          >
            <div class="q-pa-md">
              <ul class="box-list">
                <li v-if="selected.jobSummary.passed != null">passed: <strong>{{ selected.jobSummary.passed }}</strong></li>
                <li v-if="selected.jobSummary.elapsedTime != null">elapsedTime: {{ selected.jobSummary.elapsedTime }}s</li>
                <li v-if="selected.jobSummary.uuid">uuid: <span class="text-mono">{{ selected.jobSummary.uuid }}</span></li>
                <li v-if="selected.jobSummary.executionErrors">errors: {{ selected.jobSummary.executionErrors }}</li>
                <li v-if="selected.jobSummary.runId">runId: {{ selected.jobSummary.runId }}</li>
                <li v-if="selected.jobSummary.cluster">cluster: {{ selected.jobSummary.cluster }}</li>
                <li v-if="selected.jobSummary.template">template: {{ selected.jobSummary.template }}</li>
              </ul>
            </div>
          </q-expansion-item>

          <q-expansion-item
            v-if="alerts.length"
            class="dasm-panel q-mb-md box-exp"
            expand-separator
            icon="warning"
            label="Alerts"
            :caption="`${alerts.length} document(s)`"
          >
            <div class="q-pa-md">
              <ul class="box-list">
                <li v-for="(a, i) in alerts" :key="'a'+i">
                  <strong>{{ a.severity || a.metricName || 'alert' }}</strong>
                  — {{ a.description || a.query || JSON.stringify(a) }}
                </li>
              </ul>
            </div>
          </q-expansion-item>

          <div v-if="metrics.length" class="q-mb-md">
            <div class="dasm-stat-label q-mb-sm">Prometheus / kube-burner metrics</div>
            <div class="dasm-grid dasm-grid--2">
              <div class="dasm-panel" v-for="m in metrics" :key="m.metric">
                <div class="dasm-stat-label">{{ m.metric }}</div>
                <div class="dasm-stat">{{ Number(m.last || 0).toPrecision(4) }}</div>
                <div class="text-caption text-grey-7">max {{ Number(m.max || 0).toPrecision(4) }} · avg {{ Number(m.avg || 0).toPrecision(4) }} · n={{ m.count }}</div>
              </div>
            </div>
            <div v-if="selected.metricsArchive" class="text-caption text-grey-7 q-mt-sm">
              Archive in snapshot: <span class="text-mono">metrics/{{ selected.metricsArchive }}</span>
            </div>
          </div>

          <q-expansion-item
            v-if="(selected.logs || []).length"
            class="dasm-panel q-mb-md box-exp"
            expand-separator
            icon="terminal"
            label="Execute log"
            :caption="`${selected.logs.length} frozen line(s)`"
          >
            <div class="q-pa-md log-canvas">
              <div v-for="(line, i) in selected.logs" :key="i" class="log-line" :class="`lv-${line.level}`">
                <span class="log-ts">{{ fmt(line.at) }}</span>
                <span class="log-phase">{{ line.phase }}{{ line.batch ? ` #${line.batch}` : '' }}</span>
                <span>{{ line.message }}</span>
              </div>
            </div>
          </q-expansion-item>

          <q-expansion-item
            class="dasm-panel q-mb-md box-exp"
            expand-separator
            icon="article"
            label="Narrative"
            caption="Full frozen markdown"
          >
            <div class="q-pa-md narrative">{{ selected.narrative }}</div>
          </q-expansion-item>
        </template>
      </div>
    </div>
  </q-page>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getReport, getReportById, listReports } from 'src/services/api'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const error = ref('')
const list = ref([])
const selectedId = ref('')
const selected = ref(null)

const metrics = computed(() => Object.values(selected.value?.metrics || {}).sort((a, b) => a.metric.localeCompare(b.metric)))
const alerts = computed(() => selected.value?.alerts || [])
const ovnRows = computed(() => {
  const h = selected.value?.close?.health || selected.value?.health
  return h?.ovnDetail || []
})
const ovnDeltaTotal = computed(() => {
  const h = selected.value?.close?.health || selected.value?.health
  return h?.ovnRestartsDelta ?? ovnRows.value.reduce((n, p) => n + (p.restartsDelta || 0), 0)
})
const conv = computed(() => {
  const c = selected.value?.close?.convergence || selected.value?.apply?.convergence
  if (!c) return null
  if (c.overallPercent != null) return c.overallPercent
  return c.overall
})

function statusColor(st) {
  switch (st) {
    case 'passed': return 'positive'
    case 'failed':
    case 'aborted': return 'negative'
    default: return 'grey-6'
  }
}

function fmt(at) {
  try {
    return at ? new Date(at).toLocaleString() : ''
  } catch {
    return ''
  }
}

async function select(id) {
  if (!id) return
  selectedId.value = id
  error.value = ''
  try {
    selected.value = await getReportById(id)
    if (route.query.id !== id) {
      router.replace({ name: 'report', query: { id } })
    }
  } catch (e) {
    error.value = e.response?.data?.error || e.message || 'failed to load snapshot'
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await listReports()
    list.value = data.reports || []
    const want = route.query.id || list.value[0]?.snapshotId
    if (want) {
      await select(want)
    } else {
      selected.value = await getReport().catch(() => null)
      if (selected.value?.snapshotId) {
        selectedId.value = selected.value.snapshotId
      }
    }
  } catch (e) {
    error.value = e.response?.data?.error || e.message || 'failed to load reports'
  } finally {
    loading.value = false
  }
}

watch(() => route.query.id, (id) => {
  if (id && id !== selectedId.value) select(id)
})

onMounted(load)
</script>

<style scoped>
.snap-list {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  max-height: 70vh;
  overflow: auto;
}
.snap-row {
  text-align: left;
  border: 1px solid var(--dasm-border-soft);
  background: #f4f7fa;
  border-radius: 10px;
  padding: 0.65rem 0.75rem;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.snap-row:hover { border-color: #2f8f7d; }
.snap-row.is-active {
  background: #e8f5f1;
  border-color: #2f8f7d;
}
.snap-title {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  align-items: center;
  font-weight: 700;
  color: #1d2b36;
}
.snap-meta { font-size: 0.78rem; color: #607483; margin-top: 0.15rem; }
.snap-close { color: #4a6575; margin-top: 0.2rem; }
.text-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.box-list {
  margin: 0;
  padding-left: 1.1rem;
  color: #1d2b36;
}
.box-list li { margin-bottom: 0.25rem; }
.narrative {
  white-space: pre-wrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.82rem;
  line-height: 1.45;
}
.box-exp {
  padding: 0;
  overflow: hidden;
}
.ovn-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.82rem;
}
.ovn-table th,
.ovn-table td {
  text-align: left;
  padding: 0.35rem 0.45rem;
  border-bottom: 1px solid #d9e2ea;
}
.ovn-table tr.is-hot td {
  background: #fff4f0;
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
.log-phase { color: #9db4c7; margin-right: 0.45rem; }
.lv-error { color: #ff8f8f; }
.lv-warn { color: #ffd27a; }
.timing-row { border-top: 1px solid var(--dasm-border-soft); padding-top: 0.75rem; }
</style>
