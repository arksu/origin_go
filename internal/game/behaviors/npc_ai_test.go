package behaviors

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func npcTestConfig() *contracts.NPCBehaviorConfig {
	return &contracts.NPCBehaviorConfig{Profile: "passive_wander", Speed: 32, IdleMinSeconds: 8, IdleMaxSeconds: 16, WanderMinTiles: 1, WanderMaxTiles: 3, HomeRadiusTiles: 6, MaxWalkSeconds: 3}
}

func TestPassiveWanderDeterministicAndIndependent(t *testing.T) {
	cfg := *npcTestConfig()
	initial := components.NPCPolicyState{RandomState: 12345}
	obs := NPCObservation{Position: contracts.NPCPoint{X: 100, Y: 100}, Home: contracts.NPCPoint{X: 100, Y: 100}, Tick: 10, NextDecisionTick: 10, TicksPerSecond: 10}
	a := decidePassiveWander(initial, obs, cfg)
	require.Equal(t, a, decidePassiveWander(initial, obs, cfg))
	require.Equal(t, uint64(12345), initial.RandomState)
	require.Equal(t, contracts.NPCIntentMove, a.Intent.Kind)
	require.Equal(t, components.NPCWander, a.State.Activity)
	require.Equal(t, uint64(40), a.NextDecisionTick)
	require.Equal(t, uint64(40), a.Intent.Move.DeadlineTick)
	for _, p := range a.Intent.Move.Candidates {
		require.InDelta(t, 24, math.Hypot(p.X-100, p.Y-100), 12.8)
	}
	other := initial
	other.RandomState++
	require.NotEqual(t, a.Intent.Move.Candidates, decidePassiveWander(other, obs, cfg).Intent.Move.Candidates)
	obs.Tick = 11
	obs.Moving = true
	obs.ActivityDeadlineTick = 40
	require.Equal(t, a.State, decidePassiveWander(a.State, obs, cfg).State)
	obs.Tick = 40
	resting := decidePassiveWander(a.State, obs, cfg)
	require.Equal(t, contracts.NPCIntentStop, resting.Intent.Kind)
	require.Equal(t, components.NPCIdle, resting.State.Activity)
	require.GreaterOrEqual(t, resting.NextDecisionTick, uint64(120))
	require.LessOrEqual(t, resting.NextDecisionTick, uint64(200))
	obs.Tick = 12
	obs.Moving = false
	require.Equal(t, contracts.NPCIntentStop, decidePassiveWander(a.State, obs, cfg).Intent.Kind)
	obs.NextDecisionTick = 20
	require.Equal(t, initial, decidePassiveWander(initial, obs, cfg).State, "early wake must not consume RNG or end idle")
	require.Equal(t, contracts.NPCIntentContinue, decidePassiveWander(initial, obs, cfg).Intent.Kind)
}

type npcTestChunks map[types.ChunkCoord]*core.Chunk

func (c npcTestChunks) GetChunkFast(coord types.ChunkCoord) *core.Chunk { return c[coord] }

func npcTestLand(t *testing.T, coord types.ChunkCoord) *core.Chunk {
	t.Helper()
	c := core.NewChunk(coord, 1, 0, constt.ChunkSize)
	tiles := make([]byte, constt.ChunkSize*constt.ChunkSize)
	for i := range tiles {
		tiles[i] = types.TileGrass
	}
	require.NoError(t, c.RestoreTiles(tiles, 0, 0))
	c.SetState(types.ChunkStateActive)
	return c
}

func TestNPCTerrainFootprintAndSweptBoundary(t *testing.T) {
	c := npcTestLand(t, types.ChunkCoord{})
	chunks := npcTestChunks{{}: c}
	box := components.Collider{HalfWidth: 2.4, HalfHeight: 3.6}
	require.True(t, npcTerrainAllowed(chunks, 18, 18, 30, 30, box))
	c.SetTile(2, 1, types.TileDeepWater)
	require.False(t, npcTerrainAllowed(chunks, 18, 18, 42, 18, box), "cannot cross water even with a dry endpoint")
	c.SetTile(2, 1, types.TileShallowWater)
	require.False(t, npcTerrainAllowed(chunks, 18, 18, 30, 18, box))
	c.SetTile(2, 1, types.TileGrass)
	require.False(t, npcTerrainAllowed(chunks, 1530, 18, 1535, 18, box), "footprint cannot enter an unloaded neighbor")
	neighbor := npcTestLand(t, types.ChunkCoord{X: 1})
	chunks[types.ChunkCoord{X: 1}] = neighbor
	require.True(t, npcTerrainAllowed(chunks, 1530, 18, 1535, 18, box))
	neighbor.SetState(types.ChunkStatePreloaded)
	require.False(t, npcTerrainAllowed(chunks, 1530, 18, 1535, 18, box))
	require.False(t, npcTerrainAllowed(chunks, 18, 18, 10000000, 18, box), "bounded work fails closed")
	require.False(t, npcTerrainAllowed(chunks, 18, 18, math.NaN(), 18, box))
	negative := npcTestLand(t, types.ChunkCoord{X: -1, Y: -1})
	chunks[negative.Coord] = negative
	require.True(t, npcTerrainAllowed(chunks, -18, -18, -30, -30, box))
}

