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
          class="cluster-select q-mr-sm"
          label="Cluster"
          style="min-width: 220px"
          @update:model-value="onClusterChange"
        />
        <q-btn flat dense icon="add" label="Add" class="q-mr-md" @click="addOpen = true" />
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
            <q-item-label caption>cluster pulse · mixes</q-item-label>
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
            <q-item-label caption>immutable snapshots</q-item-label>
          </q-item-section>
        </q-item>
        <q-item clickable v-ripple :to="{ name: 'cleanup-reports' }" active-class="text-primary bg-grey-2">
          <q-item-section avatar><q-icon name="delete_sweep" /></q-item-section>
          <q-item-section>
            <q-item-label>Cleanup reports</q-item-label>
            <q-item-label caption>duration · NS · logs</q-item-label>
          </q-item-section>
        </q-item>
        <q-item clickable v-ripple :to="{ name: 'ovn-diagnoser' }" active-class="text-primary bg-grey-2">
          <q-item-section avatar><q-icon name="troubleshoot" /></q-item-section>
          <q-item-section>
            <q-item-label>OVN Diagnoser</q-item-label>
            <q-item-label caption>baseline · findings · why?</q-item-label>
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

    <q-dialog v-model="addOpen" persistent>
      <q-card style="min-width: 560px; max-width: 720px">
        <q-card-section>
          <div class="text-h6">Add target cluster</div>
          <div class="text-caption text-grey-7">
            In the OpenShift console: your name → Copy login command → Display Token.
            Paste either the <code>oc login</code> line, the <code>curl</code> Bearer line, or both.
          </div>
        </q-card-section>
        <q-card-section>
          <q-input
            v-model="paste"
            type="textarea"
            autogrow
            outlined
            label="Paste login command"
            hint="Accepts oc login --token … --server … and/or curl -H Authorization: Bearer …"
          />
          <q-input
            v-model="customName"
            class="q-mt-md"
            dense
            outlined
            label="Display name (optional)"
            hint="Defaults from api.&lt;cluster&gt;.…"
          />
          <p v-if="addError" class="text-negative text-caption q-mt-sm">{{ addError }}</p>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" v-close-popup @click="resetAdd" />
          <q-btn color="primary" unelevated label="Add &amp; select" :loading="adding" @click="submitAdd" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-layout>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { addClusterLogin, getVersion } from 'src/services/api'
import { useAuth } from 'src/services/auth'
import { useCluster } from 'src/services/cluster'

const auth = useAuth()
const cluster = useCluster()
const leftDrawerOpen = ref(false)
const versionLabel = ref('…')
const year = new Date().getFullYear()
const addOpen = ref(false)
const paste = ref('')
const customName = ref('')
const adding = ref(false)
const addError = ref('')

const clusterName = computed({
  get: () => cluster.currentName.value,
  set: () => {},
})

const clusterOptions = computed(() =>
  cluster.clusters.value.map((c) => ({
    label: c.source === 'login-command' ? `${c.name} (token)` : c.name,
    value: c.name,
    caption: c.server || c.source,
  })),
)

async function onClusterChange(name) {
  if (!name || name === cluster.currentName.value) return
  try {
    await cluster.select(name)
  } catch (e) {
    addError.value = e.response?.data?.error || e.message
  }
}

function resetAdd() {
  paste.value = ''
  customName.value = ''
  addError.value = ''
}

async function submitAdd() {
  adding.value = true
  addError.value = ''
  try {
    await addClusterLogin({
      paste: paste.value,
      name: customName.value || undefined,
    })
    await cluster.refresh()
    addOpen.value = false
    resetAdd()
  } catch (e) {
    addError.value = e.response?.data?.error || e.message
  } finally {
    adding.value = false
  }
}

onMounted(async () => {
  await auth.init()
  await cluster.refresh()
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
