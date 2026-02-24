package world

import (
	"origin/internal/core"
	"origin/internal/types"
)

// ObjectDespawnPersistence records persistence-side delete intent for world objects.
// Some objects are deleted immediately from DB (e.g. dropped items), while chunk-owned
// objects are recorded as chunk tombstones and deleted on the next chunk save.
type ObjectDespawnPersistence interface {
	DeleteDroppedObject(region int, entityID types.EntityID) error
	RecordChunkObjectDespawn(chunk *core.Chunk, entityID types.EntityID)
}
