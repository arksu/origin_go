package world

import (
	"encoding/json"

	"origin/internal/core"
	"origin/internal/types"
)

// ObjectDeleter removes one persistent object and all of its root inventory rows.
type ObjectDeleter interface {
	DeleteObject(region int, entityID types.EntityID) error
}

// ObjectDataUpdater persists a narrowly scoped metadata migration for one live
// object while it is loaded from a chunk.
type ObjectDataUpdater interface {
	UpdateObjectData(region int, entityID types.EntityID, data json.RawMessage) error
}

// ObjectDespawnPersistence records persistence-side delete intent for world objects.
// Some objects are deleted immediately from DB (e.g. dropped items), while chunk-owned
// objects are recorded as chunk tombstones and deleted on the next chunk save.
type ObjectDespawnPersistence interface {
	ObjectDeleter
	RecordChunkObjectDespawn(chunk *core.Chunk, entityID types.EntityID)
}
