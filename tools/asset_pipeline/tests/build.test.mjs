import assert from 'node:assert/strict'
import { chmod, copyFile, lstat, mkdir, mkdtemp, readFile, readdir, realpath, stat, symlink, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'
import test from 'node:test'
import { createFixture, blenderPath, pipelineDirectory } from './blender_helpers.ts'
import { runCli } from '../cli.ts'

const toolPaths = { blender: blenderPath, node: process.execPath,
  toktx: join(pipelineDirectory, '.tools/ktx-4.4.2/install/usr/local/bin/toktx') }

async function runtimeState(directory) {
  const entries = {}
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    entries[entry.name] = entry.isDirectory() ? await runtimeState(path) : {
      bytes: await readFile(path), modified: (await stat(path)).mtimeMs,
    }
  }
  return entries
}

async function createBuildProject() {
  const fixture = await createFixture('equipment', { packedTexture: true })
  const root = fixture.root
  await mkdir(join(root, 'web_new/public'), { recursive: true })
  await mkdir(join(root, 'data/action_animations'), { recursive: true })
  await writeFile(join(root, 'data/action_animations/empty.json'), JSON.stringify({ v: 1, bindings: [] }))
  for (const name of ['test_axe', 'test_other']) {
    const directory = join(root, 'art_source/equipment', name)
    await mkdir(directory, { recursive: true })
    await copyFile(fixture.source, join(directory, 'source.blend'))
    const { outputDirectory, equipmentSlots, ...recipe } = fixture.recipe
    await writeFile(join(directory, 'asset.yaml'), JSON.stringify({ ...recipe, id: `equipment/${name}`,
      source: 'source.blend', runtimePath: `/assets/game/equipment/${name}` }))
  }
  const builder = await import('../build.mjs')
  return { root, fixture, builder, toolPaths,
    build: target => builder.buildAssets({ root, target, animations: false, toolPaths }),
    catalogBytes: () => readFile(join(root, 'web_new/public/assets/game/asset-catalog.json')),
    source: name => join(root, `art_source/equipment/${name}/source.blend`),
    recipe: name => join(root, `art_source/equipment/${name}/asset.yaml`) }
}

async function stagingDirectories(root) {
  return (await readdir(join(root, 'build/asset-pipeline'), { withFileTypes: true }))
    .filter(entry => entry.isDirectory() && entry.name.startsWith('stage-')).map(entry => entry.name).sort()
}

async function createScratchProject() {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'asset-scratch-test-')))
  const publicRoot = join(root, 'web_new/public')
  const sourceDirectory = join(root, 'art_source/character/test_commoner')
  await mkdir(join(publicRoot, 'assets/game'), { recursive: true })
  await mkdir(sourceDirectory, { recursive: true })
  await copyFile(new URL('./fixtures/recipe.yaml', import.meta.url), join(sourceDirectory, 'asset.yaml'))
  await writeFile(join(sourceDirectory, 'source.blend'), 'preflight fails before reading Blender content')
  await mkdir(join(root, 'data/action_animations'), { recursive: true })
  await writeFile(join(root, 'data/action_animations/empty.json'), JSON.stringify({ v: 1, bindings: [] }))
  const builder = await import('../build.mjs')
  const options = { root, target: 'all', toolPaths: { blender: join(root, 'missing-blender') } }
  return { root, publicRoot, builder, options }
}

test('each operation cleans prior staging and retains its current failure diagnostics', async () => {
  const project = await createScratchProject()
  const parent = join(project.root, 'build/asset-pipeline')
  await mkdir(join(parent, 'stage-prior'), { recursive: true })
  await writeFile(join(parent, 'stage-prior/previous.log'), 'old failure')
  await mkdir(join(parent, 'keep'))
  await writeFile(join(parent, 'keep/notes.txt'), 'unrelated')
  await writeFile(join(parent, 'stage-file'), 'not a directory')
  const linked = await realpath(await mkdtemp(join(tmpdir(), 'asset-scratch-linked-')))
  await writeFile(join(linked, 'keep.txt'), 'linked content')
  await symlink(linked, join(parent, 'stage-linked'))
  let previous = 'stage-prior'
  for (const operation of ['buildAssets', 'validateAssets', 'verifyReproducible']) {
    await assert.rejects(project.builder[operation](project.options), /preflight.*failed/)
    await assert.rejects(lstat(join(parent, previous)), { code: 'ENOENT' })
    const stages = await stagingDirectories(project.root)
    assert.equal(stages.length, 1, `${operation} retains only the current failed stage`)
    previous = stages[0]
    const log = join(parent, previous, 'all-preflight.log')
    assert.match(await readFile(log, 'utf8'), /Unable to inspect|Setup requires/)
    assert.equal(await readFile(join(parent, 'keep/notes.txt'), 'utf8'), 'unrelated')
    assert.equal(await readFile(join(parent, 'stage-file'), 'utf8'), 'not a directory')
    assert.equal((await lstat(join(parent, 'stage-linked'))).isSymbolicLink(), true)
    assert.equal(await readFile(join(linked, 'keep.txt'), 'utf8'), 'linked content')
  }
})

