<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">Starting template</div>
        <h1 class="dasm-title">Compact topology</h1>
        <p class="dasm-subtitle">
          Density (Route→Service→Pod) or ObjectPressure (ConfigMaps/Secrets/CRDs via kube-burner init).
          Name and namespace count live in the bar above; kinds take the main column.
        </p>
      </div>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="dasm-panel q-mb-md">
      <div class="row items-end q-col-gutter-md">
        <div class="col-12 col-md-3">
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
        <div class="col-12 col-sm-4 col-md-2">
          <q-input v-model="model.name" dense outlined label="Name" />
        </div>
        <div class="col-6 col-sm-3 col-md-1">
          <q-input v-model.number="model.namespaces" type="number" min="1" dense outlined label="NS" />
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
      <div class="text-caption text-grey-7 q-mt-sm">
        <span v-if="activePrefix">
          Cluster prefix <code class="text-mono">{{ activePrefix }}</code>
          — Save as assigns a new prefix so copies do not share smoke’s <code>kb-6a98</code>.
        </span>
        <span v-if="isPressure">
          <span v-if="activePrefix"> · </span>
          Apply is <strong>kube-burner init</strong>. Toggle kinds below; counts are replicas per namespace.
          Missing CRDs skip with a warning unless marked required.
        </span>
      </div>
    </div>

    <div v-if="isPressure" class="row q-col-gutter-md">
      <div class="col-12 col-lg-8">
        <div class="dasm-panel object-kinds-panel">
          <div class="row items-center q-col-gutter-sm q-mb-sm">
            <div class="col-auto dasm-stat-label">Object kinds</div>
            <div class="col-12 col-sm">
              <q-input v-model="kindFilter" dense outlined debounce="100" placeholder="Filter kinds or GVK">
                <template #prepend><q-icon name="search" /></template>
                <template #append>
                  <q-icon v-if="kindFilter" name="close" class="cursor-pointer" @click="kindFilter = ''" />
                </template>
              </q-input>
            </div>
            <div class="col-auto text-caption text-grey-7">{{ enabledKindCount }} on · n = replicas / NS</div>
          </div>
          <div class="kind-scroll">
            <div v-for="g in groupedObjects" :key="g.id" class="kind-group">
              <div class="kind-group__title">{{ g.label }}</div>
              <div class="kind-grid">
                <div
                  v-for="o in g.items"
                  :key="o.id"
                  class="kind-row"
                  :class="{ on: o.enabled, active: selected === o.id }"
                  @click="selected = o.id"
                >
                  <q-toggle v-model="o.enabled" dense @click.stop />
                  <div class="kind-row__meta">
                    <div class="kind-row__name">{{ o.kind || o.id }}</div>
                    <div class="kind-row__gvk">{{ o.apiVersion }}{{ o.clusterScoped ? ' · cluster' : '' }}</div>
                  </div>
                  <q-input
                    v-model.number="o.replicasPerNamespace"
                    type="number"
                    min="1"
                    dense
                    outlined
                    input-class="text-right"
                    class="kind-row__n"
                    :disable="!o.enabled"
                    @click.stop
                  />
                </div>
              </div>
            </div>
            <div v-if="!groupedObjects.length" class="text-caption text-grey-7 q-pa-sm">No kinds match that filter.</div>
          </div>
          <q-separator class="q-my-sm" />
          <div class="row items-end q-col-gutter-sm">
            <div class="col-12 col-md-6">
              <q-input v-model="customGVK" dense outlined label="+ Add kind or apiVersion/Kind" hint="Pod, subjectaccessreviews, or example.com/v1/Widget" />
            </div>
            <div class="col-6 col-md-3">
              <q-input v-model.number="customReplicas" type="number" min="1" dense outlined label="replicas / NS" />
            </div>
            <div class="col-6 col-md-3">
              <q-btn outline color="primary" class="full-width" label="Add" @click="addCustom" />
            </div>
          </div>
          <div v-if="selectedObj" class="selected-kind q-mt-md">
            <div class="dasm-stat-label q-mb-xs">Selected · {{ selectedObj.kind || selectedObj.id }}</div>
            <div class="row q-col-gutter-sm items-end">
              <div class="col-12 col-sm-4">
                <q-input v-model="selectedObj.apiVersion" dense outlined label="apiVersion" />
              </div>
              <div class="col-12 col-sm-4">
                <q-input v-model="selectedObj.kind" dense outlined label="kind" />
              </div>
              <div class="col-12 col-sm-4">
                <q-toggle v-model="selectedObj.required" label="Required (fail if CRD missing)" dense />
                <q-btn v-if="selectedObj.custom" flat dense color="negative" label="Remove custom" @click="removeSelected" />
              </div>
              <div class="col-12" v-if="selectedObj.custom">
                <q-input v-model="selectedObj.inlineYAML" type="textarea" autogrow dense outlined label="inline YAML (optional)" />
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="col-12 col-lg-4">
        <div class="topo-sticky">
          <TopologyCanvas compact :model="model" :selected="selected" @select="selected = $event" @drop="onDropKind" />
        </div>
      </div>
    </div>

    <div v-else class="row q-col-gutter-md">
      <div class="col-12 col-md-3">
        <div class="dasm-panel">
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
      </div>
      <div class="col-12 col-md-6">
        <TopologyCanvas :model="model" :selected="selected" @select="selected = $event" @drop="onDropKind" />
      </div>
      <div class="col-12 col-md-3">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Density mix</div>
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
const kindFilter = ref('')
const catalog = ref([])
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
const enabledKindCount = computed(() => (model.objects || []).filter((o) => o.enabled).length)

