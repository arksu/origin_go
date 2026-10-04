<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, nextTick, type CSSProperties } from 'vue'
import { getWindowLayout, WINDOW_SKIN } from '@/constants/windowSkin'

interface Props {
  id: number
  title: string
  innerWidth: number
  innerHeight: number
  closable?: boolean
  centerOnOpen?: boolean
  persistPosition?: boolean
}

const props = withDefaults(defineProps<Props>(), { closable: true, centerOnOpen: false, persistPosition: true })
const emit = defineEmits<{
  close: []
}>()

const layout = computed(() => getWindowLayout(props.innerWidth, props.innerHeight))
const skinStyle: CSSProperties = {
  '--window-title': `url("${WINDOW_SKIN.title}")`,
  '--window-close': `url("${WINDOW_SKIN.close}")`,
  '--window-corner': `${WINDOW_SKIN.corner}px`,
  '--window-bottom-left-width': `${WINDOW_SKIN.bottomLeftWidth}px`,
  '--window-horizontal-tile-width': `${WINDOW_SKIN.horizontalTileWidth}px`,
  '--window-vertical-tile-height': `${WINDOW_SKIN.verticalTileHeight}px`,
  '--window-left-accent-height': `${WINDOW_SKIN.leftAccentHeight}px`,
  '--window-panel-inset': `${WINDOW_SKIN.panelInset}px`,
  '--window-panel-right': `${WINDOW_SKIN.panelRight}px`,
  '--window-panel-top': `${WINDOW_SKIN.panelTop}px`,
  '--window-panel-color': WINDOW_SKIN.panelColor,
  '--window-header-height': `${WINDOW_SKIN.headerHeight}px`,
  '--window-title-height': `${WINDOW_SKIN.titleHeight}px`,
  '--window-title-top': `${WINDOW_SKIN.titleTop}px`,
  '--window-title-left': `${WINDOW_SKIN.titleLeft}px`,
  '--window-title-line-height': `${WINDOW_SKIN.titleLineHeight}px`,
  '--window-title-text-top': `${WINDOW_SKIN.titleTextTop}px`,
  '--window-title-left-cap': `${WINDOW_SKIN.titleLeftCap}px`,
  '--window-title-right-cap': `${WINDOW_SKIN.titleRightCap}px`,
  '--window-title-left-slice': WINDOW_SKIN.titleLeftCap * WINDOW_SKIN.density,
  '--window-title-right-slice': WINDOW_SKIN.titleRightCap * WINDOW_SKIN.density,
  '--window-title-font-size': `${WINDOW_SKIN.titleFontSize}px`,
  '--window-title-color': WINDOW_SKIN.titleColor,
  '--window-close-size': `${WINDOW_SKIN.closeSize}px`,
  '--window-close-hit-size': `${WINDOW_SKIN.closeHitSize}px`,
  '--window-close-top': `${WINDOW_SKIN.closeTop}px`,
  '--window-close-right': `${WINDOW_SKIN.closeRight}px`,
}
const titleRight = computed(() => props.closable
  ? WINDOW_SKIN.closeRight + WINDOW_SKIN.closeHitSize + WINDOW_SKIN.controlGap
  : WINDOW_SKIN.titleEndInset)

const left = ref(110)
const top = ref(50)
const draggableTarget = ref<HTMLDivElement | null>(null)

let clientX = 0
let clientY = 0
let movementX = 0
let movementY = 0
let touchId = -1

const onTouchDrag = function(event: TouchEvent) {
  event.preventDefault()
  if (event.touches.length == 1 && event.touches[0]?.identifier == touchId) {
    const el = draggableTarget.value
    if (!el) return

    movementX = clientX - (event.touches[0]?.clientX || 0)
    movementY = clientY - (event.touches[0]?.clientY || 0)
    clientX = event.touches[0]?.clientX || 0
    clientY = event.touches[0]?.clientY || 0

    left.value -= movementX
    top.value -= movementY
  }
}

const onTouchDragEnd = function() {
  document.removeEventListener('touchmove', onTouchDrag)
  document.removeEventListener('touchend', onTouchDragEnd)
  document.removeEventListener('touchcancel', onTouchDragEnd)
  savePosition()
}