test('all staging operations wait for the shared lock before cleanup', async () => {
  const { withPublishLock } = await import('../publish.mjs')
  for (const operation of ['buildAssets', 'validateAssets', 'verifyReproducible']) {
    const project = await createScratchProject()
    const active = join(project.root, 'build/asset-pipeline/stage-active')
    await mkdir(active, { recursive: true })
    await writeFile(join(active, 'active.log'), 'active work')
    const entered = Promise.withResolvers(), release = Promise.withResolvers()
    const owner = withPublishLock(project.publicRoot, async () => {
      entered.resolve()
      await release.promise
    })
    await entered.promise
    let settled = false
    const waiting = project.builder[operation](project.options)
      .then(value => ({ value }), error => ({ error })).finally(() => { settled = true })
    try {
      await delay(60)
      assert.equal(settled, false, operation)
      assert.equal(await readFile(join(active, 'active.log'), 'utf8'), 'active work')
    } finally {
      release.resolve()
    }
    await owner
    assert.match((await waiting).error.message, /preflight.*failed/)
    await assert.rejects(lstat(active), { code: 'ENOENT' })
  }
})

test('staging cleanup rejects symlinked scratch parents and ancestors', async () => {
  for (const linkedPart of ['build', 'asset-pipeline']) {
    const project = await createScratchProject()
    const external = await realpath(await mkdtemp(join(tmpdir(), 'asset-scratch-external-')))
    const protectedStage = linkedPart === 'build' ? join(external, 'asset-pipeline/stage-protected') : join(external, 'stage-protected')
    await mkdir(protectedStage, { recursive: true })
    await writeFile(join(protectedStage, 'failure.log'), 'preserve outside files')
    if (linkedPart === 'asset-pipeline') await mkdir(join(project.root, 'build'))
    await symlink(external, linkedPart === 'build' ? join(project.root, 'build') : join(project.root, 'build/asset-pipeline'))
    for (const operation of ['buildAssets', 'validateAssets', 'verifyReproducible']) {
      await assert.rejects(project.builder[operation](project.options), /Staging path.*symlink/)
      assert.equal(await readFile(join(protectedStage, 'failure.log'), 'utf8'), 'preserve outside files')
    }
  }
})

test('real builds publish both assets, offline validation writes nothing, and failed source preserves catalog', async () => {
  const project = await createBuildProject()
  await project.build('all')
  assert.deepEqual(await stagingDirectories(project.root), [])
  const before = await project.catalogBytes()
  assert.deepEqual(Object.keys(JSON.parse(before).assets).sort(), ['equipment/test_axe', 'equipment/test_other'])
  const publicRoot = join(project.root, 'web_new/public')
  const { readArtifact } = await import('../publish.mjs')
  for (const artifact of Object.values(JSON.parse(before).assets)) {
    const manifest = JSON.parse(await readArtifact(publicRoot, artifact))
    await readArtifact(publicRoot, manifest.model)
    for (const texture of manifest.textures) await readArtifact(publicRoot, texture)
  }
  let error = ''
  const runtimeBefore = await runtimeState(publicRoot)
  const status = await runCli(['validate', 'all', '--blender', blenderPath, '--toktx', toolPaths.toktx], {
    root: project.root, stdout: () => {}, stderr: message => { error += message } })
  assert.equal(status, 0, error)
  assert.deepEqual(await project.catalogBytes(), before)
  assert.deepEqual(await runtimeState(publicRoot), runtimeBefore)
  assert.deepEqual(await stagingDirectories(project.root), [])
  const bad = await createFixture('equipment', { missingExport: true })
  await copyFile(bad.source, project.source('test_axe'))
  await assert.rejects(project.build('all'), /equipment\/test_axe.*export[\s\S]*EXPORT[\s\S]*log:/)
  assert.deepEqual(await project.catalogBytes(), before)
  for (const artifact of Object.values(JSON.parse(before).assets)) await readArtifact(publicRoot, artifact)
})

