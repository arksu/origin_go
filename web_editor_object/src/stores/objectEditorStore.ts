import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type {
  GeneratedImageDraft,
  ImageAssetEntry,
  LayerDefLike,
  ObjectFileEntry,
  ResourceDefLike,
  ShadowEditState,
  ValidationIssue,
} from '@/types/objectEditor'
import { loadObjectEditorInit, previewObjectDiff } from '@/loaders/objectDataApi'
import { saveObjectEditorPayload } from '@/api/objectSaveApi'
import {
  cloneJson,
  ensureNumberPair,
  flattenSingleChildWrapper,
  getNodeAtPath,
  isPlainObject,
  isResourceDefLike,
  moveNodeToParent,
  splitDotPath,
  wrapNodeWithSubpath,
} from '@/utils/objectPath'
import { buildObjectTree, findFirstResourcePath } from '@/utils/objectTree'
import { dataUrlToPngBase64 } from '@/utils/shadowPaint'

type JsonRoot = Record<string, unknown>

function editorShadowKey(fileName: string, objectPath: string, layerIndex: number): string {
  return `${fileName}::${objectPath}::${layerIndex}`
}

function getPathLeaf(path: string): string {
  const parts = splitDotPath(path)
  return parts[parts.length - 1] ?? 'shadow'
}

function getFolderFromLayer(layer: LayerDefLike): string | null {
  if (typeof layer.img === 'string' && layer.img.includes('/')) {
    return layer.img.split('/').slice(0, -1).join('/')
  }
  if (Array.isArray(layer.frames) && layer.frames.length > 0) {
    const img = layer.frames[0]?.img
    if (img && img.includes('/')) return img.split('/').slice(0, -1).join('/')
  }
  return null
}

function getFolderSuggestion(resource: ResourceDefLike, layerIndex: number): string {
  const target = resource.layers[layerIndex]
  if (target) {
    const exact = getFolderFromLayer(target)
    if (exact) return exact
  }
  for (const layer of resource.layers) {
    const folder = getFolderFromLayer(layer)
    if (folder) return folder
  }
  return 'obj'
}

