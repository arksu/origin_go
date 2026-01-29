import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      redirect: '/characters',
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { guest: true },
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('@/views/RegisterView.vue'),
      meta: { guest: true },
    },
    {
      path: '/characters',
      name: 'characters',
      component: () => import('@/views/CharactersView.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/game',
      name: 'game',
      component: () => import('@/views/GameView.vue'),
      meta: { requiresAuth: true, requiresGameSession: true },
    },
  ],
})

router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()

  if (authStore.token && authStore.isTokenExpired) {
    authStore.logout()
  }

  const isAuthenticated = authStore.isAuthenticated

  if (to.meta.requiresAuth && !isAuthenticated) {
    next({ name: 'login', query: { redirect: to.fullPath } })
    return
  }

  if (to.meta.guest && isAuthenticated) {
    next({ name: 'characters' })
    return
  }

  next()
})

export default router
