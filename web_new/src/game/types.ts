export interface DebugInfo {
  fps: number
  cameraX: number
  cameraY: number
  zoom: number
  viewportWidth: number
  viewportHeight: number
  lastClickScreenX: number
  lastClickScreenY: number
  lastClickWorldX: number
  lastClickWorldY: number
  objectsCount: number
  chunksLoaded: number
}

export interface ScreenPoint {
  x: number
  y: number
}
