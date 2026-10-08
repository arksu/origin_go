package game

import (
	"testing"
	"time"

	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestObjectDestructionCancelsUnlinkedContextApproach(t *testing.T) {
	f := newDestructionServiceFixture(t)
	w := f.w
	s := &Shard{world: w}
	f.service.deps.Quarantine = s.cancelDestroyedObjectReferences
	player := w.Spawn(2000, nil)
	ecs.AddComponent(w, player, components.Transform{X: 1, Y: 1})
	ecs.AddComponent(w, player, components.Movement{TargetType: constt.TargetEntity, TargetHandle: f.target, State: constt.StateMoving, VelocityX: 2})
	ecs.AddComponent(w, player, components.PendingContextAction{TargetEntityID: 1, TargetHandle: f.target, ActionID: "open"})
	ecs.GetResource[ecs.LinkState](w).SetIntent(2000, 1, f.target, time.Now())
	_, linked := ecs.GetResource[ecs.LinkState](w).GetLink(2000)
	require.False(t, linked)
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	_, pending := ecs.GetComponent[components.PendingContextAction](w, player)
	require.False(t, pending)
	require.Empty(t, ecs.GetResource[ecs.LinkState](w).IntentByPlayer)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, constt.TargetNone, movement.TargetType)
	require.Equal(t, constt.StateIdle, movement.State)
	require.Zero(t, movement.VelocityX)
	require.True(t, movement.Direction.UpdatePending)
	// The existing movement publication pipeline must include this stopped actor.
	systems.NewMovementSystem(w, nil, zap.NewNop()).Update(w, .05)
	moved := ecs.GetResource[ecs.MovedEntities](w)
	require.Equal(t, 1, moved.Count)
	require.Equal(t, player, moved.Handles[0])
}

func TestObjectDestructionCancelsUnlinkedNoColliderLiftAndPublishesIdle(t *testing.T) {
	f := newDestructionServiceFixture(t)
	w := f.w
	ecs.RemoveComponent[components.Collider](w, f.target)
	sender, handler := &testActionSender{}, &testActionHandler{}
	actions, err := NewActionService(w, actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "lift", Target: actiondefs.Target{Kind: actiondefs.TargetObject}},
	}), map[string]ActionHandler{"lift": handler}, sender)
	require.NoError(t, err)
	s := &Shard{world: w, actionService: actions}
	f.service.deps.Quarantine = s.cancelDestroyedObjectReferences
	player := w.Spawn(2000, nil)
	ecs.AddComponent(w, player, components.Transform{})
	ecs.AddComponent(w, player, components.Movement{TargetType: constt.TargetEntity, TargetHandle: f.target, State: constt.StateMoving})
	ecs.AddComponent(w, player, components.Collider{Phantom: &components.PhantomCollider{WorldX: 50, WorldY: 50}})
	ecs.AddComponent(w, player, components.PendingLiftTransition{Mode: components.LiftTransitionModePickupNoCollider, ObjectEntityID: 1, ObjectHandle: f.target, TargetX: 50, TargetY: 50, ActionGeneration: 3})
	ecs.AddComponent(w, player, components.ActiveGameAction{ActionID: "lift", Phase: components.GameActionApproaching, TargetID: 1, TargetHandle: f.target, Generation: 3, MovementOwned: true})
	_, err = f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	_, pending := ecs.GetComponent[components.PendingLiftTransition](w, player)
	require.False(t, pending)
	_, active := ecs.GetComponent[components.ActiveGameAction](w, player)
	require.False(t, active)
	collider, _ := ecs.GetComponent[components.Collider](w, player)
	require.Nil(t, collider.Phantom)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, constt.TargetNone, movement.TargetType)
	require.True(t, movement.Direction.UpdatePending)
	require.Equal(t, 1, handler.canceled)
	require.Len(t, sender.states, 1)
	require.Equal(t, "idle", sender.states[0].Phase)
}

