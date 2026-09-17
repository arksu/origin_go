import { build } from 'esbuild'
import { mkdtemp, mkdir, rm } from 'node:fs/promises'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'

const root = fileURLToPath(new URL('..', import.meta.url))
await mkdir(join(root, 'node_modules/.tmp'), { recursive: true })
const directory = await mkdtemp(join(root, 'node_modules/.tmp/asset-pipeline-'))
try {
  const outfile = join(directory, 'tests.mjs')
  await build({ entryPoints: [join(root, 'tests/asset-pipeline.test.ts')], outfile,
    bundle: true, platform: 'node', format: 'esm', external: ['three', 'three/*'],
    define: { ASSET_TEST_ROOT: JSON.stringify(root) } })
  const result = spawnSync(process.execPath, ['--test', outfile], { stdio: 'inherit' })
  if (result.error) throw result.error
  process.exitCode = result.status ?? 1
} finally {
  await rm(directory, { recursive: true, force: true })
}
