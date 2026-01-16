package systems

import (
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/utils"

	"go.uber.org/zap"
)

type ChunkSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	logger       *zap.Logger
}

func NewChunkSystem(chunkManager core.ChunkManager, logger *zap.Logger) *ChunkSystem {
	return &ChunkSystem{
		BaseSystem:   ecs.NewBaseSystem("ChunkSystem", 400),
		chunkManager: chunkManager,
		logger:       logger,
	}
}

func (s *ChunkSystem) Update(w *ecs.World, dt float64) {
	activeChunks := s.chunkManager.ActiveChunks()
	for _, chunk := range activeChunks {
		dynamicHandles := chunk.GetDynamicHandles()

		for _, h := range dynamicHandles {
			if !w.Alive(h) {
				continue
			}

			transform, ok := ecs.GetComponent[components.Transform](w, h)
			if !ok {
				continue
			}

			chunkRef, ok := ecs.GetComponent[components.ChunkRef](w, h)
			if !ok {
				continue
			}

			// Calculate current chunk from position
			// TODO: implement chunk size constant from config
			const chunkWorldSize = utils.ChunkSize * utils.CoordPerTile

			newChunkX := int(transform.X) / chunkWorldSize
			newChunkY := int(transform.Y) / chunkWorldSize

			// Check if entity needs to migrate to different chunk
			if newChunkX != chunkRef.CurrentChunkX || newChunkY != chunkRef.CurrentChunkY {
				// TODO: implement chunk migration
				// 1. Remove entity from old chunk spatial hash
				// 2. Add entity to new chunk spatial hash
				// 3. Update ChunkRef component
				// 4. Handle chunk activation if new chunk was not active

				ecs.WithComponent(w, h, func(cr *components.ChunkRef) {
					cr.PrevChunkX = cr.CurrentChunkX
					cr.PrevChunkY = cr.CurrentChunkY
					cr.CurrentChunkX = newChunkX
					cr.CurrentChunkY = newChunkY
					cr.IsMigrating = true
				})

				s.logger.Debug("Entity chunk migration pending",
					zap.Uint64("handle", uint64(h)),
					zap.Int("from_chunk_x", chunkRef.CurrentChunkX),
					zap.Int("from_chunk_y", chunkRef.CurrentChunkY),
					zap.Int("to_chunk_x", newChunkX),
					zap.Int("to_chunk_y", newChunkY),
				)
			}
		}
	}
}
