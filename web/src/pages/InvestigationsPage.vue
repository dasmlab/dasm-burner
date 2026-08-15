<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">North star</div>
        <h1 class="dasm-title">Investigations</h1>
        <p class="dasm-subtitle">
          Possible issues, testable patches, and the evidence we keep while we try them.
          Catalog items ship in git. This instance overlays status and notes on the PVC.
          Source map pins which piece and which files we are looking at.
        </p>
      </div>
    </div>

    <div class="row items-center q-gutter-sm q-mb-md">
      <q-btn flat color="primary" icon="refresh" label="Reload" :loading="loading" @click="loadList" />
      <q-btn
        v-if="canAdmin"
        unelevated
        color="primary"
        icon="add"
        label="New investigation"
        @click="createOpen = true"
      />
      <router-link class="text-caption" :to="{ name: 'isolation' }">Isolated wave</router-link>
      <span class="text-caption text-grey-5">·</span>
      <router-link class="text-caption" :to="{ name: 'source-map' }">Source map</router-link>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>
    <div v-if="notice" class="dasm-panel q-mb-md text-positive">{{ notice }}</div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-md-4">
        <div class="dasm-panel">
          <div class="dasm-stat-label q-mb-sm">Open work</div>
          <div v-if="!list.length" class="text-caption text-grey-7">No investigations yet.</div>
          <div
            v-for="inv in list"
            :key="inv.id"
            class="inv-row"
            :class="{ 'is-active': inv.id === selectedId }"
            @click="select(inv.id)"
          >
            <div class="row items-center justify-between">
              <q-badge :color="statusColor(inv.status)" text-color="white">{{ inv.status }}</q-badge>
              <q-badge v-if="inv.catalog" outline color="primary">catalog</q-badge>
            </div>
            <div class="text-body2 q-mt-xs">{{ inv.title }}</div>
            <div class="text-caption text-grey-7">
              {{ (inv.pieces || []).join(' · ') || 'unscoped' }}
              <span v-if="inv.protocol"> · {{ inv.protocol }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="col-12 col-md-8">
        <div v-if="!selected" class="dasm-panel text-caption text-grey-7">
          Select an investigation. The watch-cache leftover-RSS finding is seeded here so it cannot vanish with chat.
        </div>
        <div v-else class="dasm-panel">
          <div class="row items-center q-gutter-sm q-mb-sm">
            <q-badge :color="statusColor(selected.status)" text-color="white">{{ selected.status }}</q-badge>
            <span class="text-mono text-caption">{{ selected.id }}</span>
            <q-badge v-if="selected.catalog" outline color="primary">catalog</q-badge>
          </div>
          <h2 class="inv-title">{{ selected.title }}</h2>
          <p class="text-caption text-grey-7 q-mb-md">
            {{ selected.openshift }} / {{ selected.kubernetes }}
            <span v-if="selected.cluster"> · {{ selected.cluster }}</span>
          </p>

          <div v-if="canAdmin" class="row items-center q-gutter-sm q-mb-md">
            <q-select
              v-model="statusEdit"
              :options="statusOptions"
              dense
              outlined
              emit-value
              map-options
              label="Status"
              style="min-width: 180px"
              @update:model-value="saveStatus"
            />
          </div>

          <q-expansion-item class="inv-exp" dense switch-toggle-side expand-separator label="Pieces" default-opened>
            <div class="row q-gutter-xs q-mb-sm q-mt-xs">
              <q-chip
                v-for="p in selected.pieces || []"
                :key="p"
                square
                clickable
                color="primary"
                text-color="white"
                :to="{ name: 'source-map' }"
              >{{ p }}</q-chip>
            </div>
          </q-expansion-item>

          <q-expansion-item class="inv-exp" dense switch-toggle-side expand-separator label="Hypothesis" default-opened>
            <p class="q-mt-sm">{{ selected.hypothesis }}</p>
            <p v-if="selected.metric"><code>{{ selected.metric }}</code></p>
            <p>
              Protocol:
              <router-link :to="{ name: 'isolation' }">{{ selected.protocol || 'isolated-wave' }}</router-link>
            </p>
          </q-expansion-item>

          <q-expansion-item
            v-if="selected.testPlan?.length"
            class="inv-exp"
            dense
            switch-toggle-side
            expand-separator
            label="Test plan"
          >
            <ol class="detail-list q-mt-sm">
              <li v-for="(step, i) in selected.testPlan" :key="i">{{ step }}</li>
            </ol>
          </q-expansion-item>

          <q-expansion-item
            v-if="selected.sourceFiles?.length"
            class="inv-exp"
            dense
            switch-toggle-side
            expand-separator
            label="Source pins"
            default-opened
          >
            <ul class="detail-list q-mt-sm">
              <li v-for="f in selected.sourceFiles" :key="f.path">
                <a :href="f.url" target="_blank" rel="noopener"><code>{{ f.path }}</code></a>
                <span v-if="f.lines"> :{{ f.lines }}</span>
                — {{ f.why }}
                <span v-if="f.forkUrl">
                  · <a :href="f.forkUrl" target="_blank" rel="noopener">RH tree</a>
                </span>
              </li>
            </ul>
          </q-expansion-item>

          <q-expansion-item
            v-if="selected.possibleFix"
            class="inv-exp"
            dense
            switch-toggle-side
            expand-separator
            label="Possible code fix / experiment"
            default-opened
          >
            <div class="fix-box q-mt-sm q-mb-sm">
              <p>{{ selected.possibleFix.title }}</p>
              <p class="text-caption"><strong>Metric:</strong> {{ selected.possibleFix.metric }}</p>
              <p class="text-caption">{{ selected.possibleFix.action }}</p>
              <p class="text-caption text-grey-7 q-mb-none">
                A later fork patch is still this investigation — same id, status → experiment / patched, new evidence.
              </p>
            </div>
          </q-expansion-item>

          <q-expansion-item
            v-if="selected.notes"
            class="inv-exp"
            dense
            switch-toggle-side
            expand-separator
            label="Notes"
          >
            <p class="q-mt-sm">{{ selected.notes }}</p>
          </q-expansion-item>

          <q-expansion-item class="inv-exp" dense switch-toggle-side expand-separator label="Evidence" default-opened>
          <div v-if="!(selected.evidence || []).length" class="text-caption text-grey-7 q-mb-md">
            None yet. Isolated-wave samples and leftover RSS belong here.
          </div>
          <div v-for="(ev, i) in selected.evidence || []" :key="i" class="ev-row">
            <div class="text-caption text-grey-7">
              {{ fmtTime(ev.at) }}
              <span v-if="ev.runId"> · run {{ ev.runId }}</span>
              <span v-if="ev.cluster"> · {{ ev.cluster }}</span>
            </div>
            <div>{{ ev.note }}</div>
          </div>

          <div v-if="canAdmin" class="q-mt-md">
            <q-input
              v-model="evidenceNote"
              type="textarea"
              autogrow
              outlined
              dense
              label="Add evidence"
              hint="What we saw, which run, leftover RSS / capacity."
            />
            <q-input v-model="evidenceRun" class="q-mt-sm" dense outlined label="Run id (optional)" />
            <q-btn
              class="q-mt-sm"
              unelevated
              color="primary"
              label="Append evidence"
              :disable="!evidenceNote.trim()"
              :loading="saving"
              @click="appendEvidence"
            />
          </div>
          </q-expansion-item>
        </div>
      </div>
    </div>

    <q-dialog v-model="createOpen" persistent>
      <q-card style="min-width: 520px; max-width: 640px">
        <q-card-section>
          <div class="text-h6">New investigation</div>
          <div class="text-caption text-grey-7">
            A repeatable hypothesis tied to source-map pieces. Guest can read it; only admin writes.
          </div>
        </q-card-section>
        <q-card-section>
          <q-input v-model="draft.title" outlined dense label="Title" class="q-mb-sm" />
          <q-select
            v-model="draft.pieces"
            :options="pieceOptions"
            multiple
            outlined
            dense
            use-chips
            label="Pieces"
            class="q-mb-sm"
          />
          <q-input v-model="draft.hypothesis" type="textarea" autogrow outlined dense label="Hypothesis" class="q-mb-sm" />
          <q-input v-model="draft.metric" outlined dense label="Metric to watch" class="q-mb-sm" />
          <q-input v-model="draft.protocol" outlined dense label="Protocol" hint="isolated-wave" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" v-close-popup />
          <q-btn color="primary" unelevated label="Create" :loading="saving" :disable="!draft.title.trim()" @click="create" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  getInvestigations,
  getInvestigation,
  createInvestigation,
  updateInvestigation,
  addInvestigationEvidence,
} from 'src/services/api'
import { useAuth } from 'src/services/auth'

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const canAdmin = computed(() => auth.isAdmin.value)

