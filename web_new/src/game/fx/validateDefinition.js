const isObject = (value) => value !== null && typeof value === 'object' && !Array.isArray(value)

// Shared by the object-file validator and preset resolution to keep data rules identical.
export function validateFxDefinition(definition) {
  const fail = (message) => { throw new Error(`Invalid fx: ${message}`) }
  if (!isObject(definition)) fail('expected an object')
  for (const key of Object.keys(definition)) {
    if (!['preset', 'texture', 'zIndex', 'linger', 'offset', 'params'].includes(key)) fail(`unknown field ${key}`)
  }
  if (definition.preset !== 'smoke') fail(`unknown preset ${definition.preset}`)
  if (definition.texture !== undefined && (typeof definition.texture !== 'string' ||
      !/^[\w/-]+\.(png|webp|jpg|svg)$/.test(definition.texture) || definition.texture.startsWith('/'))) {
    fail('texture must be a relative game asset path')
  }
  if (definition.zIndex !== undefined && !Number.isFinite(definition.zIndex)) fail('zIndex must be finite')
  if (definition.linger !== undefined && typeof definition.linger !== 'boolean') fail('linger must be boolean')
  if (definition.offset !== undefined && (!Array.isArray(definition.offset) || definition.offset.length !== 2 ||
      definition.offset.some((value) => !Number.isFinite(value)))) {
    fail('offset must contain two finite local-pixel coordinates')
  }
  const params = definition.params ?? {}
  if (definition.params === null || !isObject(params)) fail('params must be an object')
  for (const [key, value] of Object.entries(params)) {
    if (['density', 'riseSpeed', 'puffSize', 'sway'].includes(key)) {
      if (!Number.isFinite(value) || (key === 'sway' ? value < 0 : value <= 0)) fail(`${key} must be ${key === 'sway' ? 'non-negative' : 'positive'}`)
    } else if (key === 'tint') {
      if (!Number.isInteger(value) || value < 0 || value > 0xffffff) fail('tint must be an RGB integer')
    } else if (key === 'windResponse') {
      if (!Array.isArray(value) || value.length !== 2 || value.some((entry) => !Number.isFinite(entry) || entry < 0) || value[0] > value[1]) {
        fail('windResponse must be an ordered non-negative [min, max] range')
      }
    } else fail(`unknown smoke parameter ${key}`)
  }
}
