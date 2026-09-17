import { createHash } from 'node:crypto'
import { copyFile, mkdir, readFile, stat, writeFile } from 'node:fs/promises'
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

import { type Accessor, NodeIO } from '@gltf-transform/core'
import { ALL_EXTENSIONS } from '@gltf-transform/extensions'
import sharp from 'sharp'

export const LEGACY_GLB_PATHS = [
  'web_new/public/assets/game/characters/male_commoner/realtime/commoner_meshy.glb',
  'art_source/characters/male_commoner_v4/commoner_meshy.glb',
  'art_source/characters/male_commoner_v4/source/meshy_character.glb',
  'web_new/public/assets/game/equipment/stone_axe/stone_axe.glb',
  'art_source/equipment/stone_axe/animation-donor.glb',
] as const

interface NumericSnapshot {
  count: number
  type: string
  componentType: string
  normalized: boolean
  min: number[]
  max: number[]
  valuesSha256: string
  values?: number[]
}

interface PrimitiveSnapshot {
  mode: number
  material: string | null
  attributes: Record<string, NumericSnapshot>
  indices: NumericSnapshot | null
}

interface MeshSnapshot {
  name: string
  primitives: PrimitiveSnapshot[]
}

interface NodeSnapshot {
  name: string
  parent: number | null
  children: number[]
  translation: number[]
  rotation: number[]
  scale: number[]
  matrix: number[]
  mesh: number | null
  skin: number | null
}

interface SkinSnapshot {
  name: string
  skeleton: number | null
  joints: number[]
  inverseBindMatrices: NumericSnapshot | null
}

interface TextureSnapshot {
  name: string
  mimeType: string
  width: number
  height: number
  channels: number
  pixelSha256: string
}

interface AnimationChannelSnapshot {
  targetNode: number | null
  targetPath: string | null
  interpolation: string
  times: NumericSnapshot
  values: NumericSnapshot
}

interface AnimationSnapshot {
  name: string
  channels: AnimationChannelSnapshot[]
}

export interface GlbSnapshot {
  path: string
  byteLength: number
  fileSha256: string
  nodes: NodeSnapshot[]
  skins: SkinSnapshot[]
  meshes: MeshSnapshot[]
  textures: TextureSnapshot[]
  animations: AnimationSnapshot[]
}

interface EquipmentBindingSnapshot {
  socket: string
  position: number[]
  quaternion: number[]
}

interface ClientSnapshot {
  cycleDistanceTiles: number
  equipmentBindings: Record<'left_hand' | 'right_hand', EquipmentBindingSnapshot>
}

interface ComparisonSnapshot {
  runtime: string
  source: string
  identicalBytes: boolean
  identicalSemantics: boolean
  differingSections: string[]
}

export interface LegacySnapshot {
  schemaVersion: 1
  client: ClientSnapshot
  assets: GlbSnapshot[]
  comparisons: ComparisonSnapshot[]
}

export interface SnapshotLegacyOptions {
  root: string
  output: string
}

function sha256(bytes: Uint8Array): string {
  return createHash('sha256').update(bytes).digest('hex')
}

function typedArrayBytes(values: Exclude<ReturnType<Accessor['getArray']>, null>): Uint8Array {
  return new Uint8Array(values.buffer, values.byteOffset, values.byteLength)
}

function snapshotAccessor(accessor: Accessor, includeValues = false): NumericSnapshot {
  const values = accessor.getArray()
  if (!values) throw new Error(`Accessor ${accessor.getName() || '<unnamed>'} has no decoded values`)
  const snapshot: NumericSnapshot = {
    count: accessor.getCount(),
    type: accessor.getType(),
    componentType: values.constructor.name,
    normalized: accessor.getNormalized(),
    min: accessor.getMin([]),
    max: accessor.getMax([]),
    valuesSha256: sha256(typedArrayBytes(values)),
  }
  if (includeValues) snapshot.values = Array.from(values)
  return snapshot
}

