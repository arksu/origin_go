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
      blender = readFlagValue(argv, index, flag)
      index += 1
      continue
    }
    if (flag === '--toktx') {
      if (toktx !== undefined) throw new Error('--toktx may only be specified once')
      toktx = readFlagValue(argv, index, flag)
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

async function unimplementedDispatch(arguments_: CliArguments): Promise<void> {
  throw new Error(`${arguments_.command} is not implemented yet; no artifacts were published`)
}

export async function runCli(argv: readonly string[], options: RunCliOptions = {}): Promise<number> {
  const stdout = options.stdout ?? ((message: string) => process.stdout.write(`${message}\n`))
  const stderr = options.stderr ?? ((message: string) => process.stderr.write(`${message}\n`))
  if (argv.length === 1 && (argv[0] === '--help' || argv[0] === '-h')) {
    stdout(HELP_TEXT.trimEnd())
    return 0
  }
  try {
    const arguments_ = parseArguments(argv)
    await (options.dispatch ?? unimplementedDispatch)(arguments_)
    return 0
  } catch (error) {
    stderr(`Error: ${error instanceof Error ? error.message : String(error)}`)
    return 1
  }
}
