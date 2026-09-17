import { copyFile, mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { canonicalJSON, sha256 } from './report.mjs'

const client = fileURLToPath(new URL('../../web_new/', import.meta.url))
export async function installDecoders(destination = join(client, 'public/assets/game/decoders')) {
  const lockBytes = await readFile(join(client, 'package-lock.json'))
  const lock = JSON.parse(lockBytes)
  const dependency = lock.packages['node_modules/three']
  const packageRoot = join(client, 'node_modules/three')
  const installed = JSON.parse(await readFile(join(packageRoot, 'package.json')))
  if (installed.version !== dependency.version) throw new Error('Decoder source must match locked Three package')
  const files = []
  for (const [source, file] of [
    ['examples/jsm/libs/basis/basis_transcoder.js', 'basis/basis_transcoder.js'],
    ['examples/jsm/libs/basis/basis_transcoder.wasm', 'basis/basis_transcoder.wasm'],
    ['examples/jsm/libs/basis/README.md', 'basis/README.md'],
    ['examples/jsm/libs/meshopt_decoder.module.js', 'meshopt_decoder.module.js'],
    ['LICENSE', 'THREE-LICENSE.txt'],
  ]) {
    await mkdir(dirname(join(destination, file)), { recursive: true })
    await copyFile(join(packageRoot, source), join(destination, file))
    files.push({ source, file, sha256: sha256(await readFile(join(destination, file))) })
  }
  const manifest = { schema: 1, package: 'three', version: dependency.version, resolved: dependency.resolved,
    integrity: dependency.integrity, packageLockSha256: sha256(lockBytes), files }
  await writeFile(join(destination, 'provenance.json'), canonicalJSON(manifest) + '\n')
  return manifest
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  console.log(canonicalJSON(await installDecoders()))
}
