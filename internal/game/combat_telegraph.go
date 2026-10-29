package game

import (
	"origin/internal/combatpresentation"
	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// SendCombatTelegraph runs on an accepted transition under the shard lock.
// Observers receive owned bytes and cannot affect the action's execution.
func (s *Shard) SendCombatTelegraph(actor types.Handle) {
	id, ok := s.world.GetExternalID(actor)
	if !ok || !s.world.Alive(actor) {
		return
	}
	visibility := ecs.GetResource[ecs.VisibilityState](s.world)
	visibility.Mu.RLock()
	defer visibility.Mu.RUnlock()
	observers := visibility.ObserversByVisibleTarget[actor]
	if len(observers) == 0 {
		return
	}
	state := combatpresentation.Snapshot(s.world, actor)
	if state == nil {
		return
	}
	packet := &netproto.S2C_CombatTelegraph{EntityId: uint64(id), State: state}
	message := &netproto.ServerMessage{Payload: &netproto.ServerMessage_CombatTelegraph{CombatTelegraph: packet}}
	// At most one serialization per distinct epoch. Most observers share it.
	encodedByEpoch := make(map[uint32][]byte)
	s.ClientsMu.RLock()
	defer s.ClientsMu.RUnlock()
	for observer := range observers {
		observerID, exists := s.world.GetExternalID(observer)
		if !exists || !s.world.Alive(observer) || s.world.GetHandleByEntityID(observerID) != observer {
			continue
		}
		client := s.Clients[observerID]
		if client == nil || !client.InWorld.Load() || client.CharacterID != observerID || client.Layer != s.layer {
			continue
		}
		epoch := client.StreamEpoch.Load()
		if epoch == 0 {
			continue
		}
		data, encoded := encodedByEpoch[epoch]
		if !encoded {
			packet.StreamEpoch = epoch
			var err error
			data, err = proto.Marshal(message)
			if err != nil {
				s.logger.Error("Failed to encode combat warning", zap.Error(err))
				return
			}
			encodedByEpoch[epoch] = data
		}
		client.SendCritical(data)
	}
}
