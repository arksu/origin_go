import { readFile, readdir, realpath, lstat } from 'node:fs/promises'
import { join } from 'node:path'
import { parseSoundFile, parseLocomotionAudioFile } from '../../web_new/src/types/soundDefs.ts'

async function loadFiles(root, directory, parse) {
  const path = join(root, 'data', directory)
  const entries = await readdir(path, { withFileTypes: true })
  const filenames = entries.filter(entry => entry.isFile() && entry.name.endsWith('.json')).map(entry => entry.name).sort()
  if (!filenames.length) throw new Error(`${path}: no definition files`)
  const definitions = []
  for (const name of filenames) {
    const filename = join(path, name)
    let contents
    try { contents = JSON.parse(await readFile(filename, 'utf8')) } catch (error) { throw new Error(`${filename}: ${error.message}`, { cause: error }) }
    definitions.push(...parse(contents, filename))
  }
  return definitions
}

export async function loadAudioDefinitions(root) {
  const locations = await Promise.all(['sounds', 'locomotion_audio'].map(async directory => {
    try { return (await lstat(join(root, 'data', directory))).isDirectory() } catch (error) { if (error.code === 'ENOENT') return false; throw error }
  }))
  // Old asset-only projects can publish without introducing unrelated audio definitions.
  if (locations.every(present => !present)) return {}
  if (!locations.every(Boolean)) throw new Error('Sound and locomotion-audio definition directories must be present together')
  const soundDefinitions = await loadFiles(root, 'sounds', parseSoundFile)
  parseSoundFile({ v: 1, sounds: soundDefinitions }, 'combined sound definitions')
  const locomotionAudioDefinitions = await loadFiles(root, 'locomotion_audio', parseLocomotionAudioFile)
  for (const binding of locomotionAudioDefinitions) {
    if (binding.cycle_distance_tiles !== undefined) throw new Error(`locomotion ${binding.actor}/${binding.clip}: stride is derived from the actor manifest, not authored here`)
  }
  parseLocomotionAudioFile({ v: 1, bindings: locomotionAudioDefinitions }, 'combined locomotion audio definitions')
  return { soundDefinitions, locomotionAudioDefinitions }
}

export async function validateAudioMedia(publicRoot, profiles) {
  for (const profile of profiles) for (const filename of profile.files) {
    const path = join(publicRoot, 'assets/game', filename)
    try {
      if (await realpath(path) !== path || !(await lstat(path)).isFile()) throw new Error('not a regular canonical file')
      const contents = await readFile(path)
      if (!contents.length) throw new Error('empty audio file')
    } catch (error) { throw new Error(`sound ${profile.key}: missing or invalid media ${filename}: ${error.message}`, { cause: error }) }
  }
}

export function projectLocomotionAudio(definitions, manifests, profiles) {
  const byKey = new Map(profiles.map(profile => [profile.key, profile]))
  return definitions.map(binding => {
    const actor = manifests[binding.actor], clip = actor?.clips?.[binding.clip]
    if (!actor || actor.kind !== 'character' || !clip || clip.rigHash !== actor.rigHash || !clip.loop || clip.playback !== 'distance'
      || !Number.isFinite(clip.cycleDistanceTiles) || clip.cycleDistanceTiles <= 0) {
      throw new Error(`locomotion ${binding.actor}/${binding.clip}: missing or incompatible distance-driven actor clip`)
    }
    for (const contact of binding.contacts) {
      if (byKey.get(contact.sound_key)?.mode !== 'local') throw new Error(`locomotion ${binding.actor}/${binding.clip} contact ${contact.id}: missing local sound ${contact.sound_key}`)
    }
    return { ...binding, cycle_distance_tiles: clip.cycleDistanceTiles }
  })
}
