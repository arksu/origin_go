<script setup lang="ts">
import { ref, onMounted } from 'vue'
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
    gameStore.setGameSession(response.ws_token, response.character_id)
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
  <div class="characters-view">
    <div class="characters-container">
      <header class="characters-header">
        <h1 class="characters-header__title">Персонажи</h1>
        <AppButton variant="secondary" size="sm" @click="handleLogout">
          Выйти
        </AppButton>
      </header>

      <AppAlert v-if="error" type="error" class="characters-alert">
        {{ error }}
      </AppAlert>

      <div v-if="loading" class="characters-loading">
        <AppSpinner size="lg" />
      </div>

      <template v-else>
        <div v-if="characters.length === 0" class="characters-empty">
          <p>У вас пока нет персонажей</p>
        </div>

        <ul v-else class="characters-list">
          <li
            v-for="char in characters"
            :key="char.id"
            class="character-item"
          >
            <span class="character-item__name">{{ char.name }}</span>
            <div class="character-item__actions">
              <AppButton
                size="sm"
                :loading="enteringId === char.id"
                :disabled="enteringId !== null && enteringId !== char.id"
                @click="handleEnter(char.id)"
              >
                Играть
              </AppButton>
              <AppButton
                variant="danger"
                size="sm"
                :loading="deletingId === char.id"
                :disabled="deletingId !== null && deletingId !== char.id"
                @click="handleDelete(char.id)"
              >
                Удалить
              </AppButton>
            </div>
          </li>
        </ul>

        <div class="characters-create">
          <template v-if="!showCreateForm">
            <AppButton @click="showCreateForm = true">
              Создать персонажа
            </AppButton>
          </template>

          <form v-else class="create-form" @submit.prevent="handleCreate">
            <AppAlert v-if="createError" type="error" class="create-form__alert">
              {{ createError }}
            </AppAlert>

            <AppInput
              v-model="newCharacterName"
              label="Имя персонажа"
              placeholder="Введите имя"
              :disabled="creating"
            />

            <div class="create-form__actions">
              <AppButton
                type="submit"
                :loading="creating"
                :disabled="!newCharacterName.trim()"
              >
                Создать
              </AppButton>
              <AppButton
                variant="secondary"
                :disabled="creating"
                @click="showCreateForm = false; newCharacterName = ''; createError = ''"
              >
                Отмена
              </AppButton>
            </div>
          </form>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped lang="scss">
.characters-view {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100%;
  padding: 1rem;
}

.characters-container {
  width: 100%;
  max-width: 500px;
  padding: 2rem;
  background-color: #1f2937;
  border-radius: 12px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.3);
}

.characters-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.5rem;

  &__title {
    font-size: 1.75rem;
    font-weight: 600;
    color: #e0e0e0;
  }
}

.characters-alert {
  margin-bottom: 1rem;
}

.characters-loading {
  display: flex;
  justify-content: center;
  padding: 2rem;
}

.characters-empty {
  text-align: center;
  padding: 2rem;
  color: #a0a0a0;
}

.characters-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-bottom: 1.5rem;
}

.character-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem;
  background-color: #16213e;
  border-radius: 8px;

  &__name {
    font-size: 1.125rem;
    font-weight: 500;
    color: #e0e0e0;
  }

  &__actions {
    display: flex;
    gap: 0.5rem;
  }
}

.characters-create {
  margin-top: 1rem;
}

.create-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 1rem;
  background-color: #16213e;
  border-radius: 8px;

  &__alert {
    margin-bottom: 0.5rem;
  }

  &__actions {
    display: flex;
    gap: 0.5rem;
  }
}
</style>
