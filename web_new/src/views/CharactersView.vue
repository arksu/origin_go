<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { useGameStore } from '@/stores/gameStore'
import { listCharacters, createCharacter, deleteCharacter, enterCharacter } from '@/api/characters'
import { ApiException } from '@/api/errors'
import type { Character } from '@/types/api'
import AppButton from '@/components/ui/AppButton.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppAlert from '@/components/ui/AppAlert.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'

const MAX_CHARACTERS = 5

const router = useRouter()
const authStore = useAuthStore()
const gameStore = useGameStore()

const characters = ref<Character[]>([])
const loading = ref(true)
const error = ref('')

const showCreateForm = ref(false)
const newCharacterName = ref('')
const creating = ref(false)
const createError = ref('')

const enteringId = ref<number | null>(null)
const deletingId = ref<number | null>(null)

const emptySlots = computed(() => Math.max(0, MAX_CHARACTERS - characters.value.length))

async function loadCharacters() {
  loading.value = true
  error.value = ''

  try {
    characters.value = await listCharacters()
  } catch (e) {
    if (ApiException.isAuth(e)) {
      router.push('/login')
      return
    }
    if (ApiException.isNetwork(e)) {
      error.value = 'Нет соединения с сервером'
    } else {
      error.value = 'Не удалось загрузить персонажей'
    }
  } finally {
    loading.value = false
  }
}

async function handleCreate() {
  if (!newCharacterName.value.trim() || creating.value) return

  creating.value = true
  createError.value = ''

  try {
    await createCharacter({ name: newCharacterName.value.trim() })
    newCharacterName.value = ''
    showCreateForm.value = false
    await loadCharacters()
  } catch (e) {
    if (ApiException.isValidation(e)) {
      createError.value = e.message
    } else if (ApiException.isNetwork(e)) {
      createError.value = 'Нет соединения с сервером'
    } else {
      createError.value = 'Не удалось создать персонажа'
    }
  } finally {
    creating.value = false
  }
}

async function handleDelete(id: number) {
  if (deletingId.value !== null) return

  if (!confirm('Удалить персонажа?')) return

  deletingId.value = id

  try {
    await deleteCharacter(id)
    await loadCharacters()
  } catch (e) {
    if (ApiException.isNetwork(e)) {
      error.value = 'Нет соединения с сервером'
    } else {
      error.value = 'Не удалось удалить персонажа'
    }
  } finally {
    deletingId.value = null
  }
}

async function handleEnter(id: number) {
  if (enteringId.value !== null) return

  enteringId.value = id
  error.value = ''

  try {
    const response = await enterCharacter(id)
    gameStore.setGameSession(response.auth_token, id)
    router.push('/game')
  } catch (e) {
    if (ApiException.isNetwork(e)) {
      error.value = 'Нет соединения с сервером'
    } else {
      error.value = 'Не удалось войти в игру'
    }
  } finally {
    enteringId.value = null
  }
}

function handleLogout() {
  authStore.logout()
  router.push('/login')
}

onMounted(() => {
  loadCharacters()
})
</script>

<template>
  <div class="padding-all">
    <div class="form-container">
      <div class="logo-container">
        <img src="/assets/img/origin_logo3.webp" alt="logo">
      </div>

      <div class="login-panel">
        Characters<br>

        <AppAlert v-if="error" type="error" class="panel-alert">
          {{ error }}
        </AppAlert>

        <div v-if="loading" class="loading-area">
          <AppSpinner size="lg" />
        </div>

        <template v-else>
          <!-- Existing characters -->
          <div
            v-for="char in characters"
            :key="char.id"
            class="window-container"
          >
            <div
              class="row"
              :class="{
                bg_selecting: enteringId === char.id,
                bg_deleting: deletingId === char.id,
              }"
              @click="handleEnter(char.id)"
            >
              {{ char.name }} [id {{ char.id }}]
            </div>

            <div
              class="row delete-char"
              :class="{ bg_selecting: enteringId === char.id }"
              @click="handleDelete(char.id)"
            >
              <span v-if="deletingId === char.id">&#8987;</span>
              <span v-else>&#128465;</span>
            </div>
          </div>

          <!-- Empty slots for "Create New" -->
          <div
            v-for="i in emptySlots"
            :key="'new-' + i"
            class="window-container"
          >
            <div
              class="row new_char"
              @click="showCreateForm = true"
            >
              Create New
            </div>
          </div>

          <!-- Create form overlay -->
          <div v-if="showCreateForm" class="create-overlay">
            <AppAlert v-if="createError" type="error" class="panel-alert">
              {{ createError }}
            </AppAlert>

            <form @submit.prevent="handleCreate">
              <AppInput
                v-model="newCharacterName"
                placeholder="Name"
                :disabled="creating"
                autofocus
              />

              <div class="create-actions">
                <AppButton
                  variant="secondary"
                  :disabled="creating"
                  @click="showCreateForm = false; newCharacterName = ''; createError = ''"
                >
                  back
                </AppButton>
                <AppButton
                  type="submit"
                  :loading="creating"
                  :disabled="!newCharacterName.trim()"
                >
                  create
                </AppButton>
              </div>
            </form>
          </div>
        </template>

        <AppButton class="logout-btn" @click="handleLogout">
          logout
        </AppButton>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.padding-all {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100vh;
}

