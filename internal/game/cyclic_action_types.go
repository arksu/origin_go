package game

import (
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

type cyclicActionDecision uint8

const (
	cyclicActionDecisionContinue cyclicActionDecision = iota + 1
	cyclicActionDecisionComplete
	cyclicActionDecisionCanceled
)

type entityIDAllocator interface {
	GetFreeID() types.EntityID
}

type chunkProvider interface {
	GetChunkFast(coord types.ChunkCoord) *core.Chunk
}

type cyclicActionBehavior interface {
	OnCycleComplete(
		ctx cyclicActionCycleContext,
	) cyclicActionDecision
}

type cyclicActionCycleContext struct {
	World        *ecs.World
	PlayerID     types.EntityID
	PlayerHandle types.Handle
	TargetID     types.EntityID
	TargetHandle types.Handle
	Action       components.ActiveCyclicAction
}
