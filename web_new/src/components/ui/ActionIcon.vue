<script setup lang="ts">
import { computed } from 'vue'
import type { HotbarActionId } from '@/game/hud/actionCatalog'
import { useActionPresentation } from '@/composables/useActionPresentation'

const props = defineProps<{ actionId: HotbarActionId }>()
const { presentation } = useActionPresentation()
const action = computed(() => presentation(props.actionId))
</script>

<template>
  <span class="action-icon" aria-hidden="true">
    <img v-if="action.iconPath" :src="action.iconPath" alt="" draggable="false">
    <span
      v-if="action.coolingDown"
      class="action-icon__cooldown"
      :style="{ background: `conic-gradient(from 0deg, transparent 0turn ${action.cooldownProgress}turn, rgba(0, 0, 0, 0.7) ${action.cooldownProgress}turn 1turn)` }"
    />
  </span>
</template>

<style scoped>
.action-icon {
  position: relative;
  display: inline-block;
  width: 36px;
  height: 36px;
  flex-shrink: 0;
  overflow: hidden;
  pointer-events: none;
}

.action-icon img,
.action-icon__cooldown {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.action-icon img {
  object-fit: contain;
}
</style>
