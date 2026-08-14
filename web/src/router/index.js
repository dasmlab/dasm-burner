import { createRouter, createWebHistory } from 'vue-router'
import { useAuth } from 'src/services/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('src/pages/LoginPage.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('src/layouts/MainLayout.vue'),
      children: [
        { path: '', name: 'dashboard', component: () => import('src/pages/DashboardPage.vue') },
        { path: 'topology', name: 'topology', component: () => import('src/pages/TopologyPage.vue') },
        { path: 'execute', name: 'execute', component: () => import('src/pages/ExecutePage.vue') },
        { path: 'report', name: 'report', component: () => import('src/pages/ReportPage.vue') },
        { path: 'cleanup-reports', name: 'cleanup-reports', component: () => import('src/pages/CleanupReportsPage.vue') },
        { path: 'ovn-diagnoser', name: 'ovn-diagnoser', component: () => import('src/pages/OVNDiagnoserPage.vue') },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuth()
  await auth.init()
  if (to.name === 'login') {
    if (!auth.authEnabled.value || auth.isAuthenticated.value) {
      return { name: 'dashboard' }
    }
    // Guest-first: never trap people on a login wall — send them home.
    return { name: 'dashboard' }
  }
  return true
})

export default router
