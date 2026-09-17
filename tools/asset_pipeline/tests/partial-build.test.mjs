import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import test from 'node:test'
import { createBuildProject, blenderWrapper, runtimeState } from './build_helpers.mjs'
import { runCli } from '../cli.ts'

test('editing walk preserves the model texture and idle bytes', async () => {
  const project = await createBuildProject()
  await project.build()
  const before = await project.manifest()
  // Change an interior key so the fixture's loop remains closed.
  await project.editAction('walk', { bone: 'hand.r', frame: 6, rotationZ: 0.2 })
  await project.build('character/test_actor', { animations: true, clip: 'walk' })
  const after = await project.manifest()
  assert.deepEqual(after.model, before.model)
  assert.deepEqual(after.textures, before.textures)
  assert.deepEqual(after.clips.idle, before.clips.idle)
  assert.notEqual(after.clips.walk.artifact.sha256, before.clips.walk.artifact.sha256)
  assert.equal(after.rigHash, before.rigHash)
  const metadata = JSON.parse(await readFile(join(project.publicRoot, after.metadata.url)))
  assert.deepEqual(Object.keys(metadata.clips).sort(), ['idle', 'walk'])
  assert.equal(after.metrics.animationBytes, after.clips.idle.artifact.bytes + after.clips.walk.artifact.bytes)
})

test('rest, socket, texture, modifier and mesh transform changes reject partial builds atomically', async () => {
  const project = await createBuildProject()
  await project.build()
  const before = await project.catalogBytes()
  const source = await readFile(project.source)
  for (const mutation of ['restEdit', 'socketEdit', 'textureEdit', 'modifierEdit', 'meshTransformEdit']) {
    await project.edit({ [mutation]: true })
    await assert.rejects(project.build('character/test_actor', { animations: true, clip: 'walk' }), /compatibility|model.*changed|full build/i, mutation)
    assert.deepEqual(await project.catalogBytes(), before)
    await writeFile(project.source, source)
  }
})

test('changed exported inverse binds reject even with the same authored bone fingerprint', async () => {
  const project = await createBuildProject()
  await project.build()
  const before = await project.catalogBytes()
  const beforeManifest = await project.manifest()
  const beforeMetadata = JSON.parse(await readFile(join(project.publicRoot, beforeManifest.metadata.url)))
  // Inject a changed skin contract at the exporter boundary. Bone metadata is
  // intentionally unchanged, modeling the weak-fingerprint failure case.
  const blender = await blenderWrapper(project, `if (args.includes('--request')) {
    const path = join(args[args.indexOf('--output') + 1], 'raw/model.glb');
    const bytes = fs.readFileSync(path);
    const jsonSize = bytes.readUInt32LE(12);
    const document = JSON.parse(bytes.subarray(20, 20 + jsonSize));
    const accessor = document.accessors[document.skins[0].inverseBindMatrices];
    const view = document.bufferViews[accessor.bufferView];
    const offset = 28 + jsonSize + (view.byteOffset ?? 0) + (accessor.byteOffset ?? 0);
    bytes.writeFloatLE(bytes.readFloatLE(offset) + 0.125, offset);
    fs.writeFileSync(path, bytes);
  }`)
  const toolPaths = { ...project.toolPaths, blender }
  await assert.rejects(project.build('character/test_actor', { animations: true, clip: 'walk', toolPaths }), /compatibility.*full build/i)
  assert.deepEqual(await project.catalogBytes(), before)
  await project.build('character/test_actor', { toolPaths })
  const after = await project.manifest()
  const metadata = JSON.parse(await readFile(join(project.publicRoot, after.metadata.url)))
  assert.deepEqual(metadata.rig, beforeMetadata.rig)
  assert.notEqual(after.rigHash, beforeManifest.rigHash)
})

test('saved editor state and unrelated action edits preserve model compatibility and untouched clips', async () => {
  const project = await createBuildProject()
  await project.build()
  const before = await project.manifest()
  await project.edit({ savedFrame: 6, savedAction: 'idle', savedSingularPose: true })
  await project.editAction('idle', { bone: 'hand.r', frame: 6, rotationZ: 0.15 })
  await project.build('character/test_actor', { animations: true, clip: 'walk' })
  const after = await project.manifest()
  assert.equal(after.modelInputHash, before.modelInputHash)
  assert.deepEqual(after.clips, before.clips)
  let error = ''
  assert.equal(await runCli(['build', 'character/test_actor', '--animations', '--blender', project.toolPaths.blender,
    '--toktx', project.toolPaths.toktx], { root: project.root, stderr: message => { error += message } }), 0, error)
  assert.notEqual((await project.manifest()).clips.idle.artifact.sha256, before.clips.idle.artifact.sha256)
})

test('partial builds require a prior manifest, existing selected action and compatible tools', async () => {
  const project = await createBuildProject()
  await assert.rejects(project.build('character/test_actor', { animations: true }), /published|prior|full build/i)
  await project.build()
  const before = await project.catalogBytes()
  await assert.rejects(project.build('character/test_actor', { animations: true, clip: 'absent' }), /clip/i)
  const blender = await blenderWrapper(project, '', `if (args.includes('--version')) { console.log('Blender 9.0.0\\n build hash: upgraded\\n build platform: Darwin'); process.exit(0); }`)
  await assert.rejects(project.build('character/test_actor', { animations: true, toolPaths: { ...project.toolPaths, blender } }), /required|compatib/i)
  await project.edit({ removeAction: 'idle' })
  await assert.rejects(project.build('character/test_actor', { animations: true, clip: 'idle' }), /missing action idle/i)
  assert.deepEqual(await project.catalogBytes(), before)
})

test('partial builds verify immutable model and textures before publication', async () => {
  const project = await createBuildProject()
  await project.build()
  const manifest = await project.manifest()
  for (const artifact of [manifest.model, ...manifest.textures]) {
    const path = join(project.publicRoot, artifact.url)
    const original = await readFile(path)
    await writeFile(path, 'corrupted')
    const before = await runtimeState(project.publicRoot)
    await assert.rejects(project.build('character/test_actor', { animations: true }), /mismatch/i)
    assert.deepEqual(await runtimeState(project.publicRoot), before)
    await writeFile(path, original)
  }
})
