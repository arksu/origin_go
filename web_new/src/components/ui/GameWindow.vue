<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'

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

    left.value = (el.offsetLeft - movementX)
    top.value = (el.offsetTop - movementY)
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
  left.value = (el.offsetLeft - movementX)
  top.value = (el.offsetTop - movementY)
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
      left.value = Math.max(0, (container.clientWidth - windowElement.offsetWidth) / 2)
      top.value = Math.max(0, (container.clientHeight - windowElement.offsetHeight) / 2)
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
  <div ref="draggableTarget" :style="`width: ${innerWidth + 32}px; height: ${innerHeight + 38}px; left: ${left}px; top: ${top}px;`"
       class="window-container">

    <div class="frame">
      <div class="content">
        <slot></slot>
      </div>
    </div>

    <div v-if="closable" class="close-btn-back">
      <img alt="" src="/assets/img/window_close.png">
    </div>

    <div class="header" @touchstart.prevent="onTouchStart" @mousedown.prevent="onMouseDown">
      <div class="title">
        <span class="title-text">
          {{ title }}
        </span>
      </div>
    </div>

    <div v-if="closable" class="close-btn-click">
      <div class="close-btn" @click="emit('close')"></div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.window-container {
  position: absolute;
  z-index: 10;
  text-align: center;
  user-select: none;
  pointer-events: auto;
}

.header {
  position: absolute;
  width: 100%;
  height: 25px;
  cursor: move;
}

.title {
  border-left: 18px solid transparent;
  border-right: 18px solid transparent;
  border-top: 0 solid transparent;
  border-bottom: 0 solid transparent;
  border-image: url('/assets/img/window_title.png') 0 30% 0 30% fill / 0 18px 0 18px;
  position: relative;
  height: 22px;
  display: inline-block;
}

.title-text {
  color: #eeee59;
  font-size: 13px;
  vertical-align: top;
}

.frame {
  position: absolute;
  width: 100%;
  height: 100%;
  top: 7px;
  border-width: 0;
  border-image: url('/assets/img/window_frame.png') 34% fill / 8px repeat repeat;
}

.content {
  padding: 22px 16px 16px 16px;
  width: 100%;
  height: 100%;
  font-size: 14px;
}

.close-btn-back {
  position: absolute;
  top: 10px;
  width: 100%;
}

.close-btn-back img {
  float: right;
  margin-right: 3px
}

.close-btn-click {
  text-align: right;
  position: relative;
  width: 20%;
  height: 15px;
  margin-left: auto;
  top: 10px;
}

.close-btn {
  margin-left: auto;
  float: right;
  margin-right: 3px;
  width: 13px;
  height: 13px;
  background: transparent no-repeat;
  cursor: pointer;
}

.close-btn:hover {
  background-image: url('/assets/img/btn_close_hover.png');
}
</style>
