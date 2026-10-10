import { serverConstants } from '@/network/ServerConstants'

export const TEXTURE_WIDTH = 64
export const TEXTURE_HEIGHT = 32

export const TILE_WIDTH_HALF = TEXTURE_WIDTH / 2
export const TILE_HEIGHT_HALF = TEXTURE_HEIGHT / 2

export function hasWorldParams(): boolean { return serverConstants.isReady() }

export function getCoordPerTile(): number { return serverConstants.requireSnapshot().coordPerTile }

export function getChunkSize(): number { return serverConstants.requireSnapshot().chunkSize }

export function getFullChunkSize(): number { return getChunkSize() * getCoordPerTile() }
