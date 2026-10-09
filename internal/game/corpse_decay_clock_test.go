package game

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func installCorpseDecayClockDefinitions(t *testing.T) {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 1, Key: "player", Resource: "player"},
		{DefID: 15, Key: "player_dead", HP: 100, Resource: "player", IsStatic: true, Indestructible: true,
			Behaviors: map[string]json.RawMessage{"player_dead": json.RawMessage(`{}`)}, BehaviorOrder: []string{"player_dead"}},
		{DefID: 17, Key: "player_skeleton", HP: 100, Resource: "player_skeleton", IsStatic: true, Indestructible: true},
	}))
}

// The clock tests use the actual destruction admission path, with persistence
// capabilities injected separately from the deletion-only fixture.
type corpseDecayClockChunks struct{ *destructionChunksFixture }

func (*corpseDecayClockChunks) ReplaceCommittedSource(*repository.Object) error { return nil }

type corpseDecayClockPersister struct{ *destructionPersisterFixture }

func (p *corpseDecayClockPersister) TransformObjectWithDroppedItems(ctx context.Context, source *repository.Object, maximum types.EntityID,
	next func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error),
) error {
	return p.ReplaceObjectWithDroppedItems(ctx, source.Region, types.EntityID(source.ID), maximum, next)
}

func newCorpseDecayClockFixture(t *testing.T) (*destructionServiceFixture, *Shard) {
	t.Helper()
	installCorpseDecayClockDefinitions(t)
	f := newDestructionServiceFixture(t)
	shard := &Shard{world: f.w, objectDestruction: f.service, logger: zap.NewNop()}
	f.service.deps.Chunks = &corpseDecayClockChunks{f.chunks}
	f.service.deps.Persister = &corpseDecayClockPersister{f.persister}
	f.service.deps.TransformCommitted = shard.completeCorpseDecay
	return f, shard
}

func spawnCorpseDecayClockTarget(t *testing.T, f *destructionServiceFixture, id types.EntityID, deadline int64) types.Handle {
	t.Helper()
	definition, exists := objectdefs.Global().GetByKey("player_dead")
	require.True(t, exists)
	handle := gameworld.SpawnEntityFromDef(f.w, definition, gameworld.DefSpawnParams{EntityID: id, X: 50, Y: 50, Region: 1})
	require.NotEqual(t, types.InvalidHandle, handle)
	ecs.AddComponent(f.w, handle, components.ChunkRef{})
	ecs.WithComponent(f.w, handle, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, "player_dead", &components.CorpseDecayBehaviorState{DecayAtRuntimeSeconds: deadline})
		state.IsDirty = false
	})
	require.NoError(t, behaviors.MustDefaultRegistry().InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
		World: f.w, Handle: handle, EntityID: id, EntityType: 15, Reason: contracts.ObjectBehaviorInitReasonRestore,
	}, []string{"player_dead"}))
	return handle
}

func TestCorpseDecayClockConversionAndPersistedDeadline(t *testing.T) {
	installCorpseDecayClockDefinitions(t)
	for _, deathRuntime := range []int64{0, 500} {
		t.Run(time.Duration(deathRuntime).String(), func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: deathRuntime, Tick: 99999, UnixMs: 100000})
			player := w.Spawn(10, nil)
			ecs.AddComponent(w, player, components.EntityInfo{TypeID: 1, Quality: 23, Region: 1})
			ecs.AddComponent(w, player, components.EntityHealth{LyingRevision: 2})
			ecs.AddComponent(w, player, components.Transform{X: 50, Y: 60, Direction: 1.25})
			ecs.AddComponent(w, player, components.Appearance{Resource: "player"})
			shard := &Shard{world: w, cfg: &config.Config{Game: config.GameConfig{Region: 1}}, logger: zap.NewNop()}
			shard.convertPlayerEntityToCorpse(w, 10, player)
			expected := deathRuntime + behaviors.CorpseDecaySeconds
			deadline, corpse := corpseDecayDeadline(w, player)
			require.True(t, corpse)
			require.Equal(t, expected, deadline)
			require.False(t, ecs.HasComponent[components.EntityHealth](w, player))
			visual, exists := ecs.GetComponent[components.CorpseVisualState](w, player)
			require.True(t, exists)
			require.Equal(t, uint64(3), visual.LyingRevision)
			require.Equal(t, 1, ecs.GetResource[ecs.CorpseDecaySchedule](w).PendingCount())
			ecs.AddComponent(w, player, components.ChunkRef{})
			factory := gameworld.NewObjectFactory(nil)
			raw, err := factory.Serialize(w, player)
			require.NoError(t, err)
			require.Equal(t, int64(10), raw.ID)
			require.True(t, raw.Data.Valid)
			restoredWorld := ecs.NewWorldForTesting()
			// A restarted tick counter and advanced wall clock do not age the body.
			ecs.SetResource(restoredWorld, ecs.TimeState{RuntimeSecondsTotal: deathRuntime, Tick: 1, UnixMs: 9_999_999_999})
			restored, err := factory.Build(restoredWorld, raw, nil)
			require.NoError(t, err)
			persistedState, err := factory.DeserializeObjectState(raw)
			require.NoError(t, err)
			ecs.WithComponent(restoredWorld, restored, func(state *components.ObjectInternalState) {
				state.State = persistedState
				state.IsDirty = false
			})
			require.NoError(t, behaviors.MustDefaultRegistry().InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
				World: restoredWorld, Handle: restored, EntityID: 10, EntityType: 15, Reason: contracts.ObjectBehaviorInitReasonRestore,
			}, []string{"player_dead"}))
			deadline, corpse = corpseDecayDeadline(restoredWorld, restored)
			require.True(t, corpse)
			require.Equal(t, expected, deadline)
			state, _ := ecs.GetComponent[components.ObjectInternalState](restoredWorld, restored)
			require.False(t, state.IsDirty)
			require.Equal(t, float64(100), state.HP)
			require.True(t, state.HasHP)
			queue := ecs.GetResource[ecs.CorpseDecaySchedule](restoredWorld)
			_, due := queue.PopDue(expected - 1)
			require.False(t, due)
			entry, due := queue.PopDue(expected)
			require.True(t, due)
			require.Equal(t, types.EntityID(10), entry.EntityID)
		})
	}
}

