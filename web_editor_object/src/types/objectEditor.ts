export interface FrameDefLike {
  img: string
  offset?: number[]
}

export interface SpineDefLike {
  file: string
  scale?: number
  skin?: string
  dirs?: Record<string, string[]>
}

export interface LayerDefLike {
  img?: string
  frames?: FrameDefLike[]
  spine?: SpineDefLike
  interactive?: boolean
  offset?: number[]
  z?: number
  shadow?: boolean
}

export interface ResourceDefLike {
  layers: LayerDefLike[]
  offset?: number[]
  size?: number[]
  fps?: number
}

export interface ObjectFileEntry {
  fileName: string
  baselineJson: Record<string, unknown>
  workingJson: Record<string, unknown>
}

export interface ImageAssetEntry {
  relPath: string
}

export interface ObjectEditorInitResponse {
  files: Array<{ fileName: string; json: unknown }>
  images: ImageAssetEntry[]
}

export interface GeneratedImageDraft {
  key: string
  fileName: string
  objectPath: string
  layerIndex: number
  relPath: string
  sourceRelPath: string
  dataUrl: string
  alpha: number
  brushSize: number
  width: number
  height: number
}

export interface ShadowEditState {
  key: string
  targetRelPath: string
  sourceRelPath: string
  dataUrl: string
  alpha: number
  brushSize: number
  width: number
  height: number
  dirty: boolean
}

export interface ObjectTreeNode {
  key: string
  path: string
  isResource: boolean
  children: ObjectTreeNode[]
}

export interface ValidationIssue {
  code: string
  message: string
}

export interface PreviewDiffResponse {
  unifiedDiff: string
  hasChanges: boolean
}

export interface SaveObjectEditorRequest {
  fileName: string
  json: unknown
  generatedImages: Array<{ relPath: string; pngBase64: string }>
}

export interface SaveObjectEditorResponse {
  ok: boolean
  savedJsonPath: string
  savedImages: string[]
}
