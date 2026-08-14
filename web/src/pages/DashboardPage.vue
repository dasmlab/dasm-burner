<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">Health</div>
        <h1 class="dasm-title">{{ clusterTitle }}</h1>
        <p class="dasm-subtitle">
          Live view for the cluster selected in the header. Mix cards are cluster-scoped —
          flip targets to see what is online there. OVN “since clear” is a shared watermark
          for this dasm-burner instance (all Keycloak users), not a kube reset.
        </p>
      </div>
    </div>

    <div class="row items-center justify-between q-mb-md">
      <div class="text-subtitle1 text-weight-medium">Cluster pulse</div>
      <div class="row q-gutter-sm">
        <q-btn
          outline
          dense
          color="warning"
          icon="restart_alt"
          label="Clear restart watermark"
          :loading="clearing"
          :disable="loading"
          @click="clearRestarts"
        />
        <q-btn flat dense color="primary" icon="refresh" label="Refresh" :loading="loading" @click="load" />
      </div>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>
    <div v-if="notice" class="dasm-panel q-mb-md text-positive">{{ notice }}</div>

    <div class="cluster-hero q-mb-lg">
      <div class="cluster-hero__main">
        <div class="dasm-caps">Current cluster</div>
        <div class="cluster-hero__name">{{ clusterTitle }}</div>
        <div class="text-caption text-grey-7">
          <span v-if="clusterMeta.server">{{ clusterMeta.server }}</span>
          <span v-if="clusterMeta.source"> · {{ clusterMeta.source }}</span>
        </div>
      </div>
      <div class="cluster-hero__gate" :class="gateClass">
        <div class="dasm-stat-label">Abort gate</div>
        <div class="gate-level">{{ health.level || '—' }}</div>
        <div class="text-caption">{{ health.reason || 'no abort' }}</div>
      </div>
    </div>

    <div class="dasm-grid dasm-grid--4 q-mb-lg">
      <div class="dasm-panel metric">
        <div class="dasm-stat-label">Nodes Ready</div>
        <div class="dasm-stat">{{ h.nodesReady ?? '—' }} <span class="metric-den">/ {{ nodeTotal }}</span></div>
        <div class="text-caption text-grey-7" v-if="h.notReadyNodes?.length">
          not Ready: {{ h.notReadyNodes.join(', ') }}
        </div>
      </div>
      <div class="dasm-panel metric">
        <div class="dasm-stat-label">OVN Ready</div>
        <div class="dasm-stat">{{ h.ovnReady ?? '—' }} <span class="metric-den">/ {{ h.ovnPods ?? '—' }}</span></div>
        <div class="text-caption text-grey-7">lifetime restarts {{ ovnRestarts }}</div>
      </div>
      <div class="dasm-panel metric" :class="{ 'metric-hot': restartsSince > 0 }">
        <div class="dasm-stat-label">Restarts since clear</div>
        <div class="dasm-stat">{{ restartsSinceLabel }}</div>
        <div class="text-caption text-grey-7">
          <template v-if="baseline">
            watermark {{ baseline.ovnRestarts }} · {{ fmtTime(baseline.clearedAt) }}
            <span v-if="baseline.clearedBy"> · {{ baseline.clearedBy }}</span>
          </template>
          <template v-else>no watermark yet — clear to start counting</template>
        </div>
      </div>
      <div class="dasm-panel metric">
        <div class="dasm-stat-label">OOMKilled</div>
        <div class="dasm-stat">{{ h.oomKilled ?? '—' }}</div>
        <div class="text-caption text-grey-7">warn events {{ h.warningEvents ?? 0 }}</div>
      </div>
    </div>

    <div v-if="nodeRoles.length" class="dasm-panel q-mb-lg">
      <div class="dasm-stat-label q-mb-sm">Node roles</div>
      <div class="text-caption text-grey-7 q-mb-sm">
        Counts from <code>node-role.kubernetes.io/*</code>. Multi-role nodes appear in each matching bucket.
        <span v-if="hasInfra"> Density workloads avoid infra taint when Execute avoid-taints is set.</span>
      </div>
      <table class="role-table">
        <thead>
          <tr>
            <th>Role</th>
            <th>Ready</th>
            <th>NotReady</th>
            <th>Total</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in nodeRoles" :key="r.role" :class="{ 'is-bad': (r.notReady || 0) > 0 }">
            <td>{{ r.role }}</td>
            <td>{{ r.ready }}</td>
            <td>
              {{ r.notReady }}
              <span v-if="r.nodes?.length" class="text-caption text-grey-7"> · {{ r.nodes.join(', ') }}</span>
            </td>
            <td>{{ r.total }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="row items-center justify-between q-mb-sm q-mt-lg">
      <div class="text-subtitle1 text-weight-medium">OVN-Kube Diagnoser</div>
      <div class="row q-gutter-sm">
        <q-btn outline dense color="primary" label="Capture baseline" :loading="ovnBusy" @click="captureOVNBaseline" />
        <q-btn flat dense color="primary" icon="refresh" label="Sample" :loading="ovnBusy" @click="sampleOVN" />
      </div>
    </div>
    <p class="text-caption text-grey-7 q-mb-md">
      Active interrogation of node + ovnkube-node health (not kube-burner metrics).
      Baseline first, then sample during / after Execute to see restart Δ and findings.
      <router-link :to="{ name: 'ovn-diagnoser' }">Open full OVN Diagnoser →</router-link>
    </p>
    <div v-if="ovnError" class="dasm-panel q-mb-md text-negative">{{ ovnError }}</div>
    <div v-if="ovnSnap" class="dasm-panel q-mb-lg">
      <div class="row items-center q-gutter-md q-mb-md">
        <q-badge :color="ovnStateColor" text-color="white">{{ ovnSnap.overallState || '—' }}</q-badge>
        <span class="text-caption">
          {{ ovnSnap.healthyCount ?? 0 }} healthy · {{ ovnSnap.warningCount ?? 0 }} warning · {{ ovnSnap.criticalCount ?? 0 }} critical
        </span>
        <span v-if="ovnSnap.baselineAt" class="text-caption text-grey-7">baseline {{ fmtTime(ovnSnap.baselineAt) }}</span>
      </div>
      <div v-if="ovnSnap.why" class="text-body2 q-mb-md"><strong>Why?</strong> {{ ovnSnap.why }}</div>
      <table class="ovn-diag-table">
        <thead>
          <tr>
            <th>Node</th>
            <th>State</th>
            <th>Ready</th>
            <th>Annots</th>
            <th>OVN pod</th>
            <th>Restarts Δ</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="n in (ovnSnap.nodes || [])" :key="n.nodeName" :class="{ 'is-hot': n.overallState !== 'HEALTHY' }">
            <td class="text-mono">{{ n.nodeName }}</td>
            <td>{{ n.overallState }}</td>
            <td>{{ n.node?.ready ? 'yes' : 'no' }}</td>
            <td>{{ n.network?.annotationsOK === false ? 'drift' : 'ok' }}</td>
            <td class="text-mono">{{ n.ovnKube?.podName || '—' }}</td>
            <td><strong>{{ n.ovnKube?.restartsDelta ?? 0 }}</strong> / {{ n.ovnKube?.restarts ?? 0 }}</td>
          </tr>
        </tbody>
      </table>
      <div v-if="ovnFindings.length" class="q-mt-md">
        <div class="dasm-stat-label q-mb-xs">Findings</div>
        <ul class="ovn-tl">
          <li v-for="f in ovnFindings" :key="f.id">
            <q-badge dense :color="findingColor(f.severity)" text-color="white">{{ f.ruleId }}</q-badge>
            {{ f.summary }}
            <span v-if="f.node" class="text-grey-7"> · {{ f.node }}</span>
          </li>
        </ul>
      </div>
      <div v-if="ovnSnap.timeline?.length" class="q-mt-md">
        <div class="dasm-stat-label q-mb-xs">Timeline</div>
        <ul class="ovn-tl">
          <li v-for="(t, i) in ovnSnap.timeline.slice(0, 16)" :key="i">
            <span class="text-mono">{{ fmtTime(t.at) }}</span> · {{ t.summary }}
            <span v-if="t.node" class="text-grey-7"> ({{ t.node }})</span>
          </li>
        </ul>
      </div>
    </div>

    <div class="text-subtitle1 text-weight-medium q-mb-sm">Mixes on this cluster</div>
    <p class="text-caption text-grey-7 q-mb-md">
      Intended = saved templates ready to run. Live = managed namespaces currently on
      <strong>{{ clusterTitle }}</strong>. Completed = immutable snapshots for this cluster.
    </p>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-md-4">
        <div class="mix-col">
          <div class="mix-col__head">
            <q-icon name="inventory_2" />
            <span>Intended</span>
            <q-badge outline color="primary">{{ intended.length }}</q-badge>
          </div>
          <div v-if="!intended.length" class="mix-empty">No saved templates.</div>
          <div
            v-for="t in intended"
            :key="t.name"
            class="mix-card"
            :class="{ 'is-active': t.name === activeTemplate }"
          >
            <div class="mix-card__title">{{ t.name }}</div>
            <div class="mix-card__meta">
              <template v-if="t.kind === 'OpenShiftObjectPressure'">
                pressure · {{ t.namespaces }} NS · {{ t.counts?.intendedObjects ?? '?' }} objs
              </template>
              <template v-else>
                {{ t.namespaces }} NS · {{ t.counts?.pods ?? '?' }} pods
              </template>
              <span v-if="t.name === activeTemplate"> · active</span>
            </div>
            <div class="mix-card__actions">
              <q-btn flat dense size="sm" color="primary" icon="play_circle" label="Execute" :to="{ name: 'execute' }" />
              <q-btn flat dense size="sm" icon="account_tree" label="Topology" :to="{ name: 'topology' }" />
            </div>
          </div>
        </div>
      </div>

      <div class="col-12 col-md-4">
        <div class="mix-col mix-col--live">
          <div class="mix-col__head">
            <q-icon name="cloud_done" />
            <span>Live</span>
            <q-badge color="warning" text-color="white">{{ live.length }}</q-badge>
          </div>
          <div class="text-caption text-grey-7 q-mb-sm" v-if="managedTotal != null">
            {{ managedTotal }} managed NS on cluster
          </div>
          <div v-if="!live.length" class="mix-empty">Nothing deployed here (or cleaned).</div>
          <div v-for="row in live" :key="row.runId" class="mix-card is-live">
            <div class="mix-card__title text-mono">{{ row.prefix || `kb-${row.runId}` }}</div>
            <div class="mix-card__meta">
              {{ row.template || 'unknown template' }} · {{ row.count }} NS
              <span v-if="row.objects">
                · {{ row.objects.routes ?? 0 }} rt · {{ row.objects.services ?? 0 }} svc · {{ row.objects.readyPods ?? 0 }}/{{ row.objects.pods ?? 0 }} pods
              </span>
            </div>
            <div class="mix-card__actions">
              <q-btn flat dense size="sm" color="warning" icon="delete_sweep" label="Cleanup" :to="{ name: 'execute' }" />
            </div>
          </div>
        </div>
      </div>

      <div class="col-12 col-md-4">
        <div class="mix-col mix-col--done">
          <div class="mix-col__head">
            <q-icon name="task_alt" />
            <span>Completed</span>
            <q-badge color="positive" text-color="white">{{ completed.length }}</q-badge>
          </div>
          <div v-if="!completed.length" class="mix-empty">No immutable snapshots for this cluster yet.</div>
          <div v-for="c in completed" :key="c.snapshotId" class="mix-card is-done">
            <div class="mix-card__title">{{ c.template || c.prefix || c.runId }}</div>
            <div class="mix-card__meta">
              <span class="text-mono">{{ c.prefix || `kb-${c.runId}` }}</span>
              · {{ c.status || '—' }}
              <span v-if="c.convergenceOverall != null"> · {{ Number(c.convergenceOverall).toFixed(0) }}%</span>
            </div>
            <div class="text-caption text-grey-7">
              {{ fmtTime(c.finished) }}
              <span v-if="c.duration"> · {{ c.duration }}</span>
            </div>
            <div class="mix-card__actions">
              <q-btn
                flat
                dense
                size="sm"
                color="secondary"
                icon="assessment"
                label="Report"
                :to="{ name: 'report', query: { id: c.snapshotId } }"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </q-page>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { clearHealthBaseline, getOverview, getOVNDiag, sampleOVNDiag, baselineOVNDiag } from 'src/services/api'
import { useCluster } from 'src/services/cluster'

const cluster = useCluster()
const loading = ref(false)
const clearing = ref(false)
const error = ref('')
const notice = ref('')
const overview = ref(null)
const ovnSnap = ref(null)
const ovnBusy = ref(false)
const ovnError = ref('')
let timer = null

const health = computed(() => overview.value?.health || {})
const h = computed(() => health.value.health || {})
const baseline = computed(() => overview.value?.baseline || null)
const ovnRestarts = computed(() => overview.value?.ovnRestarts ?? h.value.ovnRestarts ?? 0)
const restartsSince = computed(() => overview.value?.restartsSinceClear ?? -1)
const restartsSinceLabel = computed(() => {
  const n = restartsSince.value
  if (n < 0) return '—'
  return String(n)
})
const nodeTotal = computed(() => (h.value.nodesReady || 0) + (h.value.nodesNotReady || 0))
const nodeRoles = computed(() => h.value.nodeRoles || [])
const hasInfra = computed(() => nodeRoles.value.some((r) => r.role === 'infra'))
const clusterMeta = computed(() => overview.value?.cluster || {})
const clusterTitle = computed(() => clusterMeta.value.name || cluster.currentLabel.value || '…')
const intended = computed(() => overview.value?.intended || [])
const live = computed(() => overview.value?.live || [])
const completed = computed(() => overview.value?.completed || [])
const activeTemplate = computed(() => overview.value?.activeTemplate || '')
const managedTotal = computed(() => overview.value?.managedTotal)
const gateClass = computed(() => {
  const lvl = (health.value.level || '').toUpperCase()
  if (lvl === 'ABORT') return 'is-abort'
  if (lvl === 'WARNING') return 'is-warn'
  return 'is-ok'
})
const ovnFindings = computed(() => {
  const list = (ovnSnap.value?.findings || []).filter((f) =>
    ['WARNING', 'ERROR', 'CRITICAL', 'NOTICE'].includes(f.severity))
  return list.slice(0, 20)
})
function findingColor(sev) {
  if (sev === 'CRITICAL' || sev === 'ERROR') return 'negative'
  if (sev === 'WARNING') return 'warning'
  return 'grey-7'
}
const ovnStateColor = computed(() => {
  switch (ovnSnap.value?.overallState) {
    case 'HEALTHY': return 'positive'
    case 'WARNING':
    case 'DEGRADED': return 'warning'
    case 'CRITICAL':
    case 'FAILED': return 'negative'
    default: return 'grey-6'
  }
})

function fmtTime(at) {
  try {
    return at ? new Date(at).toLocaleString() : ''
  } catch {
    return ''
  }
}

async function sampleOVN() {
  ovnBusy.value = true
  ovnError.value = ''
  try {
    const data = await sampleOVNDiag({})
    ovnSnap.value = data.snapshot
  } catch (e) {
    ovnError.value = e.response?.data?.error || e.message
  } finally {
    ovnBusy.value = false
  }
}

async function captureOVNBaseline() {
  ovnBusy.value = true
  ovnError.value = ''
  try {
    const data = await baselineOVNDiag()
    ovnSnap.value = data.snapshot
    notice.value = 'OVN diagnoser baseline captured.'
  } catch (e) {
    ovnError.value = e.response?.data?.error || e.message
  } finally {
    ovnBusy.value = false
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    if (!cluster.ready.value) await cluster.refresh()
    overview.value = await getOverview()
    getOVNDiag().then((d) => { ovnSnap.value = d.snapshot }).catch(() => {})
  } catch (e) {
    error.value = e.response?.data?.error || e.message || 'failed to load'
  } finally {
    loading.value = false
  }
}

async function clearRestarts() {
  notice.value = ''
  error.value = ''
  if (!window.confirm('Set the OVN restart watermark to the current lifetime total on this cluster? Shared for all users of this dasm-burner instance.')) {
    return
  }
  clearing.value = true
  try {
    const data = await clearHealthBaseline()
    notice.value = data.message || 'Restart watermark cleared.'
    await load()
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    clearing.value = false
  }
}

watch(
  () => cluster.currentName.value,
  async (name, prev) => {
    if (!name || name === prev) return
    await load()
  },
)

onMounted(async () => {
  await load()
  timer = setInterval(load, 15000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.cluster-hero {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 1rem;
  padding: 1.1rem 1.25rem;
  border-radius: 16px;
  border: 1px solid var(--dasm-border-strong);
  background:
    radial-gradient(circle at 12% 20%, rgba(47, 143, 125, 0.14), transparent 42%),
    linear-gradient(135deg, #ffffff, #f3f8f6);
}
.cluster-hero__name {
  font-family: Fraunces, Georgia, serif;
  font-size: clamp(1.4rem, 2.2vw, 1.85rem);
  font-weight: 700;
  color: #1d2b36;
  margin: 0.2rem 0 0.35rem;
}
.cluster-hero__gate {
  min-width: 160px;
  padding: 0.85rem 1rem;
  border-radius: 12px;
  border: 1px solid var(--dasm-border-soft);
  background: #f4f7fa;
  text-align: right;
}
.gate-level {
  font-size: 1.45rem;
  font-weight: 800;
  letter-spacing: 0.04em;
}
.is-ok .gate-level { color: #2f8f7d; }
.is-warn {
  background: #fff8e6;
  border-color: #e0b84a;
}
.is-warn .gate-level { color: #b8860b; }
.is-abort {
  background: #fceeef;
  border-color: #cc4757;
}
.is-abort .gate-level { color: #cc4757; }

.metric-den {
  font-size: 0.85rem;
  color: #6f7f8d;
  font-weight: 600;
}
.metric-hot {
  border-color: #e0b84a;
  background: #fffaf0;
}

.mix-col {
  background: #ffffff;
  border: 1px solid var(--dasm-border-soft);
  border-radius: 14px;
  padding: 0.85rem;
  min-height: 280px;
}
.mix-col--live { border-color: rgba(224, 184, 74, 0.45); }
.mix-col--done { border-color: rgba(86, 186, 109, 0.4); }
.mix-col__head {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  font-weight: 700;
  color: #1d2b36;
  margin-bottom: 0.75rem;
}
.mix-empty {
  color: #6f7f8d;
  font-size: 0.85rem;
  padding: 0.5rem 0.15rem;
}
.mix-card {
  border: 1px solid var(--dasm-border-soft);
  border-radius: 10px;
  padding: 0.65rem 0.7rem;
  margin-bottom: 0.55rem;
  background: #f7fafc;
}
.mix-card.is-active {
  border-color: #2f8f7d;
  background: #e8f5f1;
}
.mix-card.is-live {
  background: #fff8e6;
  border-color: #e0b84a;
}
.mix-card.is-done {
  background: #eaf7f0;
  border-color: #56ba6d;
}
.mix-card__title {
  font-weight: 700;
  color: #1d2b36;
}
.mix-card__meta {
  font-size: 0.78rem;
  color: #607483;
  margin-top: 0.15rem;
}
.mix-card__actions {
  margin-top: 0.35rem;
  display: flex;
  flex-wrap: wrap;
  gap: 0.15rem;
}
.text-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.ovn-diag-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.82rem;
}
.ovn-diag-table th,
.ovn-diag-table td {
  text-align: left;
  padding: 0.35rem 0.45rem;
  border-bottom: 1px solid #d9e2ea;
}
.ovn-diag-table tr.is-hot td { background: #fff4f0; }
.ovn-tl {
  margin: 0;
  padding-left: 1.1rem;
  font-size: 0.82rem;
}

@media (max-width: 700px) {
  .cluster-hero {
    grid-template-columns: 1fr;
  }
  .cluster-hero__gate { text-align: left; }
}
.role-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9rem;
}
.role-table th,
.role-table td {
  text-align: left;
  padding: 0.35rem 0.5rem;
  border-bottom: 1px solid var(--dasm-border-soft);
}
.role-table tr.is-bad td {
  color: #b42318;
}
</style>
