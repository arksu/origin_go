import { readFile, readdir, realpath } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseActionAnimationFile, validateAnimationUniqueness } from '../../web_new/src/types/actionAnimationDefs.ts'
import { publishCatalog, readCatalog, withPublishLock } from './publish.mjs'

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
  return definitions
}

export async function publishActionAnimations({ root = fileURLToPath(new URL('../../', import.meta.url)) } = {}) {
  root = await realpath(root)
  const publicRoot = await realpath(join(root, 'web_new/public'))
  return withPublishLock(publicRoot, async () => {
    const previousCatalog = await readCatalog(publicRoot)
    const actionAnimationDefinitions = await loadActionAnimationDefinitions(root)
    return publishCatalog({ publicRoot, previousCatalog, manifests: [], actionAnimationDefinitions })
  })
}
