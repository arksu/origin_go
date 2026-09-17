import { readdir, readFile, realpath, stat } from 'node:fs/promises'
import { isAbsolute, relative, resolve, sep } from 'node:path'

import { parseDocument } from 'yaml'

export type AssetKind = 'character' | 'equipment' | 'world_object'
export type PlaybackMode = 'time' | 'distance'

export interface ResolvedSource {
  relativePath: string
  absolutePath: string
}

export interface RecipeDependencies {
  export: string[]
  preview: string[]
}

export interface RigContract {
  object: string
  bones: string[]
  sockets: string[]
}

export interface ClipRecipe {
  action: string
  range: { start: number; end: number }
  fps: number
  loop: boolean
  playback: PlaybackMode
  cycleDistanceTiles?: number
}

export type ClipPolicy =
  | { kind: 'ordinary' }
  | { kind: 'layered'; idleClip: string; walkClip?: string }

export interface BindingRecipe {
  slot: string
  socket: string
  grip: string
  policy: ClipPolicy
}

export interface RecipeBudgets {
  trianglesByLod: Record<string, number>
  textureDimensions: { width: number; height: number }
  totalPublishedBytes: number
  boneInfluences: number
}

export interface OptimizationRecipe {
  meshCompression: 'meshopt'
  texture: {
    codec: 'uastc' | 'etc1s'
    quality: number
    width: number
    height: number
    mipmaps: boolean
  }
}

export interface AssetRecipe {
  schema: 1
  id: string
  kind: AssetKind
  source: ResolvedSource
  runtimePath: string
  outputDirectory: string
  dependencies: RecipeDependencies
  rig: RigContract | null
  clips: Record<string, ClipRecipe>
  bindings: Record<string, BindingRecipe>
  budgets: RecipeBudgets
  optimization: OptimizationRecipe
}

const ASSET_KINDS = new Set<AssetKind>(['character', 'equipment', 'world_object'])
const ASSET_ID_PATTERN = /^(character|equipment|world_object)\/[a-z0-9][a-z0-9_-]*$/
const NAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_.-]*$/
const PATH_SEGMENT_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_.-]*$/

type UnknownRecord = Record<string, unknown>

function requireRecord(value: unknown, field: string): UnknownRecord {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`${field} must be an object`)
  }
  return value as UnknownRecord
}

function rejectUnknownFields(value: UnknownRecord, field: string, allowed: readonly string[]): void {
  const allowedFields = new Set(allowed)
  for (const key of Object.keys(value)) {
    if (!allowedFields.has(key)) throw new Error(`${field} has unknown field ${key}`)
  }
  for (const key of allowed) {
    if (!Object.hasOwn(value, key)) throw new Error(`${field} is missing required field ${key}`)
  }
}

function requireString(value: unknown, field: string): string {
  if (typeof value !== 'string' || value.length === 0) throw new Error(`${field} must be a non-empty string`)
  return value
}

function requireName(value: unknown, field: string): string {
  const name = requireString(value, field)
  if (!NAME_PATTERN.test(name)) throw new Error(`${field} must be a portable Blender name`)
  return name
}

function requireFiniteNumber(value: unknown, field: string): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) throw new Error(`${field} must be finite`)
  return value
}

function requirePositiveNumber(value: unknown, field: string): number {
  const number = requireFiniteNumber(value, field)
  if (number <= 0) throw new Error(`${field} must be positive`)
  return number
}

function requirePositiveInteger(value: unknown, field: string): number {
  const number = requirePositiveNumber(value, field)
  if (!Number.isInteger(number)) throw new Error(`${field} must be an integer`)
  return number
}

function requireBoolean(value: unknown, field: string): boolean {
  if (typeof value !== 'boolean') throw new Error(`${field} must be a boolean`)
  return value
}

function requireUniqueNames(value: unknown, field: string): string[] {
  if (!Array.isArray(value)) throw new Error(`${field} must be an array`)
  const names = value.map((entry, index) => requireName(entry, `${field}[${index}]`))
  if (new Set(names).size !== names.length) throw new Error(`${field} contains duplicate names`)
  return names
}

function requireAssetId(value: unknown, field: string): string {
  const id = requireString(value, field)
  if (!ASSET_ID_PATTERN.test(id)) {
    throw new Error(`${field} must be kind/id using character, equipment, or world_object`)
  }
  return id
}