func TestCorpseDecayClockAdmissionUsesRuntimeBoundary(t *testing.T) {
	f, shard := newCorpseDecayClockFixture(t)
	deadline := int64(21600)
	corpse := spawnCorpseDecayClockTarget(t, f, 10, deadline)
	clock := ecs.GetResource[ecs.TimeState](f.w)
	clock.Tick, clock.UnixMs, clock.RuntimeSecondsTotal = 9_000_000, 9_999_999_999, deadline-1
	shard.updateCorpseDecay(f.w)
	require.Zero(t, f.service.PendingCount())
	require.False(t, ecs.ObjectDestructionPending(f.w, corpse))
	clock.Tick, clock.UnixMs, clock.RuntimeSecondsTotal = 1, 1, deadline
	shard.updateCorpseDecay(f.w)
	require.Equal(t, 1, f.service.PendingCount())
	require.True(t, ecs.ObjectDestructionPending(f.w, corpse))
	require.Zero(t, ecs.GetResource[ecs.CorpseDecaySchedule](f.w).PendingCount())
	state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, corpse)
	require.Equal(t, float64(100), state.HP)
	require.False(t, state.IsDirty, "decay admission must not apply damage or alter saved HP")
	for i := 0; i < 3; i++ {
		shard.updateCorpseDecay(f.w)
	}
	require.Equal(t, 1, f.service.PendingCount(), "repeated updates must not admit a second transition")
}

func TestCorpseDecayClockAdmissionIsBounded(t *testing.T) {
	f, shard := newCorpseDecayClockFixture(t)
	clock := ecs.GetResource[ecs.TimeState](f.w)
	clock.RuntimeSecondsTotal = 100
	reconciler := &shardObjectRestoreReconciler{shard: shard}
	for id := types.EntityID(10); id <= 110; id++ {
		handle := spawnCorpseDecayClockTarget(t, f, id, 100)
		require.True(t, reconciler.ReconcileRestoredObject(f.w, handle))
	}
	require.Zero(t, f.service.PendingCount(), "restoration must use the same bounded scheduler admission")
	shard.updateCorpseDecay(f.w)
	require.Equal(t, 100, f.service.PendingCount())
	require.Equal(t, 1, ecs.GetResource[ecs.CorpseDecaySchedule](f.w).PendingCount())
	require.False(t, ecs.ObjectDestructionPending(f.w, f.w.GetHandleByEntityID(110)), "equal deadlines must admit lower IDs first")
	shard.updateCorpseDecay(f.w)
	require.Equal(t, 101, f.service.PendingCount())
	require.Zero(t, ecs.GetResource[ecs.CorpseDecaySchedule](f.w).PendingCount())
}

