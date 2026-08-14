<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">Starting template</div>
        <h1 class="dasm-title">Compact topology</h1>
        <p class="dasm-subtitle">
          Density (Route→Service→Pod) or ObjectPressure (ConfigMaps/Secrets/CRDs via kube-burner init).
          One Namespace box with instance counts — not N drawn namespaces.
        </p>
      </div>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="dasm-panel q-mb-md">
      <div class="row items-end q-col-gutter-md">
        <div class="col-12 col-md-4">
          <q-select
            v-model="activeName"
            :options="templateOptions"
            label="Saved template"
            dense
            outlined
            emit-value
            map-options
            @update:model-value="onSelectTemplate"
          />
        </div>
        <div class="col-12 col-md-3">
          <q-select
            v-model="model.kind"
            :options="kindOptions"
            label="Basetype"
            dense
            outlined
            emit-value
            map-options
            @update:model-value="onKindChange"
          />
        </div>
        <div class="col-12 col-md-2">
          <q-input v-model="saveAsName" dense outlined label="Save as (optional)" />
        </div>
        <div class="col-auto">
          <q-btn color="primary" unelevated label="Save" :loading="saving" @click="save(false)" />
        </div>
        <div class="col-auto">
          <q-btn outline color="primary" label="Save as" :loading="saving" @click="save(true)" />
        </div>
        <div class="col-auto">
          <q-btn flat color="negative" icon="delete" :disable="!activeName" @click="remove" />
        </div>
      </div>
    </div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-md-2">
        <div class="dasm-panel" v-if="!isPressure">
          <div class="dasm-stat-label q-mb-sm">Palette</div>
          <div
            v-for="p in densityPalette"
            :key="p.kind"
            class="palette-item"
            draggable="true"
            @dragstart="onDrag($event, p.kind)"
            @click="onDropKind(p.kind)"
          >
            <q-icon :name="p.icon" />
            <div>
              <div>{{ p.label }}</div>
              <div class="text-caption text-grey-7">{{ p.hint }}</div>
            </div>
          </div>
        </div>
        <div class="dasm-panel" v-else>
          <div class="dasm-stat-label q-mb-sm">Object kinds</div>
          <div v-for="o in model.objects" :key="o.id" class="q-mb-sm">
            <q-toggle v-model="o.enabled" :label="o.kind || o.id" dense />
            <q-input
              v-if="o.enabled"
              v-model.number="o.replicasPerNamespace"
              type="number"
              min="1"
              dense
              outlined
              label="replicas / NS"
              class="q-mt-xs"
            />
          </div>
          <q-separator class="q-my-sm" />
          <div class="dasm-stat-label q-mb-xs">+ Add Custom</div>
          <q-input v-model="customGVK" dense outlined label="apiVersion/Kind" class="q-mb-xs" hint="e.g. example.com/v1/Widget" />
          <q-input v-model.number="customReplicas" type="number" min="1" dense outlined label="replicas / NS" class="q-mb-xs" />
          <q-btn outline color="primary" dense label="Add custom" class="full-width" @click="addCustom" />
        </div>
      </div>
      <div class="col-12 col-md-7">
        <TopologyCanvas :model="model" :selected="selected" @select="selected = $event" @drop="onDropKind" />
      </div>
      <div class="col-12 col-md-3">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Instances</div>
          <q-input v-model="model.name" label="Name" dense outlined class="q-mb-sm" />
          <q-input v-model.number="model.namespaces" type="number" min="1" label="Namespaces" dense outlined class="q-mb-sm" />
          <template v-if="!isPressure">
            <q-input v-model.number="model.routesPerNamespace" type="number" min="1" label="Routes per NS" dense outlined class="q-mb-sm" />
            <q-input v-model.number="model.servicesPerNamespace" type="number" min="1" label="Services per NS" dense outlined class="q-mb-sm" />
            <q-input v-model.number="model.replicasPerService" type="number" min="1" label="Pods per service" dense outlined class="q-mb-sm" />
            <q-select
              v-model="model.routeToService"
              :options="relOptions"
              label="Route → Service"
              dense
              outlined
              emit-value
              map-options
            />
          </template>
          <template v-else>
            <div class="text-caption text-grey-7">
              Apply path: <strong>kube-burner init</strong>. Missing CRDs are skipped with a warning unless marked required.
            </div>
            <div v-if="selectedObj" class="q-mt-md">
              <div class="dasm-stat-label">Selected · {{ selectedObj.id }}</div>
              <q-input v-model="selectedObj.apiVersion" dense outlined label="apiVersion" class="q-mb-xs" />
              <q-input v-model="selectedObj.kind" dense outlined label="kind" class="q-mb-xs" />
              <q-toggle v-model="selectedObj.required" label="Required (fail if CRD missing)" dense />
              <q-input
                v-if="selectedObj.custom"
                v-model="selectedObj.inlineYAML"
                type="textarea"
                autogrow
                dense
                outlined
                label="inline YAML (optional)"
                class="q-mt-xs"
              />
              <q-btn flat dense color="negative" label="Remove" v-if="selectedObj.custom" @click="removeSelected" />
            </div>
          </template>
        </div>
      </div>
    </div>

    <div class="dasm-grid dasm-grid--4 q-mt-lg">
      <div class="dasm-panel" v-for="row in counts" :key="row.label">
        <div class="dasm-stat-label">{{ row.label }}</div>
        <div class="dasm-stat">{{ row.value }}</div>
      </div>
    </div>

    <div class="dasm-panel q-mt-lg">
      <div class="dasm-stat-label q-mb-sm">kube-burner mapping</div>
      <pre class="kb-pre">{{ mapping }}</pre>
      <q-btn flat dense color="primary" label="Preview init.yml" :loading="previewing" @click="loadPreview" />
      <pre v-if="preview" class="kb-pre q-mt-sm">{{ preview }}</pre>
    </div>
  </q-page>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import TopologyCanvas from 'src/components/TopologyCanvas.vue'
