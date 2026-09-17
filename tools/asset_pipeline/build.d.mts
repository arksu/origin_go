import type { ToolPaths } from './toolchain.ts'

export interface BuildOptions {
  root?: string
  target: string
  animations?: boolean
  toolPaths?: Partial<ToolPaths>
}
export interface Artifact { url: string; sha256: string; bytes: number }
export interface PublishedCatalog { schema: 1; assets: Record<string, Artifact> }
export const defaultRoot: string
export function buildAssets(options: BuildOptions): Promise<PublishedCatalog>
export function validateAssets(options: BuildOptions): Promise<PublishedCatalog>
