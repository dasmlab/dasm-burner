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
    <div v-if="interrupted" class="dasm-panel q-mb-md text-warning">
      Run <strong>{{ runPrefix }}</strong> was interrupted (server restart). Pipeline/log restored from disk —
      apply may still be live on the cluster. Use Refresh / check state, then Clean if needed.
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
            <template v-if="selectedMeta.kind === 'OpenShiftObjectPressure'">
              ObjectPressure · {{ selectedMeta.namespaces }} NS ·
              {{ (selectedMeta.objects || []).filter((o) => o.enabled).length }} kinds ·
              {{ selectedMeta.counts?.intendedObjects ?? '?' }} intended objects
            </template>
            <template v-else>
              {{ selectedMeta.namespaces }} NS ·
              {{ selectedMeta.routesPerNamespace }} routes ·
              {{ selectedMeta.servicesPerNamespace }} services ·
              {{ selectedMeta.replicasPerService }} pods/svc
              <span v-if="selectedMeta.counts"> · {{ selectedMeta.counts.pods }} pods total</span>
            </template>
          </div>
          <div class="row items-center q-gutter-sm q-mt-md">
            <q-chip
              dense
              square
              :color="deployChipColor"
              text-color="white"
              :icon="deployOnline ? 'cloud_done' : 'cloud_off'"
            >
              {{ deployLabel }}
            </q-chip>
            <span v-if="deployPrefix" class="text-caption text-mono">{{ deployPrefix }}</span>
            <q-btn
              flat
              dense
              size="sm"
              color="primary"
              icon="sync"
              label="Refresh / check state"
              :loading="checking"
              :disable="running || cleaning"
              @click="checkState('manual refresh')"
            />
          </div>
          <div class="text-caption text-grey-7 q-mt-xs">
            State is live for <strong>{{ cluster.currentLabel.value }}</strong> only — flip clusters and refresh.
          </div>
          <div v-if="deployObjectsCaption" class="text-caption text-grey-7 q-mt-xs">{{ deployObjectsCaption }}</div>
        </div>
      </div>
      <div class="col-12 col-md-4">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Safety</div>
          <q-toggle v-model="dryRun" label="Dry run (no create)" :disable="running" />
          <q-toggle v-model="confirm" label="I understand this loads the control plane" :disable="running || dryRun" />
          <q-toggle v-model="allowLarge" label="Allow >10 namespaces" :disable="running || dryRun" />
          <q-toggle v-model="skipBaseline" label="Skip baseline wait" :disable="running" />
          <q-toggle
            v-model="enableOVNDiag"
            label="Enable OVN Diagnoser samples"
            :disable="running || dryRun"
          />
          <div class="text-caption text-grey-7 q-mb-sm">
            When on: capture OVN baseline, 45s watch, and per-batch samples for the OVN Diagnoser history page.
          </div>
          <div class="dasm-stat-label q-mt-md q-mb-xs">Do not tolerate (taints)</div>
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
            :disable="running"
            hint="Pods will not tolerate these — keeps load off infra nodes"
            @new-value="addAvoidTaint"
          />
        </div>
      </div>
      <div class="col-12 col-md-3">
        <div class="dasm-panel column q-gutter-sm">
          <div class="dasm-stat-label">Cluster</div>
          <div class="text-body2">{{ cluster.currentLabel.value }}</div>
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

    <div v-if="runPrefix" class="dasm-panel q-mb-md run-meta">
      <div class="row items-center q-col-gutter-md">
        <div class="col-12 col-md-auto">
          <div class="dasm-stat-label">Generated prefix</div>
          <div class="text-mono text-weight-bold">{{ runPrefix }}</div>
        </div>
        <div class="col-12 col-md">
          <div class="dasm-stat-label">Name pattern</div>
          <div class="text-mono text-caption">{{ runPattern || `${runPrefix}-{kind}-{seq}-{sfx}` }}</div>
          <div class="text-caption text-grey-7 q-mt-xs">
            Ties burner (<code>kb</code>) → run (<code>{{ run?.runId }}</code>) → object kind → batch seq → unique suffix.
          </div>
        </div>
        <div v-if="showReportLink" class="col-12 col-md-auto">
          <q-btn
            color="secondary"
            unelevated
            icon="assessment"
            label="Open report"
            :to="reportRoute"
          />
        </div>
      </div>
    </div>

    <div class="dasm-panel q-mb-md">
      <div class="row items-center justify-between q-mb-sm">
        <div class="dasm-stat-label">Cleanup</div>
        <div class="text-caption text-grey-7">
          Managed label <code>dasm-burner.dasmlab.org/managed=true</code>
          <span v-if="managedTotal != null"> · {{ managedTotal }} NS live</span>
        </div>
      </div>
      <div class="row q-col-gutter-sm">
        <div class="col-12 col-sm-auto">
          <q-btn
            outline
            color="warning"
            icon="delete_sweep"
            label="Clean last run"
            :loading="cleaning"
            :disable="running || !templateName"
            @click="doCleanup('last')"
          />
        </div>
        <div class="col-12 col-sm-auto">
          <q-btn
            outline
            color="warning"
            icon="folder_delete"
            label="Clean this template"
            :loading="cleaning"
            :disable="running || !templateName"
            @click="doCleanup('template')"
          />
        </div>
        <div class="col-12 col-sm-auto">
          <q-btn
            outline
            color="negative"
            icon="cleaning_services"
            label="Clean all kb- runs"
            :loading="cleaning"
            :disable="running"
            @click="doCleanup('all')"
          />
        </div>
      </div>
    </div>

    <div class="dasm-panel q-mb-md">
      <div class="row items-center justify-between q-mb-sm">
        <div class="dasm-stat-label">Worker kubelet (this cluster)</div>
        <div class="text-caption text-grey-7">
          Density slots {{ capSlotsLabel }} · typical maxPods {{ capacity?.capacity?.maxPodsTypical ?? '…' }}
        </div>
      </div>
      <div class="text-caption text-grey-7 q-mb-sm">
        Replicas stay in the Topology template. This only changes OpenShift worker
        <code>maxPods</code> (KubeletConfig) so Execute precheck can fit.
      </div>
      <div class="row items-end q-col-gutter-sm">
        <div class="col-12 col-sm-3">
          <q-input
            v-model.number="maxPodsInput"
            type="number"
            outlined
            dense
            label="maxPods"
            min="110"
            max="2000"
            :disable="running || cleaning || settingMaxPods"
          />
        </div>
        <div class="col-12 col-sm-auto">
          <q-btn
            outline
            color="primary"
            icon="memory"
            label="Set worker maxPods"
            :loading="settingMaxPods"
            :disable="running || cleaning"
            @click="openMaxPodsDialog"
          />
        </div>
      </div>
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
            <div class="row items-center q-gutter-sm">
              <q-btn
                flat
                dense
                size="sm"
                icon="restart_alt"
                label="Clear / reset"
                :disable="running"
                @click="clearLog"
              />
              <q-chip dense square :color="statusColor" text-color="white">{{ runStatus }}</q-chip>
            </div>
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
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  cancelRun,
  checkCleanupState,
  clearRunLog,
  getCleanupStatus,
  getClusterCapacity,
  getRun,
  listCleanupReports,
  listTemplates,
  postCleanup,
  postWorkerMaxPods,
  selectTemplate,
  startRun,
} from 'src/services/api'
import api from 'src/services/api'
import { useCluster } from 'src/services/cluster'

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
let timer = null
let cleanupPoll = null

