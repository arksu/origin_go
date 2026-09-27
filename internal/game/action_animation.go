package game

import (
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// The ECS world lock orders transitions with visibility snapshot capture/enqueue.
func (shard *Shard) SendActionAnimation(observerID, entityID types.EntityID, state *netproto.CharacterActionAnimationState) {
	shard.ClientsMu.RLock()
	defer shard.ClientsMu.RUnlock()
	client := shard.Clients[observerID]
	if client == nil || !client.InWorld.Load() {
		return
	}
	message := &netproto.ServerMessage{Payload: &netproto.ServerMessage_CharacterActionAnimation{
		CharacterActionAnimation: &netproto.S2C_CharacterActionAnimation{EntityId: uint64(entityID), State: state, StreamEpoch: client.StreamEpoch.Load()},
	}}
	encoded, err := proto.Marshal(message)
	if err != nil {
		shard.logger.Error("Unable to encode action animation", zap.Uint64("entity_id", uint64(entityID)), zap.Error(err))
		return
	}
	client.SendCritical(encoded)
}
