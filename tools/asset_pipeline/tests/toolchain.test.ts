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
  validatePlatformIdentity,
} from '../toolchain.ts'

test('rejects a Blender patch release mismatch', () => {
  assert.throws(
    () => validateBlenderIdentity(
      { version: '5.2.0', buildHash: '9e2066aef7ef' },
      { version: '5.2.1', buildHash: '9e2066aef7ef' },
    ),
    /5\.2\.1/,
  )
})

test('parses the pinned Blender version output', () => {
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
  await writeFile(nodePath, '#!/bin/sh\necho v25.8.0\n')
  await writeFile(blenderPath, `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'Blender 5.2.1 LTS\\n  build hash: 9e2066aef7ef\\n  build platform: Darwin\\n'
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
    node: { version: 'v25.8.0' },
    blender: {
      version: '5.2.1', buildHash: '9e2066aef7ef', buildPlatform: 'Darwin',
      pythonVersion: '3.13.13', ioSceneGltf2TreeSha256: 'exporter-hash',
    },
    toktx: {
      version: '4.4.2', binarySha256,
      archive: { url: 'https://example.invalid/ktx.pkg', sha256: 'archive-hash' },
    },
  }

  assert.deepEqual(inspectToolchain({ node: nodePath, blender: blenderPath, toktx: toktxPath }, lock), {
    platform: lock.platform,
    node: lock.node,
    blender: lock.blender,
    toktx: { version: '4.4.2', binarySha256 },
  })
})