func TestObjectDestructionReferencesPreserveNewerRoutesAndStun(t *testing.T) {
	w := ecs.NewWorldForTesting()
	index := attachObjectTargetReferences(w)
	oldTarget, newerTarget := w.Spawn(1, nil), w.Spawn(2, nil)
	player := w.Spawn(3, nil)
	ecs.AddComponent(w, player, components.PendingContextAction{TargetHandle: oldTarget})
	ecs.AddComponent(w, player, components.Movement{TargetType: constt.TargetEntity, TargetHandle: newerTarget, State: constt.StateMoving})
	ecs.GetResource[ecs.LinkState](w).SetIntent(3, 2, newerTarget, time.Now())
	s := &Shard{world: w}
	s.cancelDestroyedObjectReferences(oldTarget)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, newerTarget, movement.TargetHandle)
	require.Equal(t, constt.StateMoving, movement.State)
	require.Equal(t, newerTarget, ecs.GetResource[ecs.LinkState](w).IntentByPlayer[3].TargetHandle)
	require.Empty(t, index.byTarget[oldTarget])
	ecs.WithComponent(w, player, func(m *components.Movement) { m.State = constt.StateStunned })
	s.cancelDestroyedObjectReferences(newerTarget)
	movement, _ = ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, constt.StateStunned, movement.State)
	require.Equal(t, constt.TargetNone, movement.TargetType)
	require.True(t, movement.Direction.UpdatePending)
}

func TestObjectDestructionCancelsIntentOnlyAndOwnedPutDown(t *testing.T) {
	w := ecs.NewWorldForTesting()
	attachObjectTargetReferences(w)
	target := w.Spawn(1, nil)
	intentOnly, putDown := w.Spawn(2, nil), w.Spawn(3, nil)
	links := ecs.GetResource[ecs.LinkState](w)
	links.SetIntent(2, 1, target, time.Now())
	ecs.AddComponent(w, putDown, components.Movement{TargetType: constt.TargetPoint, TargetX: 50, TargetY: 51, State: constt.StateMoving})
	ecs.AddComponent(w, putDown, components.PendingLiftTransition{Mode: components.LiftTransitionModePutDown, ObjectHandle: target,
		TargetX: 50.75, TargetY: 51.25, ActionGeneration: 4})
	ecs.AddComponent(w, putDown, components.Collider{Phantom: &components.PhantomCollider{WorldX: 50.75, WorldY: 51.25}})
	ecs.AddComponent(w, putDown, components.ActiveGameAction{ActionID: "put_down", Phase: components.GameActionApproaching, Generation: 4,
		MovementOwned: true, TargetX: 50.75, TargetY: 51.25})
	sender := &testActionSender{}
	actions, err := NewActionService(w, actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "put_down", Target: actiondefs.Target{Kind: actiondefs.TargetTile}},
	}), map[string]ActionHandler{"put_down": &testActionHandler{}}, sender)
	require.NoError(t, err)
	(&Shard{world: w, actionService: actions}).cancelDestroyedObjectReferences(target)
	require.True(t, w.Alive(intentOnly))
	require.Empty(t, links.IntentByPlayer)
	movement, _ := ecs.GetComponent[components.Movement](w, putDown)
	require.Equal(t, constt.TargetNone, movement.TargetType)
	require.True(t, movement.Direction.UpdatePending)
	_, active := ecs.GetComponent[components.ActiveGameAction](w, putDown)
	require.False(t, active)
	collider, _ := ecs.GetComponent[components.Collider](w, putDown)
	require.Nil(t, collider.Phantom)
	require.Len(t, sender.states, 1)
	require.Equal(t, "idle", sender.states[0].Phase)
}

func TestObjectDestructionLiftCancellationPreservesNewerIntentAndRoute(t *testing.T) {
	w := ecs.NewWorldForTesting()
	attachObjectTargetReferences(w)
	oldTarget, newerTarget, player := w.Spawn(1, nil), w.Spawn(2, nil), w.Spawn(3, nil)
	ecs.AddComponent(w, player, components.Movement{TargetType: constt.TargetEntity, TargetHandle: newerTarget, State: constt.StateMoving})
	ecs.AddComponent(w, player, components.PendingLiftTransition{ObjectHandle: oldTarget, ActionGeneration: 4})
	ecs.AddComponent(w, player, components.ActiveGameAction{ActionID: "lift", TargetID: 1, TargetHandle: oldTarget, Generation: 4, MovementOwned: true})
	links := ecs.GetResource[ecs.LinkState](w)
	links.SetIntent(3, 2, newerTarget, time.Now())
	lift := NewLiftService(w, nil, nil, nil, zap.NewNop())
	actions, err := NewActionService(w, actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "lift", Target: actiondefs.Target{Kind: actiondefs.TargetObject}},
	}), map[string]ActionHandler{"lift": &liftActionHandler{lift: lift}}, &testActionSender{})
	require.NoError(t, err)
	(&Shard{world: w, actionService: actions, liftService: lift}).cancelDestroyedObjectReferences(oldTarget)
	_, pending := ecs.GetComponent[components.PendingLiftTransition](w, player)
	require.False(t, pending)
	_, active := ecs.GetComponent[components.ActiveGameAction](w, player)
	require.False(t, active)
	require.Equal(t, newerTarget, links.IntentByPlayer[3].TargetHandle)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, newerTarget, movement.TargetHandle)
	require.Equal(t, constt.StateMoving, movement.State)
}

