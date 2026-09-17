import assert from 'node:assert/strict'
import { mkdtemp, mkdir, readFile, realpath, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'

import { loadRecipes, selectRecipes } from '../catalog.ts'

const fixturePath = new URL('./fixtures/recipe.yaml', import.meta.url)

async function createCatalogRoot(): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'asset-catalog-'))
  await mkdir(join(root, 'web_new/public'), { recursive: true })
  return root
}

async function writeRecipe(root: string, directory: string, yaml: string, createSource = true): Promise<void> {
  const recipeDirectory = join(root, 'art_source', directory)
  await mkdir(recipeDirectory, { recursive: true })
  await writeFile(join(recipeDirectory, 'asset.yaml'), yaml)
  if (createSource) await writeFile(join(recipeDirectory, 'source.blend'), 'fixture')
}

async function fixtureYaml(): Promise<string> {
  return readFile(fixturePath, 'utf8')
}

test('loads a strict recipe into the typed worker contract', async () => {
  const root = await createCatalogRoot()
  await writeRecipe(root, 'character/test_commoner', await fixtureYaml())

  const recipes = await loadRecipes(root)
  const recipe = recipes.get('character/test_commoner')
  const canonicalRoot = await realpath(root)

  assert.equal(recipe?.schema, 1)
  assert.deepEqual(recipe?.source, {
    relativePath: 'art_source/character/test_commoner/source.blend',
    absolutePath: join(canonicalRoot, 'art_source/character/test_commoner/source.blend'),
  })
  assert.equal(
    recipe?.outputDirectory,
    join(canonicalRoot, 'web_new/public/assets/game/characters/test_commoner'),
  )
  assert.deepEqual(recipe?.dependencies, { export: [], preview: [] })
  assert.deepEqual(recipe?.rig, {
    object: 'Armature', bones: ['root', 'hand_l', 'hand_r'], sockets: ['grip_l', 'grip_r'],
  })
  assert.deepEqual(recipe?.clips.walk, {
    action: 'walk', range: { start: 1, end: 24 }, fps: 24, loop: true,
    playback: 'distance', cycleDistanceTiles: 1.677975879375,
  })
  assert.deepEqual(recipe?.bindings.left_hand, {
    slot: 'left_hand', socket: 'grip_l', grip: 'GRIP_L', policy: { kind: 'ordinary' },
  })
  assert.deepEqual(recipe?.budgets, {
    trianglesByLod: { '0': 16000, '1': 5500 },
    textureDimensions: { width: 1024, height: 1024 },
    totalPublishedBytes: 4194304,
    boneInfluences: 4,
    bones: 64,
  })
  assert.deepEqual(recipe?.optimization, {
    meshCompression: 'meshopt',
    texture: { codec: 'uastc', quality: 2, width: 1024, height: 1024, mipmaps: true },
  })
})

test('rejects duplicate YAML keys before normalization', async () => {
  const root = await createCatalogRoot()
  const yaml = (await fixtureYaml()).replace('kind: character', 'kind: character\nkind: equipment')
  await writeRecipe(root, 'duplicate-key', yaml)

  await assert.rejects(loadRecipes(root), /duplicate key|Map keys must be unique/i)
})

test('rejects named LOD budgets that cannot match numeric node extras', async () => {
  const root = await createCatalogRoot()
  await writeRecipe(root, 'named-lod', (await fixtureYaml()).replace("trianglesByLod: { '0': 16000, '1': 5500 }", 'trianglesByLod: { high: 16000, low: 5500 }'))
  await assert.rejects(loadRecipes(root), /numeric.*LOD|LOD.*numeric/i)
})

test('rejects duplicate recipe IDs and runtime output collisions', async (context) => {
  await context.test('duplicate IDs', async () => {
    const root = await createCatalogRoot()
    const yaml = await fixtureYaml()
    await writeRecipe(root, 'first', yaml)
    await writeRecipe(root, 'second', yaml)
    await assert.rejects(loadRecipes(root), /Duplicate recipe id.*character\/test_commoner/)
  })

  await context.test('runtime output collisions', async () => {
    const root = await createCatalogRoot()
    const yaml = await fixtureYaml()
    await writeRecipe(root, 'first', yaml)
    await writeRecipe(root, 'second', yaml.replace('id: character/test_commoner', 'id: character/other'))
    await assert.rejects(loadRecipes(root), /runtimePath.*collision/i)
  })
})