.form-container {
  margin-top: -12%;
  width: 36%;
  max-width: 600px;
}

.logo-container {
  line-height: 55px;
  text-align: center;
  margin-bottom: 1rem;

  img {
    display: inline;
    max-width: 100%;
    max-height: 100%;
    vertical-align: middle;
    filter: drop-shadow(2px 11px 8px rgba(0, 0, 0, 0.8));
    transform: scale(1.2);
  }
}

.login-panel {
  border-radius: 18px;
  padding: 30px;
  text-align: center;
  background: rgba(16, 96, 109, 0.65);
  color: #d5eeed;
  margin: 0 auto;
  box-shadow: 0 0 10px rgba(0, 0, 0, 0.7);
  font-size: 19px;
}

.panel-alert {
  margin: 10px 0;
}

.loading-area {
  display: flex;
  justify-content: center;
  padding: 2rem;
}

.window-container {
  width: 100%;
  display: table;
  margin: 10px 0;
}

.row {
  display: table-cell;
  border-radius: 6px;
  border-color: rgba(30, 67, 91, 0.6);
  border-width: 1px;
  border-style: solid;
  margin: 10px;
  padding: 5px 0;
  background-color: #105858aa;
  cursor: pointer;
  text-align: center;
  width: 80%;

  &:hover:not(.bg_selecting):not(.new_char) {
    transition-duration: 0.6s;
    background: #228cbeff;
    box-shadow: 0 0 10px rgba(0, 0, 0, 0.7);
    animation: btn-glow 0.6s ease-in-out infinite alternate;
  }
}

.new_char {
  background-color: rgba(35, 93, 41, 0.6);
  width: 100%;

  &:hover {
    transition-duration: 0.6s;
    background: #4a9854ff;
    box-shadow: 0 0 10px rgba(0, 0, 0, 0.7);
    animation: btn-glow 0.6s ease-in-out infinite alternate;
  }
}

.delete-char {
  width: 12%;
  background-color: rgba(174, 35, 39, 0.6);

  &:hover {
    background: rgba(220, 41, 48, 0.6) !important;
  }
}

.bg_deleting {
  color: #c49e9e;
  background: repeating-linear-gradient(
    -45deg,
    #6c6161,
    #6c6161 10px,
    #625253 10px,
    #625253 20px
  );
  background-size: 400% 400%;
  animation: moving-back 12s linear infinite;
}

.bg_selecting {
  color: #7ec7d0;
  background: repeating-linear-gradient(
    -45deg,
    #548f8f,
    #548f8f 10px,
    #4b7d83 10px,
    #4b7d83 20px
  );
  background-size: 400% 400%;
  animation: moving-back 12s linear infinite;
}

.create-overlay {
  margin-top: 15px;
  padding: 15px;
  background: rgba(12, 70, 80, 0.5);
  border-radius: 10px;
}

.create-actions {
  display: flex;
  gap: 0.5rem;
  justify-content: center;
  margin-top: 10px;
}

.logout-btn {
  margin-top: 15px;
  width: 100%;
}

@keyframes moving-back {
  100% {
    background-position: 100% 100%;
  }
}

@keyframes btn-glow {
  from {
    box-shadow: 0 0 10px rgba(52, 78, 88, 0.71);
  }
  to {
    box-shadow: 0 0 20px rgba(23, 180, 200, 0.71);
  }
}

@media screen and (max-width: 1440px) {
  .form-container { width: 40%; }
}
@media screen and (max-width: 1280px) {
  .form-container { width: 46%; }
  .login-panel { padding: 25px; border-radius: 15px; }
}
@media screen and (max-width: 991px) {
  .form-container { width: 54%; }
}
@media screen and (max-width: 800px) {
  .form-container { width: 60%; }
}
@media screen and (max-width: 667px) {
  .form-container { width: 75%; }
}
@media screen and (max-width: 640px) {
  .login-panel { padding: 25px; border-radius: 10px; }
}
@media screen and (max-width: 480px) {
  .form-container { width: 92%; }
  .login-panel { padding: 15px; border-radius: 10px; }
}
@media screen and (max-width: 384px) {
  .form-container { width: 100%; }
}
@media screen and (max-height: 600px) {
  .form-container { margin-top: -2%; }
}
</style>