function requireRelativePath(value: unknown, field: string): string {
  const path = requireString(value, field)
  if (isAbsolute(path) || path.includes('\\')) throw new Error(`${field} must be a portable relative path`)
  const segments = path.split('/')
  if (segments.some((segment) => segment === '..')) throw new Error(`${field} must not contain parent traversal`)
  if (segments.some((segment) => segment === '' || segment === '.' || !PATH_SEGMENT_PATTERN.test(segment))) {
    throw new Error(`${field} must be a normalized portable relative path`)
  }
  return segments.join('/')
}

function isPathInside(root: string, path: string): boolean {
  const relativePath = relative(root, path)
  return relativePath === '' || (!relativePath.startsWith(`..${sep}`) && relativePath !== '..' && !isAbsolute(relativePath))
}

async function resolveSource(root: string, recipePath: string, sourceValue: unknown): Promise<ResolvedSource> {
  const declaredPath = requireRelativePath(sourceValue, 'source')
  const candidatePath = resolve(recipePath, '..', declaredPath)
  let sourcePath: string
  try {
    sourcePath = await realpath(candidatePath)
  } catch (error) {
    throw new Error(`source ${declaredPath} does not exist: ${error instanceof Error ? error.message : String(error)}`)
  }
  if (!isPathInside(root, sourcePath)) throw new Error(`source ${declaredPath} escapes catalog root`)
  if (!(await stat(sourcePath)).isFile()) throw new Error(`source ${declaredPath} must be a file`)
  return { relativePath: relative(root, sourcePath).split(sep).join('/'), absolutePath: sourcePath }
}

async function resolveRuntimeOutput(root: string, value: unknown): Promise<{ runtimePath: string; outputDirectory: string }> {
  const runtimePath = requireString(value, 'runtimePath')
  if (!/^\/assets\/game\/[a-z0-9][a-z0-9_-]*\/[a-z0-9][a-z0-9_-]*$/.test(runtimePath)) {
    throw new Error('runtimePath must be an unencoded same-origin URL under /assets/game/<category>/<asset>')
  }
  const publicDirectory = await realpath(resolve(root, 'web_new/public'))
  if (!isPathInside(root, publicDirectory)) throw new Error('runtimePath public root escapes catalog root')

  let outputDirectory = publicDirectory
  for (const segment of runtimePath.slice(1).split('/')) {
    const candidate = resolve(outputDirectory, segment)
    try {
      outputDirectory = await realpath(candidate)
    } catch (error) {
      const code = error instanceof Error && 'code' in error ? error.code : undefined
      if (code !== 'ENOENT') throw error
      outputDirectory = candidate
    }
    if (!isPathInside(publicDirectory, outputDirectory)) {
      throw new Error(`runtimePath ${runtimePath} escapes web_new/public through a symlink`)
    }
  }
  return { runtimePath, outputDirectory }
}

function normalizeDependencies(value: unknown): RecipeDependencies {
  const dependencies = requireRecord(value, 'dependencies')
  rejectUnknownFields(dependencies, 'dependencies', ['export', 'preview'])
  const normalizeList = (field: 'export' | 'preview'): string[] => {
    const entries = dependencies[field]
    if (!Array.isArray(entries)) throw new Error(`dependencies.${field} must be an array`)
    const ids = entries.map((entry, index) => requireAssetId(entry, `dependencies.${field}[${index}]`))
    if (new Set(ids).size !== ids.length) throw new Error(`dependencies.${field} contains duplicate asset IDs`)
    return ids
  }
  return { export: normalizeList('export'), preview: normalizeList('preview') }
}

function normalizeRig(value: unknown, kind: AssetKind): RigContract | null {
  if (value === null) {
    if (kind === 'character') throw new Error('rig is required for character recipes')
    return null
  }
  const rig = requireRecord(value, 'rig')
  rejectUnknownFields(rig, 'rig', ['object', 'bones', 'sockets'])
  const normalized = {
    object: requireName(rig.object, 'rig.object'),
    bones: requireUniqueNames(rig.bones, 'rig.bones'),
    sockets: requireUniqueNames(rig.sockets, 'rig.sockets'),
  }
  if (kind === 'character' && (normalized.bones.length === 0 || normalized.sockets.length === 0)) {
    throw new Error('character rig must declare required bones and sockets')
  }
  return normalized
}