import {
  deleteTemplate,
  getKubeBurnerPreview,
  getTopology,
  listTemplates,
  saveTemplate,
  selectTemplate,
} from 'src/services/api'

const error = ref('')
const saving = ref(false)
const previewing = ref(false)
const preview = ref('')
const selected = ref('ns')
const activeName = ref('')
const saveAsName = ref('')
const templates = ref([])
const customGVK = ref('')
const customReplicas = ref(5)
const iterationVar = '{{.Iteration}}'
const replicaVar = '{{.Replica}}'
const model = reactive({
  name: 'smoke',
  kind: 'OpenShiftNetworkDensity',
  namespaces: 2,
  routesPerNamespace: 2,
  servicesPerNamespace: 2,
  replicasPerService: 3,
  routeToService: 'oneToOne',
  objects: [],
  counts: {},
})

const kindOptions = [
  { label: 'OpenShiftNetworkDensity (Route→Service→Pod)', value: 'OpenShiftNetworkDensity' },
  { label: 'OpenShiftObjectPressure (CRDs / small objects)', value: 'OpenShiftObjectPressure' },
]

const densityPalette = [
  { kind: 'ns', label: 'Namespace', icon: 'folder', hint: 'container × N' },
  { kind: 'route', label: 'Route', icon: 'alt_route', hint: 'per namespace' },
  { kind: 'service', label: 'Service', icon: 'hub', hint: 'per namespace' },
  { kind: 'pod', label: 'Pod', icon: 'memory', hint: 'replicas / service' },
  { kind: 'link', label: 'Relation', icon: 'timeline', hint: 'route ↔ service' },
]

const relOptions = [
  { label: 'oneToOne (Phase 1)', value: 'oneToOne' },
  { label: 'oneToMany (later)', value: 'oneToMany', disable: true },
  { label: 'manyToOne (later)', value: 'manyToOne', disable: true },
]

const isPressure = computed(() => model.kind === 'OpenShiftObjectPressure')
const selectedObj = computed(() => (model.objects || []).find((o) => o.id === selected.value) || null)

const templateOptions = computed(() =>
  templates.value.map((t) => ({
    label: `${t.name} · ${t.kind === 'OpenShiftObjectPressure' ? 'pressure' : 'density'} · ${t.namespaces} NS`,
    value: t.name,
  })),
)

