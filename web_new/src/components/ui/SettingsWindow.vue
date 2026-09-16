<script setup lang="ts">
import GameWindow from './GameWindow.vue'
import type { ActorRenderMode } from '@/composables/useActorRenderSettings'

defineProps<{
  mode: ActorRenderMode
}>()

const emit = defineEmits<{
  close: []
  updateMode: [mode: ActorRenderMode]
}>()

const renderModes: readonly { value: ActorRenderMode; label: string; description: string }[] = [
  { value: 'hybrid3d', label: 'Hybrid 3D', description: 'Smooth turning and animation.' },
  { value: 'baked8', label: 'Baked 8 directions', description: 'Eight-direction animation steps.' },
]
</script>

<template>
  <GameWindow
    :id="7003"
    :inner-height="162"
    :inner-width="270"
    title="Settings"
    @close="emit('close')"
  >
    <section class="settings-window">
      <p class="settings-window__title">Renderer</p>
      <fieldset class="settings-window__modes">
        <label v-for="renderMode in renderModes" :key="renderMode.value" class="settings-window__mode">
          <input
            :checked="mode === renderMode.value"
            :value="renderMode.value"
            name="actor-render-mode"
            type="radio"
            @change="emit('updateMode', renderMode.value)"
          >
          <span>
            <span class="settings-window__mode-label">{{ renderMode.label }}</span>
            <span class="settings-window__mode-description">{{ renderMode.description }}</span>
          </span>
        </label>
      </fieldset>
    </section>
  </GameWindow>
</template>

<style scoped lang="scss">
.settings-window {
  width: 100%;
  height: 100%;
  text-align: left;
  color: #d7ddd3;
}

.settings-window__title {
  margin: 2px 0 10px;
  color: #d9dfc1;
  font-size: 16px;
  line-height: 1.2;
  text-shadow: 0 1px 0 #1d201f;
}

.settings-window__modes {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  border: 0;
}

.settings-window__mode {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 8px;
  align-items: start;
  cursor: pointer;
}

.settings-window__mode input {
  margin: 3px 0 0;
  accent-color: #c8b15e;
}

.settings-window__mode-label,
.settings-window__mode-description {
  display: block;
}

.settings-window__mode-label {
  color: #e1e7cf;
  font-size: 14px;
}

.settings-window__mode-description {
  margin-top: 1px;
  color: #acb8ac;
  font-size: 12px;
  line-height: 1.2;
}
</style>
