import { computed, ref } from 'vue'
import { getAuthConfig, getMe, loginUrl, logoutUrl } from 'src/services/api'

const user = ref(null)
const authEnabled = ref(false)
const ready = ref(false)
const loading = ref(false)

export function useAuth() {
  const isAuthenticated = computed(() => !!user.value)
  const isAdmin = computed(() => {
    if (!authEnabled.value) return true
    return !!user.value?.is_admin
  })
  const displayName = computed(() => {
    if (!user.value) return ''
    return user.value.name || user.value.preferred_username || user.value.email || 'User'
  })

  async function init() {
    if (ready.value) return
    loading.value = true
    try {
      const cfg = await getAuthConfig()
      authEnabled.value = !!cfg.enabled
      if (cfg.enabled) {
        try {
          user.value = await getMe()
        } catch {
          user.value = null
        }
      } else {
        user.value = { preferred_username: 'local', name: 'Local Dev', is_admin: true }
      }
    } catch {
      authEnabled.value = false
      user.value = { preferred_username: 'local', name: 'Local Dev', is_admin: true }
    } finally {
      ready.value = true
      loading.value = false
    }
  }

  function login() {
    window.location.href = loginUrl()
  }

  function logout() {
    window.location.href = logoutUrl()
  }

  return {
    user,
    authEnabled,
    ready,
    loading,
    isAuthenticated,
    isAdmin,
    displayName,
    init,
    login,
    logout,
  }
}
