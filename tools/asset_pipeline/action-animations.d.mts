import type { ActionAnimationBinding } from '../../web_new/src/types/actionAnimationDefs.ts'
import type { PublishedCatalog } from './build.mjs'
export function loadActionAnimationDefinitions(root: string): Promise<ActionAnimationBinding[]>
export function publishActionAnimations(options?: { root?: string }): Promise<PublishedCatalog>
