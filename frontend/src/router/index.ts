import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: () => import('../views/HomeView.vue') },
    { path: '/login', component: () => import('../views/LoginView.vue') },
    { path: '/register', component: () => import('../views/RegisterView.vue'), meta: { requiresAuth: true } },
    { path: '/seats', component: () => import('../views/SeatsView.vue'), meta: { requiresAuth: true, requiresCompleteProfile: true } },
    { path: '/ticket', component: () => import('../views/TicketView.vue'), meta: { requiresAuth: true, requiresCompleteProfile: true } },
    { path: '/seat-guide', component: () => import('../views/SeatGuideView.vue'), meta: { requiresAuth: true, requiresCompleteProfile: true } },
    { path: '/status', component: () => import('../views/StatusView.vue') },
    { path: '/check', component: () => import('../views/CheckView.vue') },
    { path: '/admin', component: () => import('../views/AdminView.vue') },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (to.meta.requiresAuth && !auth.token) return '/login'

  if (auth.token && !auth.student) {
    try {
      await auth.fetchMe()
    } catch {
      auth.logout()
      return '/login'
    }
  }

  if (to.path === '/login' && auth.token) {
    return auth.hasCompleteProfile ? '/seats' : '/register'
  }

  if (to.meta.requiresCompleteProfile && !auth.hasCompleteProfile) {
    return { path: '/register', query: { redirect: to.fullPath } }
  }

  if (to.path === '/register' && auth.hasCompleteProfile) {
    const redirect = typeof to.query.redirect === 'string' ? to.query.redirect : '/seats'
    return redirect
  }
})

export default router