func npcFixture(t *testing.T) (*ecs.World, types.Handle, npcTestChunks) {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 42, Key: "fox", NPCConfig: npcTestConfig(), Behaviors: map[string]json.RawMessage{npcAIKey: json.RawMessage(`{}`)}}}))
	w := ecs.NewWorldForTesting()
	ts := ecs.GetResource[ecs.TimeState](w)
	ts.TickPeriod = 100 * time.Millisecond
	chunks := npcTestChunks{{}: npcTestLand(t, types.ChunkCoord{})}
	_, err := AttachNPCController(w, NPCControllerConfig{Chunks: chunks})
	require.NoError(t, err)
	h := w.Spawn(100, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 42, Transient: true, Behaviors: []string{npcAIKey}})
		ecs.AddComponent(w, h, components.Transform{X: 300, Y: 300})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 2.4, HalfHeight: 3.6})
		ecs.AddComponent(w, h, components.ObjectInternalState{HP: 110, HasHP: true})
	})
	require.NoError(t, (npcAIBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 100, EntityType: 42, Reason: contracts.ObjectBehaviorInitReasonSpawn}))
	return w, h, chunks
}

func TestNPCInitAndTickRemainTransient(t *testing.T) {
	w, h, chunks := npcFixture(t)
	brain, _ := ecs.GetComponent[components.NPCBrain](w, h)
	require.Equal(t, components.NPCIdle, brain.Activity)
	require.InDelta(t, 300, brain.HomeX, 0)
	require.NoError(t, (npcAIBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 100, EntityType: 42, Reason: contracts.ObjectBehaviorInitReasonRestore}))
	same, _ := ecs.GetComponent[components.NPCBrain](w, h)
	require.Equal(t, brain, same)
	ctx := contracts.BehaviorTickContext{World: w, Handle: h, EntityID: 100, EntityType: 42, CurrentTick: brain.NextDecisionTick, Deps: &contracts.ExecutionDeps{Chunks: chunks}}
	result, err := (npcAIBehavior{}).OnScheduledTick(&ctx)
	require.NoError(t, err)
	require.False(t, result.StateChanged)
	brain, _ = ecs.GetComponent[components.NPCBrain](w, h)
	require.Equal(t, components.NPCWander, brain.Activity)
	movement, _ := ecs.GetComponent[components.Movement](w, h)
	require.Equal(t, constt.TargetPoint, movement.TargetType)
	internal, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.False(t, internal.IsDirty)
	require.NoError(t, (npcAIBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 100, EntityType: 42, PreviousType: 42, Reason: contracts.ObjectBehaviorInitReasonTransform}))
	unchanged, _ := ecs.GetComponent[components.NPCBrain](w, h)
	unchangedMovement, _ := ecs.GetComponent[components.Movement](w, h)
	require.Equal(t, brain, unchanged, "same-type refresh preserves current activity and home")
	require.Equal(t, movement, unchangedMovement)
	_, hasVision := ecs.GetComponent[components.Vision](w, h)
	require.False(t, hasVision)
	// A delayed callback for a destroyed generational handle cannot affect replacement.
	w.Despawn(h)
	replacement := w.Spawn(100, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.NPCBrain{NPCPolicyState: components.NPCPolicyState{RandomState: 999}})
	})
	_, err = (npcAIBehavior{}).OnScheduledTick(&ctx)
	require.NoError(t, err)
	replacementBrain, _ := ecs.GetComponent[components.NPCBrain](w, replacement)
	require.Equal(t, uint64(999), replacementBrain.RandomState)
}

