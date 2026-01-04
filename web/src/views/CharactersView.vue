<template>
  <div class="min-h-screen bg-gray-900 py-12 px-4 sm:px-6 lg:px-8">
    <div class="max-w-2xl mx-auto">
      <div class="flex justify-between items-center mb-8">
        <h1 class="text-3xl font-bold text-white">Your Characters</h1>
        <button
          @click="handleLogout"
          class="px-4 py-2 text-sm font-medium text-gray-300 hover:text-white hover:bg-gray-800 rounded-lg transition-colors"
        >
          Logout
        </button>
      </div>

      <div class="bg-gray-800 rounded-xl p-6 mb-6">
        <h2 class="text-lg font-semibold text-white mb-4">Create New Character</h2>
        <form @submit.prevent="handleCreate" class="flex gap-4">
          <input
            v-model="newCharacterName"
            type="text"
            required
            placeholder="Character name"
            class="flex-1 px-4 py-2 border border-gray-700 placeholder-gray-500 text-white bg-gray-900 rounded-lg focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
          />
          <button
            type="submit"
            :disabled="creating"
            class="px-6 py-2 border border-transparent text-sm font-medium rounded-lg text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <span v-if="creating">Creating...</span>
            <span v-else>Create</span>
          </button>
        </form>
        <div v-if="createError" class="text-red-400 text-sm mt-2">
          {{ createError }}
        </div>
      </div>

      <div v-if="loading" class="text-center text-gray-400 py-8">
        Loading characters...
      </div>

      <div v-else-if="error" class="text-center text-red-400 py-8">
        {{ error }}
      </div>

      <div v-else-if="characters.length === 0" class="text-center text-gray-400 py-8">
        No characters yet. Create one above!
      </div>

      <div v-else class="space-y-4">
        <div
          v-for="character in characters"
          :key="character.id"
          class="bg-gray-800 rounded-xl p-4 flex items-center justify-between"
        >
          <div class="flex items-center gap-4">
            <div class="w-12 h-12 bg-indigo-600 rounded-full flex items-center justify-center">
              <span class="text-white text-lg font-bold">{{ character.name.charAt(0).toUpperCase() }}</span>
            </div>
            <div>
              <h3 class="text-white font-semibold">{{ character.name }}</h3>
              <p class="text-gray-400 text-sm">ID: {{ character.id }}</p>
            </div>
          </div>
          <div class="flex gap-2">
            <button
              @click="handleEnter(character.id)"
              :disabled="enteringId === character.id"
              class="px-4 py-2 text-sm font-medium rounded-lg text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <span v-if="enteringId === character.id">Entering...</span>
              <span v-else>Enter</span>
            </button>
            <button
              @click="handleDelete(character.id)"
              :disabled="deletingId === character.id"
              class="px-4 py-2 text-sm font-medium rounded-lg text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <span v-if="deletingId === character.id">Deleting...</span>
              <span v-else>Delete</span>
            </button>
          </div>
        </div>
      </div>

    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { listCharacters, createCharacter, deleteCharacter, enterCharacter } from '../api/characters'
import { useGameStore } from '../stores/game'
import { gameConnection } from '../network/GameConnection'

const router = useRouter()
const authStore = useAuthStore()
const gameStore = useGameStore()

const characters = ref([])
const loading = ref(true)
const error = ref('')

const newCharacterName = ref('')
const creating = ref(false)
const createError = ref('')

const deletingId = ref(null)
const enteringId = ref(null)

async function fetchCharacters() {
  loading.value = true
  error.value = ''

  try {
    const data = await listCharacters()
    characters.value = data.list || []
  } catch (err) {
    error.value = err.response?.data?.message || 'Failed to load characters'
  } finally {
    loading.value = false
  }
}

async function handleCreate() {
  if (!newCharacterName.value.trim()) return

  creating.value = true
  createError.value = ''

  try {
    await createCharacter(newCharacterName.value.trim())
    newCharacterName.value = ''
    await fetchCharacters()
  } catch (err) {
    createError.value = err.response?.data?.message || 'Failed to create character'
  } finally {
    creating.value = false
  }
}

async function handleDelete(id) {
  if (!confirm('Are you sure you want to delete this character?')) return

  deletingId.value = id

  try {
    await deleteCharacter(id)
    await fetchCharacters()
  } catch (err) {
    alert(err.response?.data?.message || 'Failed to delete character')
  } finally {
    deletingId.value = null
  }
}

async function handleEnter(id) {
  enteringId.value = id

  try {
    const data = await enterCharacter(id)
    gameStore.setWsToken(data.auth_token, id)
    gameConnection.connect()
    router.push('/game')
  } catch (err) {
    alert(err.response?.data?.message || 'Failed to enter game')
  } finally {
    enteringId.value = null
  }
}

function handleLogout() {
  authStore.logout()
  router.push('/login')
}

onMounted(() => {
  fetchCharacters()
})
</script>
