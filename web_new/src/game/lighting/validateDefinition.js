const isObject = (value) => value !== null && typeof value === 'object' && !Array.isArray(value)

// Shared by object validation and runtime resolution so authored lights use one contract.
export function validateLightDefinition(definition) {
  const fail = (message) => { throw new Error(`Invalid light: ${message}`) }
  if (!isObject(definition)) fail('expected an object')
  for (const key of Object.keys(definition)) {
    if (!['radius', 'offset', 'tint', 'strength', 'flicker'].includes(key)) fail(`unknown field ${key}`)
  }
  if (!Number.isFinite(definition.radius) || definition.radius <= 0) fail('radius must be positive and finite')
  if (definition.offset !== undefined && (!Array.isArray(definition.offset) || definition.offset.length !== 2 ||
      definition.offset.some((value) => !Number.isFinite(value)))) {
    fail('offset must contain two finite local-pixel coordinates')
  }
  if (definition.tint !== undefined && (!Number.isInteger(definition.tint) || definition.tint < 0 || definition.tint > 0xffffff)) {
    fail('tint must be an RGB integer')
  }
  for (const key of ['strength', 'flicker']) {
    const value = definition[key]
    if (value !== undefined && (!Number.isFinite(value) || value < 0 || value > 1)) fail(`${key} must be between zero and one`)
  }
}
