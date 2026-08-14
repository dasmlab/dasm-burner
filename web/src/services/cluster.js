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
  const isInCluster = computed(() => {
    const c = current.value
    if (!c) return false
    return c.source === 'in-cluster' || (c.name && String(c.name).includes('in-cluster'))
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

  /** Re-fetch and require the server current name to match expected (guard stale UI). */
  async function assertCurrent(expectedName) {
    const data = await refresh()
    const got = data.current?.name || ''
    if (expectedName && got !== expectedName) {
      throw new Error(
        `Cluster selection drifted: UI expected "${expectedName}" but server is "${got || 'unknown'}". Re-select the cluster and try again.`,
      )
    }
    return data.current
  }

  return {
    clusters,
    current,
    currentName,
    currentLabel,
    isInCluster,
    ready,
    refresh,
    select,
    assertCurrent,
  }
}