function normGVK(s) {
  return (s || '').trim().toLowerCase().replace(/\s+/g, '')
}

const categoryLabels = {
  core: 'Core',
  rbac: 'RBAC',
  authz: 'Authz / reviews',
  coord: 'Coordination',
  net: 'Network',
  scale: 'Autoscaling',
  observe: 'Observability',
  openshift: 'OpenShift',
  custom: 'Custom CRDs',
}

const groupedObjects = computed(() => {
  const q = normGVK(kindFilter.value)
  const groups = []
  const by = {}
  for (const o of model.objects || []) {
    const hay = normGVK(`${o.kind || ''} ${o.id || ''} ${o.apiVersion || ''} ${o.resource || ''}`)
    if (q && !hay.includes(q)) continue
    const cat = o.category || (o.custom ? 'custom' : 'core')
    if (!by[cat]) {
      by[cat] = { id: cat, label: categoryLabels[cat] || cat, items: [] }
      groups.push(by[cat])
    }
    by[cat].items.push(o)
  }
  return groups.filter((g) => g.items.length)
})

const templateOptions = computed(() =>
  templates.value.map((t) => ({
    label: `${t.name} · ${t.prefix ? t.prefix + ' · ' : ''}${t.kind === 'OpenShiftObjectPressure' ? 'pressure' : 'density'} · ${t.namespaces} NS`,
    value: t.name,
  })),
)
const activePrefix = computed(() => templates.value.find((t) => t.name === activeName.value)?.prefix || '')

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

const defaultPressureObjects = () => structuredClone(catalog.value || [])

function applyTopo(t) {
  if (t.catalog?.length) catalog.value = t.catalog
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

function lookupCatalog(raw) {
  const key = normGVK(raw)
  if (!key) return null
  const list = [...(catalog.value || []), ...(model.objects || [])]
  return list.find((o) => {
    const ids = [o.id, o.kind, o.resource, `${o.apiVersion}/${o.kind}`, `${o.apiVersion}/${o.resource}`]
    return ids.some((x) => normGVK(x) === key)
  }) || null
}

function addCustom() {
  const raw = (customGVK.value || '').trim()
  if (!raw) {
    error.value = 'Enter a kind (subjectaccessreviews) or apiVersion/Kind (example.com/v1/Widget)'
    return
  }
  const hit = lookupCatalog(raw)
  if (hit) {
    const existing = (model.objects || []).find((o) => o.id === hit.id || (o.kind === hit.kind && o.apiVersion === hit.apiVersion))
    if (existing) {
      existing.enabled = true
      existing.custom = false
      if (hit.apiVersion) existing.apiVersion = hit.apiVersion
      if (hit.kind) existing.kind = hit.kind
      if (hit.templateRef) existing.templateRef = hit.templateRef
      if (hit.category) existing.category = hit.category
      existing.replicasPerNamespace = Number(customReplicas.value) || existing.replicasPerNamespace || 1
      selected.value = existing.id
      customGVK.value = ''
      error.value = ''
      return
    }
    const copy = structuredClone(hit)
    copy.enabled = true
    copy.custom = false
    copy.replicasPerNamespace = Number(customReplicas.value) || copy.replicasPerNamespace || 1
    model.objects.push(copy)
    selected.value = copy.id
    customGVK.value = ''
    error.value = ''
    return
  }
  const parts = raw.split('/').filter(Boolean)
  if (parts.length < 2) {
    error.value = 'Unknown kind — use apiVersion/Kind for a custom CRD (e.g. example.com/v1/Widget)'
    return
  }
  const kind = parts[parts.length - 1]
  const apiVersion = parts.slice(0, -1).join('/')
  const id = `custom-${kind.toLowerCase()}-${Date.now().toString(36)}`
  model.objects.push({
    id,
    enabled: true,
    custom: true,
    category: 'custom',
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
.object-kinds-panel {
  display: flex;
  flex-direction: column;
}
.kind-scroll {
  overflow-y: auto;
  min-height: 420px;
  max-height: min(72vh, 760px);
  padding-right: 4px;
}
.kind-group__title {
  font-size: 0.68rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #5a6b78;
  margin: 0.55rem 0 0.25rem;
}
.kind-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0 0.4rem;
}
@media (min-width: 720px) {
  .kind-grid {
    grid-template-columns: 1fr 1fr;
  }
}
@media (min-width: 1600px) {
  .kind-grid {
    grid-template-columns: 1fr 1fr 1fr;
  }
}
.kind-row {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.28rem 0.2rem;
  border-radius: 8px;
  cursor: pointer;
}
.kind-row:hover,
.kind-row.active {
  background: rgba(47, 143, 125, 0.08);
}
.kind-row.on .kind-row__name {
  font-weight: 700;
}
.kind-row__meta {
  flex: 1;
  min-width: 0;
}
.kind-row__name {
  font-size: 0.86rem;
  line-height: 1.15;
  color: #12202c;
}
.kind-row__gvk {
  font-size: 0.68rem;
  color: #5a6b78;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.kind-row__n {
  width: 64px;
  flex: 0 0 64px;
}
.selected-kind {
  padding-top: 0.5rem;
  border-top: 1px solid rgba(18, 32, 44, 0.08);
}
@media (min-width: 1024px) {
  .topo-sticky {
    position: sticky;
    top: 0.75rem;
  }
}
</style>
