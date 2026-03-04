import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/authStore'
import './assets/main.scss'

function applyStandaloneClass(): void {
  const isStandalone =
    window.matchMedia('(display-mode: standalone)').matches ||
    (window.navigator as Navigator & { standalone?: boolean }).standalone === true
  document.documentElement.classList.toggle('is-standalone', isStandalone)
}

applyStandaloneClass()
window.addEventListener('pageshow', applyStandaloneClass)
const standaloneMediaQuery = window.matchMedia('(display-mode: standalone)')
if (typeof standaloneMediaQuery.addEventListener === 'function') {
  standaloneMediaQuery.addEventListener('change', applyStandaloneClass)
} else if (typeof standaloneMediaQuery.addListener === 'function') {
  standaloneMediaQuery.addListener(applyStandaloneClass)
}

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)

const authStore = useAuthStore()
authStore.init()

app.use(router)

app.mount('#app')
