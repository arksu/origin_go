import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'

export interface BlenderIdentity {
  version: string
  buildHash: string
  buildPlatform?: string
  pythonVersion?: string
  ioSceneGltf2TreeSha256?: string
}

export interface PlatformIdentity {
  platform: NodeJS.Platform
  arch: NodeJS.Architecture
}

export interface NodeIdentity {
  version: string
}

export interface ToktxIdentity {
  version: string
  binarySha256: string
}

export interface ToolchainIdentity {
  platform: PlatformIdentity
  node: NodeIdentity
  blender: Required<BlenderIdentity>
  toktx: ToktxIdentity
}

export interface ToolchainLock extends ToolchainIdentity {
  schemaVersion: 1
  toktx: ToktxIdentity & {
    archive: {
      url: string
      sha256: string
    }
  }
}

export interface ToolPaths {
  blender: string
  node: string
  toktx: string
}

export function validateBlenderIdentity(actual: BlenderIdentity, expected: BlenderIdentity): void {
  if (actual.version !== expected.version) {
    throw new Error(`Blender ${expected.version} is required; found ${actual.version}`)
  }

  if (actual.buildHash !== expected.buildHash) {
    throw new Error(`Blender build ${expected.buildHash} is required; found ${actual.buildHash}`)
  }

  for (const field of ['buildPlatform', 'pythonVersion', 'ioSceneGltf2TreeSha256'] as const) {
    if (expected[field] !== undefined && actual[field] !== expected[field]) {
      throw new Error(`Blender ${field} ${expected[field]} is required; found ${actual[field] ?? 'unknown'}`)
    }
  }
}

export function validatePlatformIdentity(actual: PlatformIdentity, expected: PlatformIdentity): void {
  if (actual.platform !== expected.platform || actual.arch !== expected.arch) {
    throw new Error(
      `Setup requires ${expected.platform}/${expected.arch}; found ${actual.platform}/${actual.arch}. `
      + 'Install the locked tools for this platform and create a reviewed lock entry.',
    )
  }
}

export function parseBlenderVersionOutput(output: string): Pick<Required<BlenderIdentity>, 'version' | 'buildHash' | 'buildPlatform'> {
  const version = output.match(/^Blender\s+(\d+\.\d+\.\d+)/m)?.[1]
  const buildHash = output.match(/^\s*build hash:\s*(\S+)/m)?.[1]
  const buildPlatform = output.match(/^\s*build platform:\s*(\S+)/m)?.[1]
  if (!version || !buildHash || !buildPlatform) {
    throw new Error('Unable to identify Blender. Run the configured Blender executable with --version and verify the installation.')
  }
  return { version, buildHash, buildPlatform }
}

export function parseToktxVersionOutput(output: string): string {
  const version = output.match(/^toktx\s+v(\d+\.\d+\.\d+)\s*$/m)?.[1]
  if (!version) {
    throw new Error('Unable to identify toktx. Install the official locked KTX-Software package and pass --toktx.')
  }
  return version
}

function sha256File(path: string): string {
  return createHash('sha256').update(readFileSync(path)).digest('hex')
}

function run(executable: string, args: readonly string[]): string {
  const result = spawnSync(executable, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] })
  if (result.error || result.status !== 0) {
    const detail = result.error?.message || result.stderr.trim() || `exit status ${result.status ?? 'unknown'}`
    throw new Error(`Unable to inspect ${executable}: ${detail}`)
  }
  return `${result.stdout}\n${result.stderr}`
}

const BLENDER_IDENTITY_MARKER = 'ASSET_PIPELINE_IDENTITY='
const BLENDER_IDENTITY_SCRIPT = [
  'import bpy, hashlib, json, pathlib, sys',
  'root=pathlib.Path(bpy.utils.resource_path("LOCAL"))/"scripts"/"addons_core"/"io_scene_gltf2"',
  'files=sorted(path for path in root.rglob("*") if path.is_file())',
  'digest=hashlib.sha256()',
  '[(digest.update(path.relative_to(root).as_posix().encode()), digest.update(b"\\0"), digest.update(path.read_bytes()), digest.update(b"\\0")) for path in files]',
  `print("${BLENDER_IDENTITY_MARKER}"+json.dumps({"pythonVersion":".".join(map(str,sys.version_info[:3])),"ioSceneGltf2TreeSha256":digest.hexdigest()}))`,
].join(';')

function inspectBlender(executable: string): Required<BlenderIdentity> {
  const versionIdentity = parseBlenderVersionOutput(run(executable, ['--version']))
  const probeOutput = run(executable, [
    '--background',
    '--factory-startup',
    '--disable-autoexec',
    '--python-exit-code',
    '1',
    '--python-expr',
    BLENDER_IDENTITY_SCRIPT,
  ])
  const markerLine = probeOutput.split(/\r?\n/).find((line) => line.startsWith(BLENDER_IDENTITY_MARKER))
  if (!markerLine) {
    throw new Error('Blender identity probe did not report embedded Python and io_scene_gltf2 identities.')
  }
  const probe = JSON.parse(markerLine.slice(BLENDER_IDENTITY_MARKER.length)) as Partial<BlenderIdentity>
  if (!probe.pythonVersion || !probe.ioSceneGltf2TreeSha256) {
    throw new Error('Blender identity probe returned incomplete setup information.')
  }
  return { ...versionIdentity, pythonVersion: probe.pythonVersion, ioSceneGltf2TreeSha256: probe.ioSceneGltf2TreeSha256 }
}

export function inspectToolchain(paths: ToolPaths, lock: ToolchainLock): ToolchainIdentity {
  const platform: PlatformIdentity = { platform: process.platform, arch: process.arch }
  validatePlatformIdentity(platform, lock.platform)

  const node: NodeIdentity = { version: run(paths.node, ['--version']).trim() }
  if (node.version !== lock.node.version) {
    throw new Error(`Node ${lock.node.version} is required; found ${node.version}. Pass the pinned executable path.`)
  }

  const blender = inspectBlender(paths.blender)
  validateBlenderIdentity(blender, lock.blender)

  const toktx: ToktxIdentity = {
    version: parseToktxVersionOutput(run(paths.toktx, ['--version'])),
    binarySha256: sha256File(paths.toktx),
  }
  if (toktx.version !== lock.toktx.version || toktx.binarySha256 !== lock.toktx.binarySha256) {
    throw new Error(
      `toktx ${lock.toktx.version} (${lock.toktx.binarySha256}) is required; `
      + `found ${toktx.version} (${toktx.binarySha256}). Reinstall the official locked archive.`,
    )
  }

  return { platform, node, blender, toktx }
}
