import { hasTileSet, registerTileSet } from './TileSet'
import { registerTerrain } from '../terrain'
import {
    MAPGEN_TILE_IDS,
    TILE_BROADLEAF_FOREST,
    TILE_CLAY,
    TILE_CONIFEROUS_FOREST,
    TILE_DEEP_WATER,
    TILE_DIRT,
    TILE_GRASS,
    TILE_HEATH,
    TILE_MOOR,
    TILE_MOUNTAIN,
    TILE_PLOWED,
    TILE_SAND,
    TILE_SHALLOW_WATER,
    TILE_STONE_PAVING,
    TILE_SWAMP_1,
    TILE_SWAMP_2,
    TILE_SWAMP_3,
    TILE_THICKET,
    TILE_VOID,
} from './tileIds'

import water from './configs/water.json'
import water_deep from './configs/water_deep.json'
import swamp from './configs/swamp.json'
import wald from './configs/wald.json'
import heath2 from './configs/heath2.json'
import leaf from './configs/leaf.json'
import plowed from './configs/plowed.json'
import dirt from './configs/dirt.json'
import floor_stone from './configs/floor_stone.json'
import sand from './configs/sand.json'
import clay from './configs/clay.json'
import grass from './configs/grass.json'
import moor from './configs/moor.json'
import fen from './configs/fen.json'
import mountain from './configs/mountain.json'

import terrainWald from '../terrain/configs/wald.json'
import terrainHeath from '../terrain/configs/heath.json'

let initialized = false

export function initTileSets(): void {
    if (initialized) {
        console.log('[TileSetLoader] Already initialized')
        return
    }
    initialized = true
    console.log('[TileSetLoader] Initializing tile sets...')

    registerTileSet(TILE_DEEP_WATER, water_deep)
    registerTileSet(TILE_SHALLOW_WATER, water)
    registerTileSet(TILE_STONE_PAVING, floor_stone)
    registerTileSet(TILE_PLOWED, plowed)
    registerTileSet(TILE_CONIFEROUS_FOREST, wald)
    registerTerrain(TILE_CONIFEROUS_FOREST, terrainWald)
    registerTileSet(TILE_BROADLEAF_FOREST, leaf)
    registerTerrain(TILE_BROADLEAF_FOREST, terrainWald)
    registerTileSet(TILE_THICKET, fen)
    registerTileSet(TILE_GRASS, grass)
    registerTerrain(TILE_GRASS, terrainHeath)
    registerTileSet(TILE_HEATH, heath2)
    registerTerrain(TILE_HEATH, terrainHeath)
    registerTileSet(TILE_MOOR, moor)
    registerTileSet(TILE_SWAMP_1, swamp)
    registerTileSet(TILE_SWAMP_2, swamp)
    registerTileSet(TILE_SWAMP_3, swamp)
    registerTileSet(TILE_CLAY, clay)
    registerTileSet(TILE_DIRT, dirt)
    registerTileSet(TILE_SAND, sand)
    registerTileSet(TILE_MOUNTAIN, mountain)

    const missingTileSets = MAPGEN_TILE_IDS.filter((tileID) => tileID !== TILE_VOID && !hasTileSet(tileID))
    if (missingTileSets.length > 0) {
        console.warn(
            `[TileSetLoader] Tile sets initialized with missing mapgen tile IDs: ${missingTileSets.join(', ')}`,
        )
    } else {
        console.log(
            `[TileSetLoader] Tile sets initialized for mapgen contract (${MAPGEN_TILE_IDS.length - 1} IDs).`,
        )
    }
}
