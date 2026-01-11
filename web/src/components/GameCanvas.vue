<template>
  <div class="relative w-full h-full bg-black overflow-hidden" style="min-height: 868px;">
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

const TILE_SIZE_PIXELS = 2
const CHUNK_SIZE = 128
const CHUNK_PIXEL_SIZE = CHUNK_SIZE * TILE_SIZE_PIXELS
const COORD_PER_TILE = 12

const canvasWidth = ref(window.innerWidth)
const canvasHeight = ref(window.innerHeight)

// Debug flag - set to false to hide console logs
const DEBUG = false

const tileColors = {
  1: '#0013e9',   // TileWaterDeep
  3: '#177eff',   // TileWater
  10: '#a0522d',   // TileStone
  13: '#23755f',   // TileForestPine
  15: '#5ba143',   // TileForestLeaf
  32: '#f0e873',   // TileSand
  17: '#00e319',   // TileGrass
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

  if (!gameStore.worldReady) {
    ctx.value.fillStyle = '#ffffff'
    ctx.value.font = '20px Arial'
    ctx.value.fillText('World not ready yet...', 100, 50)
    return
  }

  if (DEBUG) console.debug("renderChunks", ctx.value, gameStore.worldReady, "chunks count:", gameStore.chunks.size)
  if (DEBUG) console.debug("playerPosition object:", gameStore.playerPosition)

  const playerX = gameStore.playerPosition.x || 0
  const playerY = gameStore.playerPosition.y || 0

  // Convert player coordinates from world coords to pixel coords
  // Use floor to match backend integer division behavior
  const playerTileX = Math.floor(playerX / COORD_PER_TILE)
  const playerTileY = Math.floor(playerY / COORD_PER_TILE)
  const playerPixelX = playerTileX * TILE_SIZE_PIXELS
  const playerPixelY = playerTileY * TILE_SIZE_PIXELS

  if (DEBUG) console.debug("player position:", playerX, playerY, "pixel coords:", playerPixelX, playerPixelY)

  // Center camera on player position (in pixel coordinates)
  const cameraX = playerPixelX - (canvasWidth.value / 2)
  const cameraY = playerPixelY - (canvasHeight.value / 2)

  if (DEBUG) console.debug("camera: ", cameraX, cameraY)
  if (DEBUG) console.debug("TILE_SIZE_PIXELS:", TILE_SIZE_PIXELS, "COORD_PER_TILE:", COORD_PER_TILE)
  if (DEBUG) console.debug("canvas size:", canvasWidth.value, canvasHeight.value)

  // Render all loaded chunks
  gameStore.chunks.forEach(({ coord, data }) => {
    if (DEBUG) console.debug("Rendering chunk:", coord, "data length:", data?.length)
    renderChunk(coord, data, cameraX, cameraY)
  })

  // Draw player marker at screen center (red aim crosshair)
  const playerScreenX = canvasWidth.value / 2
  const playerScreenY = canvasHeight.value / 2
  
  // Draw red aim crosshair for player
  ctx.value.strokeStyle = '#ff0000'
  ctx.value.lineWidth = 2
  ctx.value.beginPath()
  ctx.value.moveTo(playerScreenX - 10, playerScreenY)
  ctx.value.lineTo(playerScreenX + 10, playerScreenY)
  ctx.value.moveTo(playerScreenX, playerScreenY - 10)
  ctx.value.lineTo(playerScreenX, playerScreenY + 10)
  ctx.value.stroke()
  
  // Draw red circle around aim
  ctx.value.beginPath()
  ctx.value.arc(playerScreenX, playerScreenY, 5, 0, 2 * Math.PI)
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

  // Draw debug info
  const playerChunkX = Math.floor(playerTileX / CHUNK_SIZE)
  const playerChunkY = Math.floor(playerTileY / CHUNK_SIZE)
  
  ctx.value.fillStyle = '#ffffff'
  ctx.value.font = '12px monospace'
  ctx.value.fillText(`World: (${playerX}, ${playerY})`, 10, 80)
  ctx.value.fillText(`Tile: (${playerTileX}, ${playerTileY})`, 10, 95)
  ctx.value.fillText(`Chunk: (${playerChunkX}, ${playerChunkY})`, 10, 110)
  ctx.value.fillText(`Pixel: (${playerPixelX.toFixed(1)}, ${playerPixelY.toFixed(1)})`, 10, 125)
  ctx.value.fillText(`TILE_SIZE: ${TILE_SIZE_PIXELS}px`, 10, 140)

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

  const chunkPixelX = chunkX * CHUNK_PIXEL_SIZE
  const chunkPixelY = chunkY * CHUNK_PIXEL_SIZE

  if (DEBUG) console.debug("chunk pixel pos:", chunkPixelX, chunkPixelY)

  let tilesRendered = 0
  for (let ty = 0; ty < CHUNK_SIZE; ty++) {
    for (let tx = 0; tx < CHUNK_SIZE; tx++) {
      const tileIndex = ty * CHUNK_SIZE + tx
      const tileId = tiles[tileIndex] || 0

      const tilePixelX = chunkPixelX + tx * TILE_SIZE_PIXELS
      const tilePixelY = chunkPixelY + ty * TILE_SIZE_PIXELS

      const screenX = tilePixelX - cameraX
      const screenY = tilePixelY - cameraY

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
      canvasWidth.value = Math.floor(rect.width)
      canvasHeight.value = Math.floor(rect.height)
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
