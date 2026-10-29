package behaviors

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// This fake models only the executor boundary. Phase advancement is explicit and
// independent of NPC decisions; it deliberately has no damage or combat rules.
type npcExtensionExecutor struct {
	world       *ecs.World
	states      map[contracts.EntityRef]contracts.NPCAbilityState
	starts      []contracts.NPCAbilityRequest
	cancels     []npcExtensionCancellation
	nextOutcome contracts.NPCExecutionStatus
}

type npcExtensionCancellation struct {
	actor      contracts.EntityRef
	generation uint64
	reason     contracts.NPCRetireReason
}

func newNPCExtensionExecutor(w *ecs.World) *npcExtensionExecutor {
	return &npcExtensionExecutor{world: w, states: make(map[contracts.EntityRef]contracts.NPCAbilityState)}
}

func (f *npcExtensionExecutor) valid(ref contracts.EntityRef) bool {
	if !f.world.Alive(ref.Handle) {
		return false
	}
	id, ok := f.world.GetExternalID(ref.Handle)
	return ok && id == ref.EntityID
}

func (f *npcExtensionExecutor) TryStart(req contracts.NPCAbilityRequest) contracts.NPCAbilityState {
	f.starts = append(f.starts, req)
	state := contracts.NPCAbilityState{Actor: req.Actor, AbilityID: req.AbilityID, Generation: req.Generation}
	if !f.valid(req.Actor) || req.Generation == 0 || (req.Target.Kind == contracts.NPCTargetEntity && !f.valid(req.Target.Entity)) {
		state.Status = contracts.NPCExecutionRejected
		return state
	}
	if previous := f.states[req.Actor]; previous.Running() {
		state.Status = contracts.NPCExecutionBusy
		return state
	}
	if f.nextOutcome != contracts.NPCExecutionNone {
		state.Status, f.nextOutcome = f.nextOutcome, contracts.NPCExecutionNone
		return state
	}
	state.Phase, state.Status = contracts.NPCAbilityWindup, contracts.NPCExecutionAccepted
	f.states[req.Actor] = state
	if req.AbilityID == "test_charge" {
		movement, _ := ecs.GetComponent[components.Movement](f.world, req.Actor.Handle)
		movement.SetTargetPoint(int(req.Target.Point.X), int(req.Target.Point.Y))
		ecs.AddComponent(f.world, req.Actor.Handle, movement)
	}
	return state
}

func (f *npcExtensionExecutor) Cancel(actor contracts.EntityRef, generation uint64, reason contracts.NPCRetireReason) contracts.NPCAbilityState {
	f.cancels = append(f.cancels, npcExtensionCancellation{actor, generation, reason})
	state := f.State(actor, generation)
	if !f.valid(actor) || state.Generation != generation || generation == 0 || !state.Running() {
		return contracts.NPCAbilityState{Actor: actor, Generation: generation, Status: contracts.NPCExecutionRejected}
	}
	state.Phase, state.Status = contracts.NPCAbilityFinished, contracts.NPCExecutionCanceled
	f.states[actor] = state
	return state
}

func (f *npcExtensionExecutor) State(actor contracts.EntityRef, generation uint64) contracts.NPCAbilityState {
	state, ok := f.states[actor]
	if !ok || !f.valid(actor) || generation == 0 || state.Generation != generation {
		return contracts.NPCAbilityState{Actor: actor, Generation: generation, Status: contracts.NPCExecutionRejected}
	}
	return state
}

func (f *npcExtensionExecutor) advance(t *testing.T, actor contracts.EntityRef) contracts.NPCAbilityState {
	t.Helper()
	state := f.states[actor]
	require.True(t, state.Running())
	state.Phase++
	if state.Phase == contracts.NPCAbilityFinished {
		state.Status = contracts.NPCExecutionCompleted
	}
	f.states[actor] = state
	return state
}

func npcExtensionRef(w *ecs.World, handle types.Handle) contracts.EntityRef {
	id, _ := w.GetExternalID(handle)
	return contracts.EntityRef{Handle: handle, EntityID: id}
}

