package systems

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/playerstate"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type visualDelivery struct {
	observerID, entityID types.EntityID
	state                *netproto.CharacterVisualState
}
type visualSender struct{ calls []visualDelivery }

func (s *visualSender) SendCharacterVisual(observerID, entityID types.EntityID, state *netproto.CharacterVisualState) {
	s.calls = append(s.calls, visualDelivery{observerID, entityID, state})
}

func TestCharacterVisualSystemSendsOnceToOwnerAndVisibleObservers(t *testing.T) {
	w := ecs.NewWorldForTesting()
	owner := w.Spawn(101, nil)
	observer := w.Spawn(102, nil)
	w.Spawn(103, nil) // Online, but not in visibility.
	stale := w.Spawn(104, nil)
	w.Despawn(stale)
	ecs.AddComponent(w, owner, components.Appearance{Resource: "player"})
	ecs.AddComponent(w, owner, components.EntityHealth{SHP: 0, HHP: 20, IsLying: true, KOUntilUnixMs: 60000, LyingRevision: 1})
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.ObserversByVisibleTarget[owner] = map[types.Handle]struct{}{owner: {}, observer: {}, stale: {}}
	sender := &visualSender{}
	system := NewCharacterVisualSystem(sender, zap.NewNop())
	system.Update(w, 0)
	require.Empty(t, sender.calls)
	ecs.MarkCharacterVisualDirty(w, 101)
	ecs.MarkCharacterVisualDirty(w, 101)
	system.Update(w, 0)
	require.Len(t, sender.calls, 2)
	require.ElementsMatch(t, []types.EntityID{101, 102}, []types.EntityID{sender.calls[0].observerID, sender.calls[1].observerID})
	for _, call := range sender.calls {
		require.Equal(t, types.EntityID(101), call.entityID)
		require.NotEmpty(t, call.state.Generation)
		require.True(t, call.state.IsLying)
		require.Equal(t, uint64(1), call.state.Revision)
	}
	system.Update(w, 0)
	require.Len(t, sender.calls, 2, "clean actors must not broadcast every tick")
	ecs.WithComponent(w, owner, func(health *components.EntityHealth) { playerstate.SetLying(health, false) })
	ecs.MarkCharacterVisualDirty(w, 101)
	system.Update(w, 0)
	require.Len(t, sender.calls, 4)
	for _, call := range sender.calls[2:] {
		require.False(t, call.state.IsLying)
		require.Equal(t, uint64(2), call.state.Revision)
	}
	require.True(t, sender.calls[0].state.IsLying, "broadcast snapshots must own their values")
	delete(visibility.ObserversByVisibleTarget[owner], observer)
	ecs.MarkCharacterVisualDirty(w, 101)
	system.Update(w, 0)
	require.Len(t, sender.calls, 5)
	require.Equal(t, types.EntityID(101), sender.calls[4].observerID)
	ecs.MarkCharacterVisualDirty(w, 101)
	w.Despawn(owner)
	system.Update(w, 0)
	require.Len(t, sender.calls, 5, "a stale dirty handle cannot resurrect an actor")
}
