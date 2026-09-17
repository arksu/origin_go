import assert from 'node:assert/strict'
import { mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import test from 'node:test'
import { createBuildProject, blenderWrapper, runtimeState } from './build_helpers.mjs'
import { compareBuilds } from '../report.mjs'

test('comparison covers each artifact and deterministic manifest content', async () => {
  const root = await mkdtemp(join(tmpdir(), 'asset-compare-'))
  const original = join(root, 'original'), changed = join(root, 'changed')
  await writeFile(original, 'original runtime bytes')
  await writeFile(changed, 'changed runtime bytes')
  const bundle = { manifest: { id: 'character/test_actor', settings: 1 },
    result: { model: original, metadata: original, clips: { idle: original, walk: original }, textures: [original] } }
  for (const label of ['model', 'metadata', 'clip idle', 'clip walk', 'texture 0', 'manifest']) {
    const other = structuredClone(bundle)
    if (label === 'manifest') other.manifest.settings = 2
    else if (label.startsWith('clip ')) other.result.clips[label.slice(5)] = changed
    else if (label === 'texture 0') other.result.textures[0] = changed
    else other.result[label] = changed
    await assert.rejects(compareBuilds([bundle], [other]), new RegExp(`character/test_actor ${label} differs`))
  }
})

test('reproducibility compares fresh Blender exports and never publishes', async () => {
  const project = await createBuildProject()
  const log = join(project.root, 'workers.jsonl')
  const blender = await blenderWrapper(project, '', `if (args.includes('--request')) {
    const output = args[args.indexOf('--output') + 1];
    if (fs.existsSync(output)) throw new Error('staging was not empty');
    fs.appendFileSync(${JSON.stringify(log)}, JSON.stringify({ output, pid: process.pid }) + '\\n');
  }`)
  const before = await runtimeState(project.publicRoot)
  const result = await project.builder.verifyReproducible({ root: project.root, target: 'character/test_actor', toolPaths: { ...project.toolPaths, blender } })
  assert.deepEqual(result.assets, ['character/test_actor'])
  const workers = (await readFile(log, 'utf8')).trim().split('\n').map(JSON.parse)
  assert.equal(workers.length, 2)
  assert.notEqual(workers[0].pid, workers[1].pid)
  assert.notEqual(workers[0].output, workers[1].output)
  assert.deepEqual(await runtimeState(project.publicRoot), before)
})

test('reproducibility reports nondeterministic runtime metadata and leaves catalog untouched', async () => {
  const project = await createBuildProject()
  await project.build()
  const before = await runtimeState(project.publicRoot)
  const blender = await blenderWrapper(project, `if (args.includes('--request')) {
    const metadata = join(args[args.indexOf('--output') + 1], 'raw/metadata.json');
    const document = JSON.parse(fs.readFileSync(metadata));
    document.nondeterministicFixture = process.pid;
    fs.writeFileSync(metadata, JSON.stringify(document));
  }`)
  await assert.rejects(project.builder.verifyReproducible({ root: project.root, target: 'character/test_actor', toolPaths: { ...project.toolPaths, blender } }), /reproducib.*character\/test_actor.*metadata/i)
  assert.deepEqual(await runtimeState(project.publicRoot), before)
})
