import {registerTileSet} from './TileSet'
import {registerTerrain} from './terrain'

import water from './tiles/water.json'
import water_deep from './tiles/water_deep.json'
import swamp from './tiles/swamp.json'
import wald from './tiles/wald.json'
import heath2 from './tiles/heath2.json'
import leaf from './tiles/leaf.json'
import plowed from './tiles/plowed.json'
import dirt from './tiles/dirt.json'
import floor_stone from './tiles/floor_stone.json'
import sand from './tiles/sand.json'
import clay from './tiles/clay.json'
import grass from './tiles/grass.json'
import moor from './tiles/moor.json'
import fen from './tiles/fen.json'

import terrainWald from './terrain/configs/wald.json'
import terrainHeath from './terrain/configs/heath.json'

let initialized = false

export function initTileSets(): void {
    if (initialized) {
        console.log('[TileSetLoader] Already initialized')
        return
    }
    initialized = true
    console.log('[TileSetLoader] Initializing tile sets...')

    registerTileSet(1, water_deep)
    registerTileSet(3, water)
    registerTileSet(10, floor_stone)
    registerTileSet(11, plowed)
    registerTileSet(13, wald)
    registerTerrain(13, terrainWald)
    registerTileSet(15, leaf)
    registerTerrain(15, terrainWald)
    registerTileSet(16, fen)
    registerTileSet(17, grass)
    registerTerrain(17, terrainHeath)
    registerTileSet(18, heath2)
    registerTerrain(18, terrainHeath)
    registerTileSet(21, moor)
    registerTileSet(23, swamp)
    registerTileSet(29, clay)
    registerTileSet(30, dirt)
    registerTileSet(32, sand)

    console.log('[TileSetLoader] Tile sets initialized: 14 types, 2 terrain types registered')
}
