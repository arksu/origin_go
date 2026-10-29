package game

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type npcFoxFixture struct {
	shard  *Shard
	chunk  *core.Chunk
	fox    types.Handle
	moves  chan *ecs.ObjectMoveBatchEvent
	spawns chan *ecs.EntitySpawnBatchEvent
}

// These fixtures exercise movement/visibility independently of action execution.
// Production combat and its admission/cooldown rules use the real executor.
type npcFoxMovementOnlyExecutor struct{}

func (npcFoxMovementOnlyExecutor) TryStart(request contracts.NPCAbilityRequest) contracts.NPCAbilityState {
	return contracts.NPCAbilityState{Actor: request.Actor, AbilityID: request.AbilityID, Generation: request.Generation, Status: contracts.NPCExecutionRejected}
}
func (npcFoxMovementOnlyExecutor) Cancel(actor contracts.EntityRef, generation uint64, _ contracts.NPCRetireReason) contracts.NPCAbilityState {
	return contracts.NPCAbilityState{Actor: actor, Generation: generation, Status: contracts.NPCExecutionRejected}
}
func (npcFoxMovementOnlyExecutor) State(actor contracts.EntityRef, generation uint64) contracts.NPCAbilityState {
	return contracts.NPCAbilityState{Actor: actor, Generation: generation, Status: contracts.NPCExecutionRejected}
}
func (npcFoxMovementOnlyExecutor) Availability(contracts.EntityRef, string, contracts.NPCTarget) contracts.NPCAbilityAvailabilityState {
	return contracts.NPCAbilityAvailabilityState{}
}

func loadNPCFoxCatalog(t testing.TB) (*objectdefs.ObjectDef, *behaviors.Registry) {
	t.Helper()
	previous, previousItems := objectdefs.Global(), itemdefs.Global()
	t.Cleanup(func() {
		objectdefs.SetGlobalForTesting(previous)
		itemdefs.SetGlobalForTesting(previousItems)
	})
	items, err := itemdefs.LoadFromDirectory("../../data/items", zap.NewNop())
	require.NoError(t, err)
	itemdefs.SetGlobalForTesting(items)
	registry := behaviors.MustDefaultRegistry()
	definitions, err := objectdefs.LoadFromDirectory("../../data/objects", registry, zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(definitions)
	def, exists := definitions.GetByKey("fox")
	require.True(t, exists)
	return def, registry
}

func newNPCFoxFixture(t *testing.T) *npcFoxFixture {
	t.Helper()
	def, registry := loadNPCFoxCatalog(t)
	shard, _ := newPlayerSpawnTestShard(t, 64)
	w := shard.world
	core.AttachColliderSpatial(w)
	controller, err := behaviors.AttachNPCController(w, behaviors.NPCControllerConfig{Chunks: shard.chunkManager, Abilities: npcFoxMovementOnlyExecutor{}})
	require.NoError(t, err)
	ecs.SetResource(w, ecs.TimeState{Tick: 1, TickRate: 10, TickPeriod: 100 * time.Millisecond, Now: time.Unix(100, 0), UnixMs: 100000})
	chunk := shard.chunkManager.GetChunk(types.ChunkCoord{})
	chunk.SetState(types.ChunkStateActive)
	fox := gameworld.SpawnEntityFromDef(w, def, gameworld.DefSpawnParams{
		EntityID: 100, X: 200, Y: 200, Region: 1, InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: registry,
	})
	require.NotEqual(t, types.InvalidHandle, fox)
	// Match the common spawn caller's chunk membership; wildlife is a dynamic
	// target in the chunk, without registering it as an AOI owner.
	ecs.AddComponent(w, fox, components.ChunkRef{})
	chunk.Spatial().AddDynamic(fox, 200, 200)
	f := &npcFoxFixture{shard: shard, chunk: chunk, fox: fox, moves: make(chan *ecs.ObjectMoveBatchEvent, 128), spawns: make(chan *ecs.EntitySpawnBatchEvent, 8)}
	shard.eventBus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		f.moves <- event.(*ecs.ObjectMoveBatchEvent)
		return nil
	})
	shard.eventBus.SubscribeAsync(ecs.TopicGameplayEntitySpawnBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		f.spawns <- event.(*ecs.EntitySpawnBatchEvent)
		return nil
	})
	// Deliberately register out of order: World must apply the production
	// priorities, including terrain/deadline rejection before Transform sends.
	w.AddSystem(systems.NewBehaviorTickSystem(shard.logger, systems.BehaviorTickSystemConfig{
		BehaviorRegistry: registry, ExecutionDeps: &contracts.ExecutionDeps{Chunks: shard.chunkManager},
	}))
	w.AddSystem(systems.NewTransformUpdateSystem(w, shard.chunkManager, shard.eventBus, shard.logger))
	w.AddSystem(behaviors.NewNPCMovementGuard(controller))
	w.AddSystem(systems.NewCollisionSystem(w, shard.chunkManager, shard.logger, 0, constt.ChunkWorldSize, 0, constt.ChunkWorldSize, 0))
	w.AddSystem(systems.NewMovementSystem(w, shard.chunkManager, shard.logger))
	w.AddSystem(systems.NewResetSystem(shard.logger))
	return f
}

