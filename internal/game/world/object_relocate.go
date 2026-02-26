package world

import (
	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

type RelocateWorldObjectImmediateOptions struct {
	IsTeleport        bool
	ForceReindex      bool
	CarriedByEntityID types.EntityID
}

// RelocateWorldObjectImmediate moves a live world object immediately outside the normal
// movement pipeline and keeps chunk/spatial membership consistent.
func RelocateWorldObjectImmediate(
	w *ecs.World,
	chunkManager *ChunkManager,
	eb *eventbus.EventBus,
	handle types.Handle,
	opts RelocateWorldObjectImmediateOptions,
	x float64,
	y float64,
	logger *zap.Logger,
) bool {
	if w == nil || chunkManager == nil || handle == types.InvalidHandle || !w.Alive(handle) {
		return false
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	entityID, hasEntityID := w.GetExternalID(handle)
	transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, handle)
	entityInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	if !hasEntityID || !hasTransform || !hasChunkRef || !hasInfo {
		return false
	}

	oldX := transform.X
	oldY := transform.Y
	if oldX == x && oldY == y && !opts.ForceReindex {
		return true
	}

	oldChunkCoord := types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}
	newChunkCoord := types.WorldToChunkCoord(int(x), int(y), _const.ChunkSize, _const.CoordPerTile)

	oldChunk := chunkManager.GetChunkFast(oldChunkCoord)
	newChunk := oldChunk
	if newChunkCoord != oldChunkCoord {
		newChunk = chunkManager.GetChunkFast(newChunkCoord)
		if newChunk == nil || newChunk.GetState() != types.ChunkStateActive {
			logger.Warn("RelocateWorldObjectImmediate: target chunk not active",
				zap.Uint64("entity_id", uint64(entityID)),
				zap.Int("target_chunk_x", newChunkCoord.X),
				zap.Int("target_chunk_y", newChunkCoord.Y),
			)
			return false
		}
	}

	oldXi := int(oldX)
	oldYi := int(oldY)
	newXi := int(x)
	newYi := int(y)

	if oldChunk != nil {
		if newChunkCoord != oldChunkCoord {
			oldChunk.Spatial().RemoveStatic(handle, oldXi, oldYi)
			oldChunk.Spatial().RemoveDynamic(handle, oldXi, oldYi)
		} else if opts.ForceReindex || oldXi != newXi || oldYi != newYi {
			oldChunk.Spatial().RemoveStatic(handle, oldXi, oldYi)
			oldChunk.Spatial().RemoveDynamic(handle, oldXi, oldYi)
			if entityInfo.IsStatic {
				oldChunk.Spatial().AddStatic(handle, newXi, newYi)
			} else {
				oldChunk.Spatial().AddDynamic(handle, newXi, newYi)
			}
		}
	}
	if newChunkCoord != oldChunkCoord && newChunk != nil {
		if entityInfo.IsStatic {
			newChunk.Spatial().AddStatic(handle, newXi, newYi)
		} else {
			newChunk.Spatial().AddDynamic(handle, newXi, newYi)
		}
		ecs.WithComponent(w, handle, func(cr *components.ChunkRef) {
			cr.PrevChunkX = cr.CurrentChunkX
			cr.PrevChunkY = cr.CurrentChunkY
			cr.CurrentChunkX = newChunkCoord.X
			cr.CurrentChunkY = newChunkCoord.Y
		})
		chunkManager.UpdateEntityPosition(entityID, newChunkCoord)
	}

	ecs.WithComponent(w, handle, func(t *components.Transform) {
		t.X = x
		t.Y = y
	})
	ecs.WithComponent(w, handle, func(state *components.ObjectInternalState) {
		state.IsDirty = true
	})

	if eb != nil {
		serverTimeMs := ecs.GetResource[ecs.TimeState](w).UnixMs
		heading := transform.Direction
		eb.PublishAsync(ecs.NewObjectMoveBatchEvent(w.Layer, []ecs.MoveBatchEntry{{
			EntityID:          entityID,
			Handle:            handle,
			CarriedByEntityID: opts.CarriedByEntityID,
			X:                 newXi,
			Y:                 newYi,
			Heading:           heading,
			IsMoving:          false,
			ServerTimeMs:      serverTimeMs,
			IsTeleport:        opts.IsTeleport,
		}}), eventbus.PriorityMedium)
	}

	return true
}