test('concurrent real single-asset builds retain both entries', async () => {
  const project = await createBuildProject()
  let error = ''
  const [, status] = await Promise.all([project.build('equipment/test_axe'),
    runCli(['build', 'equipment/test_other', '--blender', blenderPath, '--toktx', toolPaths.toktx], {
      root: project.root, stdout: () => {}, stderr: message => { error += message },
    })])
  assert.equal(status, 0, error)
  assert.deepEqual(Object.keys(JSON.parse(await project.catalogBytes()).assets).sort(), ['equipment/test_axe', 'equipment/test_other'])
})

test('CLI reports the source, model, textures, clips, stages, and publication progress', async () => {
  const project = await createBuildProject()
  const messages = []
  const status = await runCli(['build', 'equipment/test_axe', '--blender', blenderPath, '--toktx', toolPaths.toktx], {
    root: project.root,
    stdout: () => {},
    stderr: message => { messages.push(message) },
  })
  assert.equal(status, 0)
  const output = messages.join('\n')
  assert.match(output, /\[assets\] target equipment\/test_axe/)
  assert.match(output, /\[assets\] source .*source\.blend/)
  assert.match(output, /\[assets\] equipment\/test_axe export model .*model\.glb/)
  assert.match(output, /\[assets\] equipment\/test_axe optimize texture .*\.png/)
  assert.match(output, /\[assets\] equipment\/test_axe validation complete/)
  assert.match(output, /\[assets\] publication .*asset-catalog\.json/)
})

async function blenderWrapper(project, body) {
  const path = join(project.root, 'blender-wrapper.mjs')
  await writeFile(path, `#!${process.execPath}\nimport { spawnSync } from 'node:child_process';\nimport { appendFileSync } from 'node:fs';\nconst args = process.argv.slice(2);\n${body}\nconst result = spawnSync(${JSON.stringify(blenderPath)}, args, { stdio: 'inherit' });\nprocess.exit(result.status ?? 1);\n`)
  await chmod(path, 0o755)
  return path
}

test('subprocess failures preserve catalog and capture asset, stage and a readable log', async () => {
  const project = await createBuildProject()
  await project.build('all')
  const before = await project.catalogBytes()
  const blender = await blenderWrapper(project, `if (args.includes('--request')) { console.error('deliberate worker exit'); process.exit(17); }`)
  let logPath
  await assert.rejects(project.builder.buildAssets({ root: project.root, target: 'equipment/test_axe', toolPaths: { ...toolPaths, blender } }), asyncError => {
    assert.match(asyncError.message, /equipment\/test_axe.*export[\s\S]*deliberate worker exit[\s\S]*log:/)
    logPath = asyncError.message.split('; log: ').at(-1)
    return true
  })
  assert.match(await readFile(logPath, 'utf8'), /deliberate worker exit/)
  assert.deepEqual(await project.catalogBytes(), before)
})

test('input and preview dependency mutations during raw export reject the whole build', async () => {
  const project = await createBuildProject()
  const recipe = JSON.parse(await readFile(project.recipe('test_axe'), 'utf8'))
  recipe.dependencies.preview = ['equipment/test_other']
  await writeFile(project.recipe('test_axe'), JSON.stringify(recipe))
  await project.build('all')
  const before = await project.catalogBytes()
  for (const name of ['test_axe', 'test_other']) {
    const original = await readFile(project.source(name))
    const blender = await blenderWrapper(project,
      `if (args.includes('--request')) appendFileSync(${JSON.stringify(project.source(name))}, 'changed during export');`)
    await assert.rejects(project.builder.buildAssets({ root: project.root, target: 'equipment/test_axe', toolPaths: { ...toolPaths, blender } }), /input.*changed/i)
    assert.deepEqual(await project.catalogBytes(), before)
    await writeFile(project.source(name), original)
  }
})

test('validation rejects changed source and corrupt published files without switching catalog', async () => {
  const project = await createBuildProject()
  await project.build('all')
  const before = await project.catalogBytes()
  const source = await readFile(project.source('test_axe'))
  await writeFile(project.source('test_axe'), Buffer.concat([source, Buffer.from('changed')]))
  await assert.rejects(project.builder.validateAssets({ root: project.root, target: 'all', toolPaths }), /source|input/i)
  await writeFile(project.source('test_axe'), source)
  const entry = JSON.parse(before).assets['equipment/test_axe']
  const manifest = JSON.parse(await readFile(join(project.root, 'web_new/public', entry.url), 'utf8'))
  await writeFile(join(project.root, 'web_new/public', manifest.model.url), 'corruption')
  await assert.rejects(project.builder.validateAssets({ root: project.root, target: 'all', toolPaths }), /mismatch/i)
  assert.deepEqual(await project.catalogBytes(), before)
})