func TestCorpseDecayClockBackpressureRetriesNextRuntimeSecond(t *testing.T) {
	f, shard := newCorpseDecayClockFixture(t)
	corpse := spawnCorpseDecayClockTarget(t, f, 10, 100)
	clock := ecs.GetResource[ecs.TimeState](f.w)
	clock.RuntimeSecondsTotal = 100
	// Force the same full-slot admission condition as a saturated destruction
	// queue without constructing unrelated loot operations for this clock test.
	freeCount := f.service.freeCount
	f.service.freeCount = 0
	shard.updateCorpseDecay(f.w)
	require.Zero(t, f.service.PendingCount())
	require.False(t, ecs.ObjectDestructionPending(f.w, corpse))
	require.Equal(t, 1, ecs.GetResource[ecs.CorpseDecaySchedule](f.w).PendingCount())
	f.service.freeCount = freeCount
	clock.Tick += 10000
	clock.UnixMs += 1_000_000
	shard.updateCorpseDecay(f.w)
	require.Zero(t, f.service.PendingCount(), "a free slot and elapsed ticks/wall time cannot retry before the next runtime second")
	clock.RuntimeSecondsTotal = 101
	shard.updateCorpseDecay(f.w)
	require.Equal(t, 1, f.service.PendingCount())
	require.True(t, ecs.ObjectDestructionPending(f.w, corpse))
	deadline, exists := corpseDecayDeadline(f.w, corpse)
	require.True(t, exists)
	require.Equal(t, int64(100), deadline, "backpressure changes the queue retry, not the persistent age")
}

func TestCorpseDecayClockRejectsStaleIdentityAndNonCorpse(t *testing.T) {
	f, shard := newCorpseDecayClockFixture(t)
	clock := ecs.GetResource[ecs.TimeState](f.w)
	clock.RuntimeSecondsTotal = 100
	stale := spawnCorpseDecayClockTarget(t, f, 10, 100)
	require.True(t, f.w.Despawn(stale))
	queue := ecs.EnsureCorpseDecaySchedule(f.w)
	require.Zero(t, queue.PendingCount())
	replacement := spawnCorpseDecayClockTarget(t, f, 20, 200)
	queue.Schedule(stale, 10, 100)
	queue.Schedule(replacement, 999, 100) // Same generation, wrong durable ID.
	ordinary := f.targetWithID(t, 30)
	queue.Schedule(ordinary, 30, 100)
	shard.updateCorpseDecay(f.w)
	require.Zero(t, f.service.PendingCount())
	require.False(t, ecs.ObjectDestructionPending(f.w, replacement))
	require.False(t, ecs.ObjectDestructionPending(f.w, ordinary))
}

func TestCorpseDecayClockRechecksPersistentDeadline(t *testing.T) {
	f, shard := newCorpseDecayClockFixture(t)
	corpse := spawnCorpseDecayClockTarget(t, f, 10, 200)
	queue := ecs.GetResource[ecs.CorpseDecaySchedule](f.w)
	queue.Schedule(corpse, 10, 100)
	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 100
	shard.updateCorpseDecay(f.w)
	require.Zero(t, f.service.PendingCount())
	_, due := queue.PopDue(199)
	require.False(t, due)
	entry, due := queue.PopDue(200)
	require.True(t, due)
	require.Equal(t, int64(200), entry.DueRuntimeSeconds)
}

func TestCorpseDecayClockRestoredOverdueAdmission(t *testing.T) {
	for _, queueFull := range []bool{false, true} {
		name := "accepted"
		if queueFull {
			name = "backpressure"
		}
		t.Run(name, func(t *testing.T) {
			f, shard := newCorpseDecayClockFixture(t)
			corpse := spawnCorpseDecayClockTarget(t, f, 10, 100)
			ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 200
			if queueFull {
				f.service.freeCount = 0
			}
			reconciler := &shardObjectRestoreReconciler{shard: shard}
			expose := reconciler.ReconcileRestoredObject(f.w, corpse)
			require.True(t, expose, "restoration queues overdue work for bounded admission before visibility updates")
			require.Zero(t, f.service.PendingCount())
			shard.updateCorpseDecay(f.w)
			if queueFull {
				require.False(t, ecs.ObjectDestructionPending(f.w, corpse))
				require.Equal(t, 1, ecs.GetResource[ecs.CorpseDecaySchedule](f.w).PendingCount())
				_, immediatelyDue := ecs.GetResource[ecs.CorpseDecaySchedule](f.w).PopDue(200)
				require.False(t, immediatelyDue, "restored backpressure must wait one runtime second")
			} else {
				require.True(t, ecs.ObjectDestructionPending(f.w, corpse))
				require.Equal(t, 1, f.service.PendingCount())
				require.Zero(t, ecs.GetResource[ecs.CorpseDecaySchedule](f.w).PendingCount())
				require.False(t, reconciler.ReconcileRestoredObject(f.w, corpse), "a quarantined restore must not become visible")
			}
		})
	}
}
