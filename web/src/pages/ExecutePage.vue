<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">Execute / Test</div>
        <h1 class="dasm-title">Run a saved template</h1>
        <p class="dasm-subtitle">
          Select a saved topology (edit templates on Topology). Target cluster is in the header.
          Pipeline steps light up like CI jobs while logs stream below.
        </p>
      </div>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="row q-col-gutter-md q-mb-md">
      <div class="col-12 col-md-5">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Saved template</div>
          <q-select
            v-model="templateName"
            :options="templateOptions"
            dense
            outlined
            emit-value
            map-options
            :disable="running"
            @update:model-value="onTemplate"
          />
          <div v-if="selectedMeta" class="text-caption text-grey-7 q-mt-sm">
            {{ selectedMeta.namespaces }} NS ·
            {{ selectedMeta.routesPerNamespace }} routes ·
            {{ selectedMeta.servicesPerNamespace }} services ·
            {{ selectedMeta.replicasPerService }} pods/svc
            <span v-if="selectedMeta.counts"> · {{ selectedMeta.counts.pods }} pods total</span>
          </div>
        </div>
      </div>
      <div class="col-12 col-md-4">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Safety</div>
          <q-toggle v-model="dryRun" label="Dry run (no create)" :disable="running" />
          <q-toggle v-model="confirm" label="I understand this loads the control plane" :disable="running || dryRun" />
          <q-toggle v-model="allowLarge" label="Allow >10 namespaces" :disable="running || dryRun" />
          <q-toggle v-model="skipBaseline" label="Skip baseline wait" :disable="running" />
        </div>
      </div>
      <div class="col-12 col-md-3">
        <div class="dasm-panel column q-gutter-sm">
          <div class="dasm-stat-label">Cluster</div>
          <div class="text-body2">{{ clusterLabel }}</div>
          <q-btn
            color="primary"
            unelevated
            icon="play_arrow"
            label="Execute test"
            :loading="starting"
            :disable="!templateName || running"
            @click="start"
          />
          <q-btn
            outline
            color="negative"
            icon="stop"
            label="Cancel"
            :disable="!running"
            @click="cancel"
          />
        </div>
      </div>
    </div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-lg-5">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-md">Pipeline</div>
          <div v-if="!steps.length" class="text-caption text-grey-7">No run yet.</div>
          <div class="pipeline">
            <div
              v-for="step in steps"
              :key="step.id"
              class="pipe-step"
              :class="`is-${step.status}`"
            >
              <div class="pipe-dot" />
              <div class="pipe-body">
                <div class="pipe-label">{{ step.label }}</div>
                <div class="pipe-msg">{{ step.message || step.status }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="col-12 col-lg-7">
        <div class="dasm-panel log-panel">
          <div class="row items-center justify-between q-mb-sm">
            <div class="dasm-stat-label">Live log</div>
            <q-chip dense square :color="statusColor" text-color="white">{{ runStatus }}</q-chip>
          </div>
          <div ref="logEl" class="log-canvas">
            <div v-for="(line, i) in logs" :key="i" class="log-line" :class="`lv-${line.level}`">
              <span class="log-ts">{{ fmt(line.at) }}</span>
              <span class="log-phase">{{ line.phase }}{{ line.batch ? ` #${line.batch}` : '' }}</span>
              <span>{{ line.message }}</span>
            </div>
            <div v-if="!logs.length" class="text-caption text-grey-7">Waiting for events…</div>
          </div>
        </div>
      </div>
    </div>
  </q-page>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import {
  cancelRun,
  getCluster,
  getRun,
  listTemplates,
  selectTemplate,
  startRun,
} from 'src/services/api'

const error = ref('')
const templates = ref([])
const templateName = ref('')
const dryRun = ref(true)
const confirm = ref(false)
const allowLarge = ref(false)
const skipBaseline = ref(true)
const starting = ref(false)
const run = ref(null)
const clusterLabel = ref('…')
const logEl = ref(null)
let timer = null

const templateOptions = computed(() =>
  templates.value.map((t) => ({
    label: `${t.name} · ${t.namespaces} NS · ${t.counts?.pods ?? '?'} pods`,
    value: t.name,
  })),
)
const selectedMeta = computed(() => templates.value.find((t) => t.name === templateName.value))
const steps = computed(() => run.value?.steps || [])
const logs = computed(() => run.value?.logs || [])
const runStatus = computed(() => run.value?.status || 'idle')
const running = computed(() => runStatus.value === 'running')
const statusColor = computed(() => {
  switch (runStatus.value) {
    case 'running': return 'warning'
    case 'passed': return 'positive'
    case 'failed':
    case 'aborted': return 'negative'
    default: return 'grey-6'
  }
})

function fmt(at) {
  try {
    return new Date(at).toLocaleTimeString()
  } catch {
    return ''
  }
}

async function refreshTemplates() {
  const data = await listTemplates()
  templates.value = data.templates || []
  templateName.value = data.active || templates.value[0]?.name || ''
}

async function onTemplate(name) {
  if (!name) return
  await selectTemplate(name)
  await refreshTemplates()
}

async function poll() {
  try {
    const data = await getRun()
    run.value = data.run
    await nextTick()
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
  } catch {
    /* ignore poll errors */
  }
}

async function start() {
  error.value = ''
  starting.value = true
  try {
    await selectTemplate(templateName.value)
    const data = await startRun({
      template: templateName.value,
      dryRun: dryRun.value,
      confirm: confirm.value,
      allowLarge: allowLarge.value,
      skipBaseline: skipBaseline.value,
    })
    run.value = data.run
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    starting.value = false
  }
}

async function cancel() {
  try {
    await cancelRun()
    await poll()
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
}

onMounted(async () => {
  try {
    await refreshTemplates()
    const c = await getCluster()
    clusterLabel.value = c.current?.name || 'unknown'
    await poll()
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
  timer = setInterval(poll, 1500)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.pipeline {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
}
.pipe-step {
  display: flex;
  gap: 0.75rem;
  align-items: flex-start;
  padding: 0.55rem 0.65rem;
  border-radius: 10px;
  border: 1px solid var(--dasm-border-soft);
  background: #f4f7fa;
  opacity: 0.72;
  transition: background 0.2s, opacity 0.2s, border-color 0.2s;
}
.pipe-dot {
  width: 14px;
  height: 14px;
  margin-top: 3px;
  border-radius: 50%;
  background: #9aa7b2;
  flex-shrink: 0;
}
.pipe-label {
  font-weight: 700;
  color: #1d2b36;
}
.pipe-msg {
  font-size: 0.78rem;
  color: #607483;
}
.is-pending { opacity: 0.55; }
.is-running {
  opacity: 1;
  background: #fff8e6;
  border-color: #e0b84a;
}
.is-running .pipe-dot { background: #e0b84a; box-shadow: 0 0 0 4px rgba(224, 184, 74, 0.25); }
.is-passed {
  opacity: 1;
  background: #eaf7f0;
  border-color: #56ba6d;
}
.is-passed .pipe-dot { background: #56ba6d; }
.is-failed {
  opacity: 1;
  background: #fceeef;
  border-color: #cc4757;
}
.is-failed .pipe-dot { background: #cc4757; }
.is-skipped {
  opacity: 0.65;
  background: #eef2f5;
}
.is-skipped .pipe-dot { background: #90a0ad; }

.log-panel { min-height: 420px; }
.log-canvas {
  height: 420px;
  overflow: auto;
  background: #12202c;
  color: #d7e2ea;
  border-radius: 10px;
  padding: 0.75rem 0.9rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.78rem;
  line-height: 1.45;
}
.log-line { margin-bottom: 0.2rem; }
.log-ts { color: #7f93a3; margin-right: 0.55rem; }
.log-phase {
  display: inline-block;
  min-width: 9rem;
  color: #49a998;
  margin-right: 0.45rem;
}
.lv-error { color: #ff8a96; }
.lv-warn { color: #f0d792; }
</style>