func TestObjectDestructionStaleLiftPreservesNewerActionAndPhantom(t *testing.T) {
	w := ecs.NewWorldForTesting()
	attachObjectTargetReferences(w)
	oldTarget, newerTarget, player := w.Spawn(1, nil), w.Spawn(2, nil), w.Spawn(3, nil)
	ecs.AddComponent(w, player, components.PendingLiftTransition{ObjectHandle: oldTarget, TargetX: 50, TargetY: 50,
		PhantomHalfW: 1, PhantomHalfH: 1, ActionGeneration: 4})
	ecs.AddComponent(w, player, components.ActiveGameAction{ActionID: "newer", TargetHandle: newerTarget, Generation: 5})
	phantom := &components.PhantomCollider{WorldX: 80, WorldY: 80, HalfWidth: 2, HalfHeight: 2}
	ecs.AddComponent(w, player, components.Collider{Phantom: phantom})
	ecs.AddComponent(w, player, components.Movement{TargetType: constt.TargetEntity, TargetHandle: newerTarget, State: constt.StateMoving})
	links := ecs.GetResource[ecs.LinkState](w)
	links.SetIntent(3, 2, newerTarget, time.Now())
	(&Shard{world: w}).cancelDestroyedObjectReferences(oldTarget)
	_, pending := ecs.GetComponent[components.PendingLiftTransition](w, player)
	require.False(t, pending)
	active, _ := ecs.GetComponent[components.ActiveGameAction](w, player)
	require.Equal(t, uint64(5), active.Generation)
	require.Equal(t, newerTarget, links.IntentByPlayer[3].TargetHandle)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, newerTarget, movement.TargetHandle)
	collider, _ := ecs.GetComponent[components.Collider](w, player)
	require.Same(t, phantom, collider.Phantom)
}

func TestObjectDestructionStalePutDownPreservesNewerPointRoute(t *testing.T) {
	w := ecs.NewWorldForTesting()
	attachObjectTargetReferences(w)
	target, player := w.Spawn(1, nil), w.Spawn(2, nil)
	ecs.AddComponent(w, player, components.PendingLiftTransition{Mode: components.LiftTransitionModePutDown, ObjectHandle: target,
		TargetX: 50, TargetY: 50, ActionGeneration: 4})
	ecs.AddComponent(w, player, components.ActiveGameAction{ActionID: "newer", Generation: 5})
	ecs.AddComponent(w, player, components.Movement{TargetType: constt.TargetPoint, TargetX: 50, TargetY: 50, State: constt.StateMoving})
	(&Shard{world: w}).cancelDestroyedObjectReferences(target)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, constt.TargetPoint, movement.TargetType)
	require.Equal(t, constt.StateMoving, movement.State)
	active, _ := ecs.GetComponent[components.ActiveGameAction](w, player)
	require.Equal(t, uint64(5), active.Generation)
}

