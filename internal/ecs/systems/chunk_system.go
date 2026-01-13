package systems

import (
	"origin/internal/core"
	"origin/internal/ecs"

	"go.uber.org/zap"
)

type ChunkSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	logger       *zap.Logger
}

func NewChunkSystem(chunkManager core.ChunkManager, logger *zap.Logger) *ChunkSystem {
	return &ChunkSystem{
		BaseSystem:   ecs.NewBaseSystem("ChunkSystem", 130), // After TransformUpdateSystem (120)
		chunkManager: chunkManager,
		logger:       logger,
	}
}

func (s *ChunkSystem) Update(w *ecs.World, dt float64) {
	// Placeholder: update chunk binding if needed
	// activeChunks := s.chunkManager.ActiveChunks()
	// for _, chunk := range activeChunks {
	// 	// Iterate entities, check if they moved out of chunk bounds
	//  // s.chunkManager.MigrateObject(...)
	// }
}
