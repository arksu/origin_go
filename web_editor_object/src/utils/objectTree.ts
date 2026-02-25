import type { ObjectTreeNode } from '@/types/objectEditor'
import { isPlainObject, isResourceDefLike } from '@/utils/objectPath'

export function buildObjectTree(root: Record<string, unknown>, basePath = ''): ObjectTreeNode[] {
  const nodes: ObjectTreeNode[] = []
  for (const [key, value] of Object.entries(root)) {
    const path = basePath ? `${basePath}.${key}` : key
    let children: ObjectTreeNode[] = []
    if (isPlainObject(value) && !isResourceDefLike(value)) {
      children = buildObjectTree(value, path)
    }

    nodes.push({
      key,
      path,
      isResource: isResourceDefLike(value),
      children,
    })
  }
  return nodes
}

export function findFirstResourcePath(root: Record<string, unknown>): string | null {
  for (const [key, value] of Object.entries(root)) {
    if (isResourceDefLike(value)) return key
    if (isPlainObject(value)) {
      const child = findFirstResourcePath(value)
      if (child) return `${key}.${child}`
    }
  }
  return null
}