func TestNPCMovementGuardHardDeadlineAndNetworkStop(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "water", true: "deadline"}[expired], func(t *testing.T) {
			w, h, chunks := npcFixture(t)
			brain, _ := ecs.GetComponent[components.NPCBrain](w, h)
			brain.Activity = components.NPCWander
			brain.ActivityDeadlineTick = 20
			brain.MovementLease = components.NPCMovementLease{Owner: components.NPCMovementAI, DeadlineTick: 20, HomeRadius: 72}
			ecs.AddComponent(w, h, brain)
			movement, _ := ecs.GetComponent[components.Movement](w, h)
			movement.SetTargetPoint(400, 300)
			ecs.AddComponent(w, h, movement)
			ecs.AddComponent(w, h, components.CollisionResult{FinalX: 312, FinalY: 300})
			ecs.GetResource[ecs.TimeState](w).Tick = 10
			if expired {
				ecs.GetResource[ecs.TimeState](w).Tick = 20
			} else {
				chunks[types.ChunkCoord{}].SetTile(26, 25, types.TileShallowWater)
			}
			ecs.GetResource[ecs.MovedEntities](w).Add(h, 312, 300)
			controller, _ := GetNPCController(w)
			guard := NewNPCMovementGuard(controller)
			guard.Update(w, .1)
			movement, _ = ecs.GetComponent[components.Movement](w, h)
			require.Equal(t, constt.StateIdle, movement.State)
			require.True(t, movement.Direction.UpdatePending, "Transform must publish the stop")
			result, _ := ecs.GetComponent[components.CollisionResult](w, h)
			require.Equal(t, float64(300), result.FinalX)
			// Repeated guard updates on prepared storage must not allocate.
			require.Zero(t, testing.AllocsPerRun(100, func() { guard.Update(w, .1) }))
		})
	}
}

func TestNPCMovementGuardWakesArrivalAndCollisionStop(t *testing.T) {
	for _, oscillation := range []bool{false, true} {
		t.Run(map[bool]string{false: "arrival", true: "collision"}[oscillation], func(t *testing.T) {
			w, h, _ := npcFixture(t)
			brain, _ := ecs.GetComponent[components.NPCBrain](w, h)
			brain.Activity, brain.ActivityDeadlineTick = components.NPCWander, 30
			brain.MovementLease = components.NPCMovementLease{Owner: components.NPCMovementAI, DeadlineTick: 30, HomeRadius: 72}
			ecs.AddComponent(w, h, brain)
			movement, _ := ecs.GetComponent[components.Movement](w, h)
			if oscillation {
				movement.SetTargetPoint(400, 300)
			}
			ecs.AddComponent(w, h, movement)
			ecs.AddComponent(w, h, components.CollisionResult{FinalX: 300, FinalY: 300, PerpendicularOscillation: oscillation})
			ecs.GetResource[ecs.TimeState](w).Tick = 10
			ecs.GetResource[ecs.MovedEntities](w).Add(h, 300, 300)
			controller, _ := GetNPCController(w)
			NewNPCMovementGuard(controller).Update(w, .1)
			keys := ecs.GetResource[ecs.BehaviorTickSchedule](w).PopDue(10, 1, nil)
			require.Equal(t, []ecs.BehaviorTickKey{{EntityID: 100, BehaviorKey: npcAIKey}}, keys, "rest starts at the physical stop, before the walk timeout")
		})
	}
}

func TestNPCConfigFailsFast(t *testing.T) {
	for _, change := range []func(*contracts.NPCBehaviorConfig){func(c *contracts.NPCBehaviorConfig) { c.Profile = "unknown" }, func(c *contracts.NPCBehaviorConfig) { c.Speed = 0 }, func(c *contracts.NPCBehaviorConfig) { c.IdleMinSeconds = 100 }, func(c *contracts.NPCBehaviorConfig) { c.HomeRadiusTiles = 1 }, func(c *contracts.NPCBehaviorConfig) { c.MaxWalkSeconds = 100 }} {
		cfg := npcTestConfig()
		change(cfg)
		raw, err := json.Marshal(cfg)
		require.NoError(t, err)
		dynamic := false
		_, err = (npcAIBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{RawConfig: raw, Def: &objectdefs.ObjectDef{Static: &dynamic, Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 4.8, H: 7.2}}}})
		require.Error(t, err)
	}
}

