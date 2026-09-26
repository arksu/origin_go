import { build } from 'esbuild'
import { mkdtemp, mkdir, rm } from 'node:fs/promises'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'

const root = fileURLToPath(new URL('..', import.meta.url))
const temporaryRoot = join(root, 'node_modules', '.tmp')
await mkdir(temporaryRoot, { recursive: true })
const directory = await mkdtemp(join(temporaryRoot, 'chunks-'))
try {
  const outfile = join(directory, 'tests.mjs')
  await build({
    entryPoints: [join(root, 'tests/chunks.test.ts')], outfile,
    bundle: true, platform: 'node', format: 'esm', external: ['three', 'three/*', 'vue', 'pinia'],
    banner: { js: 'import { createRequire } from "node:module"; const require = createRequire(import.meta.url); globalThis.requestAnimationFrame = () => 0; globalThis.cancelAnimationFrame = () => {};' },
    define: { 'import.meta.env': '{}', '__APP_VERSION__': '"test"', '__BUILD_TIME__': '"test"', '__COMMIT_HASH__': '"test"' },
    alias: { '@': join(root, 'src') }, sourcemap: 'inline',
  })
  const result = spawnSync(process.execPath, ['--test', outfile], { stdio: 'inherit' })
  if (result.error) throw result.error
  process.exitCode = result.status ?? 1
} finally {
  await rm(directory, { recursive: true, force: true })
}
