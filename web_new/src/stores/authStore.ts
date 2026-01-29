import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { setAuthStore } from '@/api/client'

const TOKEN_KEY = 'auth_token'

function parseJwtPayload(token: string): { exp?: number } | null {
  try {
    const base64Url = token.split('.')[1]
    if (!base64Url) return null
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/')
    const jsonPayload = decodeURIComponent(
      atob(base64)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join(''),
    )
    return JSON.parse(jsonPayload)
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem(TOKEN_KEY) || '')

  const isAuthenticated = computed(() => !!token.value && !isTokenExpired.value)

  const isTokenExpired = computed(() => {
    if (!token.value) return true
    const payload = parseJwtPayload(token.value)
    if (!payload?.exp) return false
    return Date.now() >= payload.exp * 1000
  })

  function setToken(newToken: string) {
    token.value = newToken
    localStorage.setItem(TOKEN_KEY, newToken)
  }

  function logout() {
    token.value = ''
    localStorage.removeItem(TOKEN_KEY)
  }

  function init() {
    setAuthStore({ 
      get token() { return token.value },
      logout 
    })

    if (token.value && isTokenExpired.value) {
      logout()
    }
  }

  return {
    token,
    isAuthenticated,
    isTokenExpired,
    setToken,
    logout,
    init,
  }
})
