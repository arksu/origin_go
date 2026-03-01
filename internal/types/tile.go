package types

const (
	TileDeepWater        = 1
	TileShallowWater     = 3
	TileBrickRed         = 5
	TileBrickYellow      = 6
	TileBrickBlack       = 7
	TileBrickBlue        = 8
	TileBrickWhite       = 9
	TileStonePaving      = 12
	TilePlowed           = 14
	TileConiferousForest = 20
	TileBroadleafForest  = 25
	TileThicket          = 30
	TileGrass            = 35
	TileHeath            = 40
	TileMoor             = 45
	TileSwamp1           = 50
	TileSwamp2           = 53
	TileSwamp3           = 56
	TileDirt             = 60
	TileClay             = 64
	TileSand             = 68
	TileHouse            = 80
	TileHouseCellar      = 90
	TileMineEntry        = 100
	TileMine             = 105
	TileCave             = 110
	TileMountain         = 120
	TileVoid             = 255
)

var knownTileIDs = map[int]struct{}{
	TileDeepWater:        {},
	TileShallowWater:     {},
	TileBrickRed:         {},
	TileBrickYellow:      {},
	TileBrickBlack:       {},
	TileBrickBlue:        {},
	TileBrickWhite:       {},
	TileStonePaving:      {},
	TilePlowed:           {},
	TileConiferousForest: {},
	TileBroadleafForest:  {},
	TileThicket:          {},
	TileGrass:            {},
	TileHeath:            {},
	TileMoor:             {},
	TileSwamp1:           {},
	TileSwamp2:           {},
	TileSwamp3:           {},
	TileDirt:             {},
	TileClay:             {},
	TileSand:             {},
	TileHouse:            {},
	TileHouseCellar:      {},
	TileMineEntry:        {},
	TileMine:             {},
	TileCave:             {},
	TileMountain:         {},
	TileVoid:             {},
}

func IsKnownTileID(tileID int) bool {
	_, ok := knownTileIDs[tileID]
	return ok
}

func IsTilePassable(tileID byte) bool {
	return tileID != TileDeepWater &&
		tileID != TileSwamp1 &&
		tileID != TileSwamp2 &&
		tileID != TileSwamp3 &&
		tileID != TileVoid
}

func IsTileSwimmable(tileID byte) bool {
	return tileID == TileShallowWater || tileID == TileDeepWater
}
