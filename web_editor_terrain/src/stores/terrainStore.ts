import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { TerrainConfig, TerrainFileEntry } from '@/types/terrain'
import { saveTerrain } from '@/api/saveTerrainApi'

export const useTerrainStore = defineStore('terrain', () => {
  const files = ref<TerrainFileEntry[]>([])
  const selectedFileIndex = ref<number>(-1)
  const selectedVariantIndex = ref<number>(0)
  const selectedLayerIndex = ref<number>(-1)
  const layerVisibilityMap = ref<Record<string, boolean>>({})
  const layerOffsetsMap = ref<Record<string, { dx: number; dy: number }>>({})
  const layerZEdits = ref<Record<string, number>>({})
  const variantChanceEdits = ref<Record<string, number>>({})
  const layerPEdits = ref<Record<string, number>>({})
  const renderVersion = ref(0)

  const selectedFile = computed(() =>
    selectedFileIndex.value >= 0 ? files.value[selectedFileIndex.value] : undefined,
  )

  const selectedConfig = computed<TerrainConfig | undefined>(() => selectedFile.value?.config)

  const selectedVariant = computed(() => {
    const cfg = selectedConfig.value
    if (!cfg || selectedVariantIndex.value < 0) return undefined
    return cfg[selectedVariantIndex.value]
  })

  function variantKey(variantIdx: number): string {
    return `${selectedFileIndex.value}:${variantIdx}`
  }

  function layerKey(variantIdx: number, layerIdx: number): string {
    return `${selectedFileIndex.value}:${variantIdx}:${layerIdx}`
  }

  function isLayerVisible(variantIdx: number, layerIdx: number): boolean {
    const key = layerKey(variantIdx, layerIdx)
    return layerVisibilityMap.value[key] !== false
  }

  function toggleLayerVisibility(variantIdx: number, layerIdx: number): void {
    const key = layerKey(variantIdx, layerIdx)
    const current = layerVisibilityMap.value[key] !== false
    layerVisibilityMap.value[key] = !current
    renderVersion.value++
  }

  function getLayerOffset(variantIdx: number, layerIdx: number): { dx: number; dy: number } {
    const key = layerKey(variantIdx, layerIdx)
    return layerOffsetsMap.value[key] ?? { dx: 0, dy: 0 }
  }

  function setVariantChance(variantIdx: number, value: number): void {
    const key = variantKey(variantIdx)
    const cfg = selectedConfig.value
    if (!cfg) return
    const original = cfg[variantIdx]?.chance
    if (value === original) {
      delete variantChanceEdits.value[key]
    } else {
      variantChanceEdits.value[key] = value
    }
  }

  function getVariantChance(variantIdx: number): number {
    const key = variantKey(variantIdx)
    const edit = variantChanceEdits.value[key]
    if (edit !== undefined) return edit
    const cfg = selectedConfig.value
    return cfg?.[variantIdx]?.chance ?? 0
  }

  function setLayerP(variantIdx: number, layerIdx: number, value: number): void {
    const key = layerKey(variantIdx, layerIdx)
    const cfg = selectedConfig.value
    if (!cfg) return
    const original = cfg[variantIdx]?.layers[layerIdx]?.p
    if (value === original) {
      delete layerPEdits.value[key]
    } else {
      layerPEdits.value[key] = value
    }
  }

  function getLayerP(variantIdx: number, layerIdx: number): number {
    const key = layerKey(variantIdx, layerIdx)
    const edit = layerPEdits.value[key]
    if (edit !== undefined) return edit
    const cfg = selectedConfig.value
    return cfg?.[variantIdx]?.layers[layerIdx]?.p ?? 0
  }

  function moveLayer(variantIdx: number, layerIdx: number, ddx: number, ddy: number): void {
    const key = layerKey(variantIdx, layerIdx)
    const current = layerOffsetsMap.value[key] ?? { dx: 0, dy: 0 }
    layerOffsetsMap.value[key] = { dx: current.dx + ddx, dy: current.dy + ddy }
    renderVersion.value++
  }

  function setLayerZ(variantIdx: number, layerIdx: number, z: number): void {
    if (!Number.isFinite(z)) return
    const cfg = selectedConfig.value
    const layer = cfg?.[variantIdx]?.layers[layerIdx]
    if (!layer) return

    const key = layerKey(variantIdx, layerIdx)
    if (layer.z === z) {
      delete layerZEdits.value[key]
    } else {
      layerZEdits.value[key] = z
    }
    renderVersion.value++
  }

  function getLayerZ(variantIdx: number, layerIdx: number): number {
    const key = layerKey(variantIdx, layerIdx)
    return layerZEdits.value[key] ?? selectedConfig.value?.[variantIdx]?.layers[layerIdx]?.z ?? 0
  }

  function loadFiles(entries: TerrainFileEntry[]): void {
    files.value = entries
    if (entries.length > 0) {
      selectedFileIndex.value = 0
      selectedVariantIndex.value = 0
      selectedLayerIndex.value = -1
    }
  }

  function selectFile(index: number): void {
    selectedFileIndex.value = index
    selectedVariantIndex.value = 0
    selectedLayerIndex.value = -1
  }

  function selectVariant(index: number): void {
    selectedVariantIndex.value = index
    selectedLayerIndex.value = -1
  }

  function selectLayer(index: number): void {
    selectedLayerIndex.value = index
  }

  const hasUnsavedChanges = computed(() => {
    const fileIdx = selectedFileIndex.value
    if (fileIdx < 0) return false
    for (const key of Object.keys(layerOffsetsMap.value)) {
      if (!key.startsWith(`${fileIdx}:`)) continue
      const o = layerOffsetsMap.value[key]
      if (o && (o.dx !== 0 || o.dy !== 0)) return true
    }
    for (const key of Object.keys(variantChanceEdits.value)) {
      if (key.startsWith(`${fileIdx}:`)) return true
    }
    for (const key of Object.keys(layerPEdits.value)) {
      if (key.startsWith(`${fileIdx}:`)) return true
    }
    for (const key of Object.keys(layerZEdits.value)) {
      if (key.startsWith(`${fileIdx}:`)) return true
    }
    return false
  })

  async function saveCurrentFile(): Promise<void> {
    const file = selectedFile.value
    if (!file) throw new Error('No file selected')

    const fileIdx = selectedFileIndex.value
    const modified: TerrainConfig = file.config.map((variant, vi) => {
      const vKey = `${fileIdx}:${vi}`
      const newChance = variantChanceEdits.value[vKey] ?? variant.chance
      return {
        ...variant,
        chance: newChance,
        layers: variant.layers.map((layer, li) => {
          const lKey = `${fileIdx}:${vi}:${li}`
          const o = layerOffsetsMap.value[lKey]
          const newP = layerPEdits.value[lKey] ?? layer.p
          const newZ = layerZEdits.value[lKey] ?? layer.z
          const newOffset = o && (o.dx !== 0 || o.dy !== 0)
            ? [(layer.offset[0] ?? 0) + o.dx, (layer.offset[1] ?? 0) + o.dy]
            : layer.offset
          return { ...layer, p: newP, offset: newOffset, z: newZ }
        }),
      }
    })

    await saveTerrain(file.fileName, modified)

    file.config.forEach((variant, vi) => {
      const vKey = `${fileIdx}:${vi}`
      const newChance = variantChanceEdits.value[vKey]
      if (newChance !== undefined) {
        variant.chance = newChance
        delete variantChanceEdits.value[vKey]
      }
      variant.layers.forEach((layer, li) => {
        const lKey = `${fileIdx}:${vi}:${li}`
        const newP = layerPEdits.value[lKey]
        if (newP !== undefined) {
          layer.p = newP
          delete layerPEdits.value[lKey]
        }
        const newZ = layerZEdits.value[lKey]
        if (newZ !== undefined) {
          layer.z = newZ
          delete layerZEdits.value[lKey]
        }
        const o = layerOffsetsMap.value[lKey]
        if (o && (o.dx !== 0 || o.dy !== 0)) {
          layer.offset = [
            (layer.offset[0] ?? 0) + o.dx,
            (layer.offset[1] ?? 0) + o.dy,
          ]
          delete layerOffsetsMap.value[lKey]
        }
      })
    })

    renderVersion.value++
  }

  return {
    files,
    selectedFileIndex,
    selectedVariantIndex,
    selectedLayerIndex,
    selectedFile,
    selectedConfig,
    selectedVariant,
    renderVersion,
    isLayerVisible,
    toggleLayerVisibility,
    getLayerOffset,
    setVariantChance,
    getVariantChance,
    setLayerP,
    getLayerP,
    setLayerZ,
    getLayerZ,
    moveLayer,
    loadFiles,
    selectFile,
    selectVariant,
    selectLayer,
    hasUnsavedChanges,
    saveCurrentFile,
  }
})
