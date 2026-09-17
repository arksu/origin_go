import { createHash } from 'node:crypto'
import { chmod, cp, mkdir, mkdtemp, readFile, readdir, rename, rm, stat } from 'node:fs/promises'
import { spawn } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const pipelineDirectory = fileURLToPath(new URL('./', import.meta.url))

function run(command, arguments_, label) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, arguments_, { stdio: ['ignore', 'pipe', 'pipe'] })
    let stdout = '', stderr = ''
    child.stdout.on('data', chunk => { stdout += chunk })
    child.stderr.on('data', chunk => { stderr += chunk })
    child.on('error', error => reject(new Error(`${label}: ${error.message}`)))
    child.on('close', code => {
      if (code === 0) resolve(stdout || stderr)
      else reject(new Error(`${label} exited ${code}: ${stderr || stdout}`.trim()))
    })
  })
}

function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex')
}

async function findToktx(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) {
      const result = await findToktx(path)
      if (result) return result
    } else if (entry.name === 'toktx' && path.endsWith('/usr/local/bin/toktx')) return path
  }
  return undefined
}

async function findLibrary(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) {
      const result = await findLibrary(path)
      if (result) return result
    } else if (entry.name === 'libktx.4.dylib') return path
  }
  return undefined
}

async function validToktx(path, lock) {
  try {
    const info = await stat(path)
    if (!info.isFile() || sha256(await readFile(path)) !== lock.toktx.binarySha256) return false
    const version = await run(path, ['--version'], 'Inspecting cached toktx')
    return new RegExp(`^toktx\\s+v${lock.toktx.version.replaceAll('.', '\\.')}\\s*$`, 'm').test(version)
  } catch {
    return false
  }
}

export async function setupAssetTools() {
  const lock = JSON.parse(await readFile(join(pipelineDirectory, 'toolchain.lock.json'), 'utf8'))
  if (process.platform !== lock.platform.platform || process.arch !== lock.platform.arch) {
    throw new Error(`Locked KTX setup supports ${lock.platform.platform}/${lock.platform.arch}; found ${process.platform}/${process.arch}`)
  }
  const toolsDirectory = join(pipelineDirectory, '.tools')
  const installDirectory = join(toolsDirectory, `ktx-${lock.toktx.version}`, 'install')
  const binary = join(installDirectory, 'usr/local/bin/toktx')
  if (await validToktx(binary, lock)) {
    return { binary, installed: false }
  }

  await mkdir(toolsDirectory, { recursive: true })
  const staging = await mkdtemp(join(toolsDirectory, '.setup-'))
  try {
    const archive = join(staging, 'ktx.pkg')
    await run('curl', ['--fail', '--location', '--retry', '2', '--output', archive, lock.toktx.archive.url], 'Downloading locked KTX-Software archive')
    if (sha256(await readFile(archive)) !== lock.toktx.archive.sha256) {
      throw new Error('Downloaded KTX-Software archive SHA-256 does not match toolchain.lock.json')
    }
    const expanded = join(staging, 'expanded')
    await run('pkgutil', ['--expand-full', archive, expanded], 'Expanding locked KTX-Software archive')
    const extracted = await findToktx(expanded)
    const library = await findLibrary(expanded)
    if (!extracted || !library) throw new Error('Locked KTX-Software archive does not contain both toktx and libktx.4.dylib')
    const candidateInstall = join(staging, 'install')
    const candidateBinary = join(candidateInstall, 'usr/local/bin/toktx')
    // toktx uses @rpath/libktx.4.dylib. Keep the package's bin/lib layout so
    // the executable remains relocatable inside the project-local tool cache.
    await cp(dirname(dirname(extracted)), join(candidateInstall, 'usr/local'), { recursive: true })
    await cp(dirname(library), join(candidateInstall, 'usr/local/lib'), { recursive: true, dereference: true })
    await chmod(candidateBinary, 0o755)
    if (!(await validToktx(candidateBinary, lock))) throw new Error('Extracted toktx does not match toolchain.lock.json')
    await rm(installDirectory, { recursive: true, force: true })
    await mkdir(dirname(installDirectory), { recursive: true })
    await rename(candidateInstall, installDirectory)
    return { binary, installed: true }
  } finally {
    await rm(staging, { recursive: true, force: true })
  }
}
