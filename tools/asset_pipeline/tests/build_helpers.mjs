import { chmod, copyFile, mkdir, readFile, readdir, stat, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { createFixture, blenderPath, pipelineDirectory } from './blender_helpers.ts'
import * as builder from '../build.mjs'
import { readArtifact } from '../publish.mjs'

export async function runtimeState(directory) {
  const entries = {}
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    entries[entry.name] = entry.isDirectory() ? await runtimeState(path) : {
      bytes: await readFile(path), modified: (await stat(path)).mtimeMs,
    }
  }
  return entries
}

export async function createBuildProject() {
  const fixture = await createFixture('character', { packedTexture: true })
  const { root } = fixture
  const publicRoot = join(root, 'web_new/public')
  const directory = join(root, 'art_source/character/test_actor')
  await mkdir(publicRoot, { recursive: true })
  await mkdir(directory, { recursive: true })
  const source = join(directory, 'source.blend')
  await copyFile(fixture.source, source)
  const { outputDirectory, ...recipe } = fixture.recipe
  const recipePath = join(directory, 'asset.yaml')
  await writeFile(recipePath, JSON.stringify({ ...recipe, id: 'character/test_actor',
    source: 'source.blend', runtimePath: '/assets/game/character/test_actor' }))
  const toolPaths = { blender: blenderPath, node: process.execPath,
    toktx: join(pipelineDirectory, '.tools/ktx-4.4.2/install/usr/local/bin/toktx') }
  return { root, publicRoot, source, recipePath, toolPaths, builder,
    build: (target = 'character/test_actor', options = {}) => builder.buildAssets({ root, target, toolPaths, ...options }),
    editAction: async (...args) => { await copyFile(source, fixture.source); await fixture.editAction(...args); await copyFile(fixture.source, source) },
    edit: async options => { await copyFile(source, fixture.source); await fixture.edit(options); await copyFile(fixture.source, source) },
    catalogBytes: () => readFile(join(publicRoot, 'assets/game/asset-catalog.json')),
    manifest: async (id = 'character/test_actor') => {
      const catalog = JSON.parse(await readFile(join(publicRoot, 'assets/game/asset-catalog.json')))
      return JSON.parse(await readArtifact(publicRoot, catalog.assets[id]))
    } }
}

export async function blenderWrapper(project, after = '', before = '') {
  const path = join(project.root, 'blender-wrapper.mjs')
  await writeFile(path, `#!${process.execPath}\nimport { spawnSync } from 'node:child_process';\nimport * as fs from 'node:fs';\nimport { join } from 'node:path';\nconst args = process.argv.slice(2);\n${before}\nconst result = spawnSync(${JSON.stringify(blenderPath)}, args, { stdio: 'inherit' });\nif (result.status !== 0) process.exit(result.status ?? 1);\n${after}\n`)
  await chmod(path, 0o755)
  return path
}
