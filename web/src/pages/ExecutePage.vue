<template>
  <q-page padding class="execute-page">
    <div class="dasm-shell q-mb-md execute-hero">
      <div class="dasm-shell__content">
        <div class="dasm-caps">Execute</div>
        <h1 class="dasm-title">Run a saved template</h1>
        <p class="dasm-subtitle">
          Target cluster is the header dropdown. On this page: template → refresh state → clean → maxPods → execute.
          The live log is the record.
        </p>
      </div>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>
    <div v-if="interrupted" class="dasm-panel q-mb-md text-warning">
      Run <strong>{{ runPrefix }}</strong> was interrupted (server restart). Pipeline/log restored from disk —
      apply may still be live on the cluster. Use Refresh, then Clean if needed.
    </div>
    <div v-if="cleanupMsg" class="dasm-panel q-mb-md text-positive">
      {{ cleanupMsg }}
      <q-btn
        v-if="lastCleanupReportId"
        flat
        dense
        color="primary"
        class="q-ml-sm"
        icon="delete_sweep"
        label="Open cleanup report"
        :to="{ name: 'cleanup-reports', query: { id: lastCleanupReportId } }"
      />
    </div>

    <div class="dasm-panel control-deck q-mb-md" :class="{ 'is-live': running }">
      <div class="control-deck__row">
        <div class="control-template">
          <div class="dasm-stat-label q-mb-xs">Saved template</div>
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
          <div class="meta-static q-mt-sm">
            <code v-if="templatePrefix" class="pfx-chip">{{ templatePrefix }}</code>
            <span v-if="selectedMeta">{{ templateRecipe }}</span>
            <span v-else>Pick a template (edit on Topology).</span>
          </div>
          <div class="live-row q-mt-sm">
            <q-chip
              dense
              square
              class="live-chip"
              :color="deployChipColor"
              text-color="white"
              :icon="deployOnline ? 'cloud_done' : 'cloud_off'"
            >
              {{ deployLabel }}
            </q-chip>
            <q-btn
              unelevated
              dense
              color="primary"
              icon="sync"
              label="Refresh / check state"
              :loading="checking"
              :disable="running || cleaning"
              @click="checkState('manual refresh')"
            />
            <q-btn
              v-if="showReportLink"
              unelevated
              dense
              color="secondary"
              icon="assessment"
              label="Report"
              :to="reportRoute"
            />
          </div>
          <div v-if="deployObjectsCaption" class="meta-live q-mt-xs">{{ deployObjectsCaption }}</div>
        </div>
        <div class="control-run">
          <div class="dasm-stat-label q-mb-xs">Cluster</div>
          <div class="control-run__name">{{ cluster.currentLabel.value }}</div>
          <q-btn
            color="primary"
            unelevated
            class="full-width"
            icon="play_arrow"
            label="Execute test"
            :loading="starting"
            :disable="!templateName || running || !canAdmin"
            @click="start"
          />
          <q-btn
            outline
            color="negative"
            class="full-width q-mt-sm"
            icon="stop"
            label="Cancel"
            :disable="!running || !canAdmin"
            @click="cancel"
          />
        </div>
      </div>

      <div class="stage-rail-wrap">
        <div class="row items-center justify-between q-mb-xs">
          <div class="dasm-stat-label">Stages</div>
          <q-chip dense square :color="statusColor" text-color="white">{{ runStatus }}</q-chip>
        </div>
        <div v-if="!steps.length" class="meta-static">No run yet — stages light up after Execute.</div>
        <div v-else class="stage-rail">
          <template v-for="(step, i) in steps" :key="step.id">
            <button
              type="button"
              class="stage"
              :class="[`is-${step.status}`, { active: pinnedStepId === step.id || (!pinnedStepId && step.status === 'running') }]"
              @click="pinnedStepId = pinnedStepId === step.id ? '' : step.id"
            >
              <span class="stage-dot" />
              <span class="stage-name">{{ shortStepLabel(step) }}</span>
              <q-tooltip v-if="step.message || step.label" class="text-body2" max-width="360px">
                {{ step.label }}<br />{{ step.message || step.status }}
              </q-tooltip>
            </button>
            <span v-if="i < steps.length - 1" class="stage-join" :class="`is-${step.status}`" />
          </template>
        </div>
        <div v-if="stageDetail" class="stage-detail">{{ stageDetail }}</div>
      </div>
    </div>

    <div class="execute-body">
      <aside class="execute-tools dasm-panel">
        <q-expansion-item
          v-model="tools.cleanup"
          dense
          switch-toggle-side
          header-class="tool-head"
          label="Cleanup"
        >
          <div class="tool-body">
            <div class="meta-static q-mb-sm">
              Wipe managed <code>kb-*</code> on the header cluster
              <span v-if="managedTotal != null"> · {{ managedTotal }} NS live</span>
            </div>
            <q-btn
              outline
              dense
              class="full-width q-mb-xs"
              color="warning"
              icon="delete_sweep"
              label="Clean last run"
              :loading="cleaning"
              :disable="running || !templateName || !canAdmin"
              @click="doCleanup('last')"
            />
            <q-btn
              outline
              dense
              class="full-width q-mb-xs"
              color="warning"
              icon="folder_delete"
              label="Clean this template"
              :loading="cleaning"
              :disable="running || !templateName || !canAdmin"
              @click="doCleanup('template')"
            />
            <q-btn
              outline
              dense
              class="full-width"
              color="negative"
              icon="cleaning_services"
              label="Clean all kb- runs"
              :loading="cleaning"
              :disable="running || !canAdmin"
              @click="doCleanup('all')"
            />
          </div>
        </q-expansion-item>

        <q-separator />

        <q-expansion-item
          v-model="tools.kubelet"
          dense
          switch-toggle-side
          header-class="tool-head"
          label="Worker maxPods"
        >
          <div class="tool-body">
            <div class="meta-live q-mb-sm">
              <q-chip
                dense
                square
                :color="maxPodsRolloutColor"
                text-color="white"
                :icon="maxPodsRolloutIcon"
              >
                rollout {{ workerMaxPods?.rollout || '…' }}
              </q-chip>
              <div class="q-mt-xs">
                <template v-if="workerMaxPods">
                  desired {{ workerMaxPods.configured ? workerMaxPods.desired : 'unset' }}
                  · live {{ workerMaxPods.observedMin === workerMaxPods.observedMax
                    ? workerMaxPods.observedTypical
                    : `${workerMaxPods.observedMin}–${workerMaxPods.observedMax}` }}
                  on {{ workerMaxPods.matchingNodes ?? 0 }}/{{ workerMaxPods.workerNodes ?? 0 }} workers
                </template>
                <template v-else>Get current to read this cluster’s kubelet.</template>
              </div>
              <div class="meta-static q-mt-xs">slots {{ capSlotsLabel }}</div>
            </div>
            <q-input
              v-model.number="maxPodsInput"
              type="number"
              outlined
              dense
              label="maxPods"
              min="110"
              max="2000"
              class="q-mb-sm"
              :disable="running || cleaning || settingMaxPods || !canAdmin"
            />
            <q-btn
              outline
              dense
              class="full-width q-mb-xs"
              color="primary"
              icon="refresh"
              label="Get current"
              :loading="readingMaxPods"
              :disable="running || cleaning || settingMaxPods || !canAdmin"
              @click="refreshMaxPods(true)"
            />
            <q-btn
              outline
              dense
              class="full-width"
              color="primary"
              icon="memory"
              label="Set worker maxPods"
              :loading="settingMaxPods"
              :disable="running || cleaning || !canAdmin"
            />
          </div>
        </q-expansion-item>

        <q-separator />

        <q-expansion-item
          v-model="tools.safety"
          dense
          switch-toggle-side
          header-class="tool-head"
          label="Safety"
        >
          <div class="tool-body">
            <q-toggle v-model="dryRun" dense label="Dry run (no create)" :disable="running || !canAdmin" />
            <q-toggle v-model="confirm" dense label="I understand this loads the control plane" :disable="running || dryRun || !canAdmin" />
            <q-toggle v-model="allowLarge" dense label="Allow >10 namespaces" :disable="running || dryRun || !canAdmin" />
            <q-toggle v-model="skipBaseline" dense label="Skip baseline wait" :disable="running || !canAdmin" />
            <q-toggle v-model="enableOVNDiag" dense label="OVN Diagnoser samples" :disable="running || dryRun || !canAdmin" />
            <q-toggle v-model="enableEtcdDiag" dense label="ETCD / control-plane samples" :disable="running || dryRun || !canAdmin" />
            <div class="text-caption text-grey-7 q-mt-xs">Masters / control-plane are always excluded from burn scheduling.</div>
            <div class="dasm-stat-label q-mt-sm q-mb-xs">Do not tolerate (extra)</div>
            <q-select
              v-model="avoidTaints"
              :options="avoidTaintOptions"
              dense
              outlined
              multiple
              use-input
              use-chips
              new-value-mode="add-unique"
              hide-dropdown-icon
              input-debounce="0"
              :disable="running || !canAdmin"
            />
          </div>
        </q-expansion-item>

        <q-separator />

        <q-expansion-item
          v-model="tools.details"
          dense
          switch-toggle-side
          header-class="tool-head"
          :label="`Prefix ${templatePrefix || 'kb-…'}`"
        >
          <div class="tool-body text-caption">
            <div class="text-mono">{{ displayPattern || `${templatePrefix || 'kb-xxxx'}-{kind}-{seq:05d}-{sfx}` }}</div>
            <div v-if="runPrefix && runPrefix !== templatePrefix" class="q-mt-xs">
              Last / interrupted run: <code>{{ runPrefix }}</code>
            </div>
            <div class="meta-static q-mt-xs">
              NS look like <code>{{ templatePrefix || 'kb-xxxx' }}-ns-00001-…</code>
              · labeled <code>dasm-burner.dasmlab.org/config={{ templateName || '…' }}</code>
            </div>
          </div>
        </q-expansion-item>
      </aside>

      <section class="execute-log dasm-panel log-panel">
        <div class="row items-center justify-between q-mb-sm">
          <div class="dasm-stat-label">Live log</div>
          <q-btn
            flat
            dense
            size="sm"
            icon="restart_alt"
            label="Clear / reset"
            :disable="running || !canAdmin"
            @click="clearLog"
          />
        </div>
        <div ref="logEl" class="log-canvas">
          <div v-for="(line, i) in logs" :key="i" class="log-line" :class="`lv-${line.level}`">
            <span class="log-ts">{{ fmt(line.at) }}</span>
            <span class="log-phase">{{ line.phase }}{{ line.batch ? ` #${line.batch}` : '' }}</span>
            <span>{{ line.message }}</span>
          </div>
          <div v-if="!logs.length" class="text-caption text-grey-5">Waiting for events…</div>
        </div>
      </section>
    </div>

    <q-dialog v-model="maxPodsOpen" persistent>
      <q-card style="min-width: 480px; max-width: 640px">
        <q-card-section>
          <div class="text-h6">Set workers to maxPods={{ maxPodsInput }}?</div>
        </q-card-section>
        <q-card-section class="text-body2">
          <p>
            On <strong>{{ cluster.currentLabel.value }}</strong> this will:
          </p>
          <ol>
            <li>Delete all managed <code>kb-*</code> namespaces on this cluster (required before the kubelet roll).</li>
            <li>Set the worker MachineConfigPool to serial (<code>maxUnavailable=1</code>) so nodes reboot one at a time.</li>
            <li>Apply KubeletConfig <code>dasm-burner-worker-maxpods</code> with <code>maxPods={{ maxPodsInput }}</code>.</li>
          </ol>
          <p>
            Worker-labeled nodes (including infra) will drain and reboot in series. This can take a long time.
            Watch the live log. NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT.
          </p>
          <p v-if="projectedSlots" class="text-caption">
            After roll: ~{{ projectedSlots }} density slots
            <span v-if="selectedMeta?.counts?.pods">
              vs this template’s {{ selectedMeta.counts.pods }} pods
              <span v-if="projectedSlots >= selectedMeta.counts.pods"> — precheck should pass</span>
              <span v-else> — still short; raise maxPods further, add workers, or lower replicas in Topology</span>
            </span>
          </p>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" v-close-popup />
          <q-btn unelevated color="primary" label="Accept and apply" :loading="settingMaxPods" @click="applyMaxPods" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import {
  cancelRun,
  checkCleanupState,
  clearRunLog,
  getCleanupStatus,
  getClusterCapacity,
  getRun,
  getWorkerMaxPods,
  listCleanupReports,
  listTemplates,
  postCleanup,
  postWorkerMaxPods,
  selectTemplate,
  startRun,
} from 'src/services/api'
import api from 'src/services/api'
import { useAuth } from 'src/services/auth'
import { useCluster } from 'src/services/cluster'
import { openLiveStream } from 'src/services/events'

