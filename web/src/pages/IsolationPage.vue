<template>
  <q-page padding class="iso-page">
    <div class="dasm-shell q-mb-lg">
      <div class="dasm-shell__content">
        <div class="dasm-caps">North star</div>
        <h1 class="dasm-title">Isolated Wave Test Approach / Mode</h1>
        <p class="dasm-subtitle">{{ iso.northStar }}</p>
      </div>
    </div>

    <div v-if="error" class="dasm-panel q-mb-md text-negative">{{ error }}</div>

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">Closed loop</div>
      <p class="text-caption text-grey-7 q-mb-sm">
        D3 SVG from the protocol in-repo. One wave, then recover, then the next.
        Dumping 14 waves and list-deleting everything cannot tell you which batch created the RSS.
      </p>
      <IsolationLoopSvg :steps="iso.steps" :causality="iso.causality" />
    </div>

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">What we are looking for</div>
      <ol class="detail-list">
        <li v-for="s in iso.steps" :key="s.id">
          <strong>{{ s.title }}.</strong> {{ s.see }}
        </li>
      </ol>
      <p class="text-caption q-mt-sm">{{ iso.breakpointHint }}</p>
    </div>

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">Causality chain</div>
      <div class="row q-gutter-sm">
        <q-chip v-for="(c, i) in iso.causality" :key="c" square color="primary" text-color="white">
          {{ i + 1 }}. {{ c }}
        </q-chip>
      </div>
      <p class="text-caption text-grey-7 q-mt-sm">
        Recovery (DELETE + finalizers + watch disconnect) is a second mutating load. It belongs in the same loop.
      </p>
    </div>

    <div class="dasm-panel q-mb-md">
      <div class="dasm-stat-label q-mb-sm">Possible code fix — watch cache does not shrink on DELETE</div>
      <p>
        In Kubernetes v1.34.6 (this cluster),
        <code>watchCache.Delete</code> does <em>not</em> drop the event ring.
        It appends a <code>Deleted</code> event, same path as Add/Update.
        Capacity grow <em>and</em> shrink both require the ring to be <strong>full</strong>.
        After cleanup the store is empty, the ring is usually <em>not</em> full, so
        <code>resizeCacheLocked</code> never halves the backing array.
        Go also will not return RSS to the OS until the kube-apiserver static pod restarts.
      </p>
      <p class="text-caption">
        This lives as investigation
        <router-link :to="{ name: 'investigations', params: { id: 'watch-cache-shrink-without-full' } }">watch-cache-shrink-without-full</router-link>
        ·
        <router-link :to="{ name: 'source-map' }">Source map</router-link>
        · upstream
        <a href="https://github.com/kubernetes/kubernetes/blob/v1.34.6/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go#L256-L388" target="_blank" rel="noopener">watch_cache.go L256–L388</a>
        · RH tree
        <a href="https://github.com/openshift/kubernetes/blob/dfffacdf0ad6e9aa75664c7b3167dd2ddbfc17ba/staging/src/k8s.io/apiserver/pkg/storage/cacher/watch_cache.go" target="_blank" rel="noopener">openshift/kubernetes @ dfffacdf</a>
      </p>
      <pre class="code-block">func (w *watchCache) Delete(obj interface{}) error {
    event := watch.Event{Type: watch.Deleted, Object: object}
    f := func(elem *storeElement) error { return w.store.Delete(elem) }
    return w.processEvent(event, resourceVersion, f)  // still updateCache()
}

func (w *watchCache) resizeCacheLocked(eventTime time.Time) {
    // grow 2x  — only if isCacheFullLocked()
    // shrink 2x — only if isCacheFullLocked() AND events are stale
}

func (w *watchCache) isCacheFullLocked() bool {
    return w.endIndex == w.startIndex+w.capacity
}</pre>
      <p class="text-caption text-grey-7">
        Testable action: scrape <code>apiserver_watch_cache_capacity</code> per resource at baseline,
        after wave k, and after Terminating=0. If capacity stays at the high watermark with pods≈0,
        the shrink branch never ran. That is the patch experiment — shrink when occupancy ≪ capacity
        and events are older than <code>eventFreshDuration</code>, without requiring full.
        Drive status and evidence from the Investigations panel; do not lose it in chat.
      </p>
    </div>
  </q-page>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { getIsolation } from 'src/services/api'
import IsolationLoopSvg from 'src/components/IsolationLoopSvg.vue'

const iso = ref({ steps: [], causality: [], northStar: '', breakpointHint: '' })
const error = ref('')

onMounted(async () => {
  try {
    const data = await getIsolation()
    iso.value = data.isolation || iso.value
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  }
})
</script>

<style scoped>
.detail-list { margin: 0; padding-left: 1.2rem; line-height: 1.55; }
.code-block {
  margin: 0.75rem 0;
  padding: 0.85rem 1rem;
  overflow: auto;
  font-size: 0.78rem;
  line-height: 1.45;
  background: #12202c;
  color: #d7efe9;
  border-radius: 10px;
}
</style>