func TestNPCControllerRequiresExplicitDependencies(t *testing.T) {
	_, err := AttachNPCController(nil, NPCControllerConfig{})
	require.Error(t, err)
	w := ecs.NewWorldForTesting()
	_, err = AttachNPCController(w, NPCControllerConfig{})
	require.Error(t, err)
	var nilChunks npcTestChunks
	_, err = AttachNPCController(w, NPCControllerConfig{Chunks: nilChunks})
	require.Error(t, err)
	chunks := npcTestChunks{{}: npcTestLand(t, types.ChunkCoord{})}
	controller, err := AttachNPCController(w, NPCControllerConfig{Chunks: chunks})
	require.NoError(t, err)
	require.Same(t, controller, ecs.GetResource[NPCController](w))
	require.Same(t, controller, ecs.GetResource[contracts.NPCLifecycle](w).Controller)
	_, err = AttachNPCController(w, NPCControllerConfig{Chunks: chunks})
	require.Error(t, err)
	catalog, err := NewNPCCatalog(nil, nil, []contracts.NPCAbilityDefinition{{ID: "test"}})
	require.NoError(t, err)
	_, err = AttachNPCController(ecs.NewWorldForTesting(), NPCControllerConfig{Chunks: chunks, Catalog: catalog})
	require.Error(t, err, "advertised abilities require an executor before any spawn")
}

func TestNPCBehaviorRejectsMissingControllerEvenWithLegacyDeps(t *testing.T) {
	_, _, chunks := npcFixture(t) // Install a valid definition only.
	w := ecs.NewWorldForTesting()
	h := w.Spawn(100, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 42, Behaviors: []string{npcAIKey}})
		ecs.AddComponent(w, h, components.Transform{X: 300, Y: 300})
	})
	err := (npcAIBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 100, EntityType: 42})
	require.ErrorContains(t, err, "explicitly attached")
	_, err = (npcAIBehavior{}).OnScheduledTick(&contracts.BehaviorTickContext{World: w, Handle: h, EntityID: 100, EntityType: 42, Deps: &contracts.ExecutionDeps{Chunks: chunks}})
	require.ErrorContains(t, err, "explicitly attached")
	_, exists := GetNPCController(w)
	require.False(t, exists, "the global adapter must never install a controller lazily")
}

func TestNPCMoveIntentBoundsAndGenerationExhaustion(t *testing.T) {
	w, h, chunks := npcFixture(t)
	controller, _ := GetNPCController(w)
	brain, _ := ecs.GetComponent[components.NPCBrain](w, h)
	obs := controller.observe(h, 100, brain, 1)
	valid := NPCMoveTo(contracts.NPCPoint{X: 320, Y: 300}, 20, 72)
	cases := []contracts.NPCMoveIntent{valid.Move, valid.Move, valid.Move, valid.Move}
	cases[0].Count = 5
	cases[1].DeadlineTick = 1
	cases[2].HomeRadius = math.NaN()
	cases[3].DeadlineTick = 10000
	for _, intent := range cases {
		require.Equal(t, contracts.NPCExecutionRejected, controller.move(h, &brain, obs, intent).Status)
	}
	// Exactly four candidates are available and execution chooses the first valid one.
	chunks[types.ChunkCoord{}].SetTile(26, 25, types.TileShallowWater)
	candidates := valid.Move
	candidates.Count = 4
	candidates.Candidates = [4]contracts.NPCPoint{{X: 320, Y: 300}, {X: 330, Y: 300}, {X: 340, Y: 300}, {X: 350, Y: 300}}
	require.Equal(t, contracts.NPCExecutionAccepted, controller.move(h, &brain, obs, candidates).Status)
	movement, _ := ecs.GetComponent[components.Movement](w, h)
	require.Equal(t, float64(330), movement.TargetX)
	before := movement
	controller.generation = math.MaxUint64
	valid.Move.Candidates[0] = contracts.NPCPoint{X: 330, Y: 300}
	require.Equal(t, contracts.NPCExecutionRejected, controller.move(h, &brain, obs, valid.Move).Status)
	movement, _ = ecs.GetComponent[components.Movement](w, h)
	require.Equal(t, before, movement, "exhausted generations cannot mutate the current route")
}

func TestNPCSharedChaseAndFleeHelpersArePure(t *testing.T) {
	target := NPCObservedEntity{Ref: contracts.EntityRef{Handle: 2, EntityID: 22}, Position: contracts.NPCPoint{X: 20, Y: 10}, Valid: true}
	chase := NPCChase(target, 50)
	require.Equal(t, target.Ref, chase.Move.Target)
	require.Equal(t, target.Position, chase.Move.Candidates[0])
	require.Zero(t, chase.Move.HomeRadius)
	flee := NPCFlee(contracts.NPCPoint{X: 10, Y: 10}, target, 12, 50)
	require.Equal(t, contracts.NPCPoint{X: -2, Y: 10}, flee.Move.Candidates[0])
	require.Zero(t, flee.Move.HomeRadius)
	require.Equal(t, contracts.NPCIntentStop, NPCChase(NPCObservedEntity{}, 50).Kind)
	require.Equal(t, contracts.NPCIntentStop, NPCFlee(target.Position, target, 12, 50).Kind)
}