const auth = useAuth()
const canAdmin = computed(() => auth.isAdmin.value)
const cluster = useCluster()
const error = ref('')
const cleanupMsg = ref('')
const lastCleanupReportId = ref('')
const templates = ref([])
const templateName = ref('')
const dryRun = ref(true)
const confirm = ref(false)
const allowLarge = ref(false)
const skipBaseline = ref(true)
const enableOVNDiag = ref(true)
const enableEtcdDiag = ref(true)
const avoidTaints = ref(['node-role.kubernetes.io=infra:NoSchedule'])
const avoidTaintOptions = [
  'node-role.kubernetes.io=infra:NoSchedule',
  'node-role.kubernetes.io/infra:NoSchedule',
]
const starting = ref(false)
const cleaning = ref(false)
const checking = ref(false)
const run = ref(null)
const logEl = ref(null)
const deploy = ref(null)
const managedTotal = ref(null)
const deployCluster = ref('')
const capacity = ref(null)
const maxPodsInput = ref(500)
const maxPodsOpen = ref(false)
const settingMaxPods = ref(false)
const readingMaxPods = ref(false)
const workerMaxPods = ref(null)
const pinnedStepId = ref('')
const tools = reactive({
  cleanup: true,
  kubelet: true,
  safety: false,
  details: false,
})
let liveStream = null
let keepAliveTimer = null
let busCleaning = false

