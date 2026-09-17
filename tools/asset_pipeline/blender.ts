import { execFile } from 'node:child_process'
import { readFile, realpath, writeFile } from 'node:fs/promises'
import { dirname, isAbsolute, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import type { AssetRecipe, BindingRecipe, ClipRecipe } from './catalog.ts'

export interface RigMetadata {
  object: string
  objectRestMatrix: number[]
  bones: { name: string; parent: string | null; restMatrix: number[] }[]
  fingerprint: string
}
export interface RawMetadata {
  schema: 1
  assetId: string
  rig: RigMetadata | null
  sockets: Record<string, { parent: string | null; localMatrix: number[] }>
  bindings: Record<string, BindingRecipe & { gripMatrix: number[]; gripInverse: number[] }>
  textures: { imageIndex: number; path: string; sha256: string }[]
  clips: Record<string, ClipRecipe & { durationSeconds: number; sampleCount: number; loopClosureMaxError: number; channelMask: string[]; rig: RigMetadata }>
  modelFingerprint: string
  modelFingerprintInputs: { modelSha256: string; bindings: RawMetadata['bindings']; rigFingerprint: string | null
    settings: Pick<AssetRecipe, 'kind' | 'rig' | 'equipmentSlots' | 'budgets' | 'optimization'> }
}
export interface ExportRequest {
  recipe: AssetRecipe
  selectedClips: string[]
  resolvedDependencies: { assetId: string; absolutePath: string }[]
}

const execute = promisify(execFile)
export async function exportAsset(request: ExportRequest, outputDirectory: string, blenderPath: string): Promise<RawMetadata> {
  if (!isAbsolute(outputDirectory) || resolve(outputDirectory) !== outputDirectory) throw new Error('export output must be an absolute normalized path')
  const outputParent = await realpath(dirname(outputDirectory))
  if (outputParent !== dirname(outputDirectory)) throw new Error('export output must not traverse symlinks')
  if (!isAbsolute(blenderPath)) throw new Error('Blender executable must be an absolute local path')
  const requestPath = `${outputDirectory}-request.json`
  await writeFile(requestPath, JSON.stringify(request), { flag: 'wx' })
  const environment = Object.fromEntries(Object.entries(process.env).filter(([key]) => !/^(BLENDER_|PYTHON)/.test(key)))
  environment['PYTHONDONTWRITEBYTECODE'] = '1'
  try {
    await execute(blenderPath, ['--background', '--factory-startup', '--disable-autoexec',
      '--python-exit-code', '1', '--python', fileURLToPath(new URL('./blender/export_asset.py', import.meta.url)),
      '--', '--request', requestPath, '--output', outputDirectory], { env: environment, maxBuffer: 16 * 1024 * 1024 })
  } catch (error) {
    const failure = error as Error & { stdout?: string; stderr?: string }
    throw new Error(`Blender export failed: ${failure.message}\n${failure.stdout ?? ''}\n${failure.stderr ?? ''}`)
  }
  return JSON.parse(await readFile(join(outputDirectory, 'raw/metadata.json'), 'utf8')) as RawMetadata
}