const onTouchStart = (event: TouchEvent) => {
  if (event.touches.length == 1 && event.touches[0]) {
    clientX = event.touches[0].clientX
    clientY = event.touches[0].clientY
    touchId = event.touches[0].identifier
    document.addEventListener('touchmove', onTouchDrag, { passive: false })
    document.addEventListener('touchend', onTouchDragEnd)
    document.addEventListener('touchcancel', onTouchDragEnd)
  }
}

const onDrag = function(event: MouseEvent) {
  event.preventDefault()
  movementX = clientX - event.clientX
  movementY = clientY - event.clientY
  clientX = event.clientX
  clientY = event.clientY

  const el = draggableTarget.value
  if (!el) return
  // Several input events can arrive before Vue has updated the DOM position.
  left.value -= movementX
  top.value -= movementY
}

const onDragEnd = function() {
  document.onmousemove = null
  document.onmouseup = null
  savePosition()
}

const onMouseDown = (event: MouseEvent) => {
  if (event.button == 0) {
    clientX = event.clientX
    clientY = event.clientY
    document.onmousemove = onDrag
    document.onmouseup = onDragEnd
  }
}

function savePosition() {
  if (!props.persistPosition) return
  localStorage.setItem('wnd_' + props.id + '_left', '' + left.value)
  localStorage.setItem('wnd_' + props.id + '_top', '' + top.value)
}

onMounted(async () => {
  if (props.centerOnOpen) {
    await nextTick()
    const windowElement = draggableTarget.value
    const container = windowElement?.offsetParent as HTMLElement | null
    if (windowElement && container) {
      left.value = Math.max(0, Math.round((container.clientWidth - windowElement.offsetWidth) / 2))
      top.value = Math.max(0, Math.round((container.clientHeight - windowElement.offsetHeight) / 2))
    }
  } else if (props.persistPosition) {
    const storedLeft = localStorage.getItem('wnd_' + props.id + '_left')
    const storedTop = localStorage.getItem('wnd_' + props.id + '_top')
    if (storedLeft) left.value = +storedLeft
    if (storedTop) top.value = +storedTop
  }
})

onUnmounted(() => {
  if (document.onmousemove === onDrag) document.onmousemove = null
  if (document.onmouseup === onDragEnd) document.onmouseup = null
  document.removeEventListener('touchmove', onTouchDrag)
  document.removeEventListener('touchend', onTouchDragEnd)
  document.removeEventListener('touchcancel', onTouchDragEnd)
})

</script>

<template>
  <div
    ref="draggableTarget"
    class="window-container"
    :style="[skinStyle, { width: `${layout.width}px`, height: `${layout.height}px`, left: `${left}px`, top: `${top}px` }]"
  >
    <div class="window-panel" aria-hidden="true"></div>
    <div class="frame" aria-hidden="true">
      <div
        v-for="(texture, part) in WINDOW_SKIN.frameParts"
        :key="part"
        :class="['frame-part', `frame-part--${part}`]"
        :style="{ backgroundImage: `url('${texture}')` }"
      ></div>
    </div>

    <div
      class="window-interior"
      :style="{
        left: `${WINDOW_SKIN.interiorInsets.left}px`, top: `${WINDOW_SKIN.interiorInsets.top}px`,
        width: `${layout.interiorWidth}px`, height: `${layout.interiorHeight}px`, padding: `${layout.padding}px`,
      }"
    >
      <div class="content" :style="{ width: `${innerWidth}px`, height: `${innerHeight}px` }">
        <slot></slot>
      </div>
    </div>

    <div
      class="header"
      :title="title"
      :style="{ paddingRight: `${titleRight}px` }"
      @touchstart.prevent="onTouchStart"
      @mousedown.prevent="onMouseDown"
    >
      <div class="title">
        <span class="title-text">{{ title }}</span>
      </div>
    </div>

    <button
      v-if="closable"
      type="button"
      class="close-btn"
      :aria-label="`Close ${title}`"
      :title="`Close ${title}`"
      @pointerdown.stop
      @pointerup.stop
      @mousedown.stop
      @mouseup.stop
      @touchstart.stop
      @touchend.stop
      @keydown.enter.stop
      @keydown.space.stop
      @keyup.enter.stop
      @keyup.space.stop
      @click.stop="emit('close')"
    ></button>
  </div>
