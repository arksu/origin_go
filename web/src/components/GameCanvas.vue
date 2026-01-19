<template>
  <div class="relative w-full h-full bg-black overflow-hidden" style="min-height: 868px;">
    <canvas
      ref="canvas"
      class="absolute inset-0"
      :width="canvasWidth"
      :height="canvasHeight"
      @click="handleCanvasClick"
    />
    <div v-if="!gameStore.worldReady" class="absolute inset-0 flex items-center justify-center bg-black bg-opacity-75">
      <p class="text-white text-lg">Loading world...</p>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useGameStore } from '../stores/game'
import { gameConnection } from '../network/GameConnection.js'

const gameStore = useGameStore()
const canvas = ref(null)
const ctx = ref(null)

const TILE_SIZE_PIXELS = 10
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

function handleCanvasClick(event) {
  if (!gameStore.worldReady || !gameStore.isConnected) return
  
  const rect = canvas.value.getBoundingClientRect()
  const screenX = event.clientX - rect.left
  const screenY = event.clientY - rect.top
  
  // Get current camera position from player position (NOT tile-aligned)
  const playerX = gameStore.playerPosition.x || 0
  const playerY = gameStore.playerPosition.y || 0
  const playerPixelX = (playerX / COORD_PER_TILE) * TILE_SIZE_PIXELS
  const playerPixelY = (playerY / COORD_PER_TILE) * TILE_SIZE_PIXELS
  
  // Calculate camera position
  const cameraX = playerPixelX - (canvasWidth.value / 2)
  const cameraY = playerPixelY - (canvasHeight.value / 2)
  
  // Convert screen coordinates to world pixel coordinates
  const worldPixelX = screenX + cameraX
  const worldPixelY = screenY + cameraY
  
  // Convert pixel coordinates directly to world coordinates (NOT tile-aligned)
  const worldX = (worldPixelX / TILE_SIZE_PIXELS) * COORD_PER_TILE
  const worldY = (worldPixelY / TILE_SIZE_PIXELS) * COORD_PER_TILE
  
    console.debug('Canvas click:', { screenX, screenY })
    console.debug('World coords:', { worldX, worldY })
  
  // Send MoveTo action
  try {
    gameConnection.sendPlayerAction(worldX, worldY)
  } catch (error) {
    console.error('Failed to send MoveTo action:', error)
  }
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

  // Convert player coordinates from world coords to pixel coords (NOT tile-aligned)
  const playerPixelX = (playerX / COORD_PER_TILE) * TILE_SIZE_PIXELS
  const playerPixelY = (playerY / COORD_PER_TILE) * TILE_SIZE_PIXELS

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

  // Render game objects
  gameStore.gameObjects.forEach((gameObject) => {
    renderGameObject(gameObject, cameraX, cameraY)
  })

  // Render player hitbox
  const playerPixelWidth = (gameStore.playerSize.x / COORD_PER_TILE) * TILE_SIZE_PIXELS
  const playerPixelHeight = (gameStore.playerSize.y / COORD_PER_TILE) * TILE_SIZE_PIXELS
  
  // Calculate centered screen position for player hitbox
  const playerScreenX = canvasWidth.value / 2 - (playerPixelWidth / 2)
  const playerScreenY = canvasHeight.value / 2 - (playerPixelHeight / 2)
  
  // Draw green rectangle for player hitbox
  ctx.value.fillStyle = '#ff2200'
  ctx.value.fillRect(playerScreenX, playerScreenY, playerPixelWidth, playerPixelHeight)
  
  // Draw white border for player hitbox
  ctx.value.strokeStyle = '#ffffff'
  ctx.value.lineWidth = 2
  ctx.value.strokeRect(playerScreenX, playerScreenY, playerPixelWidth, playerPixelHeight)
  ctx.value.lineWidth = 1

  // Draw movement target line for player
  if (gameStore.playerMovement && gameStore.playerMovement.targetPosition) {
    const targetWorldX = gameStore.playerMovement.targetPosition.x || 0
    const targetWorldY = gameStore.playerMovement.targetPosition.y || 0
    const targetPixelX = (targetWorldX / COORD_PER_TILE) * TILE_SIZE_PIXELS
    const targetPixelY = (targetWorldY / COORD_PER_TILE) * TILE_SIZE_PIXELS
    const targetScreenX = targetPixelX - cameraX
    const targetScreenY = targetPixelY - cameraY
    
    // Player center position for movement line
    const playerCenterScreenX = canvasWidth.value / 2
    const playerCenterScreenY = canvasHeight.value / 2
    
    // Draw line from player center to target
    ctx.value.strokeStyle = '#ff2200'  // Red line for movement target
    ctx.value.lineWidth = 2
    ctx.value.setLineDash([5, 5])  // Dashed line
    ctx.value.beginPath()
    ctx.value.moveTo(playerCenterScreenX, playerCenterScreenY)
    ctx.value.lineTo(targetScreenX, targetScreenY)
    ctx.value.stroke()
    
    // Draw target marker
    ctx.value.fillStyle = '#ff2200'
    ctx.value.beginPath()
    ctx.value.arc(targetScreenX, targetScreenY, 3, 0, 2 * Math.PI)
    ctx.value.fill()
    
    // Reset line style
    ctx.value.setLineDash([])
    ctx.value.lineWidth = 1
  }

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
  const playerTileX = Math.floor(playerX / COORD_PER_TILE)
  const playerTileY = Math.floor(playerY / COORD_PER_TILE)
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

function renderGameObject(gameObject, cameraX, cameraY) {
  let position = gameObject.position
  
  // If we have movement data, use the current position from movement
  if (gameObject.movement && gameObject.movement.position) {
    position = gameObject.movement.position
  } else if (gameObject.position && gameObject.position.position) {
    // For static entities from S2C_Object, position is nested
    position = gameObject.position.position
  }
  
  if (!position) return

  // Convert world coordinates directly to pixel coordinates (NOT tile-aligned)
  const worldX = position.x || 0
  const worldY = position.y || 0
  const pixelX = (worldX / COORD_PER_TILE) * TILE_SIZE_PIXELS
  const pixelY = (worldY / COORD_PER_TILE) * TILE_SIZE_PIXELS
  // console.debug("rendering game object:", gameObject, "at", pixelX, pixelY)
  
  // Get object size from Vector2 (width, height)
  let size = gameObject.size
  if (gameObject.position && gameObject.position.size) {
    // For static entities from S2C_Object, size is nested
    size = gameObject.position.size
  }
  if (gameObject.movement && gameObject.movement.velocity) {
    // For moving objects, use a default size if none specified
    size = size || { x: 10, y: 10 }
  }
  
  const width = size?.x || 10  // Default width if not specified
  const height = size?.y || 10  // Default height if not specified
  
  // Convert size from world units to pixels
  const pixelWidth = (width / COORD_PER_TILE) * TILE_SIZE_PIXELS
  const pixelHeight = (height / COORD_PER_TILE) * TILE_SIZE_PIXELS
  
  // Calculate screen position (centered)
  const screenX = pixelX - cameraX - (pixelWidth / 2)
  const screenY = pixelY - cameraY - (pixelHeight / 2)
  
  // Only render if visible
  if (
    screenX + pixelWidth > 0 &&
    screenX < canvasWidth.value &&
    screenY + pixelHeight > 0 &&
    screenY < canvasHeight.value
  ) {
    // Draw rectangle with color based on object type
    if (gameObject.objectType === 6) { // Player type
      ctx.value.fillStyle = '#ff0000' // Red for players
    } else {
      ctx.value.fillStyle = '#0011ff' // Blue for other objects
    }
    ctx.value.fillRect(screenX, screenY, pixelWidth, pixelHeight)
    
    if (DEBUG) {
      ctx.value.strokeStyle = '#ffffff'
      ctx.value.lineWidth = 1
      ctx.value.strokeRect(screenX, screenY, pixelWidth, pixelHeight)
      ctx.value.lineWidth = 1
      
      // Draw movement info if available
      if (gameObject.movement) {
        ctx.value.fillStyle = '#ffffff'
        ctx.value.font = '10px monospace'
        const moveText = `v:${gameObject.movement.velocity?.x || 0},${gameObject.movement.velocity?.y || 0}`
        ctx.value.fillText(moveText, screenX, screenY - 5)
      } else {
        // Draw entity ID for static objects
        ctx.value.fillStyle = '#ffffff'
        ctx.value.font = '10px monospace'
        ctx.value.fillText(`ID:${gameObject.entityId}`, screenX, screenY - 5)
      }
    }
  }
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

// Re-render when chunks, player position, player size, or game objects change
watch(
  () => [gameStore.chunks.size, gameStore.playerPosition, gameStore.playerSize, gameStore.gameObjects.size],
  () => {
    // Rendering happens in animation loop
  },
  { deep: true }
)
</script>
