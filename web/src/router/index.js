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
        { path: '', name: 'dashboard', component: () => import('src/pages/DashboardPage.vue'), meta: { admin: true } },
        { path: 'topology', name: 'topology', component: () => import('src/pages/TopologyPage.vue'), meta: { admin: true } },
        { path: 'execute', name: 'execute', component: () => import('src/pages/ExecutePage.vue'), meta: { admin: true } },
        { path: 'report', name: 'report', component: () => import('src/pages/ReportPage.vue'), meta: { admin: true } },
        { path: 'cleanup-reports', name: 'cleanup-reports', component: () => import('src/pages/CleanupReportsPage.vue'), meta: { admin: true } },
        { path: 'ovn-diagnoser', name: 'ovn-diagnoser', component: () => import('src/pages/OVNDiagnoserPage.vue'), meta: { admin: true } },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuth()
  await auth.init()
  if (to.meta.public) return true
  if (!auth.authEnabled.value) return true
  if (!auth.isAuthenticated.value) {
    return { name: 'login', query: { returnTo: to.fullPath } }
  }
  if (to.meta.admin && !auth.isAdmin.value) {
    return { name: 'login', query: { returnTo: to.fullPath } }
  }
  return true
})

export default router
