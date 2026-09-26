package world

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/mathutil"
	"origin/internal/types"
)

// These producers run under streamMu; async workers only transport captured events.
func (cm *ChunkManager) publishChunkLoad(aoi *EntityAOI, coord types.ChunkCoord) {
	if !aoi.SendChunkLoadEvents || aoi.StreamEpoch == 0 || cm.eventBus == nil {
		return
	}
	chunk := cm.GetChunkFast(coord)
	if chunk == nil {
		return
	}
	state := chunk.GetState()
	if state == types.ChunkStateLoading || state == types.ChunkStateUnloaded {
		return
	}
	snapshot := chunk.SnapshotTiles()
	aoi.NextChunkEventSeq++
	cm.eventBus.PublishAsync(ecs.NewChunkLoadEvent(aoi.EntityID, coord.X, coord.Y, cm.layer, snapshot.Tiles, aoi.StreamEpoch, snapshot.Version, aoi.NextChunkEventSeq), eventbus.PriorityMedium)
}

func (cm *ChunkManager) publishChunkUnload(aoi *EntityAOI, coord types.ChunkCoord) {
	if !aoi.SendChunkLoadEvents || aoi.StreamEpoch == 0 || cm.eventBus == nil {
		return
	}
	aoi.NextChunkEventSeq++
	cm.eventBus.PublishAsync(ecs.NewChunkUnloadEvent(aoi.EntityID, coord.X, coord.Y, cm.layer, aoi.StreamEpoch, aoi.NextChunkEventSeq), eventbus.PriorityMedium)
}

// SetTile changes terrain and captures viewer updates in the same authoritative order.
func (cm *ChunkManager) SetTile(tileX, tileY int, tileID byte) bool {
	cm.streamMu.Lock()
	defer cm.streamMu.Unlock()
	coord := types.ChunkCoord{X: mathutil.FloorDiv(tileX, constt.ChunkSize), Y: mathutil.FloorDiv(tileY, constt.ChunkSize)}
	chunk := cm.GetChunkFast(coord)
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		return false
	}
	if !chunk.SetTile(tileX-coord.X*constt.ChunkSize, tileY-coord.Y*constt.ChunkSize, tileID) {
		return false
	}
	cm.aoiMu.RLock()
	defer cm.aoiMu.RUnlock()
	for _, aoi := range cm.entityAOIs {
		if _, active := aoi.ActiveChunks[coord]; active {
			cm.publishChunkLoad(aoi, coord)
		}
	}
	return true
}