type npcFoxBenchmarkChunks struct{ chunk *core.Chunk }

func (c npcFoxBenchmarkChunks) GetChunkFast(coord types.ChunkCoord) *core.Chunk {
	if coord != c.chunk.Coord {
		return nil
	}
	return c.chunk
}

func newNPCFoxBenchmarkWorld(b *testing.B, population int) (*ecs.World, npcFoxBenchmarkChunks, []types.Handle, *behaviors.Registry) {
	b.Helper()
	def, registry := loadNPCFoxCatalog(b)
	w := ecs.NewWorldWithCapacity(uint32(population+1), nil, 0)
	core.AttachColliderSpatial(w)
	ecs.SetResource(w, ecs.TimeState{Tick: 1, TickRate: 10, TickPeriod: 100 * time.Millisecond})
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, constt.ChunkSize)
	tiles := make([]byte, constt.ChunkSize*constt.ChunkSize)
	for i := range tiles {
		tiles[i] = types.TileGrass
	}
	require.NoError(b, chunk.RestoreTiles(tiles, 0, 0))
	chunk.SetState(types.ChunkStateActive)
	_, err := behaviors.AttachNPCController(w, behaviors.NPCControllerConfig{Chunks: npcFoxBenchmarkChunks{chunk}, Abilities: npcFoxMovementOnlyExecutor{}})
	require.NoError(b, err)
	handles := make([]types.Handle, population)
	for i := range handles {
		handles[i] = gameworld.SpawnEntityFromDef(w, def, gameworld.DefSpawnParams{
			EntityID: types.EntityID(i + 1), X: 150 + float64(i%100)*12, Y: 150 + float64(i/100)*12,
			Region: 1, InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: registry,
		})
		require.NotEqual(b, types.InvalidHandle, handles[i])
	}
	return w, npcFoxBenchmarkChunks{chunk}, handles, registry
}

// Each operation processes a fixed microbenchmark budget of 200 overdue NPC callbacks,
// including real decisions, candidate terrain checks, movement writes and rearm.
// Advancing by 1000 ticks keeps the entire population eligible; start/stop
// transitions alternate as entities recur, without network or collision costs.
func BenchmarkNPCFoxScheduledAI(b *testing.B) {
	for _, population := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("population_%d", population), func(b *testing.B) {
			w, chunks, _, registry := newNPCFoxBenchmarkWorld(b, population)
			system := systems.NewBehaviorTickSystem(zap.NewNop(), systems.BehaviorTickSystemConfig{
				BudgetPerTick: 200, BehaviorRegistry: registry, ExecutionDeps: &contracts.ExecutionDeps{Chunks: chunks},
			})
			ts := ecs.GetResource[ecs.TimeState](w)
			step := func() {
				ts.Tick += 1000
				system.Update(w, .1)
			}
			for range population / 100 {
				step()
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				step()
			}
			b.StopTimer()
			b.ReportMetric(200, "callbacks/op")
		})
	}
}