func npcExtensionProfile(id string, decide NPCPolicy) NPCProfile {
	return NPCProfile{
		ID: id, Capabilities: []string{"locomotion", "abilities"}, Decide: decide,
		Init: func(state components.NPCPolicyState, observation NPCObservation, _ contracts.NPCBehaviorConfig) NPCDecision {
			return NPCDecision{State: state, NextDecisionTick: observation.Tick + 1}
		},
		OnResult: func(state components.NPCPolicyState, observation NPCObservation, _ contracts.NPCBehaviorConfig, result contracts.NPCExecutionResult) NPCDecision {
			state.RandomState = uint64(result.Status)
			return NPCDecision{State: state, NextDecisionTick: observation.Tick + 1}
		},
	}
}

func npcExtensionLoad(t *testing.T, catalog *NPCCatalog, configs ...contracts.NPCBehaviorConfig) (*objectdefs.Registry, error) {
	t.Helper()
	behaviorRegistry, err := NewRegistry(NewNPCAIBehavior(catalog))
	require.NoError(t, err)
	file := objectdefs.ObjectsFile{Version: 1}
	for i, cfg := range configs {
		raw, err := json.Marshal(cfg)
		require.NoError(t, err)
		dynamic := false
		file.Objects = append(file.Objects, objectdefs.ObjectDef{
			DefID: i + 1001, Key: fmt.Sprintf("test_npc_%d", i), Name: "Test NPC", Static: &dynamic,
			HP: 110, Transient: true, Resource: "test/npc",
			Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 2, H: 2}},
			Behaviors:  map[string]json.RawMessage{npcAIKey: raw},
		})
	}
	data, err := json.Marshal(file)
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "npcs.json"), data, 0600))
	return objectdefs.LoadFromDirectory(dir, behaviorRegistry, zap.NewNop())
}

func npcExtensionInstall(t *testing.T, catalog *NPCCatalog, configs ...contracts.NPCBehaviorConfig) {
	t.Helper()
	definitions, err := npcExtensionLoad(t, catalog, configs...)
	require.NoError(t, err)
	previous := objectdefs.Global()
	objectdefs.SetGlobalForTesting(definitions)
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
}

type npcExtensionWorld struct {
	world      *ecs.World
	controller *NPCController
	executor   *npcExtensionExecutor
	behavior   contracts.Behavior
}

func newNPCExtensionWorld(t *testing.T, catalog *NPCCatalog) npcExtensionWorld {
	t.Helper()
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).TickPeriod = 100 * time.Millisecond
	fake := newNPCExtensionExecutor(w)
	controller, err := AttachNPCController(w, NPCControllerConfig{
		Catalog: catalog, Abilities: fake, Chunks: npcTestChunks{{}: npcTestLand(t, types.ChunkCoord{})},
	})
	require.NoError(t, err)
	return npcExtensionWorld{world: w, controller: controller, executor: fake, behavior: NewNPCAIBehavior(catalog)}
}

func (f npcExtensionWorld) spawn(t *testing.T, id types.EntityID, defID uint32) types.Handle {
	t.Helper()
	h := f.world.Spawn(id, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: defID, Transient: true, Behaviors: []string{npcAIKey}})
		ecs.AddComponent(w, h, components.Transform{X: 300, Y: 300})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 1, HalfHeight: 1})
		ecs.AddComponent(w, h, components.ObjectInternalState{HP: 110, HasHP: true})
	})
	require.NoError(t, f.behavior.(contracts.ObjectLifecycleInitializer).InitObject(&contracts.BehaviorObjectInitContext{
		World: f.world, Handle: h, EntityID: id, EntityType: defID, Reason: contracts.ObjectBehaviorInitReasonSpawn,
	}))
	return h
}

func (f npcExtensionWorld) tick(t *testing.T, h types.Handle, tick uint64) components.NPCBrain {
	t.Helper()
	ecs.GetResource[ecs.TimeState](f.world).Tick = tick
	id, _ := f.world.GetExternalID(h)
	info, _ := ecs.GetComponent[components.EntityInfo](f.world, h)
	result, err := f.behavior.(contracts.ScheduledTickBehavior).OnScheduledTick(&contracts.BehaviorTickContext{
		World: f.world, Handle: h, EntityID: id, EntityType: info.TypeID, CurrentTick: tick,
	})
	require.NoError(t, err)
	require.False(t, result.StateChanged)
	brain, ok := ecs.GetComponent[components.NPCBrain](f.world, h)
	require.True(t, ok)
	return brain
}