const templateOptions = computed(() =>
  templates.value.map((t) => {
    const pfx = t.prefix ? `${t.prefix} · ` : ''
    if (t.kind === 'OpenShiftObjectPressure') {
      return {
        label: `${t.name} · ${pfx}pressure · ${t.namespaces} NS · ${t.counts?.intendedObjects ?? '?'} objs`,
        value: t.name,
      }
    }
    return {
      label: `${t.name} · ${pfx}${t.namespaces} NS · ${t.counts?.pods ?? '?'} pods`,
      value: t.name,
    }
  }),
)
const selectedMeta = computed(() => templates.value.find((t) => t.name === templateName.value))
const templateRecipe = computed(() => {
  const t = selectedMeta.value
  if (!t) return ''
  if (t.kind === 'OpenShiftObjectPressure') {
    const kinds = (t.objects || []).filter((o) => o.enabled).length
    return `${t.namespaces} NS · ${kinds} kinds · ${t.counts?.intendedObjects ?? '?'} intended`
  }
  const pods = t.counts?.pods ? ` · ${t.counts.pods} pods total` : ''
  return `${t.namespaces} NS · ${t.routesPerNamespace} rt · ${t.servicesPerNamespace} svc · ${t.replicasPerService} pods/svc${pods}`
})
const steps = computed(() => run.value?.steps || [])
const logs = computed(() => run.value?.logs || [])
const runStatus = computed(() => run.value?.status || 'idle')
const running = computed(() => runStatus.value === 'running')
const interrupted = computed(() => runStatus.value === 'interrupted')
const runPrefix = computed(() => run.value?.prefix || '')
const runPattern = computed(() => run.value?.namePattern || '')
const templatePrefix = computed(() => selectedMeta.value?.prefix || '')
const displayPattern = computed(() => {
  if (runPattern.value && runPrefix.value === templatePrefix.value) return runPattern.value
  const pfx = templatePrefix.value || runPrefix.value
  return pfx ? `${pfx}-{kind}-{seq:05d}-{sfx}` : ''
})
const showReportLink = computed(() =>
  Boolean(run.value?.snapshotId || run.value?.reportUrl) &&
  (runStatus.value === 'passed' || runStatus.value === 'failed' || runStatus.value === 'interrupted'),
)
const reportRoute = computed(() => {
  const q = {}
  if (run.value?.snapshotId) q.id = run.value.snapshotId
  return { name: 'report', query: q }
})
const deployOnline = computed(() => Boolean(deploy.value?.deployed))
const deployLabel = computed(() => {
  if (!deploy.value) return 'deploy status…'
  const on = deployCluster.value || cluster.currentLabel.value
  if (deploy.value.deployed) {
    const n = deploy.value.count || 0
    const o = deploy.value.objects
    if (o) {
      return `online on ${on} (${n} NS · ${o.routes ?? 0} rt · ${o.services ?? 0} svc · ${o.readyPods ?? 0}/${o.pods ?? 0} pods)`
    }
    return `online on ${on} (${n} NS)`
  }
  if (managedTotal.value > 0) {
    return `no NS for this template on ${on} · ${managedTotal.value} other managed NS live`
  }
  return `cleaned on ${on}`
})
const deployObjectsCaption = computed(() => {
  const o = deploy.value?.objects
  if (!o || !deploy.value?.deployed) return ''
  const phases = o.podPhases || {}
  const bits = Object.keys(phases).sort().map((k) => `${k}=${phases[k]}`)
  return bits.length ? `Pod phases: ${bits.join(' · ')}` : ''
})
const deployChipColor = computed(() => {
  if (deployOnline.value) return 'warning'
  if (deploy.value?.label === 'cleaned') return 'positive'
  return 'grey-6'
})
const capSlotsLabel = computed(() => {
  const c = capacity.value?.capacity
  if (!c) return '…'
  return `${c.slots ?? '?'} (${c.workerNodes ?? '?'} nodes)`
})
const projectedSlots = computed(() => {
  const nodes = capacity.value?.capacity?.workerNodes
  const n = Number(maxPodsInput.value)
  if (!nodes || !n) return 0
  return nodes * n
})
const maxPodsRolloutColor = computed(() => {
  switch (workerMaxPods.value?.rollout) {
    case 'yes': return 'positive'
    case 'partial': return 'warning'
    case 'no': return 'grey-7'
    default: return 'grey-6'
  }
})
const maxPodsRolloutIcon = computed(() => {
  switch (workerMaxPods.value?.rollout) {
    case 'yes': return 'check_circle'
    case 'partial': return 'timelapse'
    case 'no': return 'cancel'
    default: return 'help'
  }
})
const statusColor = computed(() => {
  switch (runStatus.value) {
    case 'running': return 'warning'
    case 'passed': return 'positive'
    case 'interrupted': return 'orange'
    case 'failed':
    case 'aborted': return 'negative'
    default: return 'grey-6'
  }
})