// Exercise the production guard on a fully populated movement buffer. Positions
// remain separated and all collider footprints are valid; no collision or
// scheduling work hides the steady cost of validating moving NPCs each tick.
func BenchmarkNPCFoxMovementGuard(b *testing.B) {
	for _, population := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("population_%d", population), func(b *testing.B) {
			w, _, handles, _ := newNPCFoxBenchmarkWorld(b, population)
			moved := ecs.GetResource[ecs.MovedEntities](w)
			for _, h := range handles {
				position, _ := ecs.GetComponent[components.Transform](w, h)
				ecs.WithComponent(w, h, func(brain *components.NPCBrain) {
					brain.Activity, brain.ActivityDeadlineTick = components.NPCWander, 100
					brain.MovementLease = components.NPCMovementLease{Owner: components.NPCMovementAI, Generation: 1, DeadlineTick: 100, HomeRadius: 6 * constt.CoordPerTile}
				})
				ecs.WithComponent(w, h, func(movement *components.Movement) { movement.SetTargetPoint(int(position.X)+12, int(position.Y)) })
				ecs.WithComponent(w, h, func(result *components.CollisionResult) { result.FinalX, result.FinalY = position.X+3.2, position.Y })
				moved.Add(h, position.X+3.2, position.Y)
			}
			controller, ok := behaviors.GetNPCController(w)
			require.True(b, ok)
			guard := behaviors.NewNPCMovementGuard(controller)
			guard.Update(w, .1)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				guard.Update(w, .1)
			}
			b.StopTimer()
			b.ReportMetric(float64(population), "NPCs/op")
		})
	}
}

func (f *npcFoxFixture) tick(tick uint64) {
	f.shard.mu.Lock()
	defer f.shard.mu.Unlock()
	ts := ecs.GetResource[ecs.TimeState](f.shard.world)
	ts.Tick = tick
	ts.Now = time.Unix(100, 0).Add(time.Duration(tick) * 100 * time.Millisecond)
	ts.UnixMs = ts.Now.UnixMilli()
	f.shard.world.Update(.1)
}

func (f *npcFoxFixture) observe(t *testing.T) []types.Handle {
	t.Helper()
	w := f.shard.world
	vision := systems.NewVisionSystem(w, f.shard.chunkManager, f.shard.eventBus, false, f.shard.logger)
	observers := make([]types.Handle, 0, 2)
	for _, id := range []types.EntityID{1, 2} {
		h := w.Spawn(id, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Transform{X: 200, Y: 200})
			ecs.AddComponent(w, h, components.Vision{Radius: 100, Power: 100})
			ecs.AddComponent(w, h, components.ChunkRef{})
			ecs.AddComponent(w, h, components.EntityInfo{})
		})
		visibility := ecs.GetResource[ecs.VisibilityState](w)
		visibility.VisibleByObserver[h] = ecs.ObserverVisibility{Known: map[types.Handle]types.EntityID{h: id}}
		vision.ForceUpdateForObserver(w, h)
		observers = append(observers, h)
	}
	// Same-priority, single-worker barrier also drains the preceding spawn events.
	require.Empty(t, drainLiftFollowEvents(t, f.shard.eventBus, f.moves))
	require.Len(t, f.spawns, 2)
	seen := make(map[types.EntityID]bool)
	for range 2 {
		batch := <-f.spawns
		require.False(t, seen[batch.ObserverID])
		seen[batch.ObserverID] = true
		require.Equal(t, []ecs.SpawnBatchEntry{{EntityID: 100, Handle: f.fox}}, batch.Entries)
	}
	return observers
}