export async function snapshotGlb(path: string, displayPath = path): Promise<GlbSnapshot> {
  const bytes = await readFile(path)
  const document = await new NodeIO().registerExtensions(ALL_EXTENSIONS).readBinary(bytes)
  const root = document.getRoot()
  const nodes = root.listNodes()
  const nodeIndex = new Map(nodes.map((node, index) => [node, index]))
  const meshes = root.listMeshes()
  const meshIndex = new Map(meshes.map((mesh, index) => [mesh, index]))
  const skins = root.listSkins()
  const skinIndex = new Map(skins.map((skin, index) => [skin, index]))

  const textureSnapshots = await Promise.all(root.listTextures().map(async (texture): Promise<TextureSnapshot> => {
    const image = texture.getImage()
    if (!image) throw new Error(`Texture ${texture.getName() || '<unnamed>'} has no embedded image`)
    const decoded = await sharp(image).ensureAlpha().raw().toBuffer({ resolveWithObject: true })
    return {
      name: texture.getName(),
      mimeType: texture.getMimeType(),
      width: decoded.info.width,
      height: decoded.info.height,
      channels: decoded.info.channels,
      pixelSha256: sha256(decoded.data),
    }
  }))

  return {
    path: displayPath,
    byteLength: (await stat(path)).size,
    fileSha256: sha256(bytes),
    nodes: nodes.map((node) => ({
      name: node.getName(),
      parent: node.getParentNode() ? (nodeIndex.get(node.getParentNode()!) ?? null) : null,
      children: node.listChildren().map((child) => requiredIndex(nodeIndex, child, 'child node')),
      translation: Array.from(node.getTranslation()),
      rotation: Array.from(node.getRotation()),
      scale: Array.from(node.getScale()),
      matrix: Array.from(node.getMatrix()),
      mesh: node.getMesh() ? requiredIndex(meshIndex, node.getMesh()!, 'mesh') : null,
      skin: node.getSkin() ? requiredIndex(skinIndex, node.getSkin()!, 'skin') : null,
    })),
    skins: skins.map((skin) => ({
      name: skin.getName(),
      skeleton: skin.getSkeleton() ? requiredIndex(nodeIndex, skin.getSkeleton()!, 'skeleton') : null,
      joints: skin.listJoints().map((joint) => requiredIndex(nodeIndex, joint, 'joint')),
      inverseBindMatrices: skin.getInverseBindMatrices() ? snapshotAccessor(skin.getInverseBindMatrices()!, true) : null,
    })),
    meshes: meshes.map((mesh): MeshSnapshot => ({
      name: mesh.getName(),
      primitives: mesh.listPrimitives().map((primitive): PrimitiveSnapshot => ({
        mode: primitive.getMode(),
        material: primitive.getMaterial()?.getName() ?? null,
        attributes: Object.fromEntries(primitive.listSemantics().map((semantic) => {
          const accessor = primitive.getAttribute(semantic)
          if (!accessor) throw new Error(`Primitive attribute ${semantic} has no accessor`)
          return [semantic, snapshotAccessor(accessor)]
        })),
        indices: primitive.getIndices() ? snapshotAccessor(primitive.getIndices()!) : null,
      })),
    })),
    textures: textureSnapshots,
    animations: root.listAnimations().map((animation): AnimationSnapshot => ({
      name: animation.getName(),
      channels: animation.listChannels().map((channel): AnimationChannelSnapshot => {
        const sampler = channel.getSampler()
        const input = sampler?.getInput()
        const output = sampler?.getOutput()
        if (!sampler || !input || !output) throw new Error(`Animation ${animation.getName()} contains an incomplete channel`)
        const target = channel.getTargetNode()
        return {
          targetNode: target ? requiredIndex(nodeIndex, target, 'animation target') : null,
          targetPath: channel.getTargetPath(),
          interpolation: sampler.getInterpolation(),
          times: snapshotAccessor(input, true),
          values: snapshotAccessor(output, true),
        }
      }),
    })),
  }
}

function requiredIndex<T>(indices: Map<T, number>, value: T, label: string): number {
  const index = indices.get(value)
  if (index === undefined) throw new Error(`GLB ${label} is not registered in its root collection`)
  return index
}

function parseNumberList(value: string, label: string): number[] {
  const result = value.split(',').map((item) => Number(item.trim()))
  if (result.length === 0 || result.some((item) => !Number.isFinite(item))) {
    throw new Error(`Unable to parse ${label} from the current client configuration`)
  }
  return result
}

