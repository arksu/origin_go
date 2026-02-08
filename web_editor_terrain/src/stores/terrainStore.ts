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

  function moveLayer(variantIdx: number, layerIdx: number, ddx: number, ddy: number): void {
    const key = layerKey(variantIdx, layerIdx)
    const current = layerOffsetsMap.value[key] ?? { dx: 0, dy: 0 }
    layerOffsetsMap.value[key] = { dx: current.dx + ddx, dy: current.dy + ddy }
    renderVersion.value++
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
    return false
  })

  async function saveCurrentFile(): Promise<void> {
    const file = selectedFile.value
    if (!file) throw new Error('No file selected')

    const fileIdx = selectedFileIndex.value
    const modified: TerrainConfig = file.config.map((variant, vi) => ({
      ...variant,
      layers: variant.layers.map((layer, li) => {
        const key = `${fileIdx}:${vi}:${li}`
        const o = layerOffsetsMap.value[key]
        if (!o || (o.dx === 0 && o.dy === 0)) return layer
        return {
          ...layer,
          offset: [
            (layer.offset[0] ?? 0) + o.dx,
            (layer.offset[1] ?? 0) + o.dy,
          ],
        }
      }),
    }))

    await saveTerrain(file.fileName, modified)

    file.config.forEach((variant, vi) => {
      variant.layers.forEach((layer, li) => {
        const key = `${fileIdx}:${vi}:${li}`
        const o = layerOffsetsMap.value[key]
        if (o && (o.dx !== 0 || o.dy !== 0)) {
          layer.offset = [
            (layer.offset[0] ?? 0) + o.dx,
            (layer.offset[1] ?? 0) + o.dy,
          ]
          delete layerOffsetsMap.value[key]
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
    moveLayer,
    loadFiles,
    selectFile,
    selectVariant,
    selectLayer,
    hasUnsavedChanges,
    saveCurrentFile,
  }
})
