package utils

const (
	ChunkSize      = 128
	CoordPerTile   = 12
	ChunkWorldSize = ChunkSize * CoordPerTile
)

const (
	PlayerColliderSize = 10
	PlayerLayer        = uint64(1)
	PlayerMask         = uint64(1)
)

const (
	LAST_USED_ID = "last_used_id"
)
