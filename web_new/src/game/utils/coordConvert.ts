import type { Coord } from './Coord'
import { TILE_WIDTH_HALF, TILE_HEIGHT_HALF, getCoordPerTile } from '../Tile'

export function coordGame2Screen(gameX: number, gameY: number): Coord {
  const tileX = gameX / getCoordPerTile()
  const tileY = gameY / getCoordPerTile()

  return {
    x: tileX * TILE_WIDTH_HALF - tileY * TILE_WIDTH_HALF,
    y: tileX * TILE_HEIGHT_HALF + tileY * TILE_HEIGHT_HALF,
  }
}

export function coordScreen2Game(screenX: number, screenY: number): Coord {
  const tileX = screenX / TILE_WIDTH_HALF + screenY / TILE_HEIGHT_HALF
  const tileY = screenY / TILE_HEIGHT_HALF - screenX / TILE_WIDTH_HALF

  return {
    x: (tileX / 2) * getCoordPerTile(),
    y: (tileY / 2) * getCoordPerTile(),
  }
}

export function coordGame2ScreenWithCamera(
  gameX: number,
  gameY: number,
  cameraX: number,
  cameraY: number,
  zoom: number,
  viewportWidth: number,
  viewportHeight: number,
): Coord {
  const screen = coordGame2Screen(gameX, gameY)
  const cameraScreen = coordGame2Screen(cameraX, cameraY)

  return {
    x: (screen.x - cameraScreen.x) * zoom + viewportWidth / 2,
    y: (screen.y - cameraScreen.y) * zoom + viewportHeight / 2,
  }
}

export function coordScreen2GameWithCamera(
  screenX: number,
  screenY: number,
  cameraX: number,
  cameraY: number,
  zoom: number,
  viewportWidth: number,
  viewportHeight: number,
): Coord {
  const cameraScreen = coordGame2Screen(cameraX, cameraY)

  const worldScreenX = (screenX - viewportWidth / 2) / zoom + cameraScreen.x
  const worldScreenY = (screenY - viewportHeight / 2) / zoom + cameraScreen.y

  return coordScreen2Game(worldScreenX, worldScreenY)
}