func TestNPCRegisteredProfilesUseIndependentMovementLimits(t *testing.T) {
	movePolicy := func(radius, distance float64) NPCPolicy {
		return func(state components.NPCPolicyState, observation NPCObservation, _ contracts.NPCBehaviorConfig) NPCDecision {
			state.Step++
			return NPCDecision{State: state, NextDecisionTick: observation.Tick + 1,
				Intent: NPCMoveTo(contracts.NPCPoint{X: observation.Position.X + distance, Y: observation.Position.Y}, observation.Tick+10, radius)}
		}
	}
	catalog, err := NewNPCCatalog([]NPCProfile{
		npcExtensionProfile("test_home", movePolicy(12, 6)),
		npcExtensionProfile("test_roam", movePolicy(0, 96)),
	}, nil, nil)
	require.NoError(t, err)
	npcExtensionInstall(t, catalog,
		contracts.NPCBehaviorConfig{Profile: "test_home", Speed: 32},
		contracts.NPCBehaviorConfig{Profile: "test_roam", Speed: 32})
	f := newNPCExtensionWorld(t, catalog)
	home, roam := f.spawn(t, 100, 1001), f.spawn(t, 101, 1002)
	for _, h := range []types.Handle{home, roam} {
		brain := f.tick(t, h, 1)
		require.Equal(t, contracts.NPCExecutionAccepted, brain.LastResult)
		require.Equal(t, uint64(contracts.NPCExecutionAccepted), brain.RandomState, "execution feedback reaches the registered policy")
	}
	roaming, _ := ecs.GetComponent[components.Movement](f.world, roam)
	require.Equal(t, float64(396), roaming.TargetX, "a new profile is not constrained by the fox's 6-tile home radius")
	ecs.AddComponent(f.world, home, components.CollisionResult{FinalX: 324, FinalY: 300})
	ecs.AddComponent(f.world, roam, components.CollisionResult{FinalX: 396, FinalY: 300})
	moved := ecs.GetResource[ecs.MovedEntities](f.world)
	moved.Add(home, 324, 300)
	moved.Add(roam, 396, 300)
	ecs.GetResource[ecs.TimeState](f.world).Tick = 2
	NewNPCMovementGuard(f.controller).Update(f.world, .1)
	homeResult, _ := ecs.GetComponent[components.CollisionResult](f.world, home)
	roamResult, _ := ecs.GetComponent[components.CollisionResult](f.world, roam)
	require.Equal(t, float64(300), homeResult.FinalX)
	require.Equal(t, float64(396), roamResult.FinalX)
	homeMove, _ := ecs.GetComponent[components.Movement](f.world, home)
	roamMove, _ := ecs.GetComponent[components.Movement](f.world, roam)
	require.Equal(t, constt.StateIdle, homeMove.State)
	require.Equal(t, constt.StateMoving, roamMove.State)
}