test('rejects unknown fields including numerical grip transforms', async (context) => {
  await context.test('unknown top-level field', async () => {
    const root = await createCatalogRoot()
    await writeRecipe(root, 'unknown', `${await fixtureYaml()}surprise: true\n`)
    await assert.rejects(loadRecipes(root), /unknown field.*surprise/i)
  })

  await context.test('binding transform', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml()).replace(
      '    policy: { kind: ordinary }',
      '    policy: { kind: ordinary }\n    transform: { position: [0, 0, 0] }',
    )
    await writeRecipe(root, 'transform', yaml)
    await assert.rejects(loadRecipes(root), /unknown field.*transform/i)
  })
})

test('rejects traversal and source symlinks that escape the repository', async (context) => {
  await context.test('parent traversal', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml()).replace('source: source.blend', 'source: ../source.blend')
    await writeRecipe(root, 'traversal', yaml)
    await assert.rejects(loadRecipes(root), /source.*parent traversal/i)
  })

  await context.test('escaping symlink', async () => {
    const root = await createCatalogRoot()
    const outside = await mkdtemp(join(tmpdir(), 'asset-source-outside-'))
    const source = join(outside, 'source.blend')
    await writeFile(source, 'outside')
    await writeRecipe(root, 'symlink', await fixtureYaml(), false)
    await symlink(source, join(root, 'art_source/symlink/source.blend'))
    await assert.rejects(loadRecipes(root), /source.*escapes.*root/i)
  })

  await context.test('non-Blender source payload', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml()).replace('source: source.blend', 'source: payload.txt')
    await writeRecipe(root, 'source-extension', yaml, false)
    await writeFile(join(root, 'art_source/source-extension/payload.txt'), 'not a blend')
    await assert.rejects(loadRecipes(root), /source.*\.blend/i)
  })

  await context.test('runtime path outside the game asset namespace', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml()).replace(
      '/assets/game/characters/test_commoner',
      '/uploads/characters/test_commoner',
    )
    await writeRecipe(root, 'runtime-namespace', yaml)
    await assert.rejects(loadRecipes(root), /runtimePath.*\/assets\/game\//i)
  })

  await context.test('runtime output symlink escape', async () => {
    const root = await createCatalogRoot()
    const outside = await mkdtemp(join(tmpdir(), 'asset-output-outside-'))
    await symlink(outside, join(root, 'web_new/public/assets'))
    await writeRecipe(root, 'runtime-symlink', await fixtureYaml())
    await assert.rejects(loadRecipes(root), /runtimePath.*escapes.*public/i)
  })

  await context.test('dangling runtime output symlink', async () => {
    const root = await createCatalogRoot()
    const outside = await mkdtemp(join(tmpdir(), 'asset-output-dangling-'))
    await symlink(join(outside, 'missing'), join(root, 'web_new/public/assets'))
    await writeRecipe(root, 'runtime-dangling-symlink', await fixtureYaml())
    await assert.rejects(loadRecipes(root), /runtimePath.*dangling symlink/i)
  })
})

test('rejects unsupported kinds, invalid ranges, and incomplete positive budgets', async (context) => {
  await context.test('unsupported kind', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml())
      .replace('id: character/test_commoner', 'id: vehicle/cart')
      .replace('kind: character', 'kind: vehicle')
    await writeRecipe(root, 'kind', yaml)
    await assert.rejects(loadRecipes(root), /kind.*character.*equipment.*world_object/i)
  })

  await context.test('reversed clip range', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml()).replace(
      'range: { start: 1, end: 24 }',
      'range: { start: 24, end: 1 }',
    )
    await writeRecipe(root, 'clip-range', yaml)
    await assert.rejects(loadRecipes(root), /range.*end must not precede start/i)
  })

  await context.test('non-finite clip FPS', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml()).replace('fps: 24', 'fps: .nan')
    await writeRecipe(root, 'clip-fps', yaml)
    await assert.rejects(loadRecipes(root), /clips\.idle\.fps must be finite/i)
  })

  await context.test('zero triangle budget', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml()).replace("'0': 16000", "'0': 0")
    await writeRecipe(root, 'budget', yaml)
    await assert.rejects(loadRecipes(root), /trianglesByLod\.0.*positive/i)
  })

  await context.test('out-of-range UASTC quality', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml()).replace('quality: 2', 'quality: 5')
    await writeRecipe(root, 'quality', yaml)
    await assert.rejects(loadRecipes(root), /uastc quality.*0.*4/i)
  })
})