function shortStepLabel(step) {
  const l = step?.label || ''
  const batch = l.match(/^Batch\s+(\d+)/i)
  if (batch) return `B${batch[1]}`
  if (/settle/i.test(l)) return 'Settle'
  if (/convergence/i.test(l)) return 'Conv'
  return l.split(/[·]/)[0].trim() || step.id
}

const stageDetail = computed(() => {
  const id = pinnedStepId.value || steps.value.find((s) => s.status === 'running')?.id
  const s = steps.value.find((x) => x.id === id)
  if (!s) return ''
  const msg = s.message || s.status
  return msg && msg !== s.label ? `${s.label} — ${msg}` : s.label
})

function fmt(at) {
  try {
    return new Date(at).toLocaleTimeString()
  } catch {
    return ''
  }
}

async function refreshCapacity() {
  try {
    capacity.value = await getClusterCapacity()
    if (capacity.value?.maxPods) {
      applyMaxPodsStatus(capacity.value.maxPods, true)
    } else {
      const typical = capacity.value?.capacity?.maxPodsTypical
      if (typical && !maxPodsOpen.value) {
        maxPodsInput.value = typical < 500 ? 500 : typical
      }
    }
  } catch {
    /* cluster may be unset */
  }
}

function applyMaxPodsStatus(st, fillInput) {
  workerMaxPods.value = st
  if (!fillInput || maxPodsOpen.value) return
  if (st?.configured && st.desired) {
    maxPodsInput.value = st.desired
  } else if (st?.observedTypical) {
    maxPodsInput.value = st.observedTypical
  }
}

