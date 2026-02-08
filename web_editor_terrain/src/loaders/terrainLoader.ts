import type { TerrainConfig, TerrainFileEntry } from '@/types/terrain'

const configModules = import.meta.glob<TerrainConfig>('/src/terrain/*.json', { eager: true, import: 'default' })

export function loadTerrainFiles(): TerrainFileEntry[] {
  const entries: TerrainFileEntry[] = []

  for (const [path, config] of Object.entries(configModules)) {
    const fileName = path.split('/').pop()?.replace('.json', '') ?? path
    entries.push({ fileName, config: config as TerrainConfig })
  }

  entries.sort((a, b) => a.fileName.localeCompare(b.fileName))
  return entries
}
