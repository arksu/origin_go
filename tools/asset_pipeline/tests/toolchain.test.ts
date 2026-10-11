import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { chmod, mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import {
  inspectToolchain,
  parseBlenderVersionOutput,
  parseToktxVersionOutput,
  type ToolchainLock,
  validateBlenderIdentity,
  validateNodeIdentity,
  validatePlatformIdentity,
} from '../toolchain.ts'

test('accepts Blender 5 and newer without pinning build, Python or exporter identities', () => {
  for (const version of ['5.0.0', '5.2.1', '5.2.2', '6.0.0']) {
    assert.doesNotThrow(() => validateBlenderIdentity({ version, buildHash: 'another-build',
      buildPlatform: 'Darwin', pythonVersion: '3.14.0', ioSceneGltf2TreeSha256: 'another-exporter' }, { minimumMajor: 5 }))
  }
  for (const version of ['4.9.99', 'unknown', '']) {
    assert.throws(() => validateBlenderIdentity({ version, buildHash: 'any' }, { minimumMajor: 5 }), /Blender 5\+ is required/)
  }
})

test('accepts Node 25 and newer and rejects older or malformed versions', () => {
  for (const version of ['v25.0.0', 'v25.8.0', 'v26.10.0', 'v30.0.0']) {
    assert.doesNotThrow(() => validateNodeIdentity({ version }, { minimumMajor: 25 }))
  }
  for (const version of ['v24.99.99', 'unknown', '']) {
    assert.throws(() => validateNodeIdentity({ version }, { minimumMajor: 25 }), /Node 25\+ is required/)
  }
  assert.throws(() => validateNodeIdentity({ version: 'v26.10.0' }, { minimumMajor: NaN }), /Invalid Node minimum/)
})

test('parses the actual Blender version output', () => {
  assert.deepEqual(
    parseBlenderVersionOutput(`Blender 5.2.1 LTS
\tbuild hash: 9e2066aef7ef
\tbuild platform: Darwin
`),
    { version: '5.2.1', buildHash: '9e2066aef7ef', buildPlatform: 'Darwin' },
  )
})

test('parses the official KTX encoder version', () => {
  assert.equal(parseToktxVersionOutput('toktx v4.4.2\n'), '4.4.2')
})

test('rejects a toolchain lock for another platform', () => {
  assert.throws(
    () => validatePlatformIdentity(
      { platform: 'linux', arch: 'arm64' },
      { platform: 'darwin', arch: 'arm64' },
    ),
    /Setup requires darwin\/arm64/,
  )
})

test('inspects tools that report their version on stderr', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'asset-toolchain-'))
  const nodePath = join(directory, 'node')
  const blenderPath = join(directory, 'blender')
  const toktxPath = join(directory, 'toktx')
  await writeFile(nodePath, '#!/bin/sh\necho v26.10.0\n')
  await writeFile(blenderPath, `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'Blender 5.2.2 LTS\\n  build hash: d13f752e3b9c\\n  build platform: Darwin\\n'
else
  echo 'ASSET_PIPELINE_IDENTITY={"pythonVersion":"3.13.13","ioSceneGltf2TreeSha256":"exporter-hash"}'
fi
`)
  await writeFile(toktxPath, '#!/bin/sh\necho "toktx v4.4.2" >&2\n')
  await Promise.all([nodePath, blenderPath, toktxPath].map((path) => chmod(path, 0o755)))
  const binarySha256 = createHash('sha256').update(await readFile(toktxPath)).digest('hex')
  const lock: ToolchainLock = {
    schemaVersion: 1,
    platform: { platform: process.platform, arch: process.arch },
    node: { minimumMajor: 25 },
    blender: { minimumMajor: 5 },
    toktx: {
      version: '4.4.2', binarySha256,
      archive: { url: 'https://example.invalid/ktx.pkg', sha256: 'archive-hash' },
    },
  }

  assert.deepEqual(inspectToolchain({ node: nodePath, blender: blenderPath, toktx: toktxPath }, lock), {
    platform: lock.platform,
    node: { version: 'v26.10.0' },
    blender: { version: '5.2.2', buildHash: 'd13f752e3b9c', buildPlatform: 'Darwin',
      pythonVersion: '3.13.13', ioSceneGltf2TreeSha256: 'exporter-hash' },
    toktx: { version: '4.4.2', binarySha256 },
  })
  await writeFile(nodePath, '#!/bin/sh\necho v24.0.0\n')
  assert.throws(() => inspectToolchain({ node: nodePath, blender: blenderPath, toktx: toktxPath }, lock), /Node 25\+ is required/)
  await writeFile(nodePath, '#!/bin/sh\necho v26.10.0\n')
  await writeFile(blenderPath, (await readFile(blenderPath, 'utf8')).replace('Blender 5.2.2', 'Blender 4.5.0'))
  assert.throws(() => inspectToolchain({ node: nodePath, blender: blenderPath, toktx: toktxPath }, lock), /Blender 5\+ is required/)
})