async function refreshMaxPods(fillInput) {
  readingMaxPods.value = true
  error.value = ''
  try {
    const data = await getWorkerMaxPods()
    applyMaxPodsStatus(data.maxPods || data, fillInput !== false)
    if (data.run) adoptRun(data.run)
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    readingMaxPods.value = false
  }
}

function openMaxPodsDialog() {
  error.value = ''
  const n = Number(maxPodsInput.value)
  if (!n || n < 110 || n > 2000) {
    error.value = 'maxPods must be between 110 and 2000'
    return
  }
  maxPodsOpen.value = true
}

async function applyMaxPods() {
  error.value = ''
  cleanupMsg.value = ''
  settingMaxPods.value = true
  cleaning.value = true
  startCleanupPoll()
  try {
    const expected = cluster.currentName.value
    await cluster.assertCurrent(expected)
    const data = await postWorkerMaxPods({
      maxPods: Number(maxPodsInput.value),
      confirm: true,
    })
    if (data.run) adoptRun(data.run)
    maxPodsOpen.value = false
    cleanupMsg.value = `Worker maxPods=${maxPodsInput.value} started on ${data.cluster || cluster.currentLabel.value} — watch live log for serial MCP roll.`
    const deadline = Date.now() + 100 * 60 * 1000
    busCleaning = true
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 2000))
      if (!busCleaning) break
    }
    await refreshCapacity()
    await refreshDeploy()
    await refreshMaxPods(true)
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    stopCleanupPoll()
    settingMaxPods.value = false
    cleaning.value = false
  }
}

async function refreshTemplates() {
  const keep = templateName.value
  const data = await listTemplates()
  templates.value = data.templates || []
  const names = new Set(templates.value.map((t) => t.name))
  if (keep && names.has(keep)) {
    return
  }
  templateName.value = data.active || templates.value[0]?.name || ''
}

