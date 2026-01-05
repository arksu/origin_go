<template>
  <div class="relative w-full h-full bg-black overflow-hidden" style="min-height: 768px;">
    <canvas
      ref="canvas"
      class="absolute inset-0"
      :width="canvasWidth"
      :height="canvasHeight"
    />
    <div v-if="!gameStore.worldReady" class="absolute inset-0 flex items-center justify-center bg-black bg-opacity-75">
      <p class="text-white text-lg">Loading world...</p>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useGameStore } from '../stores/game'

const gameStore = useGameStore()
const canvas = ref(null)
const ctx = ref(null)

const TILE_SIZE_PIXELS = 3
const CHUNK_SIZE = 128
const CHUNK_PIXEL_SIZE = CHUNK_SIZE * TILE_SIZE_PIXELS

const canvasWidth = ref(1024)
const canvasHeight = ref(768)

// Debug flag - set to false to hide console logs
const DEBUG = false

const tileColors = {
  0: '#2d5016',   // grass
  1: '#8b7355',   // dirt
  32: '#4a90e2',   // water
  3: '#90ee90',   // light grass
  4: '#a0522d',   // brown
  17: '#696969',   // dark gray
  255: '#000000'  // default/unknown
}

function getTileColor(tileId) {
  // console.debug("tileId", tileId)
  return tileColors[tileId] || tileColors[255]
}

function decompressTiles(compressedData) {
  // For now, assume tiles are raw bytes (128x128 = 16384 bytes)
  // In production, this would decompress zlib data
  if (!compressedData) return new Uint8Array(CHUNK_SIZE * CHUNK_SIZE)
  return new Uint8Array(compressedData)
}

function renderChunks() {
  if (!ctx.value) return

  // Clear canvas
  ctx.value.fillStyle = '#000000'
  ctx.value.fillRect(0, 0, canvasWidth.value, canvasHeight.value)

  // Always draw a test pattern to verify canvas works
  ctx.value.fillStyle = '#00ff00'
  ctx.value.fillRect(10, 10, 50, 50)

  if (!gameStore.worldReady) {
    ctx.value.fillStyle = '#ffffff'
    ctx.value.font = '20px Arial'
    ctx.value.fillText('World not ready yet...', 100, 50)
    return
  }

  if (DEBUG) console.debug("renderChunks", ctx.value, gameStore.worldReady, "chunks count:", gameStore.chunks.size)

  const playerX = gameStore.playerPosition.x || 0
  const playerY = gameStore.playerPosition.y || 0

  if (DEBUG) console.debug("player position:", playerX, playerY)

  // Center camera at (0,0) to see chunks around origin
  const cameraX = 0
  const cameraY = 0

  if (DEBUG) console.debug("camera: ", cameraX, cameraY)
  if (DEBUG) console.debug("canvas size:", canvasWidth.value, canvasHeight.value)

  // Render all loaded chunks
  gameStore.chunks.forEach(({ coord, data }) => {
    if (DEBUG) console.debug("Rendering chunk:", coord, "data length:", data?.length)
    renderChunk(coord, data, cameraX, cameraY)
  })

  // Draw player marker at actual player position (not center for now)
  const playerScreenX = (playerX * TILE_SIZE_PIXELS) - cameraX
  const playerScreenY = (playerY * TILE_SIZE_PIXELS) - cameraY
  
  // Draw red cross for player
  ctx.value.strokeStyle = '#ff0000'
  ctx.value.lineWidth = 2
  ctx.value.beginPath()
  ctx.value.moveTo(playerScreenX - 8, playerScreenY)
  ctx.value.lineTo(playerScreenX + 8, playerScreenY)
  ctx.value.moveTo(playerScreenX, playerScreenY - 8)
  ctx.value.lineTo(playerScreenX, playerScreenY + 8)
  ctx.value.stroke()
  ctx.value.lineWidth = 1

  // Draw center crosshair at world origin (0,0)
  const originScreenX = 0 - cameraX
  const originScreenY = 0 - cameraY
  ctx.value.strokeStyle = '#ffffff'
  ctx.value.beginPath()
  ctx.value.moveTo(originScreenX - 10, originScreenY)
  ctx.value.lineTo(originScreenX + 10, originScreenY)
  ctx.value.moveTo(originScreenX, originScreenY - 10)
  ctx.value.lineTo(originScreenX, originScreenY + 10)
  ctx.value.stroke()

  // Draw chunk boundaries for debugging
  ctx.value.strokeStyle = '#ffff00'
  ctx.value.lineWidth = 1
  gameStore.chunks.forEach(({ coord }) => {
    const chunkX = coord.x || 0
    const chunkY = coord.y || 0
    const chunkScreenX = (chunkX * CHUNK_PIXEL_SIZE) - cameraX
    const chunkScreenY = (chunkY * CHUNK_PIXEL_SIZE) - cameraY
    
    ctx.value.strokeRect(chunkScreenX, chunkScreenY, CHUNK_PIXEL_SIZE, CHUNK_PIXEL_SIZE)
  })
  ctx.value.lineWidth = 1
}

