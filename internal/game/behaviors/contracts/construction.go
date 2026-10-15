package contracts

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// ConstructionContext identifies the worker processing a build's final material.
type ConstructionContext struct {
	World        *ecs.World
	PlayerID     types.EntityID
	PlayerHandle types.Handle
	TargetID     types.EntityID
	TargetHandle types.Handle
	ResultType   uint32
}

type ConstructionResult struct {
	BehaviorResult
	// Retry keeps the ordinary build action running without calling complete.
	Retry bool
}

// ConstructionCompleter wraps the ordinary final material, stamina and transform
// work. It runs under the owning shard lock and must call complete at most once,
// synchronously, after any feature preflight. The callback must not be retained.
// A false callback result means no feature completion should be applied.
type ConstructionCompleter interface {
	CompleteConstruction(ctx *ConstructionContext, complete func() bool) ConstructionResult
}