async function refreshDeploy() {
  if (!templateName.value) {
    deploy.value = null
    return
  }
  try {
    const data = await getCleanupStatus(templateName.value)
    deploy.value = data.template || null
    managedTotal.value = data.managedTotal ?? null
    deployCluster.value = data.cluster || cluster.currentLabel.value
  } catch {
    deploy.value = { deployed: false, label: 'unknown' }
  }
}

async function checkState(reason) {
  error.value = ''
  checking.value = true
  try {
    const data = await checkCleanupState({
      template: templateName.value,
      reason: reason || 'manual refresh',
    })
    if (data.run) adoptRun(data.run)
    deploy.value = data.template || null
    managedTotal.value = data.managedTotal ?? null
    deployCluster.value = data.cluster || cluster.currentLabel.value
    await nextTick()
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    checking.value = false
  }
}

function adoptRun(next) {
  if (!next) return
  const prev = run.value
  if (prev?.id && next.id === prev.id && (prev.logs || []).length > (next.logs || []).length) {
    next = { ...next, logs: prev.logs }
  }
  run.value = next
}

function applyLogEvent(ev) {
  const line = ev?.data
  if (!line) return
  const cl = cluster.currentName.value
  if (ev.cluster && cl && ev.cluster !== cl) return
  const cur = run.value || { status: 'idle', logs: [], steps: [] }
  const logs = [...(cur.logs || []), line]
  if (logs.length > 400) logs.splice(0, logs.length - 400)
  run.value = { ...cur, logs, logSeq: ev.seq || cur.logSeq }
  nextTick(() => {
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
  })
}

function applyRunEvent(ev) {
  const meta = ev?.data
  if (!meta) return
  const cl = cluster.currentName.value
  if (meta.cluster && cl && meta.cluster !== cl) return
  const logs = run.value?.logs || []
  run.value = { ...meta, logs: logs.length ? logs : (meta.logs || []) }
}

function applyCleanupEvent(ev) {
  const d = ev?.data || {}
  if (typeof d.cleaning === 'boolean') busCleaning = d.cleaning
  if (d.template) deploy.value = d.template
  if (d.managedTotal != null) managedTotal.value = d.managedTotal
  if (d.cluster) deployCluster.value = d.cluster
}

function connectLiveStream(after) {
  if (liveStream) {
    liveStream.close()
    liveStream = null
  }
  liveStream = openLiveStream({
    after: after || 0,
    onLog: applyLogEvent,
    onRun: applyRunEvent,
    onCleanup: applyCleanupEvent,
  })
}

function startCleanupPoll() {
  busCleaning = true
}

function stopCleanupPoll() {
  /* SSE carries cleanup progress; no JSON poll loop */
}

async function poll() {
  try {
    const data = await getRun()
    if (data.run) {
      adoptRun(data.run)
      await nextTick()
      if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
    }
    if (!liveStream || liveStream.readyState === EventSource.CLOSED) {
      connectLiveStream(data.run?.logSeq || 0)
    }
  } catch {
    /* ignore */
  }
}

function addAvoidTaint(val, done) {
  const v = String(val || '').trim()
  if (!v) return
  done(v, 'add-unique')
}

