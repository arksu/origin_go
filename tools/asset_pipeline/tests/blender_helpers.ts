import { execFile } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdtemp, readFile, realpath, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { promisify } from 'node:util'
import { fileURLToPath } from 'node:url'
import type { AssetKind, AssetRecipe } from '../catalog.ts'
import { exportAsset } from '../blender.ts'

const execute = promisify(execFile)
export const blenderPath = process.env['ASSET_PIPELINE_BLENDER'] ?? '/Applications/Blender.app/Contents/MacOS/Blender'
export const pipelineDirectory = fileURLToPath(new URL('../', import.meta.url))
export async function runBlender(script: string, args: string[]): Promise<string> {
  try {
    const result = await execute(blenderPath, ['--background', '--factory-startup', '--disable-autoexec',
      '--python-exit-code', '1', '--python', script, '--', ...args], { maxBuffer: 8 * 1024 * 1024 })
    return result.stdout
  } catch (error) {
    const failure = error as Error & { stdout?: string; stderr?: string }
    throw new Error(`${failure.message}\n${failure.stdout ?? ''}\n${failure.stderr ?? ''}`)
  }
}
export async function fileHash(path: string): Promise<string> {
  return createHash('sha256').update(await readFile(path)).digest('hex')
}
export async function createFixture(kind: AssetKind, overrides: Record<string, unknown> = {}) {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'asset-pipeline-test-')))
  const source = join(root, 'fixture.blend')
  const request = join(root, 'fixture-request.json')
  async function generate(options: Record<string, unknown>, edit = false) {
    await writeFile(request, JSON.stringify({ source, kind, overrides: options, edit }))
    await runBlender(join(pipelineDirectory, 'tests/blender_fixtures.py'), ['--request', request])
  }
  await generate(overrides)
  const recipe: AssetRecipe = {
    schema: 1, id: `${kind}/fixture`, kind,
    source: { relativePath: 'fixture.blend', absolutePath: source },
    runtimePath: '/assets/game/fixtures/test', outputDirectory: join(root, 'published'),
    dependencies: { export: [], preview: [] },
    rig: kind === 'character' ? { object: 'Rig', bones: ['root', 'hand.r'], sockets: ['socket_hand_right'] } : null,
    clips: kind === 'character' ? Object.fromEntries(['idle', 'walk'].map(name => [name, {
      action: name, range: { start: 1, end: 12 }, fps: 24, loop: true, playback: 'time' as const,
    }])) : {},
    bindings: kind === 'equipment' ? Object.fromEntries(['left', 'right'].map(side => [side, {
      slot: side, socket: `socket_hand_${side}`, grip: `grip_${side}`, policy: { kind: 'ordinary' as const },
    }])) : {},
    budgets: { trianglesByLod: { '0': 10 }, textureDimensions: { width: 256, height: 256 }, totalPublishedBytes: 1e6, boneInfluences: 4, bones: 64 },
    optimization: { meshCompression: 'meshopt', texture: { codec: 'uastc', quality: 2, width: 256, height: 256, mipmaps: true } },
  }
  return { root, source, recipe, sourceHash: () => fileHash(source), directory: (name: string) => join(root, name),
    edit: (options: Record<string, unknown>) => generate(options, true),
    editAction: (action: string, edit: { bone: string; frame: number; rotationZ: number }) =>
      generate({ actionEdit: { action, ...edit } }, true),
    saveEditorState: ({ frame, action, singularPose = false }: { frame: number; action: string; singularPose?: boolean }) =>
      generate({ savedFrame: frame, savedAction: action, savedSingularPose: singularPose }, true) }
}
export function readGlb(bytes: Buffer): Record<string, any> {
  return JSON.parse(bytes.subarray(20, 20 + bytes.readUInt32LE(12)).toString()) as Record<string, any>
}
export function accessorValues(bytes: Buffer, accessorIndex: number): number[][] {
  const document = readGlb(bytes)
  const accessor = document.accessors[accessorIndex]
  const view = document.bufferViews[accessor.bufferView]
  assertFloatAccessor(accessor.componentType)
  const components = ({ SCALAR: 1, VEC2: 2, VEC3: 3, VEC4: 4, MAT4: 16 } as Record<string, number>)[accessor.type]!
  const binaryOffset = 28 + bytes.readUInt32LE(12)
  return Array.from({ length: accessor.count }, (_, index) => Array.from({ length: components }, (_, component) =>
    bytes.readFloatLE(binaryOffset + (view.byteOffset ?? 0) + (accessor.byteOffset ?? 0) + index * (view.byteStride ?? components * 4) + component * 4)))
}
function assertFloatAccessor(componentType: number): void {
  if (componentType !== 5126) throw new Error('test helper requires float accessor')
}
export async function runExport(recipe: AssetRecipe, output: string, selectedClips = Object.keys(recipe.clips)) {
  await exportAsset({ recipe, selectedClips, resolvedDependencies: [] }, output, blenderPath)
  const modelPath = join(output, 'raw/model.glb')
  return { modelHash: await fileHash(modelPath), model: readGlb(await readFile(modelPath)),
    metadata: JSON.parse(await readFile(join(output, 'raw/metadata.json'), 'utf8')) as Record<string, any>,
    clip: async (name: string) => readGlb(await readFile(join(output, `raw/animations/${name}.glb`))) }
}