const list = ref([])
const selected = ref(null)
const selectedId = computed(() => selected.value?.id || route.params.id || '')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const createOpen = ref(false)
const evidenceNote = ref('')
const evidenceRun = ref('')
const statusEdit = ref('')

const pieceOptions = ['kube-apiserver', 'etcd', 'ovn-kube', 'oauth-apiserver']
const statusOptions = [
  { label: 'open', value: 'open' },
  { label: 'hypothesis', value: 'hypothesis' },
  { label: 'experiment', value: 'experiment' },
  { label: 'patched', value: 'patched' },
  { label: 'closed', value: 'closed' },
]
const draft = ref({
  title: '',
  pieces: ['kube-apiserver'],
  hypothesis: '',
  metric: '',
  protocol: 'isolated-wave',
})

function statusColor(st) {
  return {
    open: 'grey-7',
    hypothesis: 'primary',
    experiment: 'orange-8',
    patched: 'teal-7',
    closed: 'grey-5',
  }[st] || 'grey-7'
}

function fmtTime(iso) {
  if (!iso) return '—'
  try {
    return new Date(iso).toISOString().replace('T', ' ').replace(/\.\d+Z$/, 'Z')
  } catch {
    return iso
  }
}

function select(id) {
  if (route.params.id === id) {
    loadOne(id)
    return
  }
  router.push({ name: 'investigations', params: { id } })
}

