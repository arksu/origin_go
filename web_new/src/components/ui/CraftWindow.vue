<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useGameStore } from '@/stores/gameStore'
import { sendStartCraftMany, sendStartCraftOne } from '@/network'
import type { proto } from '@/network/proto/packets.js'
import GameWindow from './GameWindow.vue'
import AppButton from './AppButton.vue'

const emit = defineEmits<{
  close: []
}>()

const gameStore = useGameStore()
const searchQuery = ref('')

const CRAFT_ALL_CYCLES = 0xffffffff

const recipes = computed(() => gameStore.craftRecipes)
const selectedCraftKey = computed(() => gameStore.selectedCraftKey)

const filteredRecipes = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return recipes.value

  return recipes.value.filter((recipe) => {
    const name = (recipe.name || '').toLowerCase()
    const key = (recipe.craftKey || '').toLowerCase()
    return name.includes(query) || key.includes(query)
  })
})

const selectedRecipe = computed<proto.ICraftRecipeEntry | null>(() => {
  const selected = filteredRecipes.value.find((recipe) => (recipe.craftKey || '') === selectedCraftKey.value)
  if (selected) return selected
  return filteredRecipes.value[0] || null
})

watch(
  selectedRecipe,
  (recipe) => {
    const key = recipe?.craftKey || ''
    if (key !== gameStore.selectedCraftKey) {
      gameStore.selectCraftRecipe(key)
    }
  },
  { immediate: true },
)

const canStartSelected = computed(() => Boolean(selectedRecipe.value?.flags?.canStartNow))
const selectedNeedsTool = computed(() => Boolean((selectedRecipe.value?.requiredLinkedObjectKey || '').trim()))
const selectedHasLinkedTool = computed(() => Boolean(selectedRecipe.value?.flags?.hasRequiredLinkedObject))

function onClose() {
  emit('close')
}

function selectRecipe(craftKey: string) {
  gameStore.selectCraftRecipe(craftKey)
}

function onCraftOne() {
  const craftKey = (selectedRecipe.value?.craftKey || '').trim()
  if (!craftKey || !canStartSelected.value) return
  sendStartCraftOne(craftKey)
}

function onCraftAll() {
  const craftKey = (selectedRecipe.value?.craftKey || '').trim()
  if (!craftKey || !canStartSelected.value) return
  sendStartCraftMany(craftKey, CRAFT_ALL_CYCLES)
}

function prettifyKey(value: string | null | undefined): string {
  const normalized = (value || '').trim()
  if (!normalized) return '-'
  return normalized
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function itemLabel(entry: { itemKey?: string | null; count?: number | null }): string {
  const name = prettifyKey(entry.itemKey)
  const count = Math.max(1, Number(entry.count || 0))
  return `${name} x${count}`
}
</script>

<template>
  <GameWindow
    :id="7101"
    :inner-width="680"
    :inner-height="360"
    title="Craft"
    @close="onClose"
  >
    <div class="craft-window">
      <div class="craft-window__search-row">
        <input
          v-model="searchQuery"
          type="text"
          class="craft-window__search"
          placeholder="Search recipes..."
          autocomplete="off"
        >
      </div>

      <div class="craft-window__body">
        <section class="craft-window__panel craft-window__recipes">
          <div class="craft-window__panel-title">Recipes</div>
          <div class="craft-window__recipe-list">
            <button
              v-for="(recipe, idx) in filteredRecipes"
              :key="recipe.craftKey || recipe.name || idx"
              type="button"
              class="craft-window__recipe-row"
              :class="{
                'is-selected': (recipe.craftKey || '') === (selectedRecipe?.craftKey || ''),
                'is-disabled': !(recipe.flags?.canStartNow ?? false),
              }"
              @click="selectRecipe(recipe.craftKey || '')"
            >
              <span class="craft-window__recipe-name">{{ recipe.name || prettifyKey(recipe.craftKey) }}</span>
            </button>

            <div v-if="filteredRecipes.length === 0" class="craft-window__empty">
              No recipes
            </div>
          </div>
        </section>

        <section class="craft-window__panel craft-window__details">
          <template v-if="selectedRecipe">
            <div class="craft-window__panel-title">{{ selectedRecipe.name || prettifyKey(selectedRecipe.craftKey) }}</div>

            <div class="craft-window__section">
              <div class="craft-window__section-title">Inputs</div>
              <div class="craft-window__chips">
                <div
                  v-for="(input, idx) in selectedRecipe.inputs || []"
                  :key="`${selectedRecipe.craftKey || 'recipe'}-in-${idx}`"
                  class="craft-window__chip"
                  :class="{ 'is-muted': !(selectedRecipe.flags?.hasInputs ?? false) }"
                >
                  {{ itemLabel(input) }}
                </div>
              </div>
            </div>

            <div class="craft-window__section">
              <div class="craft-window__section-title">Output</div>
              <div class="craft-window__chips">
                <div
                  v-for="(output, idx) in selectedRecipe.outputs || []"
                  :key="`${selectedRecipe.craftKey || 'recipe'}-out-${idx}`"
                  class="craft-window__chip craft-window__chip--output"
                  :class="{ 'is-muted': !(selectedRecipe.flags?.hasOutputSpace ?? false) }"
                >
                  {{ itemLabel(output) }}
                </div>
              </div>
            </div>

            <div v-if="selectedNeedsTool" class="craft-window__section">
              <div class="craft-window__section-title">Tools</div>
              <div class="craft-window__chips">
                <div
                  class="craft-window__chip craft-window__chip--tool"
                  :class="{ 'is-muted': !selectedHasLinkedTool }"
                >
                  {{ prettifyKey(selectedRecipe.requiredLinkedObjectKey) }}
                </div>
              </div>
            </div>
          </template>

          <div v-else class="craft-window__empty craft-window__empty--details">
            Select a recipe
          </div>
        </section>
      </div>

      <div class="craft-window__actions">
        <AppButton size="sm" :disabled="!selectedRecipe || !canStartSelected" @click="onCraftOne">
          Craft One
        </AppButton>
        <AppButton size="sm" :disabled="!selectedRecipe || !canStartSelected" @click="onCraftAll">
          Craft All
        </AppButton>
      </div>
    </div>
  </GameWindow>