async function start() {
  if (!canAdmin.value) return
  error.value = ''
  cleanupMsg.value = ''
  starting.value = true
  try {
    const expected = cluster.currentName.value
    const cur = await cluster.assertCurrent(expected)
    if (cluster.isInCluster.value) {
      const ok = window.confirm(
        `This will burn the HOST in-cluster target:\n\n  ${cur?.name || expected}\n  ${cur?.server || ''}\n\nNOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT.\n\nContinue?`,
      )
      if (!ok) return
    }
    await selectTemplate(templateName.value)
    const payload = {
      template: templateName.value,
      dryRun: dryRun.value,
      confirm: confirm.value,
      allowLarge: allowLarge.value,
      skipBaseline: skipBaseline.value,
      enableOVNDiag: enableOVNDiag.value,
      enableEtcdDiag: enableEtcdDiag.value,
      avoidTaints: [...avoidTaints.value],
    }
    let data
    try {
      data = await startRun(payload)
    } catch (e) {
      const body = e.response?.data
      if (body?.code === 'capacity_exceeded') {
        const cap = body.capacity || {}
        const detail = [
          body.error || 'Run exceeds density pod slots.',
          '',
          `Slots: ${cap.slots ?? '?'} (${cap.workerNodes ?? '?'} nodes × ~${cap.maxPodsTypical ?? '?'} maxPods)`,
          `Run wants: ${cap.podsAsked ?? '?'} pods`,
          `Largest wave: ~${cap.wavePods ?? '?'} pods (${cap.waveNamespaces ?? '?'} NS)`,
          '',
          'Better: raise maxPods, add workers, or lower replicasPerService.',
          '',
          'Proceed anyway? (expect readiness timeouts / Unschedulable)',
        ].join('\n')
        if (!window.confirm(detail)) {
          error.value = body.error || 'capacity exceeded'
          return
        }
        data = await startRun({ ...payload, allowOverCapacity: true })
      } else {
        throw e
      }
    }
    adoptRun(data.run)
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

async function clearLog() {
  error.value = ''
  try {
    const data = await clearRunLog()
    if (data.run) adoptRun(data.run)
    else if (run.value) run.value = { ...run.value, logs: [] }
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
}

async function onTemplate(name) {
  if (!name) return
  templateName.value = name
  await selectTemplate(name)
  await checkState('template selected')
}

async function doCleanup(scope) {
  if (!canAdmin.value) return
  error.value = ''
  cleanupMsg.value = ''
  const labels = {
    last: 'Clean last run for this template on the CURRENT cluster?',
    template: 'Clean ALL recorded runs for this template on the CURRENT cluster?',
    all: 'Clean ALL managed kb-* namespaces on the CURRENT cluster (every session/template)?',
  }
  try {
    const expected = cluster.currentName.value
    const cur = await cluster.assertCurrent(expected)
    if (cluster.isInCluster.value) {
      const ok = window.confirm(
        `Cleanup will run on the HOST in-cluster target:\n\n  ${cur?.name || expected}\n  ${cur?.server || ''}\n\n${labels[scope] || 'Clean?'}\n\nContinue?`,
      )
      if (!ok) return
    } else if (!window.confirm(`${labels[scope] || 'Clean?'}\n\nCluster: ${cur?.name || expected}`)) {
      return
    }
  } catch (e) {
    error.value = e.response?.data?.error || e.message
    return
  }
  cleaning.value = true
  startCleanupPoll()
  try {
    const data = await postCleanup({
      scope,
      template: templateName.value,
      wait: true,
      dryRun: false,
    })
    if (data.run) adoptRun(data.run)
    cleanupMsg.value = `Cleanup ${scope} started in background on ${data.cluster || cluster.currentLabel.value} — watch live log (waits up to ~45m for slow NS deletes).`
    // Poll until server reports cleaning=false (survives route timeouts).
    const deadline = Date.now() + 50 * 60 * 1000
    let reportId = ''
    busCleaning = true
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 2000))
      if (!busCleaning) {
        const latest = await listCleanupReports().catch(() => null)
        reportId = latest?.reports?.[0]?.id || ''
        const st = await getCleanupStatus(templateName.value).catch(() => null)
        cleanupMsg.value = st?.template?.deployed
          ? `Cleanup finished but namespaces remain on ${st.cluster || ''} — see live log.`
          : `Cleanup ${scope} finished on ${st?.cluster || cluster.currentLabel.value}${reportId ? ` · report ${reportId}` : ''}.`
        break
      }
    }
    await refreshDeploy()
    await poll()
    if (reportId) lastCleanupReportId.value = reportId
  } catch (e) {
    error.value = e.response?.data?.error || e.message
    await poll()
  } finally {
    stopCleanupPoll()
    cleaning.value = false
  }
}

watch(templateName, () => {
  refreshDeploy()
})

watch(
  () => cluster.currentName.value,
  async (name, prev) => {
    if (!name || name === prev) return
    await checkState(`cluster switched to ${name}`)
    if (canAdmin.value) await refreshCapacity()
  },
)

