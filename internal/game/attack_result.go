package game

import (
	"errors"
	"math"
	"sync/atomic"

	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var ErrAttackEventSequenceExhausted = errors.New("combat: attack event sequence exhausted")

// AttackEventSequence is shared by every shard of a server instance. Allocating
// before the cost commit may leave gaps but never reuses or wraps an event ID.
type AttackEventSequence struct{ next atomic.Uint64 }

func (s *AttackEventSequence) Next() (uint64, error) {
	for {
		current := s.next.Load()
		if current == math.MaxUint64 {
			return 0, ErrAttackEventSequenceExhausted
		}
		if s.next.CompareAndSwap(current, current+1) {
			return current + 1, nil
		}
	}
}

// SendAttackResult runs synchronously after combat completion under shard lock.
// Encoding owns its output: scratch hit messages are never queued or published
// asynchronously. Failure to deliver cannot undo or repeat committed gameplay.
func (s *Shard) SendAttackResult(actor types.Handle, actorID types.EntityID, eventID uint64, hits []netproto.AttackHit) {
	if eventID == 0 || !s.world.Alive(actor) || s.world.GetHandleByEntityID(actorID) != actor {
		return
	}
	visibility := ecs.GetResource[ecs.VisibilityState](s.world)
	visibility.Mu.RLock()
	observers := visibility.ObserversByVisibleTarget[actor]
	recipients := make([]types.EntityID, 0, len(observers)+1)
	recipients = append(recipients, actorID)
	for observer := range observers {
		if observer == actor || !s.world.Alive(observer) {
			continue
		}
		id, exists := s.world.GetExternalID(observer)
		if exists && id != actorID && s.world.GetHandleByEntityID(id) == observer {
			recipients = append(recipients, id)
		}
	}
	visibility.Mu.RUnlock()
	hitPointers := make([]*netproto.AttackHit, len(hits))
	for i := range hits {
		hitPointers[i] = &hits[i]
	}
	result := &netproto.S2C_AttackResult{EventId: eventID, AttackerId: uint64(actorID), Hits: hitPointers}
	message := &netproto.ServerMessage{Payload: &netproto.ServerMessage_AttackResult{AttackResult: result}}
	// Most listeners share an epoch; reuse the immutable encoded payload for it.
	encodedByEpoch := make(map[uint32][]byte)
	s.ClientsMu.RLock()
	defer s.ClientsMu.RUnlock()
	send := func(id types.EntityID) {
		client := s.Clients[id]
		if client == nil || !client.InWorld.Load() || client.CharacterID != id || client.Layer != s.layer {
			return
		}
		epoch := client.StreamEpoch.Load()
		if epoch == 0 {
			return
		}
		data, encoded := encodedByEpoch[epoch]
		if !encoded {
			result.StreamEpoch = epoch
			var err error
			data, err = proto.Marshal(message)
			if err != nil {
				s.logger.Error("Failed to encode attack result", zap.Error(err))
				return
			}
			encodedByEpoch[epoch] = data
		}
		client.SendCritical(data)
	}
	// Exact generational observer identities are unique in the reverse index.
	for _, id := range recipients {
		send(id)
	}
}