test('validates missing and cyclic dependencies while keeping preview out of build selection', async (context) => {
  await context.test('missing dependency', async () => {
    const root = await createCatalogRoot()
    const yaml = (await fixtureYaml()).replace('export: []', 'export: [equipment/missing]')
    await writeRecipe(root, 'missing', yaml)
    await assert.rejects(loadRecipes(root), /missing dependency.*equipment\/missing/i)
  })

  await context.test('export cycle', async () => {
    const root = await createCatalogRoot()
    const first = (await fixtureYaml())
      .replace('id: character/test_commoner', 'id: character/first')
      .replace('export: []', 'export: [equipment/second]')
    const second = (await fixtureYaml())
      .replace('id: character/test_commoner', 'id: equipment/second')
      .replace('kind: character', 'kind: equipment')
      .replace('/assets/game/characters/test_commoner', '/assets/game/equipment/second')
      .replace('export: []', 'export: [character/first]')
      .replace(/rig:\n(?:  .*\n){3}/, 'rig: null\n')
      .replace(/clips:\n(?:  .*\n|    .*\n)+?bindings:/, 'clips: {}\nbindings:')
    await writeRecipe(root, 'first', first)
    await writeRecipe(root, 'second', second)
    await assert.rejects(loadRecipes(root), /cyclic dependencies/i)
  })

  await context.test('preview cycle', async () => {
    const root = await createCatalogRoot()
    const first = (await fixtureYaml())
      .replace('id: character/test_commoner', 'id: character/first')
      .replace('preview: []', 'preview: [equipment/second]')
    const second = (await fixtureYaml())
      .replace('id: character/test_commoner', 'id: equipment/second')
      .replace('kind: character', 'kind: equipment')
      .replace('/assets/game/characters/test_commoner', '/assets/game/equipment/second')
      .replace('preview: []', 'preview: [character/first]')
      .replace(/rig:\n(?:  .*\n){3}/, 'rig: null\n')
      .replace(/clips:\n(?:  .*\n|    .*\n)+?bindings:/, 'clips: {}\nbindings:')
    await writeRecipe(root, 'first', first)
    await writeRecipe(root, 'second', second)
    await assert.rejects(loadRecipes(root), /cyclic dependencies/i)
  })

  await context.test('mixed export and preview cycle', async () => {
    const root = await createCatalogRoot()
    const first = (await fixtureYaml())
      .replace('id: character/test_commoner', 'id: character/first')
      .replace('export: []', 'export: [equipment/second]')
    const second = (await fixtureYaml())
      .replace('id: character/test_commoner', 'id: equipment/second')
      .replace('kind: character', 'kind: equipment')
      .replace('/assets/game/characters/test_commoner', '/assets/game/equipment/second')
      .replace('preview: []', 'preview: [character/first]')
      .replace(/rig:\n(?:  .*\n){3}/, 'rig: null\n')
      .replace(/clips:\n(?:  .*\n|    .*\n)+?bindings:/, 'clips: {}\nbindings:')
    await writeRecipe(root, 'first', first)
    await writeRecipe(root, 'second', second)
    await assert.rejects(loadRecipes(root), /cyclic dependencies/i)
  })
})

test('selects export dependencies in stable topological order', async () => {
  const root = await createCatalogRoot()
  const character = (await fixtureYaml())
    .replace('id: character/test_commoner', 'id: character/hero')
    .replace('export: []', 'export: [equipment/sword, world_object/shadow]')
  const equipment = (await fixtureYaml())
    .replace('id: character/test_commoner', 'id: equipment/sword')
    .replace('kind: character', 'kind: equipment')
    .replace('/assets/game/characters/test_commoner', '/assets/game/equipment/sword')
    .replace(/rig:\n(?:  .*\n){3}/, 'rig: null\n')
    .replace(/clips:\n(?:  .*\n|    .*\n)+?bindings:/, 'clips: {}\nbindings:')
  const worldObject = equipment
    .replace('id: equipment/sword', 'id: world_object/shadow')
    .replace('kind: equipment', 'kind: world_object')
    .replace('/assets/game/equipment/sword', '/assets/game/world_objects/shadow')
  await writeRecipe(root, 'hero', character)
  await writeRecipe(root, 'sword', equipment)
  await writeRecipe(root, 'shadow', worldObject)

  const recipes = await loadRecipes(root)
  assert.deepEqual(selectRecipes(recipes, 'character/hero').map((recipe) => recipe.id), [
    'equipment/sword', 'world_object/shadow', 'character/hero',
  ])
  assert.deepEqual(selectRecipes(recipes, 'all').map((recipe) => recipe.id), [
    'equipment/sword', 'world_object/shadow', 'character/hero',
  ])
  assert.throws(() => selectRecipes(recipes, 'character/missing'), /Unknown asset target/)
})
