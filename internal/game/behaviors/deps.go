package behaviors

import (
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

// TreeChunkProvider resolves chunks for tree behavior spawn/transform flow.
type TreeChunkProvider interface {
	GetChunkFast(coord types.ChunkCoord) *core.Chunk
}

// EntityIDAllocator allocates new entity IDs.
type EntityIDAllocator interface {
	GetFreeID() types.EntityID
}

// VisionUpdateForcer forces observer vision refresh.
type VisionUpdateForcer interface {
	ForceUpdateForObserver(w *ecs.World, observerHandle types.Handle)
}

// ActionExecutionDeps contains shared dependencies for context action execution.
type ActionExecutionDeps struct {
	OpenService      any
	EventBus         *eventbus.EventBus
	Chunks           TreeChunkProvider
	IDAllocator      EntityIDAllocator
	VisionForcer     VisionUpdateForcer
	BehaviorRegistry types.BehaviorRegistry
	Logger           *zap.Logger
}

func resolveActionExecutionDeps(extra any) ActionExecutionDeps {
	switch deps := extra.(type) {
	case *ActionExecutionDeps:
		if deps == nil {
			return ActionExecutionDeps{}
		}
		return *deps
	case ActionExecutionDeps:
		return deps
	default:
		return ActionExecutionDeps{}
	}
}
