import type { TerrainConfig } from '@/types/terrain'

export async function saveTerrain(fileName: string, config: TerrainConfig): Promise<void> {
  const res = await fetch('/__api/save-terrain', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ fileName, config }),
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error ?? `Save failed: ${res.status}`)
  }
}
