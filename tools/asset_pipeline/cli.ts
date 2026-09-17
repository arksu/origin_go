import { constants } from 'node:fs'
import { access, realpath, stat } from 'node:fs/promises'
import { isAbsolute } from 'node:path'

export type AssetCommand = 'build' | 'validate' | 'verify-reproducible'

export interface CliArguments {
  command: AssetCommand
  target: string
  animations: boolean
  clip: string | undefined
  blender: string | undefined
  toktx: string | undefined
}

export type CommandDispatch = (arguments_: CliArguments) => Promise<void>

export interface RunCliOptions {
  root?: string
  dispatch?: CommandDispatch
  stdout?: (message: string) => void
  stderr?: (message: string) => void
}

const COMMANDS = new Set<AssetCommand>(['build', 'validate', 'verify-reproducible'])
const TARGET_PATTERN = /^(character|equipment|world_object)\/[a-z0-9][a-z0-9_-]*$/
const CLIP_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_.-]*$/

export const HELP_TEXT = `Usage: tools/assets <command> <target> [options]

Commands:
  build                 Build and publish selected assets
  validate              Validate sources and recipes
  verify-reproducible   Compare two clean builds

Options:
  --animations          Build only animation artifacts
  --clip <name>         Build one clip; requires --animations and a character target
  --blender <path>      Use the configured Blender executable
  --toktx <path>        Use the configured KTX encoder
  --help                 Show this help

Examples:
  tools/assets build character/male_commoner
  tools/assets build character/male_commoner --animations
  tools/assets build character/male_commoner --animations --clip walk
  tools/assets build all
  tools/assets validate all
  tools/assets verify-reproducible character/male_commoner

Setup errors:
  Install the locked Node version, Blender 5.2.1, and official KTX-Software.
  Pass non-default executable locations with --blender and --toktx.
  Builds run offline and never install missing tools.
`

function parseCommand(value: string | undefined): AssetCommand {
  if (!value || !COMMANDS.has(value as AssetCommand)) {
    throw new Error(`Unsupported command ${value ?? '(missing)'}. Expected build, validate, or verify-reproducible.`)
  }
  return value as AssetCommand
}

function parseTarget(value: string | undefined): string {
  if (!value) throw new Error('A target is required: all or kind/id')
  if (value !== 'all' && !TARGET_PATTERN.test(value)) throw new Error(`Invalid asset target ${value}`)
  return value
}

function readFlagValue(argv: readonly string[], index: number, flag: string): string {
  const value = argv[index + 1]
  if (!value || value.startsWith('--')) throw new Error(`${flag} requires a value`)
  return value
}

function readExecutableOverride(argv: readonly string[], index: number, flag: string): string {
  const value = readFlagValue(argv, index, flag)
  if (!isAbsolute(value)) throw new Error(`${flag} requires an absolute path`)
  return value
}

export function parseArguments(argv: readonly string[]): CliArguments {
  const command = parseCommand(argv[0])
  const target = parseTarget(argv[1])
  let animations = false
  let clip: string | undefined
  let blender: string | undefined
  let toktx: string | undefined

  for (let index = 2; index < argv.length; index += 1) {
    const flag = argv[index]
    if (flag === '--animations') {
      if (animations) throw new Error('--animations may only be specified once')
      animations = true
      continue
    }
    if (flag === '--clip') {
      if (clip !== undefined) throw new Error('--clip may only be specified once')
      clip = readFlagValue(argv, index, flag)
      if (!CLIP_PATTERN.test(clip)) throw new Error('--clip must be a portable clip name')
      index += 1
      continue
    }
    if (flag === '--blender') {
      if (blender !== undefined) throw new Error('--blender may only be specified once')
      blender = readExecutableOverride(argv, index, flag)
      index += 1
      continue
    }
    if (flag === '--toktx') {
      if (toktx !== undefined) throw new Error('--toktx may only be specified once')
      toktx = readExecutableOverride(argv, index, flag)
      index += 1
      continue
    }
    throw new Error(`Unsupported flag ${flag ?? '(missing)'}`)
  }

  if (command !== 'build' && (animations || clip !== undefined)) {
    throw new Error('--animations and --clip are only supported by build')
  }
  if (clip !== undefined && !animations) throw new Error('--clip requires --animations')
  if (clip !== undefined && !target.startsWith('character/')) {
    throw new Error('--clip requires a character target')
  }
  return { command, target, animations, clip, blender, toktx }
}

async function dispatchCommand(arguments_: CliArguments, options: RunCliOptions): Promise<void> {
  if (arguments_.command === 'verify-reproducible' || arguments_.animations) {
    throw new Error(`${arguments_.animations ? 'Animation-only builds' : arguments_.command} are not implemented yet; no artifacts were published`)
  }
  const { buildAssets, validateAssets, defaultRoot } = await import('./build.mjs')
  const toolPaths = {
    ...(arguments_.blender ? { blender: arguments_.blender } : {}),
    ...(arguments_.toktx ? { toktx: arguments_.toktx } : {}),
  }
  const operation = arguments_.command === 'build' ? buildAssets : validateAssets
  await operation({ root: options.root ?? defaultRoot, target: arguments_.target, toolPaths })
}

async function canonicalizeExecutable(path: string, flag: '--blender' | '--toktx'): Promise<string> {
  let canonicalPath: string
  try {
    canonicalPath = await realpath(path)
  } catch (error) {
    const code = error instanceof Error && 'code' in error ? error.code : undefined
    if (code === 'ENOENT') throw new Error(`${flag} executable does not exist: ${path}`)
    throw new Error(`Unable to inspect ${flag} executable ${path}: ${error instanceof Error ? error.message : String(error)}`)
  }
  if (!(await stat(canonicalPath)).isFile()) throw new Error(`${flag} must name an executable file`)
  try {
    await access(canonicalPath, constants.X_OK)
  } catch {
    throw new Error(`${flag} file is not executable: ${canonicalPath}`)
  }
  return canonicalPath
}

async function validateExecutableOverrides(arguments_: CliArguments): Promise<CliArguments> {
  const blender = arguments_.blender === undefined
    ? undefined
    : await canonicalizeExecutable(arguments_.blender, '--blender')
  const toktx = arguments_.toktx === undefined
    ? undefined
    : await canonicalizeExecutable(arguments_.toktx, '--toktx')
  return { ...arguments_, blender, toktx }
}

export async function runCli(argv: readonly string[], options: RunCliOptions = {}): Promise<number> {
  const stdout = options.stdout ?? ((message: string) => process.stdout.write(`${message}\n`))
  const stderr = options.stderr ?? ((message: string) => process.stderr.write(`${message}\n`))
  if (argv.length === 1 && (argv[0] === '--help' || argv[0] === '-h')) {
    stdout(HELP_TEXT.trimEnd())
    return 0
  }
  try {
    const arguments_ = await validateExecutableOverrides(parseArguments(argv))
    if (options.dispatch) await options.dispatch(arguments_)
    else await dispatchCommand(arguments_, options)
    return 0
  } catch (error) {
    stderr(`Error: ${error instanceof Error ? error.message : String(error)}`)
    return 1
  }
}