type npcTestParameters struct {
	Distance float64 `json:"distance"`
}

func (npcTestParameters) NPCProfileParameters() {}
func TestNPCProfileParametersCompileOnceAndCatalogCopiesRegistrations(t *testing.T) {
	compiled := 0
	caps := []string{"locomotion"}
	profile := NPCProfile{ID: "configured", Capabilities: caps,
		CompileParams: func(raw json.RawMessage) (contracts.NPCProfileParameters, error) {
			compiled++
			var p npcTestParameters
			err := decodeStrictJSON(raw, &p)
			return p, err
		}, Decide: func(state components.NPCPolicyState, obs NPCObservation, cfg contracts.NPCBehaviorConfig) NPCDecision {
			p := cfg.Parameters.(npcTestParameters)
			return NPCDecision{State: state, Intent: NPCMoveTo(contracts.NPCPoint{X: obs.Position.X + p.Distance, Y: obs.Position.Y}, obs.Tick+10, 0), NextDecisionTick: obs.Tick + 10}
		}}
	catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, nil)
	require.NoError(t, err)
	caps[0] = "unregistered"
	cfg := contracts.NPCBehaviorConfig{Profile: "configured", Speed: 32, Params: json.RawMessage(`{"distance":10}`)}
	require.NoError(t, catalog.validate(&cfg))
	require.Equal(t, []string{"locomotion"}, cfg.Capabilities)
	require.Equal(t, 1, compiled)
	for i := 0; i < 10; i++ {
		d := catalog.profiles[cfg.Profile].Decide(components.NPCPolicyState{}, NPCObservation{Tick: uint64(i)}, cfg)
		require.Equal(t, float64(10), d.Intent.Move.Candidates[0].X)
	}
	require.Equal(t, 1, compiled)
	cfg.Params = json.RawMessage(`{"unknown":10}`)
	require.Error(t, catalog.validate(&cfg))
}

func TestNPCMovementGuardPreservesStationaryStopNotification(t *testing.T) {
	w, h, _ := npcFixture(t)
	c, _ := GetNPCController(w)
	brain, _ := ecs.GetComponent[components.NPCBrain](w, h)
	brain.LastResult = contracts.NPCExecutionCompleted
	ecs.AddComponent(w, h, brain)
	movement, _ := ecs.GetComponent[components.Movement](w, h)
	movement.Direction.UpdatePending = true
	ecs.AddComponent(w, h, movement)
	ecs.GetResource[ecs.MovedEntities](w).Add(h, 300, 300)
	NewNPCMovementGuard(c).Update(w, .1)
	actual, _ := ecs.GetComponent[components.NPCBrain](w, h)
	require.Equal(t, brain, actual)
	movement, _ = ecs.GetComponent[components.Movement](w, h)
	require.True(t, movement.Direction.UpdatePending)
	require.Empty(t, ecs.GetResource[ecs.BehaviorTickSchedule](w).PopDue(1, 1, nil), "stationary stop must not wake idle policy")
}

func TestNPCFleeRetainsRememberedThreatAcrossDecisions(t *testing.T) {
	profile := npcExtensionProfile("test_remembered_flee", func(state components.NPCPolicyState, obs NPCObservation, _ contracts.NPCBehaviorConfig) NPCDecision {
		state.Step++
		return NPCDecision{State: state, NextDecisionTick: obs.Tick + 1, Intent: NPCFlee(obs.Position, obs.Target, 12, obs.Tick+10)}
	})
	catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, nil)
	require.NoError(t, err)
	npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32})
	f := newNPCExtensionWorld(t, catalog)
	actor := f.spawn(t, 100, 1001)
	threat := f.world.Spawn(200, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Transform{X: 330, Y: 300}) })
	threatRef := npcExtensionRef(f.world, threat)
	require.True(t, f.controller.SetTarget(npcExtensionRef(f.world, actor), threatRef))
	first := f.tick(t, actor, 1)
	require.Equal(t, contracts.NPCExecutionAccepted, first.LastResult)
	require.Equal(t, threatRef, first.Target, "point movement must preserve the remembered threat")
	require.Zero(t, first.MovementLease.Target, "fleeing to a point does not depend on a chase route target")
	movement, _ := ecs.GetComponent[components.Movement](f.world, actor)
	require.Equal(t, float64(288), movement.TargetX)
	// Observe a subsequent position as the real movement pipeline would apply it.
	ecs.AddComponent(f.world, actor, components.Transform{X: 288, Y: 300})
	second := f.tick(t, actor, 2)
	require.Equal(t, contracts.NPCExecutionAccepted, second.LastResult)
	require.Equal(t, threatRef, second.Target)
	require.Greater(t, second.MovementLease.Generation, first.MovementLease.Generation)
	require.Equal(t, uint32(2), second.Step)
	movement, _ = ecs.GetComponent[components.Movement](f.world, actor)
	require.Equal(t, float64(276), movement.TargetX, "the next decision still sees the live threat and continues fleeing")
	require.Equal(t, constt.StateMoving, movement.State)
}

