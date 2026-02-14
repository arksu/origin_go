<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
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
const route = useRoute()
const authStore = useAuthStore()
const gameStore = useGameStore()

const characters = ref<Character[]>([])
const loading = ref(true)
const error = ref('')
const disconnectReasonMessage = ref('')

const showCreateForm = ref(false)
const newCharacterName = ref('')
const creating = ref(false)
const createError = ref('')
const showDeleteDialog = ref(false)
const pendingDeleteCharacter = ref<Character | null>(null)

const enteringId = ref<number | null>(null)
const deletingId = ref<number | null>(null)
const listUnavailable = ref(false)

const emptySlots = computed(() => Math.max(0, MAX_CHARACTERS - characters.value.length))
const uiErrorMessage = computed(() => error.value || disconnectReasonMessage.value)

async function loadCharacters() {
  loading.value = true
  listUnavailable.value = false
  error.value = ''
  disconnectReasonMessage.value = ''
  createError.value = ''

  try {
    characters.value = await listCharacters()
  } catch (e) {
    listUnavailable.value = true
    if (ApiException.isAuth(e)) {
      router.push('/login')
      return
    }
    if (ApiException.isNetwork(e)) {
      error.value = 'No connection to server'
    } else {
      error.value = 'Failed to load characters'
    }
  } finally {
    loading.value = false
  }
}

async function handleCreate() {
  if (!newCharacterName.value.trim() || creating.value) return

  creating.value = true
  error.value = ''
  disconnectReasonMessage.value = ''
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
      createError.value = 'No connection to server'
    } else {
      createError.value = 'Failed to create character'
    }
  } finally {
    creating.value = false
  }
}

function openCreateDialog() {
  if (showDeleteDialog.value || creating.value || listUnavailable.value) return
  createError.value = ''
  showCreateForm.value = true
}

function closeCreateDialog() {
  if (creating.value) return
  showCreateForm.value = false
  newCharacterName.value = ''
  createError.value = ''
}

async function handleDelete(id: number) {
  if (listUnavailable.value) return
  const character = characters.value.find((char) => char.id === id) || null
  if (!character || deletingId.value !== null) return
  pendingDeleteCharacter.value = character
  showDeleteDialog.value = true
}

function cancelDeleteDialog() {
  if (deletingId.value !== null) return
  showDeleteDialog.value = false
  pendingDeleteCharacter.value = null
}

async function confirmDelete() {
  if (!pendingDeleteCharacter.value || deletingId.value !== null) return

  deletingId.value = pendingDeleteCharacter.value.id
  error.value = ''
  disconnectReasonMessage.value = ''

  try {
    await deleteCharacter(pendingDeleteCharacter.value.id)
    showDeleteDialog.value = false
    pendingDeleteCharacter.value = null
    await loadCharacters()
  } catch (e) {
    if (ApiException.isNetwork(e)) {
      error.value = 'No connection to server'
    } else {
      error.value = 'Failed to delete character'
    }
  } finally {
    deletingId.value = null
  }
}

async function handleEnter(id: number) {
  if (listUnavailable.value || enteringId.value !== null || showDeleteDialog.value || showCreateForm.value) return

  enteringId.value = id
  error.value = ''
  disconnectReasonMessage.value = ''

  try {
    const response = await enterCharacter(id)
    gameStore.setGameSession(response.auth_token, id)
    router.push('/game')
  } catch (e) {
    if (ApiException.isNetwork(e)) {
      error.value = 'No connection to server'
    } else {
      error.value = 'Failed to enter game'
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
  const disconnectReason = route.query.disconnectReason
  if (typeof disconnectReason === 'string' && disconnectReason.trim()) {
    disconnectReasonMessage.value = disconnectReason.trim()
    const nextQuery = { ...route.query }
    delete nextQuery.disconnectReason
    router.replace({ query: nextQuery })
  }
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

        <AppAlert v-if="uiErrorMessage" type="error" class="panel-alert">
          {{ uiErrorMessage }}
        </AppAlert>

        <div v-if="loading" class="loading-area">
          <AppSpinner size="lg" />
        </div>

        <template v-else>
          <div v-if="listUnavailable" class="list-unavailable">
            <p class="list-unavailable__text">Character list is unavailable.</p>
            <AppButton @click="loadCharacters">Retry</AppButton>
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
              <span v-if="enteringId === char.id">Entering {{ char.name }}...</span>
              <span v-else>{{ char.name }}</span>
            </div>

            <div
              class="row delete-char"
              :class="{ bg_selecting: enteringId === char.id }"
              @click.stop="handleDelete(char.id)"
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
                @click="openCreateDialog"
              >
                Create New
            </div>
          </div>
          </template>
        </template>

        <div v-if="showCreateForm" class="dialog-backdrop" @click.self="closeCreateDialog">
          <div class="create-dialog" role="dialog" aria-modal="true" aria-label="Create character">
            <h3 class="create-dialog__title">Create character</h3>
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
              <div class="dialog-actions">
                <AppButton
                  variant="secondary"
                  :disabled="creating"
                  @click="closeCreateDialog"
                >
                  Cancel
                </AppButton>
                <AppButton
                  type="submit"
                  :loading="creating"
                  :disabled="!newCharacterName.trim()"
                >
                  Create
                </AppButton>
              </div>
            </form>
          </div>
        </div>

        <div v-if="showDeleteDialog" class="dialog-backdrop" @click.self="cancelDeleteDialog">
          <div class="delete-dialog" role="dialog" aria-modal="true" aria-label="Delete character">
            <h3 class="delete-dialog__title">Delete character</h3>
            <p class="delete-dialog__text">
              This permanently deletes
              <strong>{{ pendingDeleteCharacter?.name || 'this character' }}</strong>.
              This action cannot be undone.
            </p>
            <div class="dialog-actions">
              <AppButton variant="secondary" :disabled="deletingId !== null" @click="cancelDeleteDialog">
                Cancel
              </AppButton>
              <AppButton variant="danger" :loading="deletingId !== null" @click="confirmDelete">
                Delete
              </AppButton>
            </div>
          </div>
        </div>

        <AppButton class="logout-btn" @click="handleLogout">
          Logout
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

.list-unavailable {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 14px 0;

  &__text {
    font-size: 16px;
    color: #d5eeed;
  }
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

.logout-btn {
  margin-top: 15px;
  width: 100%;
}

.dialog-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1200;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

.delete-dialog {
  width: min(92vw, 430px);
  border-radius: 10px;
  padding: 18px;
  background: rgba(13, 65, 76, 0.96);
  border: 1px solid rgba(126, 208, 252, 0.32);
  box-shadow: 0 20px 35px rgba(0, 0, 0, 0.5);

  &__title {
    font-size: 20px;
    margin-bottom: 10px;
  }

  &__text {
    font-size: 15px;
    color: #d5eeed;
    margin-bottom: 14px;
    line-height: 1.45;
  }

}

.create-dialog {
  width: min(92vw, 430px);
  border-radius: 10px;
  padding: 18px;
  background: rgba(13, 65, 76, 0.96);
  border: 1px solid rgba(126, 208, 252, 0.32);
  box-shadow: 0 20px 35px rgba(0, 0, 0, 0.5);

  &__title {
    font-size: 20px;
    margin-bottom: 10px;
  }
}

.dialog-actions {
  display: flex;
  justify-content: space-between;
  gap: 10px;

  :deep(.app-button) {
    width: 48%;
  }
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