function normalizeClips(value: unknown): Record<string, ClipRecipe> {
  const clipEntries = requireRecord(value, 'clips')
  const normalized: Record<string, ClipRecipe> = {}
  for (const [clipId, rawClip] of Object.entries(clipEntries)) {
    requireName(clipId, 'clip id')
    const clip = requireRecord(rawClip, `clips.${clipId}`)
    const allowed = ['action', 'range', 'fps', 'loop', 'playback']
    if (Object.hasOwn(clip, 'cycleDistanceTiles')) allowed.push('cycleDistanceTiles')
    rejectUnknownFields(clip, `clips.${clipId}`, allowed)
    const range = requireRecord(clip.range, `clips.${clipId}.range`)
    rejectUnknownFields(range, `clips.${clipId}.range`, ['start', 'end'])
    const start = requireFiniteNumber(range.start, `clips.${clipId}.range.start`)
    const end = requireFiniteNumber(range.end, `clips.${clipId}.range.end`)
    if (end < start) throw new Error(`clips.${clipId}.range end must not precede start`)
    const playback = requireString(clip.playback, `clips.${clipId}.playback`)
    if (playback !== 'time' && playback !== 'distance') {
      throw new Error(`clips.${clipId}.playback must be time or distance`)
    }
    const base: Omit<ClipRecipe, 'cycleDistanceTiles'> = {
      action: requireName(clip.action, `clips.${clipId}.action`),
      range: { start, end },
      fps: requirePositiveNumber(clip.fps, `clips.${clipId}.fps`),
      loop: requireBoolean(clip.loop, `clips.${clipId}.loop`),
      playback,
    }
    if (playback === 'distance') {
      if (!Object.hasOwn(clip, 'cycleDistanceTiles')) {
        throw new Error(`clips.${clipId}.cycleDistanceTiles is required for distance playback`)
      }
      normalized[clipId] = {
        ...base,
        cycleDistanceTiles: requirePositiveNumber(clip.cycleDistanceTiles, `clips.${clipId}.cycleDistanceTiles`),
      }
    } else {
      if (Object.hasOwn(clip, 'cycleDistanceTiles')) {
        throw new Error(`clips.${clipId}.cycleDistanceTiles is only valid for distance playback`)
      }
      normalized[clipId] = base
    }
  }
  return normalized
}

function normalizePolicy(value: unknown, field: string): ClipPolicy {
  const policy = requireRecord(value, field)
  const kind = requireString(policy.kind, `${field}.kind`)
  if (kind === 'ordinary') {
    rejectUnknownFields(policy, field, ['kind'])
    return { kind }
  }
  if (kind === 'layered') {
    const allowed = ['kind', 'idleClip']
    if (Object.hasOwn(policy, 'walkClip')) allowed.push('walkClip')
    rejectUnknownFields(policy, field, allowed)
    const idleClip = requireName(policy.idleClip, `${field}.idleClip`)
    if (Object.hasOwn(policy, 'walkClip')) {
      return { kind, idleClip, walkClip: requireName(policy.walkClip, `${field}.walkClip`) }
    }
    return { kind, idleClip }
  }
  throw new Error(`${field}.kind must be ordinary or layered`)
}

function normalizeBindings(value: unknown): Record<string, BindingRecipe> {
  const bindingEntries = requireRecord(value, 'bindings')
  const normalized: Record<string, BindingRecipe> = {}
  for (const [bindingId, rawBinding] of Object.entries(bindingEntries)) {
    requireName(bindingId, 'binding id')
    const binding = requireRecord(rawBinding, `bindings.${bindingId}`)
    rejectUnknownFields(binding, `bindings.${bindingId}`, ['slot', 'socket', 'grip', 'policy'])
    normalized[bindingId] = {
      slot: requireName(binding.slot, `bindings.${bindingId}.slot`),
      socket: requireName(binding.socket, `bindings.${bindingId}.socket`),
      grip: requireName(binding.grip, `bindings.${bindingId}.grip`),
      policy: normalizePolicy(binding.policy, `bindings.${bindingId}.policy`),
    }
  }
  return normalized
}

