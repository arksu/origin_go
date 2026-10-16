package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// SendObjectFx runs under the owning shard lock. Only current observers receive
// the event; neither future visibility nor the source view owns its playback.
func (s *Shard) SendObjectFx(handle types.Handle, fxKey string) {
	if fxKey == "" || !s.world.Alive(handle) {
		return
	}
	position, ok := ecs.GetComponent[components.Transform](s.world, handle)
	if !ok {
		return
	}
	visibility := ecs.GetResource[ecs.VisibilityState](s.world)
	visibility.Mu.RLock()
	recipients := make([]types.EntityID, 0, len(visibility.ObserversByVisibleTarget[handle]))
	for observer := range visibility.ObserversByVisibleTarget[handle] {
		if !s.world.Alive(observer) {
			continue
		}
		if id, exists := s.world.GetExternalID(observer); exists && s.world.GetHandleByEntityID(id) == observer {
			recipients = append(recipients, id)
		}
	}
	visibility.Mu.RUnlock()
	fx := &netproto.S2C_Fx{FxKey: fxKey, Position: &netproto.Vector2{X: int32(position.X), Y: int32(position.Y)}}
	for _, recipient := range recipients {
		s.SendFx(recipient, fx)
	}
}
