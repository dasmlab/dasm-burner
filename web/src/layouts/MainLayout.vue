<template>
  <q-layout view="lHh Lpr lFf">
    <q-header elevated class="bg-primary text-white">
      <q-toolbar>
        <q-btn flat dense round icon="menu" aria-label="Menu" @click="leftDrawerOpen = !leftDrawerOpen" />
        <q-toolbar-title>dasm-burner</q-toolbar-title>
        <q-chip square dense color="white" text-color="primary">{{ versionLabel }}</q-chip>
        <q-space />
        <q-select
          v-model="clusterName"
          :options="clusterOptions"
          dense
          dark
          outlined
          emit-value
          map-options
          class="cluster-select q-mr-md"
          label="Cluster"
          style="min-width: 240px"
          @update:model-value="onClusterChange"
        />
        <div class="text-caption q-mr-md">{{ auth.displayName.value }}</div>
        <q-btn
          v-if="auth.authEnabled.value"
          flat
          dense
          icon="logout"
          label="Logout"
          @click="auth.logout()"
        />
      </q-toolbar>
    </q-header>

    <q-drawer v-model="leftDrawerOpen" show-if-above bordered>
      <q-list padding>
        <q-item-label header class="nav-section-label">Run</q-item-label>
        <q-item clickable v-ripple :to="{ name: 'dashboard' }" exact active-class="text-primary bg-grey-2">
          <q-item-section avatar><q-icon name="monitor_heart" /></q-item-section>
          <q-item-section>
            <q-item-label>Health</q-item-label>
            <q-item-label caption>nodes, OVN, abort gates</q-item-label>
          </q-item-section>
        </q-item>
        <q-item clickable v-ripple :to="{ name: 'topology' }" active-class="text-primary bg-grey-2">
          <q-item-section avatar><q-icon name="account_tree" /></q-item-section>
          <q-item-section>
            <q-item-label>Topology</q-item-label>
            <q-item-label caption>templates · NS × N</q-item-label>
          </q-item-section>
        </q-item>
        <q-item clickable v-ripple :to="{ name: 'execute' }" active-class="text-primary bg-grey-2">
          <q-item-section avatar><q-icon name="play_circle" /></q-item-section>
          <q-item-section>
            <q-item-label>Execute</q-item-label>
            <q-item-label caption>run test · batches · logs</q-item-label>
          </q-item-section>
        </q-item>
        <q-item clickable v-ripple :to="{ name: 'report' }" active-class="text-primary bg-grey-2">
          <q-item-section avatar><q-icon name="summarize" /></q-item-section>
          <q-item-section>
            <q-item-label>Report</q-item-label>
            <q-item-label caption>last apply + Prometheus</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </q-drawer>

    <q-page-container>
      <router-view />
    </q-page-container>

    <q-footer class="db-footer text-center q-pa-sm">
      <span>NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT</span>
      <span class="q-ml-md">© {{ year }} DASMLAB Inc.</span>
    </q-footer>
  </q-layout>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getCluster, getVersion, selectCluster } from 'src/services/api'
import { useAuth } from 'src/services/auth'

const auth = useAuth()
const leftDrawerOpen = ref(false)
const versionLabel = ref('…')
const year = new Date().getFullYear()
const clusters = ref([])
const clusterName = ref('')

const clusterOptions = computed(() =>
  clusters.value.map((c) => ({
    label: c.name,
    value: c.name,
    caption: c.server || c.source,
  })),
)

async function loadCluster() {
  try {
    const data = await getCluster()
    clusters.value = data.clusters || []
    clusterName.value = data.current?.name || ''
  } catch {
    clusters.value = []
  }
}

async function onClusterChange(name) {
  const c = clusters.value.find((x) => x.name === name)
  if (!c) return
  await selectCluster({ name: c.name, kubeconfig: c.kubeconfig, context: c.context })
  await loadCluster()
}

onMounted(async () => {
  await auth.init()
  await loadCluster()
  try {
    const v = await getVersion()
    versionLabel.value = v.version || 'dev'
  } catch {
    versionLabel.value = 'offline'
  }
})
</script>

<style scoped>
.nav-section-label {
  font-family: Fraunces, Georgia, serif;
  font-size: 0.95rem;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #2f8f7d;
  padding-top: 0.75rem;
}
.cluster-select :deep(.q-field__control) {
  background: rgba(255, 255, 255, 0.12);
}
</style>