const templateOptions = computed(() =>
  templates.value.map((t) => {
    if (t.kind === 'OpenShiftObjectPressure') {
      return {
        label: `${t.name} · pressure · ${t.namespaces} NS · ${t.counts?.intendedObjects ?? '?'} objs`,
        value: t.name,
      }
    }
    return {
      label: `${t.name} · ${t.namespaces} NS · ${t.counts?.pods ?? '?'} pods`,
      value: t.name,
    }
  }),
)
const selectedMeta = computed(() => templates.value.find((t) => t.name === templateName.value))
const steps = computed(() => run.value?.steps || [])
const logs = computed(() => run.value?.logs || [])
const runStatus = computed(() => run.value?.status || 'idle')
const running = computed(() => runStatus.value === 'running')
const interrupted = computed(() => runStatus.value === 'interrupted')
const runPrefix = computed(() => run.value?.prefix || '')
const runPattern = computed(() => run.value?.namePattern || '')
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
const deployPrefix = computed(() => deploy.value?.prefix || '')
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
    const typical = capacity.value?.capacity?.maxPodsTypical
    if (typical && !maxPodsOpen.value) {
      maxPodsInput.value = typical < 500 ? 500 : typical
    }
  } catch {
    /* cluster may be unset */
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
    if (data.run) run.value = data.run
    maxPodsOpen.value = false
    cleanupMsg.value = `Worker maxPods=${maxPodsInput.value} started on ${data.cluster || cluster.currentLabel.value} — watch live log for serial MCP roll.`
    const deadline = Date.now() + 100 * 60 * 1000
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 4000))
      await poll()
      const st = await getCleanupStatus(templateName.value).catch(() => null)
      if (st && !st.cleaning) break
    }
    await refreshCapacity()
    await refreshDeploy()
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    stopCleanupPoll()
    settingMaxPods.value = false
    cleaning.value = false
  }
}