func TestNPCAbilityPhasesAdvanceWithoutAIAndRetainMotionOwnership(t *testing.T) {
	profile := npcExtensionProfile("test_phases", func(state components.NPCPolicyState, observation NPCObservation, _ contracts.NPCBehaviorConfig) NPCDecision {
		state.Step++
		d := NPCDecision{State: state, NextDecisionTick: observation.Tick + 1}
		if observation.LastResult == contracts.NPCExecutionCompleted {
			d.State.RandomState = uint64(observation.Ability.Status)
			d.Intent.Kind = contracts.NPCIntentWait
			return d
		}
		switch state.Step {
		case 2:
			d.Intent = NPCMoveTo(contracts.NPCPoint{X: 700, Y: 700}, observation.Tick+10, 0)
		case 3:
			d.Intent.Kind = contracts.NPCIntentStop
		default:
			d.Intent = contracts.NPCIntent{Kind: contracts.NPCIntentUseAbility, Ability: contracts.NPCAbilityRequest{
				AbilityID: "test_charge", Target: contracts.NPCTarget{Kind: contracts.NPCTargetPoint, Point: contracts.NPCPoint{X: 450, Y: 300}},
			}}
		}
		return d
	})
	catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_charge", TargetKind: contracts.NPCTargetPoint, OwnsMovement: true}})
	require.NoError(t, err)
	npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_charge"}})
	f := newNPCExtensionWorld(t, catalog)
	h := f.spawn(t, 100, 1001)
	actor := npcExtensionRef(f.world, h)
	brain := f.tick(t, h, 1)
	require.Equal(t, contracts.NPCExecutionAccepted, brain.LastResult)
	require.Equal(t, uint64(contracts.NPCExecutionAccepted), brain.RandomState)
	require.Equal(t, components.NPCMovementAbility, brain.MovementLease.Owner)
	ownedMovement, _ := ecs.GetComponent[components.Movement](f.world, h)
	for tick := uint64(2); tick <= 4; tick++ {
		brain = f.tick(t, h, tick)
		require.Equal(t, contracts.NPCExecutionBusy, brain.LastResult)
		require.Equal(t, uint64(contracts.NPCExecutionBusy), brain.RandomState)
		movement, _ := ecs.GetComponent[components.Movement](f.world, h)
		require.Equal(t, ownedMovement, movement, "AI move/stop/restart cannot overwrite ability-owned movement")
	}
	require.Len(t, f.executor.starts, 1, "running execution is not restarted by repeated AI decisions")
	for _, phase := range []contracts.NPCAbilityPhase{contracts.NPCAbilityActive, contracts.NPCAbilityRecovery, contracts.NPCAbilityFinished} {
		state := f.executor.advance(t, actor)
		require.Equal(t, phase, state.Phase)
		unchanged, _ := ecs.GetComponent[components.NPCBrain](f.world, h)
		require.Equal(t, brain, unchanged, "executor progression does not require an AI tick or mutate its memory")
	}
	internal, _ := ecs.GetComponent[components.ObjectInternalState](f.world, h)
	require.Equal(t, float64(110), internal.HP)
	require.False(t, internal.IsDirty, "the fake executor implements no gameplay effects")
	ecs.GetResource[ecs.TimeState](f.world).Tick = 5
	ecs.AddComponent(f.world, h, components.CollisionResult{FinalX: 312, FinalY: 300})
	ecs.GetResource[ecs.MovedEntities](f.world).Add(h, 312, 300)
	NewNPCMovementGuard(f.controller).Update(f.world, .1)
	stopped, _ := ecs.GetComponent[components.Movement](f.world, h)
	position, _ := ecs.GetComponent[components.CollisionResult](f.world, h)
	guardedBrain, _ := ecs.GetComponent[components.NPCBrain](f.world, h)
	require.Equal(t, constt.StateIdle, stopped.State, "finished execution stops before Transform even when AI has not run")
	require.True(t, stopped.Direction.UpdatePending)
	require.Equal(t, float64(300), position.FinalX)
	require.Equal(t, brain.NPCPolicyState, guardedBrain.NPCPolicyState, "the movement guard does not advance the decision policy")
	require.Equal(t, brain.Ability, guardedBrain.Ability, "terminal feedback remains available to the next policy decision")
	require.Empty(t, f.executor.cancels, "completion must not be replaced by a redundant cancellation")
	brain = f.tick(t, h, 5)
	require.Equal(t, contracts.NPCExecutionCompleted, brain.LastResult)
	require.Equal(t, uint64(contracts.NPCExecutionCompleted), brain.RandomState, "the policy observes completion on its next decision")
	require.Zero(t, brain.Ability.Generation)
	require.Equal(t, components.NPCMovementUnowned, brain.MovementLease.Owner)
	require.Len(t, f.executor.starts, 1)
}

