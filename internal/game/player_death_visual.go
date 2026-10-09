package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"
)

// Death runs after the transform system. Publish its final sequence before
// removing Movement, so a delayed earlier update cannot keep a corpse moving.
func (s *Shard) publishDeathMovementStop(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, transform components.Transform) {
	movement, exists := ecs.GetComponent[components.Movement](w, playerHandle)
	if !exists || s.eventBus == nil {
		return
	}
	s.eventBus.PublishAsync(ecs.NewObjectMoveBatchEvent(w.Layer, []ecs.MoveBatchEntry{{
		EntityID:     playerID,
		Handle:       playerHandle,
		X:            int(transform.X),
		Y:            int(transform.Y),
		Heading:      transform.Direction,
		MoveMode:     movement.Mode,
		ServerTimeMs: ecs.GetResource[ecs.TimeState](w).UnixMs,
		MoveSeq:      movement.MoveSeq,
	}}), eventbus.PriorityMedium)
}