function normalizeBudgets(value: unknown): RecipeBudgets {
  const budgets = requireRecord(value, 'budgets')
  rejectUnknownFields(budgets, 'budgets', [
    'trianglesByLod', 'textureDimensions', 'totalPublishedBytes', 'boneInfluences',
  ])
  const triangleEntries = requireRecord(budgets.trianglesByLod, 'budgets.trianglesByLod')
  if (Object.keys(triangleEntries).length === 0) throw new Error('budgets.trianglesByLod must not be empty')
  const trianglesByLod: Record<string, number> = {}
  for (const [lod, limit] of Object.entries(triangleEntries)) {
    requireName(lod, 'LOD name')
    trianglesByLod[lod] = requirePositiveInteger(limit, `budgets.trianglesByLod.${lod}`)
  }
  const dimensions = requireRecord(budgets.textureDimensions, 'budgets.textureDimensions')
  rejectUnknownFields(dimensions, 'budgets.textureDimensions', ['width', 'height'])
  return {
    trianglesByLod,
    textureDimensions: {
      width: requirePositiveInteger(dimensions.width, 'budgets.textureDimensions.width'),
      height: requirePositiveInteger(dimensions.height, 'budgets.textureDimensions.height'),
    },
    totalPublishedBytes: requirePositiveInteger(budgets.totalPublishedBytes, 'budgets.totalPublishedBytes'),
    boneInfluences: requirePositiveInteger(budgets.boneInfluences, 'budgets.boneInfluences'),
  }
}

function normalizeOptimization(value: unknown, budgets: RecipeBudgets): OptimizationRecipe {
  const optimization = requireRecord(value, 'optimization')
  rejectUnknownFields(optimization, 'optimization', ['meshCompression', 'texture'])
  if (optimization.meshCompression !== 'meshopt') throw new Error('optimization.meshCompression must be meshopt')
  const texture = requireRecord(optimization.texture, 'optimization.texture')
  rejectUnknownFields(texture, 'optimization.texture', ['codec', 'quality', 'width', 'height', 'mipmaps'])
  const codec = texture.codec
  if (codec !== 'uastc' && codec !== 'etc1s') {
    throw new Error('optimization.texture.codec must be uastc or etc1s')
  }
  const quality = requireFiniteNumber(texture.quality, 'optimization.texture.quality')
  if (!Number.isInteger(quality)) throw new Error('optimization.texture.quality must be an integer')
  if (codec === 'uastc' && (quality < 0 || quality > 4)) {
    throw new Error('UASTC quality must be an integer from 0 through 4')
  }
  if (codec === 'etc1s' && (quality < 1 || quality > 255)) {
    throw new Error('ETC1S quality must be an integer from 1 through 255')
  }
  const normalizedTexture: OptimizationRecipe['texture'] = {
    codec,
    quality,
    width: requirePositiveInteger(texture.width, 'optimization.texture.width'),
    height: requirePositiveInteger(texture.height, 'optimization.texture.height'),
    mipmaps: requireBoolean(texture.mipmaps, 'optimization.texture.mipmaps'),
  }
  if (
    normalizedTexture.width > budgets.textureDimensions.width
    || normalizedTexture.height > budgets.textureDimensions.height
  ) {
    throw new Error('optimization.texture dimensions exceed the declared texture budget')
  }
  return { meshCompression: 'meshopt', texture: normalizedTexture }
}

async function normalizeRecipe(root: string, recipePath: string, value: unknown): Promise<AssetRecipe> {
  const recipe = requireRecord(value, recipePath)
  rejectUnknownFields(recipe, recipePath, [
    'schema', 'id', 'kind', 'source', 'runtimePath', 'dependencies', 'rig', 'clips', 'bindings',
    'budgets', 'optimization',
  ])
  if (recipe.schema !== 1) throw new Error(`${recipePath} schema must be 1`)
  if (typeof recipe.kind !== 'string' || !ASSET_KINDS.has(recipe.kind as AssetKind)) {
    throw new Error(`${recipePath} kind must be character, equipment, or world_object`)
  }
  const kind = recipe.kind as AssetKind
  const id = requireAssetId(recipe.id, `${recipePath}.id`)
  if (!id.startsWith(`${kind}/`)) throw new Error(`${recipePath}.id kind must match ${kind}`)
  const runtime = await resolveRuntimeOutput(root, recipe.runtimePath)
  const budgets = normalizeBudgets(recipe.budgets)
  return {
    schema: 1,
    id,
    kind,
    source: await resolveSource(root, recipePath, recipe.source),
    runtimePath: runtime.runtimePath,
    outputDirectory: runtime.outputDirectory,
    dependencies: normalizeDependencies(recipe.dependencies),
    rig: normalizeRig(recipe.rig, kind),
    clips: normalizeClips(recipe.clips),
    bindings: normalizeBindings(recipe.bindings),
    budgets,
    optimization: normalizeOptimization(recipe.optimization, budgets),
  }
}