func TestNPCFoxSpawnWanderAndStopUseSharedPipeline(t *testing.T) {
	f := newNPCFoxFixture(t)
	w := f.shard.world
	observers := f.observe(t)
	info, _ := ecs.GetComponent[components.EntityInfo](w, f.fox)
	require.True(t, info.Transient)
	require.False(t, info.IsStatic)
	appearance, _ := ecs.GetComponent[components.Appearance](w, f.fox)
	require.Equal(t, "fox", appearance.Resource)
	_, hasHealth := ecs.GetComponent[components.EntityHealth](w, f.fox)
	_, hasVision := ecs.GetComponent[components.Vision](w, f.fox)
	require.False(t, hasHealth)
	require.False(t, hasVision)
	require.Empty(t, f.shard.chunkManager.GetEntityActiveChunks(100))
	_, hasAOI := f.shard.chunkManager.GetEntityChunk(100)
	require.False(t, hasAOI)
	brain, exists := ecs.GetComponent[components.NPCBrain](w, f.fox)
	require.True(t, exists)
	require.Equal(t, components.NPCIdle, brain.Activity)
	require.Equal(t, 200.0, brain.HomeX)
	require.Equal(t, 200.0, brain.HomeY)
	require.LessOrEqual(t, brain.NextDecisionTick, uint64(11), "idle perception polls at most one second apart")
	require.InDelta(t, 120, float64(brain.Cautious.IdleUntilTick-1), 40)
	f.tick(brain.Cautious.IdleUntilTick - 1)
	require.Empty(t, drainLiftFollowEvents(t, f.shard.eventBus, f.moves))
	brain, _ = ecs.GetComponent[components.NPCBrain](w, f.fox)
	startTick := brain.NextDecisionTick
	f.tick(startTick)
	movement, _ := ecs.GetComponent[components.Movement](w, f.fox)
	require.Equal(t, constt.StateMoving, movement.State)
	require.Equal(t, constt.TargetPoint, movement.TargetType)
	require.Equal(t, 32.0, movement.GetCurrentSpeed())
	targetDistance := math.Hypot(movement.TargetX-200, movement.TargetY-200)
	require.GreaterOrEqual(t, targetDistance, 11.3)
	require.LessOrEqual(t, targetDistance, 36.8)
	for tick := startTick + 1; tick <= startTick+31; tick++ {
		f.tick(tick)
	}
	events := drainLiftFollowEvents(t, f.shard.eventBus, f.moves)
	require.Greater(t, len(events), 1)
	for index, event := range events {
		require.Len(t, event.Entries, 1)
		entry := event.Entries[0]
		require.Equal(t, types.EntityID(100), entry.EntityID)
		require.Equal(t, f.fox, entry.Handle)
		require.Equal(t, uint32(index), entry.MoveSeq)
		require.False(t, entry.IsTeleport)
		require.Equal(t, constt.Walk, entry.MoveMode)
		if index == len(events)-1 {
			require.False(t, entry.IsMoving, "the final packet must settle both observers")
			require.Zero(t, entry.VelocityX)
			require.Zero(t, entry.VelocityY)
		} else {
			require.True(t, entry.IsMoving)
			require.InDelta(t, math.Atan2(movement.TargetY-200, movement.TargetX-200), entry.Heading, 1e-9)
		}
	}
	position, _ := ecs.GetComponent[components.Transform](w, f.fox)
	require.InDelta(t, movement.TargetX, position.X, 1)
	require.InDelta(t, movement.TargetY, position.Y, 1)
	brain, _ = ecs.GetComponent[components.NPCBrain](w, f.fox)
	require.Equal(t, components.NPCIdle, brain.Activity)
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, f.fox)
	require.True(t, state.HasHP)
	require.Equal(t, 110.0, state.HP)
	require.False(t, state.IsDirty)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	require.Len(t, visibility.VisibleByObserver, 2)
	require.Len(t, visibility.ObserversByVisibleTarget[f.fox], 2)
	for _, observer := range observers {
		require.Equal(t, types.EntityID(100), visibility.VisibleByObserver[observer].Known[f.fox])
	}
	f.tick(startTick + 32)
	require.Empty(t, drainLiftFollowEvents(t, f.shard.eventBus, f.moves), "idle must not emit repeated movement")
}