async function loadList() {
  loading.value = true
  error.value = ''
  try {
    const data = await getInvestigations()
    list.value = data.investigations || []
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

async function loadOne(id) {
  if (!id) {
    selected.value = null
    return
  }
  try {
    const data = await getInvestigation(id)
    selected.value = data.investigation
    statusEdit.value = selected.value?.status || ''
  } catch (e) {
    error.value = e.response?.data?.error || e.message
    selected.value = null
  }
}

async function saveStatus(status) {
  if (!selected.value || !canAdmin.value) return
  saving.value = true
  error.value = ''
  try {
    const data = await updateInvestigation(selected.value.id, { status })
    selected.value = data.investigation
    await loadList()
    notice.value = 'Status saved'
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    saving.value = false
  }
}

async function appendEvidence() {
  if (!selected.value || !evidenceNote.value.trim()) return
  saving.value = true
  error.value = ''
  try {
    const data = await addInvestigationEvidence(selected.value.id, {
      note: evidenceNote.value.trim(),
      runId: evidenceRun.value.trim() || undefined,
    })
    selected.value = data.investigation
    evidenceNote.value = ''
    evidenceRun.value = ''
    await loadList()
    notice.value = 'Evidence appended'
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    saving.value = false
  }
}

async function create() {
  saving.value = true
  error.value = ''
  try {
    const data = await createInvestigation({
      title: draft.value.title.trim(),
      pieces: draft.value.pieces,
      hypothesis: draft.value.hypothesis.trim(),
      metric: draft.value.metric.trim(),
      protocol: draft.value.protocol.trim() || 'isolated-wave',
    })
    createOpen.value = false
    draft.value = { title: '', pieces: ['kube-apiserver'], hypothesis: '', metric: '', protocol: 'isolated-wave' }
    await loadList()
    select(data.investigation.id)
    notice.value = 'Investigation created'
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    saving.value = false
  }
}

watch(
  () => route.params.id,
  (id) => {
    if (id) loadOne(id)
  },
)

onMounted(async () => {
  await loadList()
  const id = route.params.id || list.value[0]?.id
  if (id && !route.params.id) {
    router.replace({ name: 'investigations', params: { id } })
    return
  }
  if (id) await loadOne(id)
})
</script>

<style scoped>
.inv-row {
  cursor: pointer;
  padding: 0.65rem 0.5rem;
  border-bottom: 1px solid var(--dasm-border, #d5e4e0);
}
.inv-row.is-active {
  background: rgba(63, 122, 107, 0.1);
}
.inv-title {
  font-size: 1.25rem;
  margin: 0 0 0.35rem;
  line-height: 1.3;
}
.inv-exp :deep(.q-item) { padding-left: 0; min-height: 2.1rem; }
.inv-exp :deep(.q-item__label) {
  font-size: 0.78rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #6f7f8d;
  font-weight: 600;
}
.detail-list { margin: 0; padding-left: 1.2rem; line-height: 1.55; }
.fix-box {
  border: 1px solid var(--dasm-border-strong);
  border-radius: 12px;
  padding: 0.9rem 1rem;
  background: #f6fafc;
}
.ev-row {
  padding: 0.45rem 0;
  border-bottom: 1px solid var(--dasm-border, #d5e4e0);
}
.text-mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
</style>
