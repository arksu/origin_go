package systems

import (
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestPendingObjectRejectsCachedCollisionLinkAndBreaksExistingLink(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := spawnPlayerForLinkTests(w, 1, 10, 10, 2)
	target := spawnTargetForLinkTests(w, 2, 12, 10)
	ecs.InitResource(w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{target: true}})
	links := ecs.GetResource[ecs.LinkState](w)
	links.SetIntent(1, 2, target, time.Now())
	system := NewLinkSystem(nil, nil)
	system.Update(w, 0)
	require.Empty(t, links.IntentByPlayer)
	require.Empty(t, links.LinkedByPlayer)
	links.SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target})
	system.Update(w, 0)
	require.Empty(t, links.LinkedByPlayer)
	require.Empty(t, links.PlayersByTarget)
}

func TestPendingStationAndScheduledBehaviorRemainFrozen(t *testing.T) {
	w := ecs.NewWorldForTesting()
	target := w.Spawn(1, nil)
	ecs.AddComponent(w, target, components.EntityInfo{TypeID: 1, Behaviors: []string{"grow"}})
	ecs.AddComponent(w, target, components.ObjectInternalState{HP: 0, HasHP: true})
	ecs.AddComponent(w, target, components.StationState{
		CurrentState: "burning", Resources: map[string]uint32{"fuel": 1},
		AutonomousConsumption: []components.StationAutonomousConsumption{{ResourceKey: "fuel", AmountPerTick: 1, RequiredState: "burning", StateWhenDepleted: "unlit"}},
	})
	ecs.InitResource(w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{target: true}})
	NewStationSystem(nil).Update(w, 0)
	station, _ := ecs.GetComponent[components.StationState](w, target)
	require.Equal(t, "burning", station.CurrentState)
	require.Equal(t, uint32(1), station.Resources["fuel"])
	behavior := &testScheduledTickBehavior{}
	registry := &testBehaviorTickRegistry{byKey: map[string]contracts.Behavior{"grow": behavior}}
	system := NewBehaviorTickSystem(nil, BehaviorTickSystemConfig{BehaviorRegistry: registry})
	system.processTickKey(w, 1, ecs.BehaviorTickKey{EntityID: 1, BehaviorKey: "grow"})
	require.Zero(t, behavior.calls)
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, target)
	require.False(t, state.IsDirty)
}