async function refreshTemplates() {
  const data = await listTemplates()
  templates.value = data.templates || []
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
    if (data.run) run.value = data.run
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

function startCleanupPoll() {
  stopCleanupPoll()
  cleanupPoll = setInterval(async () => {
    try {
      const data = await getRun()
      run.value = data.run
      await nextTick()
      if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
    } catch {
      /* ignore */
    }
  }, 800)
}

function stopCleanupPoll() {
  if (cleanupPoll) {
    clearInterval(cleanupPoll)
    cleanupPoll = null
  }
}

async function poll() {
  try {
    const data = await getRun()
    run.value = data.run
    // Rehydrate template selection from the active/restored run so nav-away/back works.
    if (data.run?.template && data.run.template !== templateName.value) {
      if (data.run.status === 'running' || data.run.status === 'interrupted') {
        templateName.value = data.run.template
      }
    }
    await nextTick()
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
    if (!running.value && !cleaning.value) await refreshDeploy()
    // Keep OIDC session alive during long applies so the page doesn't bounce to login.
    if (running.value) {
      api.get('/auth/keepalive').catch(() => {})
    }
  } catch {
    /* ignore poll errors */
  }
}

function addAvoidTaint(val, done) {
  const v = String(val || '').trim()
  if (!v) return
  done(v, 'add-unique')
}

async function start() {
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

async function clearLog() {
  error.value = ''
  try {
    const data = await clearRunLog()
    if (data.run) run.value = data.run
    else if (run.value) run.value = { ...run.value, logs: [] }
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
}

async function onTemplate(name) {
  if (!name) return
  await selectTemplate(name)
  await refreshTemplates()
  await checkState('template selected')
}

async function doCleanup(scope) {
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
    if (data.run) run.value = data.run
    cleanupMsg.value = `Cleanup ${scope} started in background on ${data.cluster || cluster.currentLabel.value} — watch live log (waits up to ~45m for slow NS deletes).`
    // Poll until server reports cleaning=false (survives route timeouts).
    const deadline = Date.now() + 50 * 60 * 1000
    let reportId = ''
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 3000))
      await poll()
      const st = await getCleanupStatus(templateName.value).catch(() => null)
      if (st) {
        deploy.value = st.template || null
        if (!st.cleaning) {
          const latest = await listCleanupReports().catch(() => null)
          reportId = latest?.reports?.[0]?.id || ''
          cleanupMsg.value = st.template?.deployed
            ? `Cleanup finished but namespaces remain on ${st.cluster || ''} — see live log.`
            : `Cleanup ${scope} finished on ${st.cluster || cluster.currentLabel.value}${reportId ? ` · report ${reportId}` : ''}.`
          break
        }
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
    await refreshCapacity()
  },
)

onMounted(async () => {
  try {
    await refreshTemplates()
    if (!cluster.ready.value) await cluster.refresh()
    await poll()
    await checkState('page load')
    await refreshCapacity()
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
  timer = setInterval(poll, 1500)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  stopCleanupPoll()
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
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
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

.run-meta .text-mono,
.text-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

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
