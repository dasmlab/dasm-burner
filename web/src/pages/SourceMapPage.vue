<template>
  <q-page padding>
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">Source map</div>
        <h1 class="dasm-title">{{ cluster || 'Cluster' }} · {{ mmap.openshift }} / {{ mmap.kubernetes }}</h1>
        <p class="dasm-subtitle">
          Piece, clone URL, payload SHA for the four control-plane parts we actually burn.
          Header cluster is the context. Recipe for the next version:
          <code>{{ mmap.recipe }}</code>
        </p>
      </div>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">Relations</div>
      <SourceMapGraph
        :cluster="cluster"
        :openshift="mmap.openshift"
        :pieces="mmap.pieces || []"
        @select="selected = $event"
      />
    </div>

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">Pins</div>
      <q-markup-table flat dense wrap-cells>
        <thead>
          <tr>
            <th class="text-left">Piece</th>
            <th class="text-left">Clone this</th>
            <th class="text-left">Branch</th>
            <th class="text-left">Payload SHA</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="p in mmap.pieces"
            :key="p.id"
            class="piece-row"
            :class="{ 'is-sel': selected?.id === p.id }"
            @click="selected = p"
          >
            <td>
              <div class="text-weight-medium">{{ p.name }}</div>
              <div class="text-caption text-grey-7">{{ p.role }}</div>
            </td>
            <td><a :href="p.repo" target="_blank" rel="noopener">{{ p.clone }}</a></td>
            <td><code>{{ p.branch }}</code></td>
            <td>
              <a :href="p.commitUrl" target="_blank" rel="noopener"><code>{{ short(p.payloadSha) }}</code></a>
              <div v-if="p.upstreamTag" class="text-caption">upstream {{ p.upstreamTag }}</div>
            </td>
          </tr>
        </tbody>
      </q-markup-table>
    </div>

    <div v-if="selected" class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">{{ selected.name }}</div>
      <p>{{ selected.role }}</p>
      <ul class="detail-list" v-if="selected.files?.length">
        <li v-for="f in selected.files" :key="f.path">
          <a :href="f.url" target="_blank" rel="noopener"><code>{{ f.path }}</code></a>
          <span v-if="f.lines"> :{{ f.lines }}</span>
          — {{ f.why }}
          <span v-if="f.forkUrl"> · <a :href="f.forkUrl" target="_blank" rel="noopener">RH tree</a></span>
        </li>
      </ul>
      <div v-if="selected.possibleFix" class="fix-box q-mt-md">
        <div class="text-weight-medium">Possible code fix</div>
        <p>{{ selected.possibleFix.title }}</p>
        <p class="text-caption"><strong>Metric:</strong> {{ selected.possibleFix.metric }}</p>
        <p class="text-caption">{{ selected.possibleFix.action }}</p>
        <q-btn
          unelevated
          color="primary"
          label="Open investigation"
          :to="{ name: 'investigations', params: { id: selected.possibleFix.id || 'watch-cache-shrink-without-full' } }"
        />
        <q-btn
          flat
          color="primary"
          label="Isolated wave"
          :to="{ name: 'isolation' }"
        />
      </div>
    </div>
  </q-page>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { getSourceMap } from 'src/services/api'
import SourceMapGraph from 'src/components/SourceMapGraph.vue'

const cluster = ref('')
const mmap = ref({ pieces: [], recipe: '', openshift: '', kubernetes: '' })
const selected = ref(null)
const error = ref('')

function short(sha) {
  return sha ? sha.slice(0, 12) : '—'
}

onMounted(async () => {
  try {
    const data = await getSourceMap()
    cluster.value = data.cluster || ''
    mmap.value = data.map || mmap.value
    selected.value = (mmap.value.pieces || []).find((p) => p.id === 'kube-apiserver') || mmap.value.pieces?.[0] || null
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
})
</script>

<style scoped>
.piece-row { cursor: pointer; }
.piece-row.is-sel { background: rgba(63, 122, 107, 0.1); }
.detail-list { margin: 0; padding-left: 1.2rem; line-height: 1.55; }
.fix-box {
  border: 1px solid var(--dasm-border-strong);
  border-radius: 12px;
  padding: 0.9rem 1rem;
  background: #f6fafc;
}
</style>
