import type { ResourceDefLike } from '@/types/objectEditor'

export function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function isResourceDefLike(value: unknown): value is ResourceDefLike {
  return isPlainObject(value) && Array.isArray((value as Record<string, unknown>).layers)
}

export function splitDotPath(path: string): string[] {
  if (!path.trim()) return []
  return path
    .split('.')
    .map((part) => part.trim())
    .filter(Boolean)
}

export function joinDotPath(parts: string[]): string {
  return parts.join('.')
}

export function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function getNodeAtPath(root: Record<string, unknown>, path: string): unknown {
  const parts = splitDotPath(path)
  let cursor: unknown = root
  for (const part of parts) {
    if (!isPlainObject(cursor)) return undefined
    cursor = cursor[part]
  }
  return cursor
}

export function getParentAtPath(root: Record<string, unknown>, path: string): {
  parent: Record<string, unknown>
  key: string
} | null {
  const parts = splitDotPath(path)
  if (parts.length === 0) return null
  const key = parts[parts.length - 1]!
  let cursor: unknown = root
  for (let i = 0; i < parts.length - 1; i++) {
    const part = parts[i]!
    if (!isPlainObject(cursor)) return null
    cursor = cursor[part]
  }
  if (!isPlainObject(cursor)) return null
  return { parent: cursor, key }
}

function setNodeAtParts(root: Record<string, unknown>, parts: string[], value: unknown): void {
  if (parts.length === 0) throw new Error('Cannot replace root object via setNodeAtParts')
  let cursor: Record<string, unknown> = root
  for (let i = 0; i < parts.length - 1; i++) {
    const part = parts[i]!
    const next = cursor[part]
    if (!isPlainObject(next)) {
      const created: Record<string, unknown> = {}
      cursor[part] = created
      cursor = created
      continue
    }
    cursor = next
  }
  cursor[parts[parts.length - 1]!] = value
}

function deleteNodeAtParts(root: Record<string, unknown>, parts: string[]): unknown {
  if (parts.length === 0) throw new Error('Cannot delete root')
  const parentPath = parts.slice(0, -1)
  const key = parts[parts.length - 1]!
  let cursor: unknown = root
  for (const part of parentPath) {
    if (!isPlainObject(cursor)) throw new Error('Invalid path')
    cursor = cursor[part]
  }
  if (!isPlainObject(cursor) || !(key in cursor)) throw new Error('Path not found')
  const value = cursor[key]
  delete cursor[key]
  return value
}

export function moveNodeToParent(
  root: Record<string, unknown>,
  sourcePath: string,
  targetParentPath: string,
  preferredKey?: string,
): string {
  const sourceParts = splitDotPath(sourcePath)
  const targetParentParts = splitDotPath(targetParentPath)

  if (sourceParts.length === 0) throw new Error('Cannot move root')
  if (targetParentParts.join('.') === sourceParts.join('.')) throw new Error('Cannot move node into itself')

  if (
    targetParentParts.length >= sourceParts.length &&
    sourceParts.every((part, idx) => targetParentParts[idx] === part)
  ) {
    throw new Error('Cannot move node into its descendant')
  }

  let targetParent: unknown = root
  for (const part of targetParentParts) {
    if (!isPlainObject(targetParent)) throw new Error('Target parent is not an object')
    targetParent = targetParent[part]
  }
  if (!isPlainObject(targetParent)) throw new Error('Target parent is not an object')

  const sourceKey = sourceParts[sourceParts.length - 1]!
  const nextKey = preferredKey?.trim() || sourceKey
  if (nextKey.includes('.')) throw new Error('Target key cannot include dot')
  if (nextKey.length === 0) throw new Error('Target key is empty')

  const movedValue = deleteNodeAtParts(root, sourceParts)

  if (nextKey in targetParent) {
    // Restore before failing so operation is atomic in memory.
    setNodeAtParts(root, sourceParts, movedValue)
    throw new Error(`Target key already exists: ${nextKey}`)
  }

  targetParent[nextKey] = movedValue
  return joinDotPath([...targetParentParts, nextKey])
}

export function wrapNodeWithSubpath(
  root: Record<string, unknown>,
  nodePath: string,
  relativeSubPath: string,
): string {
  const subParts = splitDotPath(relativeSubPath)
  if (subParts.length === 0) throw new Error('Sub path is empty')
  const parentRef = getParentAtPath(root, nodePath)
  if (!parentRef) throw new Error('Cannot wrap root')
  const { parent, key } = parentRef
  if (!(key in parent)) throw new Error('Path not found')

  const currentValue = parent[key]
  let wrapped: unknown = currentValue
  for (let i = subParts.length - 1; i >= 0; i--) {
    wrapped = { [subParts[i]!]: wrapped }
  }
  parent[key] = wrapped
  return joinDotPath([...splitDotPath(nodePath), ...subParts])
}

export function flattenSingleChildWrapper(root: Record<string, unknown>, nodePath: string): string {
  const parentRef = getParentAtPath(root, nodePath)
  if (!parentRef) throw new Error('Cannot flatten root')
  const { parent, key } = parentRef
  const node = parent[key]
  if (!isPlainObject(node)) throw new Error('Selected node is not an object wrapper')

  const entries = Object.entries(node)
  if (entries.length !== 1) throw new Error('Wrapper must contain exactly one child')

  const [childKey, childValue] = entries[0]!
  if (childKey !== key && childKey in parent) {
    throw new Error(`Cannot flatten: sibling key "${childKey}" already exists`)
  }

  delete parent[key]
  parent[childKey] = childValue

  const parentParts = splitDotPath(nodePath).slice(0, -1)
  return joinDotPath([...parentParts, childKey])
}

export function ensureNumberPair(target: Record<string, unknown>, key: string): number[] {
  const value = target[key]
  if (!Array.isArray(value)) {
    const next = [0, 0]
    target[key] = next
    return next
  }
  if (value.length < 2) {
    value[0] = Number(value[0] ?? 0)
    value[1] = Number(value[1] ?? 0)
  }
  return value as number[]
}