const counts = computed(() => {
  const c = model.counts || {}
  if (isPressure.value) {
    const enabled = (model.objects || []).filter((o) => o.enabled)
    const objs = enabled.reduce((a, o) => a + model.namespaces * (o.replicasPerNamespace || 0), 0)
    return [
      { label: 'Namespaces', value: model.namespaces },
      { label: 'Enabled kinds', value: enabled.length },
      { label: 'Objects', value: objs },
      { label: 'Intended', value: (c.intendedObjects ?? model.namespaces + objs) },
    ]
  }
  return [
    { label: 'Namespaces', value: c.namespaces ?? model.namespaces },
    { label: 'Routes', value: c.routes ?? model.namespaces * model.routesPerNamespace },
    { label: 'Services', value: c.services ?? model.namespaces * model.servicesPerNamespace },
    { label: 'Pods', value: c.pods ?? model.namespaces * model.servicesPerNamespace * model.replicasPerService },
  ]
})

const mapping = computed(() => {
  if (isPressure.value) {
    const lines = (model.objects || [])
      .filter((o) => o.enabled)
      .map((o) => `      - ${o.kind} replicas: ${o.replicasPerNamespace}`)
      .join('\n')
    return `jobs:
  - name: object-pressure
    jobIterations: ${model.namespaces}
    namespacedIterations: true
    # apply: kube-burner init
    objects:
${lines || '      # none enabled'}`
  }
  return `global:
  measurements: [podLatency, serviceLatency]
jobs:
  - name: route-service-density
    jobIterations: ${model.namespaces}
    namespacedIterations: true
    objects:
      - objectTemplate: objectTemplates/route.yml      replicas: ${model.routesPerNamespace}
      - objectTemplate: objectTemplates/service.yml    replicas: ${model.servicesPerNamespace}
      - objectTemplate: objectTemplates/deployment.yml replicas: ${model.servicesPerNamespace}
        # spec.replicas: ${model.replicasPerService}
    # templates use ${iterationVar} / ${replicaVar}
    # relation: ${model.routeToService}`
})

const defaultPressureObjects = () => ([
  { id: 'configmap', enabled: true, apiVersion: 'v1', kind: 'ConfigMap', replicasPerNamespace: 10, templateRef: 'configmap' },
  { id: 'secret', enabled: true, apiVersion: 'v1', kind: 'Secret', replicasPerNamespace: 10, templateRef: 'secret' },
  { id: 'serviceaccount', enabled: true, apiVersion: 'v1', kind: 'ServiceAccount', replicasPerNamespace: 5, templateRef: 'serviceaccount' },
  { id: 'rolebinding', enabled: true, apiVersion: 'rbac.authorization.k8s.io/v1', kind: 'RoleBinding', replicasPerNamespace: 5, templateRef: 'rolebinding' },
  { id: 'networkpolicy', enabled: false, apiVersion: 'networking.k8s.io/v1', kind: 'NetworkPolicy', replicasPerNamespace: 2, templateRef: 'networkpolicy' },
  { id: 'limitrange', enabled: false, apiVersion: 'v1', kind: 'LimitRange', replicasPerNamespace: 1, templateRef: 'limitrange' },
  { id: 'resourcequota', enabled: false, apiVersion: 'v1', kind: 'ResourceQuota', replicasPerNamespace: 1, templateRef: 'resourcequota' },
  { id: 'egressfirewall', enabled: false, apiVersion: 'k8s.ovn.org/v1', kind: 'EgressFirewall', replicasPerNamespace: 1, templateRef: 'egressfirewall' },
])

function applyTopo(t) {
  Object.assign(model, {
    name: t.name,
    kind: t.kind || 'OpenShiftNetworkDensity',
    namespaces: t.namespaces,
    routesPerNamespace: t.routesPerNamespace,
    servicesPerNamespace: t.servicesPerNamespace,
    replicasPerService: t.replicasPerService,
    routeToService: t.routeToService,
    objects: t.objects?.length ? structuredClone(t.objects) : (t.kind === 'OpenShiftObjectPressure' ? defaultPressureObjects() : []),
    counts: t.counts || {},
  })
}

function onKindChange(kind) {
  if (kind === 'OpenShiftObjectPressure') {
    model.name = model.name === 'smoke' ? 'object-pressure' : model.name
    if (!model.objects?.length) model.objects = defaultPressureObjects()
  } else if (!model.objects) {
    model.objects = []
  }
}

