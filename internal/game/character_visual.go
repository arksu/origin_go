package game

import (
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// Called on the ECS thread. Enqueue while the world is locked so a spawn cannot
// capture an older snapshot, enqueue it after this update and lose an equip.
func (s *Shard) SendCharacterVisual(observerID, entityID types.EntityID, state *netproto.CharacterVisualState) {
	s.ClientsMu.RLock()
	defer s.ClientsMu.RUnlock()
	client := s.Clients[observerID]
	if client == nil || !client.InWorld.Load() {
		return
	}
	message := &netproto.ServerMessage{Payload: &netproto.ServerMessage_CharacterVisual{
		CharacterVisual: &netproto.S2C_CharacterVisual{EntityId: uint64(entityID), State: state, StreamEpoch: client.StreamEpoch.Load()},
	}}
	encoded, err := proto.Marshal(message)
	if err != nil {
		s.logger.Error("Unable to encode character visual", zap.Uint64("entity_id", uint64(entityID)), zap.Error(err))
		return
	}
	client.Send(encoded)
}
