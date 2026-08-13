import { computed, ref } from 'vue'
import { getCluster, selectCluster as apiSelectCluster } from 'src/services/api'

const clusters = ref([])
const current = ref(null)
const ready = ref(false)

export function useCluster() {
  const currentName = computed(() => current.value?.name || '')
  const currentLabel = computed(() => {
    const c = current.value
    if (!c) return '…'
    if (c.source === 'in-cluster' || (c.name && String(c.name).includes('in-cluster'))) {
      return c.name
    }
    return c.name || 'unknown'
  })

  async function refresh() {
    const data = await getCluster()
    clusters.value = data.clusters || []
    current.value = data.current || null
    ready.value = true
    return data
  }

  async function select(name) {
    const c = clusters.value.find((x) => x.name === name)
    if (!c) return
    await apiSelectCluster({
      name: c.name,
      kubeconfig: c.kubeconfig,
      context: c.context,
      source: c.source,
    })
    await refresh()
  }

  return {
    clusters,
    current,
    currentName,
    currentLabel,
    ready,
    refresh,
    select,
  }
}