func TestObjectDestructionPostgresQuarantineCancelsApproachingPlayers(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 64, 50)
	f.shard.mu.Lock()
	w := f.shard.world
	contextPlayer, liftPlayer := w.Spawn(2000, nil), w.Spawn(2001, nil)
	for _, player := range []types.Handle{contextPlayer, liftPlayer} {
		ecs.AddComponent(w, player, components.Transform{X: 1, Y: 1})
		ecs.AddComponent(w, player, components.Movement{TargetType: constt.TargetEntity, TargetHandle: f.target, State: constt.StateMoving})
	}
	ecs.AddComponent(w, contextPlayer, components.PendingContextAction{TargetEntityID: 1, TargetHandle: f.target, ActionID: "open"})
	ecs.GetResource[ecs.LinkState](w).SetIntent(2000, 1, f.target, time.Now())
	ecs.RemoveComponent[components.Collider](w, f.target)
	ecs.AddComponent(w, liftPlayer, components.PendingLiftTransition{Mode: components.LiftTransitionModePickupNoCollider, ObjectEntityID: 1, ObjectHandle: f.target, TargetX: 50, TargetY: 50, ActionGeneration: 3})
	ecs.AddComponent(w, liftPlayer, components.Collider{Phantom: &components.PhantomCollider{WorldX: 50, WorldY: 50}})
	ecs.AddComponent(w, liftPlayer, components.ActiveGameAction{ActionID: "lift", Phase: components.GameActionApproaching, TargetID: 1, TargetHandle: f.target, Generation: 3, MovementOwned: true})
	f.shard.liftService = NewLiftService(w, f.cm, nil, nil, f.shard.logger)
	sender := &testActionSender{}
	var err error
	f.shard.actionService, err = NewActionService(w, actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "lift", Target: actiondefs.Target{Kind: actiondefs.TargetObject}},
	}), map[string]ActionHandler{"lift": &liftActionHandler{lift: f.shard.liftService}}, sender)
	f.shard.mu.Unlock()
	require.NoError(t, err)
	f.hit(t)
	f.shard.mu.RLock()
	defer f.shard.mu.RUnlock()
	_, pending := ecs.GetComponent[components.PendingContextAction](w, contextPlayer)
	require.False(t, pending)
	_, lifting := ecs.GetComponent[components.PendingLiftTransition](w, liftPlayer)
	require.False(t, lifting)
	_, active := ecs.GetComponent[components.ActiveGameAction](w, liftPlayer)
	require.False(t, active)
	for _, player := range []types.Handle{contextPlayer, liftPlayer} {
		movement, _ := ecs.GetComponent[components.Movement](w, player)
		require.Equal(t, constt.TargetNone, movement.TargetType)
		require.True(t, movement.Direction.UpdatePending)
	}
	collider, _ := ecs.GetComponent[components.Collider](w, liftPlayer)
	require.Nil(t, collider.Phantom)
	require.Empty(t, ecs.GetResource[ecs.LinkState](w).IntentByPlayer)
	require.Len(t, sender.states, 1)
	require.Equal(t, "idle", sender.states[0].Phase)
}

func TestObjectTargetReferencesRetargetDespawnAndGeneration(t *testing.T) {
	w := ecs.NewWorldForTesting()
	index := attachObjectTargetReferences(w)
	oldTarget, newerTarget := w.Spawn(1, nil), w.Spawn(2, nil)
	player := w.Spawn(3, nil)
	ecs.AddComponent(w, player, components.Movement{TargetHandle: oldTarget})
	ecs.AddComponent(w, player, components.PendingContextAction{TargetHandle: oldTarget})
	ecs.WithComponent(w, player, func(m *components.Movement) { m.TargetHandle = newerTarget })
	require.Contains(t, index.byTarget[oldTarget], player)
	ecs.RemoveComponent[components.PendingContextAction](w, player)
	require.Empty(t, index.byTarget[oldTarget])
	ecs.GetResource[ecs.LinkState](w).SetIntent(3, 2, newerTarget, time.Now())
	require.True(t, w.Despawn(newerTarget))
	replacement := w.Spawn(2, nil)
	require.NotEqual(t, replacement, newerTarget)
	require.Empty(t, index.byTarget[newerTarget])
	require.Empty(t, index.byActor)
	require.Empty(t, ecs.GetResource[ecs.LinkState](w).IntentPlayersByTarget)
	(&Shard{world: w}).cancelDestroyedObjectReferences(replacement)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	require.Equal(t, newerTarget, movement.TargetHandle)
	ecs.WithComponent(w, player, func(m *components.Movement) { m.TargetHandle = replacement })
	require.True(t, w.Despawn(player))
	require.Empty(t, index.byActor)
	require.Empty(t, index.byTarget)
}

func TestObjectTargetReferenceUnchangedMovementAllocatesNothing(t *testing.T) {
	w := ecs.NewWorldForTesting()
	attachObjectTargetReferences(w)
	target, player := w.Spawn(1, nil), w.Spawn(2, nil)
	ecs.AddComponent(w, player, components.Movement{TargetHandle: target})
	require.Zero(t, testing.AllocsPerRun(100, func() {
		ecs.WithComponent(w, player, func(m *components.Movement) { m.VelocityX++ })
	}))
}