func TestNPCMovementLeaseKeepsItsTargetAfterPerceptionChanges(t *testing.T) {
	profile := npcExtensionProfile("test_route_target", func(state components.NPCPolicyState, obs NPCObservation, _ contracts.NPCBehaviorConfig) NPCDecision {
		return NPCDecision{State: state, NextDecisionTick: obs.Tick + 1, Intent: NPCChase(obs.Target, obs.Tick+10)}
	})
	catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, nil)
	require.NoError(t, err)
	npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32})
	f := newNPCExtensionWorld(t, catalog)
	actor := f.spawn(t, 100, 1001)
	oldTarget := f.world.Spawn(200, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Transform{X: 340, Y: 300}) })
	newTarget := f.world.Spawn(201, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Transform{X: 360, Y: 300}) })
	actorRef, oldRef, newRef := npcExtensionRef(f.world, actor), npcExtensionRef(f.world, oldTarget), npcExtensionRef(f.world, newTarget)
	require.True(t, f.controller.SetTarget(actorRef, oldRef))
	brain := f.tick(t, actor, 1)
	require.Equal(t, oldRef, brain.MovementLease.Target)
	require.True(t, f.controller.SetTarget(actorRef, newRef))
	brain, _ = ecs.GetComponent[components.NPCBrain](f.world, actor)
	require.Equal(t, newRef, brain.Target)
	require.Equal(t, oldRef, brain.MovementLease.Target, "changing perception cannot retarget an already accepted route")
	require.True(t, f.world.Despawn(oldTarget))
	ecs.AddComponent(f.world, actor, components.CollisionResult{FinalX: 312, FinalY: 300})
	ecs.GetResource[ecs.MovedEntities](f.world).Add(actor, 312, 300)
	ecs.GetResource[ecs.TimeState](f.world).Tick = 2
	NewNPCMovementGuard(f.controller).Update(f.world, .1)
	brain, _ = ecs.GetComponent[components.NPCBrain](f.world, actor)
	require.Equal(t, contracts.NPCExecutionRejected, brain.LastResult)
	require.Equal(t, newRef, brain.Target, "rejecting the old route does not erase the new remembered target")
	require.Zero(t, brain.MovementLease)
	movement, _ := ecs.GetComponent[components.Movement](f.world, actor)
	require.Equal(t, constt.StateIdle, movement.State)
	result, _ := ecs.GetComponent[components.CollisionResult](f.world, actor)
	require.Equal(t, float64(300), result.FinalX, "the valid remembered target cannot shield a stale route target")
}

