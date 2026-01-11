package types

type ChunkState uint8

/*
Unloaded → Loading (requestLoad)
Loading → Preloaded (loadChunkFromDB завершён)
Preloaded → Active (ActivateChunk)
Active → Preloaded (DeactivateChunk)
Preloaded → Inactive (выход из preload-зоны в updatePreloadZone)
Inactive → Unloaded (LRU eviction)
Preloaded → Inactive (выход из preload-зоны)
Inactive → Preloaded (возврат в preload-зону)
*/

const (
	ChunkStateUnloaded ChunkState = iota
	ChunkStateLoading
	ChunkStatePreloaded
	ChunkStateActive
	ChunkStateInactive
)

func (s ChunkState) String() string {
	switch s {
	case ChunkStateUnloaded:
		return "unloaded"
	case ChunkStateLoading:
		return "loading"
	case ChunkStatePreloaded:
		return "preloaded"
	case ChunkStateActive:
		return "active"
	case ChunkStateInactive:
		return "inactive"
	default:
		return "unknown"
	}
}

type ChunkCoord struct {
	X, Y int
}

// ChunkData represents the core data of a chunk without dependencies
type ChunkData struct {
	Coord    ChunkCoord
	Region   int32
	Layer    int32
	State    ChunkState
	Tiles    []byte
	LastTick uint64
}

func NewChunkData(coord ChunkCoord, region int32, layer int32, chunkSize int) *ChunkData {
	return &ChunkData{
		Coord:  coord,
		Region: region,
		Layer:  layer,
		State:  ChunkStateUnloaded,
		Tiles:  make([]byte, chunkSize*chunkSize),
	}
}

func WorldToChunkCoord(worldX, worldY int, chunkSize, coordPerTile int) ChunkCoord {
	tileX := worldX / coordPerTile
	tileY := worldY / coordPerTile

	// Handle negative coordinates correctly
	if worldX < 0 && worldX%coordPerTile != 0 {
		tileX--
	}
	if worldY < 0 && worldY%coordPerTile != 0 {
		tileY--
	}

	chunkX := tileX / chunkSize
	chunkY := tileY / chunkSize

	// Handle negative chunk coordinates correctly
	if tileX < 0 && tileX%chunkSize != 0 {
		chunkX--
	}
	if tileY < 0 && tileY%chunkSize != 0 {
		chunkY--
	}

	return ChunkCoord{
		X: chunkX,
		Y: chunkY,
	}
}