async function findRecipeFiles(directory: string): Promise<string[]> {
  let entries
  try {
    entries = await readdir(directory, { withFileTypes: true })
  } catch (error) {
    const code = error instanceof Error && 'code' in error ? error.code : undefined
    if (code === 'ENOENT') return []
    throw error
  }
  const files: string[] = []
  for (const entry of entries.sort((left, right) => left.name.localeCompare(right.name))) {
    const path = resolve(directory, entry.name)
    if (entry.isDirectory()) files.push(...await findRecipeFiles(path))
    else if (entry.isFile() && entry.name === 'asset.yaml') files.push(path)
  }
  return files
}

function assertDependencies(recipes: ReadonlyMap<string, AssetRecipe>): void {
  for (const recipe of recipes.values()) {
    for (const dependency of [...recipe.dependencies.export, ...recipe.dependencies.preview]) {
      if (!recipes.has(dependency)) throw new Error(`${recipe.id} has missing dependency ${dependency}`)
      if (dependency === recipe.id) throw new Error(`Cyclic dependencies include ${recipe.id}`)
    }
  }

  const visiting = new Set<string>()
  const visited = new Set<string>()
  const visit = (id: string, chain: string[]): void => {
    if (visiting.has(id)) throw new Error(`Cyclic dependencies: ${[...chain, id].join(' -> ')}`)
    if (visited.has(id)) return
    visiting.add(id)
    const recipe = recipes.get(id)
    if (!recipe) throw new Error(`Missing dependency ${id}`)
    const dependencies = [...recipe.dependencies.export, ...recipe.dependencies.preview].sort()
    for (const dependency of dependencies) visit(dependency, [...chain, id])
    visiting.delete(id)
    visited.add(id)
  }
  for (const id of [...recipes.keys()].sort()) visit(id, [])
}

function assertOutputPaths(recipes: ReadonlyMap<string, AssetRecipe>): void {
  const ordered = [...recipes.values()].sort((left, right) => left.outputDirectory.localeCompare(right.outputDirectory))
  for (let index = 0; index < ordered.length; index += 1) {
    const recipe = ordered[index]
    if (!recipe) continue
    for (let otherIndex = index + 1; otherIndex < ordered.length; otherIndex += 1) {
      const other = ordered[otherIndex]
      if (!other) continue
      if (
        other.outputDirectory === recipe.outputDirectory
        || other.outputDirectory.startsWith(`${recipe.outputDirectory}${sep}`)
      ) {
        throw new Error(`runtimePath output collision between ${recipe.id} and ${other.id}`)
      }
    }
  }
}

export async function loadRecipes(root: string): Promise<Map<string, AssetRecipe>> {
  if (!isAbsolute(root)) throw new Error('Catalog root must be absolute')
  const catalogRoot = await realpath(root)
  const recipeFiles = await findRecipeFiles(resolve(catalogRoot, 'art_source'))
  const recipes = new Map<string, AssetRecipe>()
  for (const recipeFile of recipeFiles) {
    const canonicalRecipePath = await realpath(recipeFile)
    if (!isPathInside(catalogRoot, canonicalRecipePath)) throw new Error(`Recipe ${recipeFile} escapes catalog root`)
    const document = parseDocument(await readFile(canonicalRecipePath, 'utf8'), { uniqueKeys: true })
    if (document.errors.length > 0) {
      throw new Error(`Invalid YAML in ${relative(catalogRoot, canonicalRecipePath)}: ${document.errors[0]?.message}`)
    }
    const recipe = await normalizeRecipe(catalogRoot, canonicalRecipePath, document.toJS({ maxAliasCount: 0 }))
    if (recipes.has(recipe.id)) throw new Error(`Duplicate recipe id ${recipe.id}`)
    recipes.set(recipe.id, recipe)
  }
  assertOutputPaths(recipes)
  assertDependencies(recipes)
  return recipes
}

export function selectRecipes(recipes: ReadonlyMap<string, AssetRecipe>, target: string): AssetRecipe[] {
  if (target !== 'all' && !recipes.has(target)) throw new Error(`Unknown asset target ${target}`)
  const selected: AssetRecipe[] = []
  const visited = new Set<string>()
  const visit = (id: string): void => {
    if (visited.has(id)) return
    const recipe = recipes.get(id)
    if (!recipe) throw new Error(`Missing export dependency ${id}`)
    for (const dependency of [...recipe.dependencies.export].sort()) visit(dependency)
    visited.add(id)
    selected.push(recipe)
  }
  if (target === 'all') {
    for (const id of [...recipes.keys()].sort()) visit(id)
  } else {
    visit(target)
  }
  return selected
}