func TestNPCAbilityMovementIgnoresRetiredAIRouteTarget(t *testing.T) {
	profile := npcExtensionProfile("test_movement_transfer", func(state components.NPCPolicyState, observation NPCObservation, _ contracts.NPCBehaviorConfig) NPCDecision {
		state.Step++
		d := NPCDecision{State: state, NextDecisionTick: observation.Tick + 1}
		if state.Step == 1 {
			d.Intent = NPCChase(observation.Target, observation.Tick+10)
		} else {
			d.Intent = contracts.NPCIntent{Kind: contracts.NPCIntentUseAbility, Ability: contracts.NPCAbilityRequest{
				AbilityID: "test_charge", Target: contracts.NPCTarget{Kind: contracts.NPCTargetPoint, Point: contracts.NPCPoint{X: 450, Y: 300}},
			}}
		}
		return d
	})
	catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_charge", TargetKind: contracts.NPCTargetPoint, OwnsMovement: true}})
	require.NoError(t, err)
	npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_charge"}})
	f := newNPCExtensionWorld(t, catalog)
	h := f.spawn(t, 100, 1001)
	actor := npcExtensionRef(f.world, h)
	target := f.world.Spawn(200, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 306, Y: 300})
	})
	targetRef := npcExtensionRef(f.world, target)
	require.True(t, f.controller.SetTarget(actor, targetRef))
	chasing := f.tick(t, h, 1)
	require.Equal(t, contracts.NPCExecutionAccepted, chasing.LastResult)
	require.Equal(t, components.NPCMovementAI, chasing.MovementLease.Owner)
	require.Equal(t, targetRef, chasing.Target)
	charging := f.tick(t, h, 2)
	require.Equal(t, contracts.NPCExecutionAccepted, charging.LastResult)
	require.Equal(t, components.NPCMovementAbility, charging.MovementLease.Owner)
	require.Equal(t, targetRef, charging.Target, "AI target memory is separate from the ability's point target")
	require.True(t, f.world.Despawn(target))
	ecs.GetResource[ecs.TimeState](f.world).Tick = 3
	ecs.AddComponent(f.world, h, components.CollisionResult{FinalX: 312, FinalY: 300})
	ecs.GetResource[ecs.MovedEntities](f.world).Add(h, 312, 300)
	NewNPCMovementGuard(f.controller).Update(f.world, .1)
	result, _ := ecs.GetComponent[components.CollisionResult](f.world, h)
	movement, _ := ecs.GetComponent[components.Movement](f.world, h)
	brain, _ := ecs.GetComponent[components.NPCBrain](f.world, h)
	require.Equal(t, float64(312), result.FinalX, "a retired AI route target cannot block the ability's movement")
	require.Equal(t, constt.StateMoving, movement.State)
	require.Equal(t, float64(450), movement.TargetX)
	require.Equal(t, charging, brain)
	require.True(t, f.executor.State(actor, charging.Ability.Generation).Running())
	require.Empty(t, f.executor.cancels)
}

func npcExtensionAbilityProfile(id, ability string, target func() contracts.NPCTarget) NPCProfile {
	return npcExtensionProfile(id, func(state components.NPCPolicyState, observation NPCObservation, _ contracts.NPCBehaviorConfig) NPCDecision {
		state.Step++
		return NPCDecision{State: state, NextDecisionTick: observation.Tick + 1,
			Intent: contracts.NPCIntent{Kind: contracts.NPCIntentUseAbility, Ability: contracts.NPCAbilityRequest{AbilityID: ability, Target: target()}},
		}
	})
}