func TestNPCTerminalAbilitySurvivesTargetRemoval(t *testing.T) {
	for _, throughGuard := range []bool{false, true} {
		t.Run(map[bool]string{false: "decision", true: "movement_guard"}[throughGuard], func(t *testing.T) {
			profile := npcExtensionProfile("test_terminal_target", func(state components.NPCPolicyState, obs NPCObservation, _ contracts.NPCBehaviorConfig) NPCDecision {
				d := NPCDecision{State: state, NextDecisionTick: obs.Tick + 1}
				if obs.LastResult == contracts.NPCExecutionCompleted {
					d.Intent.Kind = contracts.NPCIntentWait
					return d
				}
				d.Intent = contracts.NPCIntent{Kind: contracts.NPCIntentUseAbility, Ability: contracts.NPCAbilityRequest{AbilityID: "test_entity", Target: contracts.NPCTarget{Kind: contracts.NPCTargetEntity, Entity: obs.Target.Ref}}}
				return d
			})
			catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_entity", TargetKind: contracts.NPCTargetEntity, OwnsMovement: true}})
			require.NoError(t, err)
			npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_entity"}})
			f := newNPCExtensionWorld(t, catalog)
			actor := f.spawn(t, 100, 1001)
			target := f.world.Spawn(200, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Transform{X: 340, Y: 300}) })
			actorRef := npcExtensionRef(f.world, actor)
			require.True(t, f.controller.SetTarget(actorRef, npcExtensionRef(f.world, target)))
			brain := f.tick(t, actor, 1)
			require.Equal(t, contracts.NPCExecutionAccepted, brain.LastResult)
			for range 3 {
				f.executor.advance(t, actorRef)
			}
			require.True(t, f.world.Despawn(target), "an ability effect may remove its own target")
			if throughGuard {
				movement, _ := ecs.GetComponent[components.Movement](f.world, actor)
				movement.SetTargetPoint(340, 300)
				ecs.AddComponent(f.world, actor, movement)
				ecs.AddComponent(f.world, actor, components.CollisionResult{FinalX: 312, FinalY: 300})
				ecs.GetResource[ecs.MovedEntities](f.world).Add(actor, 312, 300)
				ecs.GetResource[ecs.TimeState](f.world).Tick = 2
				NewNPCMovementGuard(f.controller).Update(f.world, .1)
				brain, _ = ecs.GetComponent[components.NPCBrain](f.world, actor)
				require.Equal(t, contracts.NPCExecutionCompleted, brain.LastResult)
				require.Empty(t, f.executor.cancels, "the guard must consume completion before validating the removed target")
				result, _ := ecs.GetComponent[components.CollisionResult](f.world, actor)
				require.Equal(t, float64(300), result.FinalX)
			}
			brain = f.tick(t, actor, 2)
			require.Equal(t, contracts.NPCExecutionCompleted, brain.LastResult)
			require.Zero(t, brain.Ability.Generation)
			require.Zero(t, brain.MovementLease)
			require.Empty(t, f.executor.cancels, "a matching terminal execution must never be canceled because its target vanished")
			require.Len(t, f.executor.starts, 1)
		})
	}
}

// Requested interruptions may be refused; forced lifecycle retirement still
// releases the execution through the normal fake executor contract.
type npcInterruptExecutor struct {
	*npcExtensionExecutor
	refuse bool
}

func (f *npcInterruptExecutor) Cancel(actor contracts.EntityRef, generation uint64, reason contracts.NPCRetireReason) contracts.NPCAbilityState {
	if reason == contracts.NPCRetireRequested && f.refuse {
		f.cancels = append(f.cancels, npcExtensionCancellation{actor, generation, reason})
		state := f.State(actor, generation)
		if state.Running() {
			state.Status = contracts.NPCExecutionBusy
		}
		return state
	}
	return f.npcExtensionExecutor.Cancel(actor, generation, reason)
}
func newNPCInterruptWorld(t *testing.T, catalog *NPCCatalog) (npcExtensionWorld, *npcInterruptExecutor) {
	t.Helper()
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).TickPeriod = 100 * time.Millisecond
	executor := &npcInterruptExecutor{npcExtensionExecutor: newNPCExtensionExecutor(w)}
	controller, err := AttachNPCController(w, NPCControllerConfig{Catalog: catalog, Abilities: executor, Chunks: npcTestChunks{{}: npcTestLand(t, types.ChunkCoord{})}})
	require.NoError(t, err)
	return npcExtensionWorld{world: w, controller: controller, executor: executor.npcExtensionExecutor, behavior: NewNPCAIBehavior(catalog)}, executor
}

func TestNPCRequestedCancellationPreservesExecutorResult(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(map[bool]string{false: "refused", true: "already_completed"}[completed], func(t *testing.T) {
			profile := npcExtensionAbilityProfile("test_requested_cancel", "test_charge", func() contracts.NPCTarget {
				return contracts.NPCTarget{Kind: contracts.NPCTargetPoint, Point: contracts.NPCPoint{X: 450, Y: 300}}
			})
			catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_charge", TargetKind: contracts.NPCTargetPoint, OwnsMovement: true}})
			require.NoError(t, err)
			npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_charge"}})
			f, executor := newNPCInterruptWorld(t, catalog)
			actor := f.spawn(t, 100, 1001)
			actorRef := npcExtensionRef(f.world, actor)
			before := f.tick(t, actor, 1)
			movementBefore, _ := ecs.GetComponent[components.Movement](f.world, actor)
			if completed {
				for range 3 {
					executor.advance(t, actorRef)
				}
			} else {
				executor.refuse = true
			}
			result := f.controller.CancelAbility(actorRef, before.Ability.Generation, contracts.NPCRetireRequested)
			after, _ := ecs.GetComponent[components.NPCBrain](f.world, actor)
			movementAfter, _ := ecs.GetComponent[components.Movement](f.world, actor)
			if completed {
				require.Equal(t, contracts.NPCExecutionCompleted, result.Status)
				require.Equal(t, contracts.NPCExecutionCompleted, after.LastResult)
				require.Zero(t, after.Ability.Generation)
				require.Zero(t, after.MovementLease)
				require.Equal(t, constt.StateIdle, movementAfter.State)
				require.Empty(t, executor.cancels, "completed execution must be observed without a cancellation call")
			} else {
				require.Equal(t, contracts.NPCExecutionBusy, result.Status)
				require.Equal(t, contracts.NPCExecutionBusy, after.LastResult)
				require.Equal(t, before.Ability, after.Ability)
				require.Equal(t, before.MovementLease, after.MovementLease)
				require.Equal(t, movementBefore, movementAfter)
				require.True(t, executor.State(actorRef, before.Ability.Generation).Running())
				require.Len(t, executor.cancels, 1)
				require.Equal(t, contracts.NPCRetireRequested, executor.cancels[0].reason)
			}
		})
	}
}

