import { readFile, readdir, realpath } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseActionAnimationFile, validateAnimationUniqueness } from '../../web_new/src/types/actionAnimationDefs.ts'
import { publishCatalog, readCatalog, withPublishLock } from './publish.mjs'
import { loadAudioDefinitions } from './audio-definitions.mjs'

export async function loadActionAnimationDefinitions(root) {
  const directory = join(root, 'data/action_animations')
  const entries = await readdir(directory, { withFileTypes: true })
  const filenames = entries.filter(entry => entry.isFile() && entry.name.endsWith('.json')).map(entry => entry.name).sort()
  if (!filenames.length) throw new Error(`${directory}: no action animation definition files`)
  const definitions = []
  for (const name of filenames) {
    const filename = join(directory, name)
    let parsed
    try { parsed = JSON.parse(await readFile(filename, 'utf8')) } catch (error) { throw new Error(`${filename}: ${error.message}`, { cause: error }) }
    definitions.push(...parseActionAnimationFile(parsed, filename))
    validateAnimationUniqueness(definitions, filename)
  }
  await validateMenuSoundTargets(root, definitions)
  return definitions
}

async function validateMenuSoundTargets(root, definitions) {
  const bindings = definitions.filter(binding => binding.source.kind === 'menu' && binding.sound_cues?.some(cue => cue.source === 'target'))
  if (!bindings.length) return
  const directory = join(root, 'data/actions')
  const entries = await readdir(directory, { withFileTypes: true })
  const targets = new Map()
  for (const name of entries.filter(entry => entry.isFile() && entry.name.endsWith('.json')).map(entry => entry.name).sort()) {
    const filename = join(directory, name)
    let file
    try { file = JSON.parse(await readFile(filename, 'utf8')) } catch (error) { throw new Error(`${filename}: ${error.message}`, { cause: error }) }
    if (file?.v !== 1 || !Array.isArray(file.actions) || !file.actions.length) throw new Error(`${filename}: expected v=1 and nonempty actions`)
    for (const action of file.actions) {
      if (typeof action?.id !== 'string' || !/^[a-z][a-z0-9_]*$/.test(action.id) || !['none', 'object', 'tile'].includes(action.target?.kind)) throw new Error(`${filename}: invalid action ID or target kind`)
      if (targets.has(action.id)) throw new Error(`${filename}: duplicate menu action ${action.id}`)
      targets.set(action.id, action.target.kind)
    }
  }
  for (const binding of bindings) {
    const kind = targets.get(binding.source.id)
    if (kind !== 'object' && kind !== 'tile') throw new Error(`binding ${binding.key}: menu action ${binding.source.id} has no available target sound source (target kind ${kind ?? 'missing'})`)
  }
}

export async function publishActionAnimations({ root = fileURLToPath(new URL('../../', import.meta.url)) } = {}) {
  root = await realpath(root)
  const publicRoot = await realpath(join(root, 'web_new/public'))
  return withPublishLock(publicRoot, async () => {
    const previousCatalog = await readCatalog(publicRoot)
    const actionAnimationDefinitions = await loadActionAnimationDefinitions(root)
    const audioDefinitions = await loadAudioDefinitions(root)
    return publishCatalog({ publicRoot, previousCatalog, manifests: [], actionAnimationDefinitions, ...audioDefinitions })
  })
}