onMounted(async () => {
  try {
    await refreshTemplates()
    if (!cluster.ready.value) await cluster.refresh()
    await poll()
    await checkState('page load')
    if (canAdmin.value) await refreshCapacity()
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
  keepAliveTimer = setInterval(() => {
    if (running.value) api.get('/auth/keepalive').catch(() => {})
  }, 240000)
})

onUnmounted(() => {
  if (keepAliveTimer) clearInterval(keepAliveTimer)
  if (liveStream) liveStream.close()
  stopCleanupPoll()
})
</script>

<style scoped>
.execute-hero :deep(.dasm-shell__content) {
  padding: 0.7rem 1rem;
}
.execute-hero .dasm-title {
  margin: 0.15rem 0 0.25rem;
  font-size: clamp(1.2rem, 2vw, 1.65rem);
}
.execute-hero .dasm-subtitle {
  font-size: 0.92rem;
  line-height: 1.4;
}

.control-deck.is-live {
  box-shadow: 0 0 0 1px rgba(224, 184, 74, 0.35), 0 10px 28px rgba(18, 32, 44, 0.08);
}
.control-deck__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(200px, 240px);
  gap: 1rem;
  align-items: stretch;
}
.control-run {
  padding: 0.15rem 0 0.15rem 1rem;
  border-left: 1px solid var(--dasm-border-soft);
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
}
.control-run__name {
  font-family: Fraunces, Georgia, serif;
  font-size: 1.15rem;
  font-weight: 700;
  color: #1d2b36;
  margin-bottom: 0.65rem;
  word-break: break-word;
}
.meta-static {
  font-size: 0.78rem;
  color: #6f7f8d;
  line-height: 1.4;
}
.meta-live {
  font-size: 0.8rem;
  color: #1d2b36;
  font-weight: 600;
}
.pfx-chip {
  display: inline-block;
  margin-right: 0.4rem;
  padding: 0.05rem 0.4rem;
  border-radius: 6px;
  background: #eef3f8;
  color: #1d2b36;
  font-weight: 700;
}
.live-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.45rem;
}
.live-chip {
  max-width: 100%;
}

.stage-rail-wrap {
  margin-top: 0.85rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--dasm-border-soft);
}
.stage-rail {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  row-gap: 0.45rem;
}
.stage {
  appearance: none;
  border: 0;
  background: transparent;
  width: 52px;
  padding: 0;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.2rem;
}
.stage-dot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: #9aa7b2;
  box-shadow: 0 0 0 3px rgba(154, 167, 178, 0.18);
}
.stage-name {
  font-size: 0.68rem;
  letter-spacing: 0.02em;
  color: #607483;
  font-weight: 600;
  line-height: 1.1;
  text-align: center;
}
.stage-join {
  width: 16px;
  height: 2px;
  background: #c5d0d8;
  margin: 6px 1px 0;
  flex-shrink: 0;
}
.stage.is-pending { opacity: 0.5; }
.stage.is-running { opacity: 1; }
.stage.is-running .stage-dot {
  background: #e0b84a;
  box-shadow: 0 0 0 4px rgba(224, 184, 74, 0.28);
  animation: stage-pulse 1.4s ease-in-out infinite;
}
.stage.is-running .stage-name { color: #8a6a12; font-weight: 800; }
.stage.is-passed { opacity: 1; }
.stage.is-passed .stage-dot { background: #56ba6d; box-shadow: 0 0 0 3px rgba(86, 186, 109, 0.22); }
.stage.is-passed .stage-name { color: #2d6b3e; }
.stage.is-failed { opacity: 1; }
.stage.is-failed .stage-dot { background: #cc4757; box-shadow: 0 0 0 3px rgba(204, 71, 87, 0.22); }
.stage.is-failed .stage-name { color: #9a2b38; }
.stage.is-skipped { opacity: 0.6; }
.stage.active .stage-name { text-decoration: underline; text-underline-offset: 2px; }
.stage-join.is-passed { background: #56ba6d; }
.stage-join.is-running { background: #e0b84a; }
.stage-join.is-failed { background: #cc4757; }
.stage-detail {
  margin-top: 0.55rem;
  font-size: 0.78rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  color: #1d2b36;
  background: #f4f7fa;
  border-radius: 8px;
  padding: 0.4rem 0.6rem;
}
@keyframes stage-pulse {
  0%, 100% { box-shadow: 0 0 0 4px rgba(224, 184, 74, 0.22); }
  50% { box-shadow: 0 0 0 7px rgba(224, 184, 74, 0.12); }
}

.execute-body {
  display: grid;
  grid-template-columns: minmax(240px, 300px) minmax(0, 1fr);
  gap: 1rem;
  align-items: start;
}
.execute-tools {
  padding: 0.35rem 0.55rem 0.55rem;
}
.tool-head {
  font-size: 0.78rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #6f7f8d;
  font-weight: 700;
  min-height: 36px;
}
.tool-body {
  padding: 0 0.35rem 0.75rem;
}
.execute-log {
  min-width: 0;
}

.text-mono,
.pfx-chip {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

.log-canvas {
  height: calc(100vh - 17.5rem);
  min-height: 520px;
  max-height: 860px;
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

@media (max-width: 1023px) {
  .control-deck__row,
  .execute-body {
    grid-template-columns: 1fr;
  }
  .control-run {
    border-left: 0;
    padding-left: 0;
    padding-top: 0.75rem;
    border-top: 1px solid var(--dasm-border-soft);
  }
  .log-canvas {
    height: min(50vh, 560px);
    min-height: 360px;
  }
}
</style>