func TestNPCControllerWorldIsolationGenerationsAndDespawn(t *testing.T) {
	profile := npcExtensionAbilityProfile("test_isolation", "test_pulse", func() contracts.NPCTarget { return contracts.NPCTarget{} })
	catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_pulse"}})
	require.NoError(t, err)
	npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_pulse"}})
	a, b := newNPCExtensionWorld(t, catalog), newNPCExtensionWorld(t, catalog)
	ha, hb := a.spawn(t, 100, 1001), b.spawn(t, 100, 1001)
	actorA, actorB := npcExtensionRef(a.world, ha), npcExtensionRef(b.world, hb)
	require.Equal(t, actorA, actorB, "the test deliberately reuses both identities across Worlds")
	beforeB, _ := ecs.GetComponent[components.NPCBrain](b.world, hb)
	first := a.tick(t, ha, 1)
	untouchedB, _ := ecs.GetComponent[components.NPCBrain](b.world, hb)
	require.Equal(t, beforeB, untouchedB)
	require.Empty(t, b.executor.starts)
	other := b.tick(t, hb, 1)
	require.Equal(t, contracts.NPCExecutionAccepted, other.LastResult)
	require.Equal(t, contracts.NPCExecutionRejected, a.controller.CancelAbility(actorA, first.Ability.Generation+1, contracts.NPCRetireTimeout).Status)
	require.Empty(t, a.executor.cancels)
	require.Equal(t, contracts.NPCExecutionCanceled, a.controller.CancelAbility(actorA, first.Ability.Generation, contracts.NPCRetireTimeout).Status)
	second := a.tick(t, ha, 2)
	require.Greater(t, second.Ability.Generation, first.Ability.Generation)
	require.Equal(t, contracts.NPCExecutionRejected, a.controller.CancelAbility(actorA, first.Ability.Generation, contracts.NPCRetireTimeout).Status)
	require.Len(t, a.executor.cancels, 1, "a stale cancellation must not reach the current execution")
	require.True(t, a.executor.State(actorA, second.Ability.Generation).Running())
	require.True(t, b.executor.State(actorB, other.Ability.Generation).Running())
	require.Empty(t, b.executor.cancels)
	require.True(t, a.world.Despawn(ha))
	require.Len(t, a.executor.cancels, 2)
	require.Equal(t, npcExtensionCancellation{actorA, second.Ability.Generation, contracts.NPCRetireDespawn}, a.executor.cancels[1])
	require.Equal(t, contracts.NPCExecutionCanceled, a.executor.states[actorA].Status)
	replacement := a.spawn(t, 100, 1001)
	require.NotEqual(t, ha, replacement)
	replacementBrain, _ := ecs.GetComponent[components.NPCBrain](a.world, replacement)
	require.Zero(t, replacementBrain.Ability.Generation)
	require.Equal(t, contracts.NPCExecutionRejected, a.controller.CancelAbility(actorA, second.Ability.Generation, contracts.NPCRetireTimeout).Status)
	_, err = a.behavior.(contracts.ScheduledTickBehavior).OnScheduledTick(&contracts.BehaviorTickContext{
		World: a.world, Handle: ha, EntityID: actorA.EntityID, EntityType: 1001, CurrentTick: 3,
	})
	require.NoError(t, err)
	unchanged, _ := ecs.GetComponent[components.NPCBrain](a.world, replacement)
	require.Equal(t, replacementBrain, unchanged, "old callbacks cannot mutate a replacement actor")
	require.Len(t, a.executor.cancels, 2)
	require.True(t, b.executor.State(actorB, other.Ability.Generation).Running())
}

func TestNPCExecutorAdmissionFeedbackReachesProfile(t *testing.T) {
	for _, outcome := range []contracts.NPCExecutionStatus{contracts.NPCExecutionBusy, contracts.NPCExecutionRejected} {
		t.Run(fmt.Sprintf("status_%d", outcome), func(t *testing.T) {
			profile := npcExtensionAbilityProfile("test_admission", "test_pulse", func() contracts.NPCTarget { return contracts.NPCTarget{} })
			catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_pulse"}})
			require.NoError(t, err)
			npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_pulse"}})
			f := newNPCExtensionWorld(t, catalog)
			h := f.spawn(t, 100, 1001)
			f.executor.nextOutcome = outcome
			brain := f.tick(t, h, 1)
			require.Equal(t, outcome, brain.LastResult)
			require.Equal(t, uint64(outcome), brain.RandomState)
			require.Zero(t, brain.Ability.Generation)
			require.Len(t, f.executor.starts, 1)
			brain = f.tick(t, h, 2)
			require.Equal(t, contracts.NPCExecutionAccepted, brain.LastResult)
			require.Greater(t, brain.Ability.Generation, f.executor.starts[0].Generation)
		})
	}
}

