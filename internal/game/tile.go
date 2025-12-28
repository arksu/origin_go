package game

// TileType represents a tile in the world
type TileType uint8

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

// Tile represents a single tile
type Tile struct {
	tileType TileType
	walkable bool
	data     uint16 // extra tile data
}
