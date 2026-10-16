package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// The caller holds the shard lock. Attachment death can precede the first
// visibility update, so the dead player's own client may need a direct send.
func (s *Shard) sendPlayerDeathFx(playerID types.EntityID, playerHandle types.Handle) {
	const fxKey = "skull_smoke"
	s.SendObjectFx(playerHandle, fxKey)
	visibility := ecs.GetResource[ecs.VisibilityState](s.world)
	visibility.Mu.RLock()
	_, selfVisible := visibility.ObserversByVisibleTarget[playerHandle][playerHandle]
	visibility.Mu.RUnlock()
	if selfVisible {
		return
	}
	position, ok := ecs.GetComponent[components.Transform](s.world, playerHandle)
	if !ok {
		return
	}
	s.SendFx(playerID, &netproto.S2C_Fx{FxKey: fxKey, Position: &netproto.Vector2{X: int32(position.X), Y: int32(position.Y)}})
}

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