function onDrag(ev, kind) {
  ev.dataTransfer.setData('text/plain', kind)
}

function onDropKind(kind) {
  if (isPressure.value) return
  if (kind === 'ns') {
    if (model.namespaces < 1) model.namespaces = 2
    selected.value = 'ns'
    return
  }
  if (kind === 'route') {
    model.routesPerNamespace += 1
    if (model.routeToService === 'oneToOne') model.servicesPerNamespace = model.routesPerNamespace
    selected.value = 'route'
    return
  }
  if (kind === 'service') {
    model.servicesPerNamespace += 1
    if (model.routeToService === 'oneToOne') model.routesPerNamespace = model.servicesPerNamespace
    selected.value = 'service'
    return
  }
  if (kind === 'pod') {
    model.replicasPerService += 1
    selected.value = 'pod'
    return
  }
  if (kind === 'link') {
    model.routeToService = 'oneToOne'
    const n = Math.max(model.routesPerNamespace, model.servicesPerNamespace, 1)
    model.routesPerNamespace = n
    model.servicesPerNamespace = n
  }
}

function addCustom() {
  const raw = (customGVK.value || '').trim()
  if (!raw) {
    error.value = 'Enter apiVersion/Kind (e.g. example.com/v1/Widget)'
    return
  }
  const parts = raw.split('/').filter(Boolean)
  if (parts.length < 2) {
    error.value = 'Custom GVK needs at least group/version/kind or v1/Kind'
    return
  }
  const kind = parts[parts.length - 1]
  const apiVersion = parts.slice(0, -1).join('/')
  const id = `custom-${kind.toLowerCase()}-${Date.now().toString(36)}`
  model.objects.push({
    id,
    enabled: true,
    custom: true,
    apiVersion,
    kind,
    replicasPerNamespace: Number(customReplicas.value) || 1,
    inlineYAML: '',
  })
  selected.value = id
  customGVK.value = ''
  error.value = ''
}

function removeSelected() {
  if (!selectedObj.value?.custom) return
  model.objects = model.objects.filter((o) => o.id !== selectedObj.value.id)
  selected.value = 'ns'
}

async function refreshTemplates() {
  const data = await listTemplates()
  templates.value = data.templates || []
  activeName.value = data.active || templates.value[0]?.name || ''
}

async function load() {
  error.value = ''
  try {
    await refreshTemplates()
    const t = await getTopology()
    applyTopo(t)
    if (t.activeTemplate) activeName.value = t.activeTemplate
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
}

async function onSelectTemplate(name) {
  if (!name) return
  error.value = ''
  try {
    const data = await selectTemplate(name)
    applyTopo(data.topology)
    activeName.value = name
    saveAsName.value = ''
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
}

async function save(asNew) {
  saving.value = true
  error.value = ''
  try {
    const body = {
      name: model.name || activeName.value || 'smoke',
      kind: model.kind,
      namespaces: Number(model.namespaces),
      routesPerNamespace: Number(model.routesPerNamespace),
      servicesPerNamespace: Number(model.servicesPerNamespace),
      replicasPerService: Number(model.replicasPerService),
      routeToService: model.routeToService,
      objects: model.objects || [],
    }
    if (asNew) {
      body.saveAs = (saveAsName.value || model.name || '').trim()
      if (!body.saveAs) throw new Error('Save as requires a new name')
    }
    const data = await saveTemplate(body)
    templates.value = data.templates || []
    activeName.value = data.saved
    applyTopo(data.topology)
    saveAsName.value = ''
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!activeName.value) return
  error.value = ''
  try {
    await deleteTemplate(activeName.value)
    await load()
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
}

async function loadPreview() {
  previewing.value = true
  error.value = ''
  try {
    await save(false)
    const p = await getKubeBurnerPreview()
    preview.value = p.initYaml || ''
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    previewing.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.palette-item {
  display: flex;
  gap: 0.6rem;
  align-items: flex-start;
  padding: 0.55rem 0.4rem;
  border-radius: 10px;
  cursor: grab;
}
.palette-item:hover {
  background: rgba(47, 143, 125, 0.08);
}
.kb-pre {
  margin: 0;
  white-space: pre-wrap;
  font-size: 0.78rem;
  line-height: 1.45;
  color: #1d2b36;
}
</style>