func TestNPCFoxMovementPipelineRejectsObstaclesExpiredWalkAndUnsafeFootprints(t *testing.T) {
	for _, scenario := range []string{"object", "deadline", "shallow_water_edge", "inactive_chunk"} {
		t.Run(scenario, func(t *testing.T) {
			f := newNPCFoxFixture(t)
			w := f.shard.world
			f.observe(t)
			deadline := uint64(30)
			if scenario == "deadline" {
				deadline = 2
			}
			ecs.WithComponent(w, f.fox, func(brain *components.NPCBrain) {
				brain.Activity, brain.ActivityDeadlineTick = components.NPCWander, deadline
				brain.NextDecisionTick = deadline
				brain.MovementLease = components.NPCMovementLease{Owner: components.NPCMovementAI, Generation: 1, DeadlineTick: deadline, HomeRadius: 6 * constt.CoordPerTile}
			})
			ecs.ScheduleBehaviorTick(w, 100, "npc_ai", deadline)
			ecs.WithComponent(w, f.fox, func(movement *components.Movement) { movement.SetTargetPoint(260, 200) })
			switch scenario {
			case "object":
				wall := w.Spawn(200, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.Transform{X: 210, Y: 200})
					ecs.AddComponent(w, h, components.Collider{HalfWidth: 4, HalfHeight: 30, Layer: 1, Mask: 1})
				})
				f.chunk.Spatial().AddStatic(wall, 210, 200)
			case "shallow_water_edge":
				// The next anchor is still on grass, but its rightmost 2.4
				// units overlap water. The guard must reject before commit.
				require.True(t, f.chunk.SetTile(17, 16, types.TileShallowWater))
			case "inactive_chunk":
				f.chunk.SetState(types.ChunkStatePreloaded)
			}
			var stoppedAt uint64
			for tick := uint64(1); tick <= deadline+1; tick++ {
				f.tick(tick)
				movement, _ := ecs.GetComponent[components.Movement](w, f.fox)
				if stoppedAt == 0 && movement.State == constt.StateIdle {
					stoppedAt = tick
				}
			}
			position, _ := ecs.GetComponent[components.Transform](w, f.fox)
			movement, _ := ecs.GetComponent[components.Movement](w, f.fox)
			brain, _ := ecs.GetComponent[components.NPCBrain](w, f.fox)
			require.Equal(t, constt.StateIdle, movement.State)
			require.Equal(t, components.NPCIdle, brain.Activity)
			switch scenario {
			case "object":
				require.LessOrEqual(t, position.X, 210.0-4-2.4)
				require.Less(t, stoppedAt, deadline, "collision must stop the fox before its walk timeout")
			case "deadline":
				require.InDelta(t, 203.2, position.X, 1e-9)
				require.Equal(t, deadline, stoppedAt)
			default:
				require.Equal(t, 200.0, position.X)
				require.Equal(t, uint64(1), stoppedAt)
			}
			events := drainLiftFollowEvents(t, f.shard.eventBus, f.moves)
			require.NotEmpty(t, events)
			last := events[len(events)-1].Entries[0]
			require.False(t, last.IsMoving)
			require.Zero(t, last.VelocityX)
			require.Zero(t, last.VelocityY)
			require.Equal(t, int(position.X), last.X)
			require.Empty(t, f.shard.chunkManager.GetEntityActiveChunks(100))
		})
	}
}

func TestNPCFoxWatchPublishesStationaryFacingThroughMovement(t *testing.T) {
	f := newNPCFoxFixture(t)
	w := f.shard.world
	f.observe(t)
	w.Spawn(250, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 200, Y: 240})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 2, HalfHeight: 2})
		ecs.AddComponent(w, h, components.CharacterProfile{})
		ecs.AddComponent(w, h, components.EntityHealth{SHP: 100, HHP: 100})
	})
	brain, _ := ecs.GetComponent[components.NPCBrain](w, f.fox)
	f.tick(brain.NextDecisionTick)
	brain, _ = ecs.GetComponent[components.NPCBrain](w, f.fox)
	require.Equal(t, components.NPCWatch, brain.Activity)
	f.tick(ecs.GetResource[ecs.TimeState](w).Tick + 1)
	events := drainLiftFollowEvents(t, f.shard.eventBus, f.moves)
	require.Len(t, events, 1)
	require.Len(t, events[0].Entries, 1)
	entry := events[0].Entries[0]
	require.Equal(t, types.EntityID(100), entry.EntityID)
	require.False(t, entry.IsMoving)
	require.InDelta(t, math.Pi/2, entry.Heading, 1e-9)
	require.Equal(t, 200, entry.X)
	require.Equal(t, 200, entry.Y)
	brain, _ = ecs.GetComponent[components.NPCBrain](w, f.fox)
	require.Empty(t, brain.DamageSource, "peaceful perception never authorizes combat")
}