func TestNPCPolicyRequestsInterruptionBeforeFleeing(t *testing.T) {
	profile := npcExtensionProfile("test_interrupt_flee", func(state components.NPCPolicyState, obs NPCObservation, _ contracts.NPCBehaviorConfig) NPCDecision {
		state.Step++
		d := NPCDecision{State: state, NextDecisionTick: obs.Tick + 1}
		if state.Step == 1 {
			d.Intent = contracts.NPCIntent{Kind: contracts.NPCIntentUseAbility, Ability: contracts.NPCAbilityRequest{AbilityID: "test_charge", Target: contracts.NPCTarget{Kind: contracts.NPCTargetPoint, Point: contracts.NPCPoint{X: 450, Y: 300}}}}
		} else if obs.Ability.Running() {
			d.Intent = contracts.NPCIntent{Kind: contracts.NPCIntentCancelAbility, CancelGeneration: obs.Ability.Generation}
		} else {
			d.Intent = NPCFlee(obs.Position, obs.Target, 12, obs.Tick+10)
		}
		return d
	})
	profile.OnResult = func(state components.NPCPolicyState, obs NPCObservation, _ contracts.NPCBehaviorConfig, result contracts.NPCExecutionResult) NPCDecision {
		d := NPCDecision{State: state, NextDecisionTick: obs.Tick + 1}
		if result.Status == contracts.NPCExecutionCanceled {
			d.Intent = NPCFlee(obs.Position, obs.Target, 12, obs.Tick+10)
		}
		return d
	}
	catalog, err := NewNPCCatalog([]NPCProfile{profile}, nil, []contracts.NPCAbilityDefinition{{ID: "test_charge", TargetKind: contracts.NPCTargetPoint, OwnsMovement: true}})
	require.NoError(t, err)
	npcExtensionInstall(t, catalog, contracts.NPCBehaviorConfig{Profile: profile.ID, Speed: 32, Abilities: []string{"test_charge"}})
	f, executor := newNPCInterruptWorld(t, catalog)
	actor := f.spawn(t, 100, 1001)
	threat := f.world.Spawn(200, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Transform{X: 330, Y: 300}) })
	require.True(t, f.controller.SetTarget(npcExtensionRef(f.world, actor), npcExtensionRef(f.world, threat)))
	first := f.tick(t, actor, 1)
	ownedMovement, _ := ecs.GetComponent[components.Movement](f.world, actor)
	executor.refuse = true
	refused := f.tick(t, actor, 2)
	require.Equal(t, contracts.NPCExecutionBusy, refused.LastResult)
	require.Equal(t, first.Ability, refused.Ability)
	movement, _ := ecs.GetComponent[components.Movement](f.world, actor)
	require.Equal(t, ownedMovement, movement, "a refused policy interrupt cannot start fleeing")
	executor.refuse = false
	canceled := f.tick(t, actor, 3)
	require.Equal(t, contracts.NPCExecutionCanceled, canceled.LastResult)
	require.Zero(t, canceled.Ability.Generation)
	require.Equal(t, components.NPCMovementAI, canceled.MovementLease.Owner)
	movement, _ = ecs.GetComponent[components.Movement](f.world, actor)
	require.Equal(t, float64(288), movement.TargetX, "an accepted policy interrupt may transition directly to a flee intent")
	require.Len(t, executor.starts, 1)
	require.Len(t, executor.cancels, 2)
	for _, cancel := range executor.cancels {
		require.Equal(t, contracts.NPCRetireRequested, cancel.reason)
		require.Equal(t, first.Ability.Generation, cancel.generation)
	}
}
