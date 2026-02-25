<template>
  <div class="app-layout" :class="{ resizing: !!resizeState }">
    <aside class="panel panel-left" :style="{ width: `${leftPanelWidth}px` }">
      <FileObjectTree />
    </aside>

    <div class="splitter splitter-v" @pointerdown="startResize('left', $event)" />

    <main class="panel-center">
      <div class="preview-pane">
        <PreviewCanvas />
      </div>
      <div class="splitter splitter-h" @pointerdown="startResize('shadow', $event)" />
      <div class="shadow-pane" :style="{ height: `${shadowPanelHeight}px` }">
        <ShadowPaintPanel />
      </div>
    </main>

    <div class="splitter splitter-v" @pointerdown="startResize('right', $event)" />

    <aside class="panel panel-right" :style="{ width: `${rightPanelWidth}px` }">
      <div class="right-top">
        <LayerPanel />
      </div>
      <div class="right-bottom">
        <DiffPanel />
      </div>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useObjectEditorStore } from '@/stores/objectEditorStore'
import FileObjectTree from '@/components/FileObjectTree.vue'
import PreviewCanvas from '@/components/PreviewCanvas.vue'
import ShadowPaintPanel from '@/components/ShadowPaintPanel.vue'
import LayerPanel from '@/components/LayerPanel.vue'
import DiffPanel from '@/components/DiffPanel.vue'

const store = useObjectEditorStore()
const leftPanelWidth = ref(320)
const rightPanelWidth = ref(420)
const shadowPanelHeight = ref(240)

const resizeState = ref<null | {
  type: 'left' | 'right' | 'shadow'
  startX: number
  startY: number
  startValue: number
}>(null)

onMounted(() => {
  void store.init()
  window.addEventListener('pointermove', onGlobalPointerMove)
  window.addEventListener('pointerup', stopResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', onGlobalPointerMove)
  window.removeEventListener('pointerup', stopResize)
})

function startResize(type: 'left' | 'right' | 'shadow', event: PointerEvent): void {
  event.preventDefault()
  const startValue =
    type === 'left'
      ? leftPanelWidth.value
      : type === 'right'
        ? rightPanelWidth.value
        : shadowPanelHeight.value
  resizeState.value = {
    type,
    startX: event.clientX,
    startY: event.clientY,
    startValue,
  }
}

function onGlobalPointerMove(event: PointerEvent): void {
  const state = resizeState.value
  if (!state) return

  if (state.type === 'left') {
    leftPanelWidth.value = state.startValue + (event.clientX - state.startX)
    return
  }
  if (state.type === 'right') {
    rightPanelWidth.value = state.startValue - (event.clientX - state.startX)
    return
  }
  shadowPanelHeight.value = state.startValue - (event.clientY - state.startY)
}

function stopResize(): void {
  resizeState.value = null
}
</script>

<style>
* {
  box-sizing: border-box;
}

html,
body,
#app {
  width: 100%;
  height: 100%;
  margin: 0;
  padding: 0;
  overflow: hidden;
  background: #1e1e1e;
  color: #ccc;
  font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
  font-size: 14px;
}

.app-layout {
  display: flex;
  width: 100%;
  height: 100%;
}

.app-layout.resizing {
  user-select: none;
  cursor: col-resize;
}

.app-layout.resizing .splitter-h {
  cursor: row-resize;
}

.panel {
  background: #252525;
  overflow: auto;
}

.panel-left {
  border-right: 1px solid #333;
  flex-shrink: 0;
}

.panel-right {
  border-left: 1px solid #333;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.panel-center {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.splitter {
  flex-shrink: 0;
  background: #2a2a2a;
  position: relative;
}

.splitter::after {
  content: '';
  position: absolute;
  inset: 0;
}

.splitter-v {
  width: 6px;
  cursor: col-resize;
  border-left: 1px solid #1f1f1f;
  border-right: 1px solid #3a3a3a;
}

.splitter-v:hover {
  background: #325a48;
}

.splitter-h {
  height: 6px;
  cursor: row-resize;
  border-top: 1px solid #1f1f1f;
  border-bottom: 1px solid #3a3a3a;
}

.splitter-h:hover {
  background: #325a48;
}

.preview-pane {
  flex: 1;
  min-height: 0;
}

.shadow-pane {
  flex-shrink: 0;
  overflow: auto;
}

.right-top {
  flex: 1;
  min-height: 0;
  border-bottom: 1px solid #333;
}

.right-bottom {
  height: 42%;
}
</style>
