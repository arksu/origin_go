package types

const (
	TileWaterDeep  = 1
	TileWater      = 3
	TileStone      = 10
	TilePlowed     = 11
	TileForestPine = 13
	TileForestLeaf = 15
	TileGrass      = 17
	TileSwamp      = 23
	TileClay       = 29
	TileDirt       = 30
	TileSand       = 32
	TileCave       = 42
)

func IsTilePassable(tileID byte) bool {
	return tileID != TileWaterDeep && tileID != TileSwamp
}

func IsTileSwimmable(tileID byte) bool {
	return tileID == TileWater || tileID == TileWaterDeep
}
