import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import { HELP_TEXT, parseArguments, runCli, type CliArguments } from '../cli.ts'

test('a clip cannot silently turn a full build into a partial build', () => {
  assert.throws(
    () => parseArguments(['build', 'character/male_commoner', '--clip', 'walk']),
    /--clip requires --animations/,
  )
})

test('one explicit clip is preserved', () => {
  assert.deepEqual(
    parseArguments(['build', 'character/male_commoner', '--animations', '--clip', 'walk']),
    {
      command: 'build', target: 'character/male_commoner', animations: true, clip: 'walk',
      blender: undefined, toktx: undefined,
    },
  )
})

test('rejects unsupported commands, flags, and clip targets', () => {
  assert.throws(() => parseArguments(['publish', 'all']), /Unsupported command.*publish/)
  assert.throws(() => parseArguments(['build', 'all', '--wat']), /Unsupported flag.*--wat/)
  assert.throws(
    () => parseArguments(['build', 'equipment/stone_axe', '--animations', '--clip', 'idle']),
    /--clip requires a character target/,
  )
  assert.throws(() => parseArguments(['validate', 'all', '--animations']), /only supported by build/)
})

test('requires values for executable flags and rejects duplicate flags', () => {
  assert.throws(() => parseArguments(['build', 'all', '--blender']), /--blender requires a value/)
  assert.throws(
    () => parseArguments(['build', 'all', '--toktx', '/one', '--toktx', '/two']),
    /--toktx may only be specified once/,
  )
})

test('dispatch is injectable and importing the CLI has no side effects', async () => {
  let dispatched: CliArguments | undefined
  let standardError = ''
  const exitCode = await runCli(
    ['validate', 'all', '--blender', '/opt/blender', '--toktx', '/opt/toktx'],
    {
      dispatch: async (arguments_) => { dispatched = arguments_ },
      stdout: () => undefined,
      stderr: (message) => { standardError += message },
    },
  )

  assert.equal(exitCode, 0)
  assert.equal(standardError, '')
  assert.deepEqual(dispatched, {
    command: 'validate', target: 'all', animations: false, clip: undefined,
    blender: '/opt/blender', toktx: '/opt/toktx',
  })
})

test('unimplemented handlers fail explicitly instead of reporting build success', async () => {
  let standardError = ''
  const exitCode = await runCli(['build', 'all'], {
    stdout: () => undefined,
    stderr: (message) => { standardError += message },
  })

  assert.equal(exitCode, 1)
  assert.match(standardError, /build is not implemented.*no artifacts were published/i)
})

test('help documents all accepted workflows and setup failures', () => {
  const examples = [
    'tools/assets build character/male_commoner',
    'tools/assets build character/male_commoner --animations',
    'tools/assets build character/male_commoner --animations --clip walk',
    'tools/assets build all',
    'tools/assets validate all',
    'tools/assets verify-reproducible character/male_commoner',
  ]
  for (const example of examples) assert.match(HELP_TEXT, new RegExp(example.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))
  assert.match(HELP_TEXT, /Setup errors:/)
  assert.match(HELP_TEXT, /Blender 5\.2\.1/)
  assert.match(HELP_TEXT, /--toktx/)
})

test('the executable launcher resolves the CLI from its own location', () => {
  const pipelineDirectory = resolve(dirname(fileURLToPath(import.meta.url)), '..')
  const launcher = resolve(pipelineDirectory, '..', 'assets')
  const result = spawnSync(launcher, ['--help'], { cwd: dirname(pipelineDirectory), encoding: 'utf8' })

  assert.equal(result.status, 0, result.stderr)
  assert.match(result.stdout, /verify-reproducible character\/male_commoner/)
})
