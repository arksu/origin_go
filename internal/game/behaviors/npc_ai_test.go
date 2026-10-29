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
	cfg := npcTestConfig()
	initial := components.NPCBrain{HomeX: 100, HomeY: 100, DeadlineTick: 10, RandomState: 12345}
	obs := npcObservation{X: 100, Y: 100, Tick: 10}
	a := decidePassiveWander(initial, obs, cfg, 10)
	require.Equal(t, a, decidePassiveWander(initial, obs, cfg, 10))
	require.Equal(t, uint64(12345), initial.RandomState, "the observation/decision boundary must not mutate shared inputs")
	require.True(t, a.StartWalk)
	require.Equal(t, components.NPCWander, a.Brain.Activity)
	require.Equal(t, uint64(40), a.Brain.DeadlineTick)
	for _, p := range a.Candidates {
		distance := math.Hypot(p.X-100, p.Y-100)
		require.InDelta(t, 24, distance, 12.8)
	}
	other := initial
	other.RandomState++
	require.NotEqual(t, a.Candidates, decidePassiveWander(other, obs, cfg, 10).Candidates)
	obs.Tick = 11
	obs.Moving = true
	require.Equal(t, a.Brain, decidePassiveWander(a.Brain, obs, cfg, 10).Brain)
	obs.Tick = 40
	resting := decidePassiveWander(a.Brain, obs, cfg, 10)
	require.True(t, resting.Stop)
	require.Equal(t, components.NPCIdle, resting.Brain.Activity)
	require.GreaterOrEqual(t, resting.Brain.DeadlineTick, uint64(120))
	require.LessOrEqual(t, resting.Brain.DeadlineTick, uint64(200))
	obs.Tick = 12
	obs.Moving = false
	require.True(t, decidePassiveWander(a.Brain, obs, cfg, 10).Stop, "arrival also starts a fresh rest interval")
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
	h := w.Spawn(100, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 42, Transient: true, Behaviors: []string{npcAIKey}})
		ecs.AddComponent(w, h, components.Transform{X: 300, Y: 300})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 2.4, HalfHeight: 3.6})
		ecs.AddComponent(w, h, components.ObjectInternalState{HP: 110, HasHP: true})
	})
	require.NoError(t, (npcAIBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 100, EntityType: 42, Reason: contracts.ObjectBehaviorInitReasonSpawn}))
	return w, h, npcTestChunks{{}: npcTestLand(t, types.ChunkCoord{})}
}

func TestNPCInitAndTickRemainTransient(t *testing.T) {
	w, h, chunks := npcFixture(t)
	brain, _ := ecs.GetComponent[components.NPCBrain](w, h)
	require.Equal(t, components.NPCIdle, brain.Activity)
	require.InDelta(t, 300, brain.HomeX, 0)
	require.NoError(t, (npcAIBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 100, EntityType: 42, Reason: contracts.ObjectBehaviorInitReasonRestore}))
	same, _ := ecs.GetComponent[components.NPCBrain](w, h)
	require.Equal(t, brain, same)
	ctx := contracts.BehaviorTickContext{World: w, Handle: h, EntityID: 100, EntityType: 42, CurrentTick: brain.DeadlineTick, Deps: &contracts.ExecutionDeps{Chunks: chunks}}
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
	replacement := w.Spawn(100, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.NPCBrain{RandomState: 999}) })
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
			brain.DeadlineTick = 20
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
			guard := NewNPCMovementGuard(w, chunks)
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
			w, h, chunks := npcFixture(t)
			brain, _ := ecs.GetComponent[components.NPCBrain](w, h)
			brain.Activity, brain.DeadlineTick = components.NPCWander, 30
			ecs.AddComponent(w, h, brain)
			movement, _ := ecs.GetComponent[components.Movement](w, h)
			if oscillation {
				movement.SetTargetPoint(400, 300)
			}
			ecs.AddComponent(w, h, movement)
			ecs.AddComponent(w, h, components.CollisionResult{FinalX: 300, FinalY: 300, PerpendicularOscillation: oscillation})
			ecs.GetResource[ecs.TimeState](w).Tick = 10
			ecs.GetResource[ecs.MovedEntities](w).Add(h, 300, 300)
			NewNPCMovementGuard(w, chunks).Update(w, .1)
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