</template>

<style lang="scss" scoped>
.window-container {
  position: absolute;
  z-index: 10;
  text-align: center;
  user-select: none;
  pointer-events: auto;
  box-sizing: border-box;
}

.window-panel {
  position: absolute;
  inset: var(--window-panel-top) var(--window-panel-right) var(--window-panel-inset) var(--window-panel-inset);
  border-radius: 8px;
  background: var(--window-panel-color);
  pointer-events: none;
}

.frame {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.frame-part {
  position: absolute;
  width: var(--window-corner);
  height: var(--window-corner);
  background-size: var(--window-corner) var(--window-corner);
  background-repeat: no-repeat;
}

.frame-part--top-left { top: 0; left: 0; }
.frame-part--top-right { top: 0; right: 0; }
.frame-part--bottom-right { bottom: 0; right: 0; }
.frame-part--bottom-left {
  bottom: 0;
  left: 0;
  width: var(--window-bottom-left-width);
  background-size: var(--window-bottom-left-width) var(--window-corner);
}

.frame-part--top,
.frame-part--bottom {
  left: var(--window-corner);
  right: var(--window-corner);
  width: auto;
  background-size: var(--window-horizontal-tile-width) var(--window-corner);
  background-repeat: repeat-x;
}
.frame-part--top { top: 0; }
.frame-part--bottom { bottom: 0; left: var(--window-bottom-left-width); }

.frame-part--left,
.frame-part--right {
  top: var(--window-corner);
  bottom: var(--window-corner);
  height: auto;
  background-size: var(--window-corner) var(--window-vertical-tile-height);
  background-repeat: repeat-y;
}
.frame-part--left { left: 0; }
.frame-part--right { right: 0; }
.frame-part--left-accent {
  top: calc(50% - var(--window-left-accent-height) / 2);
  left: 0;
  height: var(--window-left-accent-height);
  background-size: var(--window-corner) var(--window-left-accent-height);
}

.window-interior {
  position: absolute;
  box-sizing: border-box;
}

.content {
  position: relative;
  // Keep nested headings and paragraphs from collapsing their margins outside.
  display: flow-root;
  font-size: 14px;
}

.header {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: var(--window-header-height);
  padding: var(--window-title-top) 0 0 var(--window-title-left);
  display: flex;
  align-items: flex-start;
  box-sizing: border-box;
  cursor: move;
  touch-action: none;
}

.title {
  display: inline-flex;
  max-width: 100%;
  min-width: 0;
  height: var(--window-title-height);
  padding: 0 2px;
  border: 0 solid transparent;
  border-left-width: var(--window-title-left-cap);
  border-right-width: var(--window-title-right-cap);
  border-image: var(--window-title) 0 var(--window-title-right-slice) 0 var(--window-title-left-slice) fill / 0 var(--window-title-right-cap) 0 var(--window-title-left-cap) stretch;
  box-sizing: border-box;
  pointer-events: none;
}

.title-text {
  display: block;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  margin-top: var(--window-title-text-top);
  line-height: var(--window-title-line-height);
  font-size: var(--window-title-font-size);
  color: var(--window-title-color);
  text-shadow: 0 1px 1px #17120b;
}

.close-btn {
  position: absolute;
  top: var(--window-close-top);
  right: var(--window-close-right);
  width: var(--window-close-hit-size);
  height: var(--window-close-hit-size);
  padding: 0;
  border: 0;
  border-radius: 3px;
  background: transparent var(--window-close) center / var(--window-close-size) var(--window-close-size) no-repeat;
  box-sizing: border-box;
  cursor: pointer;

  &:hover {
    filter: brightness(1.2);
  }

  &:active {
    filter: brightness(0.85);
  }

  &:focus-visible {
    outline: 2px solid var(--window-title-color);
    outline-offset: 1px;
  }
}
</style>