function deepEqualJson(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

export const useObjectEditorStore = defineStore('objectEditor', () => {
  const files = ref<ObjectFileEntry[]>([])
  const images = ref<ImageAssetEntry[]>([])
  const selectedFileIndex = ref(-1)
  const selectedObjectPath = ref('')
  const selectedLayerIndex = ref(-1)
  const frameSelectionByLayer = ref<Record<string, number>>({})
  const shadowDrafts = ref<Record<string, ShadowEditState>>({})
  const validationIssues = ref<ValidationIssue[]>([])
  const renderVersion = ref(0)
  const treeVersion = ref(0)
  const loading = ref(false)
  const saving = ref(false)
  const saveStatus = ref<{ ok: boolean; message: string } | null>(null)
  const diffText = ref('')
  const diffLoading = ref(false)
  let diffTimer: ReturnType<typeof setTimeout> | null = null
  let diffRequestSeq = 0

  const selectedFile = computed(() => files.value[selectedFileIndex.value])
  const selectedFileName = computed(() => selectedFile.value?.fileName ?? '')
  const selectedWorkingRoot = computed<JsonRoot | null>(() => selectedFile.value?.workingJson ?? null)
  const selectedBaselineRoot = computed<JsonRoot | null>(() => selectedFile.value?.baselineJson ?? null)

  const selectedNode = computed(() =>
    selectedWorkingRoot.value ? getNodeAtPath(selectedWorkingRoot.value, selectedObjectPath.value) : undefined,
  )

  const selectedResource = computed<ResourceDefLike | null>(() => {
    const node = selectedNode.value
    return isResourceDefLike(node) ? node : null
  })

  const selectedLayer = computed<LayerDefLike | null>(() => {
    const resource = selectedResource.value
    if (!resource) return null
    const idx = selectedLayerIndex.value
    return idx >= 0 && idx < resource.layers.length ? (resource.layers[idx] ?? null) : null
  })

  const selectedTree = computed(() => {
    const root = selectedWorkingRoot.value
    return root ? buildObjectTree(root) : []
  })

  const selectedLayerFrameIndex = computed(() => {
    const key = `${selectedFileName.value}::${selectedObjectPath.value}::${selectedLayerIndex.value}`
    return frameSelectionByLayer.value[key] ?? 0
  })

  const selectedShadowDraftKey = computed(() => {
    if (!selectedFile.value || !selectedResource.value) return ''
    if (selectedLayerIndex.value < 0) return ''
    return editorShadowKey(selectedFile.value.fileName, selectedObjectPath.value, selectedLayerIndex.value)
  })

  const selectedShadowDraft = computed(() =>
    selectedShadowDraftKey.value ? shadowDrafts.value[selectedShadowDraftKey.value] ?? null : null,
  )

  const selectedLayerPreviewOverride = computed<string | undefined>(() => {
    const layer = selectedLayer.value
    if (!layer?.shadow) return undefined
    return selectedShadowDraft.value?.dataUrl
  })

  const hasUnsavedJsonChanges = computed(() => {
    const file = selectedFile.value
    if (!file) return false
    return !deepEqualJson(file.baselineJson, file.workingJson)
  })

  const pendingGeneratedImagesForSelectedFile = computed(() => {
    const fileName = selectedFileName.value
    return Object.values(shadowDrafts.value)
      .filter((draft) => draft.dirty && draft.key.startsWith(`${fileName}::`))
      .map((draft): GeneratedImageDraft => {
        const [, objectPath, layerIndexString] = draft.key.split('::')
        return {
          key: draft.key,
          fileName,
          objectPath: objectPath ?? '',
          layerIndex: Number(layerIndexString ?? -1),
          relPath: draft.targetRelPath,
          sourceRelPath: draft.sourceRelPath,
          dataUrl: draft.dataUrl,
          alpha: draft.alpha,
          brushSize: draft.brushSize,
          width: draft.width,
          height: draft.height,
        }
      })
  })

  const hasUnsavedChanges = computed(() =>
    hasUnsavedJsonChanges.value || pendingGeneratedImagesForSelectedFile.value.length > 0,
  )

  const canPaintSelectedShadow = computed(() => {
    const layer = selectedLayer.value
    return !!(layer?.shadow && typeof layer.img === 'string')
  })

  const selectedResourceHasShadowLayer = computed(() => {
    const resource = selectedResource.value
    if (!resource) return false
    return resource.layers.some((layer) => layer.shadow === true)
  })

  function markChanged(options?: { tree?: boolean; skipDiff?: boolean }): void {
    renderVersion.value++
    if (options?.tree) treeVersion.value++
    if (!options?.skipDiff) {
      scheduleDiffRefresh()
    }
  }

  function clearValidation(): void {
    validationIssues.value = []
  }

  function pushValidation(code: string, message: string): void {
    validationIssues.value.push({ code, message })
  }

  async function init(): Promise<void> {
    loading.value = true
    saveStatus.value = null
    clearValidation()
    try {
      const data = await loadObjectEditorInit()
      files.value = data.files
        .map((entry) => {
          if (!isPlainObject(entry.json)) {
            throw new Error(`File ${entry.fileName} is not a JSON object`)
          }
          const baseline = cloneJson(entry.json as JsonRoot)
          const working = cloneJson(entry.json as JsonRoot)
          return {
            fileName: entry.fileName,
            baselineJson: baseline,
            workingJson: working,
          }
        })
        .sort((a, b) => a.fileName.localeCompare(b.fileName))
      images.value = [...data.images].sort((a, b) => a.relPath.localeCompare(b.relPath))

      if (files.value.length > 0) {
        selectFile(0)
      }
    } finally {
      loading.value = false
    }
  }

  function selectFile(index: number): void {
    if (index < 0 || index >= files.value.length) return
    selectedFileIndex.value = index
    const root = files.value[index]!.workingJson
    selectedObjectPath.value = findFirstResourcePath(root) ?? ''
    selectedLayerIndex.value = -1
    if (selectedResource.value) {
      selectedLayerIndex.value = selectedResource.value.layers.length > 0 ? 0 : -1
    }
    clearValidation()
    markChanged({ tree: true })
  }

  function selectObjectPath(path: string): void {
    selectedObjectPath.value = path
    selectedLayerIndex.value = 0
    if (!selectedResource.value) {
      selectedLayerIndex.value = -1
    } else if (selectedResource.value.layers.length === 0) {
      selectedLayerIndex.value = -1
    } else if (selectedLayerIndex.value >= selectedResource.value.layers.length) {
      selectedLayerIndex.value = 0
    }
    clearValidation()
    markChanged({ skipDiff: true })
  }

  function selectLayer(index: number): void {
    selectedLayerIndex.value = index
    clearValidation()
    markChanged({ skipDiff: true })
  }

  function setSelectedLayerFrameIndex(index: number): void {
    if (selectedLayerIndex.value < 0) return
    const key = `${selectedFileName.value}::${selectedObjectPath.value}::${selectedLayerIndex.value}`
    frameSelectionByLayer.value[key] = Math.max(0, index)
    markChanged({ skipDiff: true })
  }

  function requireSelectedResource(): ResourceDefLike {
    const resource = selectedResource.value
    if (!resource) throw new Error('Selected node is not a resource object (missing layers)')
    return resource
  }

  function ensureSelectedLayerOffset(): number[] {
    const layer = selectedLayer.value
    if (!layer) throw new Error('No layer selected')
    if (!Array.isArray(layer.offset)) {
      layer.offset = [0, 0]
    }
    if (layer.offset.length < 2) {
      layer.offset[0] = Number(layer.offset[0] ?? 0)
      layer.offset[1] = Number(layer.offset[1] ?? 0)
    }
    return layer.offset
  }

  function setRootOffsetAxis(axis: 0 | 1, value: number): void {
    const resource = requireSelectedResource() as unknown as Record<string, unknown>
    const offset = ensureNumberPair(resource, 'offset')
    offset[axis] = Number(value)
    markChanged({ skipDiff: true })
  }

  function setSelectedLayerOffsetAxis(axis: 0 | 1, value: number): void {
    const offset = ensureSelectedLayerOffset()
    offset[axis] = Number(value)
    markChanged({ skipDiff: true })
  }

  function nudgeSelectedLayer(dx: number, dy: number): void {
    const layer = selectedLayer.value
    if (!layer) return
    if (Array.isArray(layer.frames)) return // frames editing deferred
    if (layer.spine) return
    const offset = ensureSelectedLayerOffset()
    offset[0] = Number(offset[0] ?? 0) + dx
    offset[1] = Number(offset[1] ?? 0) + dy
    markChanged({ skipDiff: true })
  }

  function setSelectedLayerZ(value: number): void {
    const layer = selectedLayer.value
    if (!layer) throw new Error('No layer selected')
    layer.z = Number(value)
    markChanged()
  }

  function setSelectedLayerImage(relPath: string): void {
    const layer = selectedLayer.value
    if (!layer) throw new Error('No layer selected')
    if (!layer.img) throw new Error('Selected layer has no image path')
    layer.img = relPath
    const key = selectedShadowDraftKey.value
    if (key) delete shadowDrafts.value[key]
    markChanged()
  }

  function addShadowLayerToSelectedResource(): void {
    const resource = requireSelectedResource()

    const existingShadowIndex = resource.layers.findIndex((layer) => layer.shadow === true)
    if (existingShadowIndex >= 0) {
      selectedLayerIndex.value = existingShadowIndex
      markChanged({ skipDiff: true })
      return
    }

    const suggestedRelPath = suggestShadowRelPathForSelected() ?? `obj/${getPathLeaf(selectedObjectPath.value)}_.png`
    const sourceLayer = selectedLayer.value ?? resource.layers.find((layer) => !!layer.img || !!layer.frames)
    const sourceOffset = Array.isArray(sourceLayer?.offset)
      ? [Number(sourceLayer.offset[0] ?? 0), Number(sourceLayer.offset[1] ?? 0)]
      : [0, 0]

    const newLayer: LayerDefLike = {
      img: suggestedRelPath,
      offset: sourceOffset,
      shadow: true,
    }

    resource.layers.push(newLayer)
    selectedLayerIndex.value = resource.layers.length - 1
    markChanged()
  }

  function moveLayer(fromIndex: number, toIndex: number): void {
    const resource = requireSelectedResource()
    if (fromIndex < 0 || fromIndex >= resource.layers.length) return
    if (toIndex < 0 || toIndex >= resource.layers.length) return
    if (fromIndex === toIndex) return

    const [moved] = resource.layers.splice(fromIndex, 1)
    resource.layers.splice(toIndex, 0, moved!)
    selectedLayerIndex.value = toIndex
    markChanged()
  }

  function moveSelectedLayerUp(): void {
    if (selectedLayerIndex.value <= 0) return
    moveLayer(selectedLayerIndex.value, selectedLayerIndex.value - 1)
  }

  function moveSelectedLayerDown(): void {
    const resource = selectedResource.value
    if (!resource) return
    if (selectedLayerIndex.value < 0 || selectedLayerIndex.value >= resource.layers.length - 1) return
    moveLayer(selectedLayerIndex.value, selectedLayerIndex.value + 1)
  }

  function moveNodeAsChild(sourcePath: string, targetParentPath: string): void {
    const root = selectedWorkingRoot.value
    if (!root) return
    clearValidation()
    try {
      const targetNode = getNodeAtPath(root, targetParentPath)
      if (isResourceDefLike(targetNode)) {
        throw new Error('Cannot move node inside a resource definition (layers object)')
      }
      const nextPath = moveNodeToParent(root, sourcePath, targetParentPath)
      selectedObjectPath.value = nextPath
      selectedLayerIndex.value = selectedResource.value ? Math.min(selectedLayerIndex.value, selectedResource.value.layers.length - 1) : -1
      markChanged({ tree: true })
    } catch (error) {
      pushValidation('move-node', String(error))
    }
  }

  function moveNodeToRoot(sourcePath: string): void {
    const root = selectedWorkingRoot.value
    if (!root) return
    clearValidation()
    try {
      const nextPath = moveNodeToParent(root, sourcePath, '')
      selectedObjectPath.value = nextPath
      markChanged({ tree: true })
    } catch (error) {
      pushValidation('move-root', String(error))
    }
  }

  function renameSelectedObject(newKeyRaw: string): void {
    const root = selectedWorkingRoot.value
    const currentPath = selectedObjectPath.value
    if (!root || !currentPath) return

    const nextKey = newKeyRaw.trim()
    clearValidation()
    try {
      if (!nextKey) {
        throw new Error('Rename key cannot be empty')
      }
      if (nextKey.includes('.')) {
        throw new Error('Rename key cannot contain dot')
      }

      const parts = splitDotPath(currentPath)
      if (parts.length === 0) {
        throw new Error('Cannot rename root')
      }
      const currentKey = parts[parts.length - 1]!
      if (currentKey === nextKey) return

      const parentPath = parts.slice(0, -1).join('.')
      const nextPath = moveNodeToParent(root, currentPath, parentPath, nextKey)
      selectedObjectPath.value = nextPath
      markChanged({ tree: true })
    } catch (error) {
      pushValidation('rename-node', String(error))
    }
  }

  function addSubPathToSelected(relativeSubPath: string): void {
    const root = selectedWorkingRoot.value
    if (!root) return
    clearValidation()
    try {
      selectedObjectPath.value = wrapNodeWithSubpath(root, selectedObjectPath.value, relativeSubPath)
      markChanged({ tree: true })
    } catch (error) {
      pushValidation('wrap-subpath', String(error))
    }
  }

  function flattenSelectedWrapper(): void {
    const root = selectedWorkingRoot.value
    if (!root) return
    clearValidation()
    try {
      selectedObjectPath.value = flattenSingleChildWrapper(root, selectedObjectPath.value)
      markChanged({ tree: true })
    } catch (error) {
      pushValidation('flatten-wrapper', String(error))
    }
  }

  function getSelectedShadowLayerSourcePath(): string | null {
    const layer = selectedLayer.value
    if (!layer?.shadow || typeof layer.img !== 'string') return null
    return layer.img
  }

  function getSelectedShadowFallbackSourcePath(): string | null {
    const resource = selectedResource.value
    const layer = selectedLayer.value
    if (!resource || !layer?.shadow) return null

    for (const candidate of resource.layers) {
      if (candidate === layer) continue
      if (typeof candidate.img === 'string') return candidate.img
      if (Array.isArray(candidate.frames) && candidate.frames[0]?.img) return candidate.frames[0].img
    }
    return null
  }

  function getShadowDraft(fileName: string, objectPath: string, layerIndex: number): ShadowEditState | null {
    const key = editorShadowKey(fileName, objectPath, layerIndex)
    return shadowDrafts.value[key] ?? null
  }

  function getSelectedShadowDraft(): ShadowEditState | null {
    return selectedShadowDraft.value
  }

  function suggestShadowRelPathForSelected(): string | null {
    const file = selectedFile.value
    const resource = selectedResource.value
    const layer = selectedLayer.value
    if (!file || !resource || !layer) return null
    const folder = getFolderSuggestion(resource, selectedLayerIndex.value)
    const baseName = getPathLeaf(selectedObjectPath.value)
    let candidate = `${folder}/${baseName}_.png`

    const known = new Set(images.value.map((i) => i.relPath))
    for (const draft of Object.values(shadowDrafts.value)) {
      known.add(draft.targetRelPath)
    }

    if (!known.has(candidate)) return candidate
    let i = 2
    while (known.has(`${folder}/${baseName}_${i}_.png`)) i++
    candidate = `${folder}/${baseName}_${i}_.png`
    return candidate
  }

  function upsertSelectedShadowDraft(input: {
    dataUrl: string
    width: number
    height: number
    targetRelPath?: string
    alpha: number
    brushSize: number
  }): void {
    const file = selectedFile.value
    const layer = selectedLayer.value
    if (!file || !layer || typeof layer.img !== 'string') {
      throw new Error('No shadow layer selected')
    }
    const key = editorShadowKey(file.fileName, selectedObjectPath.value, selectedLayerIndex.value)
    const previous = shadowDrafts.value[key]
    const targetRelPath = (input.targetRelPath ?? previous?.targetRelPath ?? suggestShadowRelPathForSelected() ?? layer.img)
    shadowDrafts.value[key] = {
      key,
      sourceRelPath: layer.img,
      targetRelPath,
      dataUrl: input.dataUrl,
      alpha: input.alpha,
      brushSize: input.brushSize,
      width: input.width,
      height: input.height,
      dirty: true,
    }
    markChanged()
  }

  function setSelectedShadowTargetRelPath(relPath: string): void {
    const key = selectedShadowDraftKey.value
    if (!key) return
    const draft = shadowDrafts.value[key]
    if (!draft) return
    draft.targetRelPath = relPath
    draft.dirty = true
    markChanged()
  }

  function resetSelectedShadowDraft(): void {
    const key = selectedShadowDraftKey.value
    if (!key) return
    delete shadowDrafts.value[key]
    markChanged()
  }

  function applyShadowPathPatchesToJson(root: JsonRoot, drafts: GeneratedImageDraft[]): void {
    for (const draft of drafts) {
      const node = getNodeAtPath(root, draft.objectPath)
      if (!isResourceDefLike(node)) continue
      const layer = node.layers[draft.layerIndex]
      if (!layer || !layer.shadow || typeof layer.img !== 'string') continue
      layer.img = draft.relPath
    }
  }

  async function saveCurrentFile(): Promise<void> {
    const file = selectedFile.value
    if (!file) throw new Error('No file selected')

    saving.value = true
    saveStatus.value = null
    clearValidation()
    try {
      const payloadJson = cloneJson(file.workingJson)
      const drafts = pendingGeneratedImagesForSelectedFile.value
      applyShadowPathPatchesToJson(payloadJson, drafts)

      const generatedImages = drafts.map((draft) => ({
        relPath: draft.relPath,
        pngBase64: dataUrlToPngBase64(draft.dataUrl),
      }))

      await saveObjectEditorPayload({
        fileName: file.fileName,
        json: payloadJson,
        generatedImages,
      })

      file.workingJson = cloneJson(payloadJson)
      file.baselineJson = cloneJson(payloadJson)

      for (const draft of drafts) {
        delete shadowDrafts.value[draft.key]
      }

      for (const img of generatedImages) {
        if (!images.value.some((e) => e.relPath === img.relPath)) {
          images.value.push({ relPath: img.relPath })
        }
      }
      images.value.sort((a, b) => a.relPath.localeCompare(b.relPath))

      saveStatus.value = { ok: true, message: 'Saved successfully' }
      markChanged({ tree: true })
      await refreshDiffNow()
    } catch (error) {
      saveStatus.value = { ok: false, message: String(error) }
      throw error
    } finally {
      saving.value = false
    }
  }

  function scheduleDiffRefresh(): void {
    if (diffTimer) clearTimeout(diffTimer)
    diffTimer = setTimeout(() => {
      void refreshDiffNow()
    }, 180)
  }

  async function refreshDiffNow(): Promise<void> {
    const file = selectedFile.value
    if (!file) {
      diffText.value = ''
      return
    }

    const seq = ++diffRequestSeq
    diffLoading.value = true
    try {
      const result = await previewObjectDiff(file.fileName, file.workingJson)
      if (seq !== diffRequestSeq) return
      diffText.value = result.unifiedDiff
    } catch (error) {
      if (seq !== diffRequestSeq) return
      diffText.value = `Diff error: ${String(error)}`
    } finally {
      if (seq === diffRequestSeq) diffLoading.value = false
    }
  }

  function getSelectedRootOffset(): number[] {
    const resource = selectedResource.value
    if (!resource) return [0, 0]
    if (!Array.isArray(resource.offset)) return [0, 0]
    return [Number(resource.offset[0] ?? 0), Number(resource.offset[1] ?? 0)]
  }

  function getSelectedLayerOffset(): number[] {
    const layer = selectedLayer.value
    if (!layer || !Array.isArray(layer.offset)) return [0, 0]
    return [Number(layer.offset[0] ?? 0), Number(layer.offset[1] ?? 0)]
  }

  function isSelectedLayerEditableImage(): boolean {
    const layer = selectedLayer.value
    if (!layer) return false
    return typeof layer.img === 'string' && !layer.spine && !Array.isArray(layer.frames)
  }

  return {
    files,
    images,
    selectedFileIndex,
    selectedObjectPath,
    selectedLayerIndex,
    loading,
    saving,
    saveStatus,
    diffText,
    diffLoading,
    validationIssues,
    renderVersion,
    treeVersion,
    selectedFile,
    selectedFileName,
    selectedWorkingRoot,
    selectedBaselineRoot,
    selectedNode,
    selectedResource,
    selectedLayer,
    selectedTree,
    selectedLayerFrameIndex,
    selectedShadowDraft,
    selectedLayerPreviewOverride,
    hasUnsavedJsonChanges,
    pendingGeneratedImagesForSelectedFile,
    hasUnsavedChanges,
    canPaintSelectedShadow,
    selectedResourceHasShadowLayer,
    init,
    selectFile,
    selectObjectPath,
    selectLayer,
    setSelectedLayerFrameIndex,
    setRootOffsetAxis,
    setSelectedLayerOffsetAxis,
    nudgeSelectedLayer,
    setSelectedLayerZ,
    setSelectedLayerImage,
    addShadowLayerToSelectedResource,
    moveSelectedLayerUp,
    moveSelectedLayerDown,
    moveNodeAsChild,
    moveNodeToRoot,
    renameSelectedObject,
    addSubPathToSelected,
    flattenSelectedWrapper,
    getSelectedShadowLayerSourcePath,
    getSelectedShadowFallbackSourcePath,
    getShadowDraft,
    getSelectedShadowDraft,
    suggestShadowRelPathForSelected,
    upsertSelectedShadowDraft,
    setSelectedShadowTargetRelPath,
    resetSelectedShadowDraft,
    refreshDiffNow,
    saveCurrentFile,
    getSelectedRootOffset,
    getSelectedLayerOffset,
    isSelectedLayerEditableImage,
  }
})