function renderChunk(coord, tileData, cameraX, cameraY) {
  const tiles = decompressTiles(tileData)
  if (DEBUG) console.debug("tiles", tiles.length, "coord:", coord)

  // Handle missing coord properties
  const chunkX = coord.x || 0
  const chunkY = coord.y || 0

  const chunkWorldX = chunkX * CHUNK_PIXEL_SIZE
  const chunkWorldY = chunkY * CHUNK_PIXEL_SIZE

  if (DEBUG) console.debug("chunk world pos:", chunkWorldX, chunkWorldY)

  let tilesRendered = 0
  for (let ty = 0; ty < CHUNK_SIZE; ty++) {
    for (let tx = 0; tx < CHUNK_SIZE; tx++) {
      const tileIndex = ty * CHUNK_SIZE + tx
      const tileId = tiles[tileIndex] || 0

      const worldX = chunkWorldX + tx * TILE_SIZE_PIXELS
      const worldY = chunkWorldY + ty * TILE_SIZE_PIXELS

      const screenX = worldX - cameraX
      const screenY = worldY - cameraY

      // Only render if visible
      if (
        screenX + TILE_SIZE_PIXELS > 0 &&
        screenX < canvasWidth.value &&
        screenY + TILE_SIZE_PIXELS > 0 &&
        screenY < canvasHeight.value
      ) {
        ctx.value.fillStyle = getTileColor(tileId)
        ctx.value.fillRect(screenX, screenY, TILE_SIZE_PIXELS, TILE_SIZE_PIXELS)
        tilesRendered++
      }
    }
  }
  if (DEBUG) console.debug("rendered", tilesRendered, "tiles for chunk", coord)
}

function animate() {
  renderChunks()
  requestAnimationFrame(animate)
}

onMounted(() => {
  if (canvas.value) {
    ctx.value = canvas.value.getContext('2d')
    
    // Resize canvas to match container
    const resizeCanvas = () => {
      const rect = canvas.value.parentElement.getBoundingClientRect()
      canvasWidth.value = Math.floor(rect.width) || 1024
      canvasHeight.value = Math.floor(rect.height) || 768
      canvas.value.width = canvasWidth.value
      canvas.value.height = canvasHeight.value
      if (DEBUG) console.debug('Canvas resized to:', canvasWidth.value, canvasHeight.value)
    }
    
    resizeCanvas()
    window.addEventListener('resize', resizeCanvas)
    
    animate()
  }
})

onUnmounted(() => {
  // Animation frame will stop when component unmounts
})

// Re-render when chunks or player position changes
watch(
  () => [gameStore.chunks.size, gameStore.playerPosition],
  () => {
    // Rendering happens in animation loop
  },
  { deep: true }
)
</script>
