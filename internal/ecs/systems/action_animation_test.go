package systems

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"testing"
)

type actionAnimationDelivery struct {
	observer, entity types.EntityID
	state            *netproto.CharacterActionAnimationState
}
type actionAnimationSender struct{ calls []actionAnimationDelivery }

func (sender *actionAnimationSender) SendActionAnimation(observer, entity types.EntityID, state *netproto.CharacterActionAnimationState) {
	sender.calls = append(sender.calls, actionAnimationDelivery{observer, entity, state})
}

func TestActionAnimationDeliveryIsDirtyAndVisibilityScoped(t *testing.T) {
	w := ecs.NewWorldForTesting()
	owner, observer := w.Spawn(101, nil), w.Spawn(102, nil)
	w.Spawn(103, nil)
	stale := w.Spawn(104, nil)
	w.Despawn(stale)
	ecs.AddComponent(w, owner, components.Appearance{Resource: "player"})
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.ObserversByVisibleTarget[owner] = map[types.Handle]struct{}{owner: {}, observer: {}, stale: {}}
	sender := &actionAnimationSender{}
	system := NewActionAnimationSystem(sender, zap.NewNop())
	queue := ecs.GetResource[ecs.ActionAnimationDirtyQueue](w)
	queue.Mark(owner)
	queue.Mark(owner)
	system.Update(w, 0)
	require.Len(t, sender.calls, 2)
	require.ElementsMatch(t, []types.EntityID{101, 102}, []types.EntityID{sender.calls[0].observer, sender.calls[1].observer})
	system.Update(w, 0)
	require.Len(t, sender.calls, 2)
	delete(visibility.ObserversByVisibleTarget, owner)
	queue.Mark(owner)
	system.Update(w, 0)
	require.Len(t, sender.calls, 3)
	require.Equal(t, types.EntityID(101), sender.calls[2].observer, "owner does not require self-visibility")
	queue.Mark(owner)
	w.Despawn(owner)
	replacement := w.Spawn(101, nil)
	ecs.AddComponent(w, replacement, components.Appearance{Resource: "player"})
	system.Update(w, 0)
	require.Len(t, sender.calls, 3, "stale dirty handles cannot address reused entities")
}