async function snapshotClient(root: string): Promise<ClientSnapshot> {
  const config = await readFile(join(root, 'web_new/src/game/actors/config.ts'), 'utf8')
  const cycleDistanceText = config.match(/\bcycleDistanceTiles\s*:\s*([+\-]?(?:\d+\.?\d*|\.\d+)(?:e[+\-]?\d+)?)/i)?.[1]
  const cycleDistanceTiles = cycleDistanceText === undefined ? Number.NaN : Number(cycleDistanceText)
  if (!Number.isFinite(cycleDistanceTiles) || cycleDistanceTiles <= 0) {
    throw new Error('Unable to read a positive cycleDistanceTiles from web_new/src/game/actors/config.ts')
  }

  const equipment = await readFile(join(root, 'web_new/src/game/actors/equipment.ts'), 'utf8')
  const parseBinding = (slot: 'left_hand' | 'right_hand'): EquipmentBindingSnapshot => {
    const match = equipment.match(new RegExp(
      `${slot}\\s*:\\s*\\{\\s*socket:\\s*'([^']+)'\\s*,\\s*transform:\\s*\\{\\s*position:\\s*\\[([^\\]]+)\\]\\s*,\\s*quaternion:\\s*\\[([^\\]]+)\\]`,
      's',
    ))
    if (!match?.[1] || !match[2] || !match[3]) throw new Error(`Unable to read ${slot} binding from equipment.ts`)
    const position = parseNumberList(match[2], `${slot} position`)
    const quaternion = parseNumberList(match[3], `${slot} quaternion`)
    if (position.length !== 3 || quaternion.length !== 4) throw new Error(`Invalid ${slot} binding dimensions in equipment.ts`)
    return { socket: match[1], position, quaternion }
  }

  return { cycleDistanceTiles, equipmentBindings: { left_hand: parseBinding('left_hand'), right_hand: parseBinding('right_hand') } }
}

function semanticSections(snapshot: GlbSnapshot): Record<string, unknown> {
  return {
    nodes: snapshot.nodes,
    skins: snapshot.skins,
    meshes: snapshot.meshes,
    textures: snapshot.textures,
    animations: snapshot.animations,
  }
}

function compareAssets(runtime: GlbSnapshot, source: GlbSnapshot): ComparisonSnapshot {
  const runtimeSections = semanticSections(runtime)
  const sourceSections = semanticSections(source)
  const differingSections = Object.keys(runtimeSections).filter((section) => (
    JSON.stringify(runtimeSections[section]) !== JSON.stringify(sourceSections[section])
  ))
  return {
    runtime: runtime.path,
    source: source.path,
    identicalBytes: runtime.fileSha256 === source.fileSha256,
    identicalSemantics: differingSections.length === 0,
    differingSections,
  }
}

function validateOutputPath(root: string, output: string): string {
  const rootPath = resolve(root)
  const outputPath = resolve(output)
  const relativeOutput = relative(rootPath, outputPath)
  if (isAbsolute(relativeOutput) || relativeOutput === '..' || relativeOutput.startsWith(`..${sep}`)) {
    throw new Error('Baseline output must be inside the workspace root')
  }
  if (relativeOutput === 'art_source' || relativeOutput.startsWith(`art_source${sep}`)
    || relativeOutput === join('web_new', 'public', 'assets')
    || relativeOutput.startsWith(`${join('web_new', 'public', 'assets')}${sep}`)) {
    throw new Error('Baseline output must be outside legacy source and runtime asset directories')
  }
  return outputPath
}

export async function snapshotLegacy({ root, output }: SnapshotLegacyOptions): Promise<LegacySnapshot> {
  const workspaceRoot = resolve(root)
  const outputPath = validateOutputPath(workspaceRoot, output)
  const assets = await Promise.all(LEGACY_GLB_PATHS.map((relativePath) => (
    snapshotGlb(join(workspaceRoot, relativePath), relativePath)
  )))
  const client = await snapshotClient(workspaceRoot)
  const byPath = new Map(assets.map((asset) => [asset.path, asset]))
  const runtimeCommoner = byPath.get(LEGACY_GLB_PATHS[0])
  const sourceCommoner = byPath.get(LEGACY_GLB_PATHS[1])
  if (!runtimeCommoner || !sourceCommoner) throw new Error('Commoner runtime/source baseline pair is incomplete')

  const snapshot: LegacySnapshot = {
    schemaVersion: 1,
    client,
    assets,
    comparisons: [compareAssets(runtimeCommoner, sourceCommoner)],
  }

  await mkdir(outputPath, { recursive: true })
  for (const relativePath of LEGACY_GLB_PATHS) {
    const destination = join(outputPath, 'artifacts', relativePath)
    await mkdir(dirname(destination), { recursive: true })
    await copyFile(join(workspaceRoot, relativePath), destination)
  }
  await writeFile(join(outputPath, 'baseline.json'), `${JSON.stringify(snapshot, null, 2)}\n`)
  return snapshot
}

async function main(): Promise<void> {
  const [root, output] = process.argv.slice(2)
  if (!root || !output) throw new Error('Usage: node migration/baseline.ts <workspace-root> <output-directory>')
  const snapshot = await snapshotLegacy({ root, output })
  process.stdout.write(`${JSON.stringify({ assets: snapshot.assets.length, comparisons: snapshot.comparisons }, null, 2)}\n`)
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await main()
}