func TestNPCInvalidEntityTargetsAreRejectedBeforeExecution(t *testing.T) {
	for _, invalidation := range []string{"removed", "reused", "wrong_entity_id"} {
		t.Run(invalidation, func(t *testing.T) {
			var target contracts.EntityRef
			profile := npcExtensionAbilityProfile("test_target", "test_mark", func() contracts.NPCTarget {
				return contracts.NPCTarget{Kind: contracts.NPCTargetEntity, Entity: target}
			})
			catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_mark", TargetKind: contracts.NPCTargetEntity}})
			require.NoError(t, err)
			npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_mark"}})
			f := newNPCExtensionWorld(t, catalog)
			h := f.spawn(t, 100, 1001)
			target = npcExtensionRef(f.world, f.world.Spawn(200, nil))
			switch invalidation {
			case "removed":
				require.True(t, f.world.Despawn(target.Handle))
			case "reused":
				require.True(t, f.world.Despawn(target.Handle))
				replacement := f.world.Spawn(target.EntityID, nil)
				require.NotEqual(t, target.Handle, replacement)
			case "wrong_entity_id":
				target.EntityID++
			}
			require.False(t, f.controller.SetTarget(npcExtensionRef(f.world, h), target))
			brain := f.tick(t, h, 1)
			require.Equal(t, contracts.NPCExecutionRejected, brain.LastResult)
			require.Equal(t, uint64(contracts.NPCExecutionRejected), brain.RandomState)
			require.Empty(t, f.executor.starts, "invalid target must not reach the executor boundary")
		})
	}
}

func TestNPCRunningAbilityRetiresWhenTargetIsReplaced(t *testing.T) {
	var target contracts.EntityRef
	profile := npcExtensionAbilityProfile("test_target_lifetime", "test_mark", func() contracts.NPCTarget {
		return contracts.NPCTarget{Kind: contracts.NPCTargetEntity, Entity: target}
	})
	catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_mark", TargetKind: contracts.NPCTargetEntity}})
	require.NoError(t, err)
	npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_mark"}})
	f := newNPCExtensionWorld(t, catalog)
	h := f.spawn(t, 100, 1001)
	target = npcExtensionRef(f.world, f.world.Spawn(200, nil))
	first := f.tick(t, h, 1)
	require.Equal(t, contracts.NPCExecutionAccepted, first.LastResult)
	require.True(t, f.world.Despawn(target.Handle))
	f.world.Spawn(target.EntityID, nil)
	brain := f.tick(t, h, 2)
	require.Equal(t, contracts.NPCExecutionRejected, brain.LastResult)
	require.Zero(t, brain.Ability.Generation)
	require.Len(t, f.executor.starts, 1)
	require.Equal(t, []npcExtensionCancellation{{npcExtensionRef(f.world, h), first.Ability.Generation, contracts.NPCRetireInvalidTarget}}, f.executor.cancels)
}

func TestNPCExtensionCatalogFailsFastDuringDefinitionLoading(t *testing.T) {
	profile := npcExtensionAbilityProfile("test_catalog", "test_pulse", func() contracts.NPCTarget { return contracts.NPCTarget{} })
	catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_pulse"}})
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		change func(*contracts.NPCBehaviorConfig)
		want   string
	}{
		{"unknown_profile", func(c *contracts.NPCBehaviorConfig) { c.Profile = "missing_profile" }, "unknown npc_ai profile"},
		{"unknown_capability", func(c *contracts.NPCBehaviorConfig) { c.Capabilities = []string{"missing_capability"} }, "unknown NPC capability"},
		{"missing_required_capability", func(c *contracts.NPCBehaviorConfig) { c.Capabilities = []string{"locomotion"} }, "requires capability"},
		{"unknown_ability", func(c *contracts.NPCBehaviorConfig) { c.Abilities = []string{"missing_ability"} }, "unknown NPC ability"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_pulse"}}
			test.change(&cfg)
			_, err := npcExtensionLoad(t, catalog, cfg)
			require.ErrorContains(t, err, test.want)
		})
	}
	_, err = NewNPCCatalog([]NPCProfile{profile, profile}, nil, nil)
	require.ErrorContains(t, err, "duplicate NPC profile")
	_, err = NewNPCCatalog(nil, []NPCCapability{{ID: "locomotion", Move: true}}, nil)
	require.ErrorContains(t, err, "duplicate NPC capability")
	_, err = NewNPCCatalog(nil, nil, []contracts.NPCAbilityDefinition{{ID: "test_pulse"}, {ID: "test_pulse"}})
	require.ErrorContains(t, err, "duplicate NPC ability")
}
