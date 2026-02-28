#!/usr/bin/env node

import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'

const OBJECTS_DIR = path.resolve(process.cwd(), 'src/game/objects')

function isPlainObject(value) {
  return value != null && typeof value === 'object' && !Array.isArray(value)
}

function collectObjectFiles(dir) {
  return fs
    .readdirSync(dir)
    .filter((name) => name.endsWith('.json'))
    .map((name) => path.join(dir, name))
}

function validateLayerSource(layer, context, errors) {
  const hasImg = typeof layer.img === 'string'
  const hasFrames = Array.isArray(layer.frames)
  const hasSpine = isPlainObject(layer.spine)
  const sourceCount = Number(hasImg) + Number(hasFrames) + Number(hasSpine)

  if (sourceCount !== 1) {
    errors.push(`${context}: layer must have exactly one source (img | frames | spine)`)
  }
}

function validateAnimatedLayer(layer, context, errors) {
  if (!Array.isArray(layer.frames)) {
    return null
  }

  if (layer.frames.length < 1) {
    errors.push(`${context}: animated layer must have non-empty frames`)
    return null
  }

  if (typeof layer.fps !== 'number' || !Number.isFinite(layer.fps) || layer.fps <= 0) {
    errors.push(`${context}: animated layer must define fps > 0`)
    return null
  }

  if (layer.loop != null && typeof layer.loop !== 'boolean') {
    errors.push(`${context}: loop must be boolean when provided`)
  }

  return {
    fps: layer.fps,
    frameCount: layer.frames.length,
  }
}

function validateResource(resource, context, errors) {
  if (!Array.isArray(resource.layers)) {
    return
  }

  if (resource.fps != null) {
    errors.push(`${context}: resource.fps is not allowed; move timing to layer.fps`)
  }

  const animated = []
  for (let index = 0; index < resource.layers.length; index += 1) {
    const layer = resource.layers[index]
    const layerContext = `${context}.layers[${index}]`
    if (!isPlainObject(layer)) {
      errors.push(`${layerContext}: layer must be an object`)
      continue
    }

    validateLayerSource(layer, layerContext, errors)
    const animatedSpec = validateAnimatedLayer(layer, layerContext, errors)
    if (animatedSpec) {
      animated.push(animatedSpec)
    }
  }

  if (animated.length > 1) {
    const expected = animated[0]
    for (let i = 1; i < animated.length; i += 1) {
      const current = animated[i]
      if (current.fps !== expected.fps || current.frameCount !== expected.frameCount) {
        errors.push(
          `${context}: animated layers must be synced (same fps and frame count across all animated layers)`,
        )
        break
      }
    }
  }
}

function walkObjectTree(node, context, errors) {
  if (!isPlainObject(node)) {
    return
  }
  if (Array.isArray(node.layers)) {
    validateResource(node, context, errors)
    return
  }

  for (const [key, value] of Object.entries(node)) {
    walkObjectTree(value, `${context}.${key}`, errors)
  }
}

function validateFile(filePath) {
  const raw = fs.readFileSync(filePath, 'utf8')
  const json = JSON.parse(raw)
  const errors = []
  walkObjectTree(json, path.basename(filePath), errors)
  return errors
}

function main() {
  const files = collectObjectFiles(OBJECTS_DIR)
  const allErrors = []

  for (const filePath of files) {
    const fileErrors = validateFile(filePath)
    allErrors.push(...fileErrors)
  }

  if (allErrors.length > 0) {
    console.error('Object schema validation failed:')
    for (const err of allErrors) {
      console.error(`- ${err}`)
    }
    process.exit(1)
  }

  console.log(`Object schema validation passed (${files.length} files).`)
}

main()