</template>

<style scoped lang="scss">
.craft-window {
  width: 100%;
  height: 100%;
  display: grid;
  grid-template-rows: auto 1fr auto;
  gap: 10px;
  color: #dbe5ea;
  text-align: left;
}

.craft-window__search-row {
  display: flex;
}

.craft-window__search {
  width: 100%;
  height: 30px;
  padding: 6px 10px;
  border-radius: 7px;
  border: 1px solid rgba(183, 204, 216, 0.28);
  background: rgba(247, 250, 252, 0.05);
  color: #e7f0f5;
  font-size: 13px;
  outline: none;

  &::placeholder {
    color: rgba(215, 228, 235, 0.65);
  }

  &:focus {
    border-color: rgba(107, 188, 220, 0.65);
    box-shadow: 0 0 0 1px rgba(77, 160, 194, 0.18);
  }
}

.craft-window__body {
  min-height: 0;
  display: grid;
  grid-template-columns: 240px 1fr;
  gap: 10px;
}

.craft-window__panel {
  min-height: 0;
  border: 1px solid rgba(196, 214, 224, 0.16);
  border-radius: 8px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.035), rgba(0, 0, 0, 0.05)),
    rgba(18, 33, 41, 0.42);
  padding: 10px;
}

.craft-window__panel-title {
  margin-bottom: 8px;
  color: #eef7fb;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.2px;
}

.craft-window__recipes {
  display: flex;
  flex-direction: column;
}

.craft-window__recipe-list {
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
  overflow-y: auto;
  padding-right: 2px;
}

.craft-window__recipe-row {
  width: 100%;
  text-align: left;
  padding: 8px 10px;
  border-radius: 6px;
  border: 1px solid transparent;
  background: rgba(246, 249, 251, 0.03);
  color: #d9e4ea;
  cursor: pointer;
  transition: background-color 0.12s ease, border-color 0.12s ease;

  &:hover {
    background: rgba(255, 255, 255, 0.07);
  }

  &.is-selected {
    background: rgba(40, 122, 95, 0.22);
    border-color: rgba(69, 186, 145, 0.45);
    color: #f1fbf8;
  }

  &.is-disabled {
    color: rgba(217, 228, 234, 0.58);
  }
}

.craft-window__recipe-name {
  font-size: 13px;
}

.craft-window__details {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.craft-window__section {
  margin-bottom: 12px;
}

.craft-window__section:last-child {
  margin-bottom: 0;
}

.craft-window__section-title {
  margin-bottom: 6px;
  color: #c5d5dd;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.8px;
}

.craft-window__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.craft-window__chip {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 4px 9px;
  border-radius: 6px;
  border: 1px solid rgba(190, 208, 218, 0.16);
  background: rgba(249, 252, 253, 0.04);
  color: #e4edf2;
  font-size: 12px;
  line-height: 1.2;
}

.craft-window__chip--output {
  border-color: rgba(104, 176, 149, 0.24);
  background: rgba(48, 106, 88, 0.11);
}

.craft-window__chip--tool {
  border-color: rgba(120, 162, 205, 0.26);
  background: rgba(45, 72, 100, 0.13);
}

.craft-window__chip.is-muted {
  opacity: 0.5;
}

.craft-window__empty {
  padding: 8px 4px;
  color: rgba(219, 229, 234, 0.62);
  font-size: 13px;
}

.craft-window__empty--details {
  margin-top: 24px;
}

.craft-window__actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  padding-top: 4px;
}

.craft-window__actions :deep(.app-button) {
  min-width: 130px;
}

@media (max-width: 900px) {
  .craft-window__body {
    grid-template-columns: 1fr;
    grid-template-rows: 120px 1fr;
  }

  .craft-window__recipe-list {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    align-content: start;
  }
}
</style>
