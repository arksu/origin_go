package world

import (
	"encoding/json"
	"math"
	"testing"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	objectHealthLifecycleTypeID = 9811
	objectHealthContainerTypeID = 9812
)

func installObjectHealthLifecycleDefinitions(t *testing.T) {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: objectHealthLifecycleTypeID, Key: "health-fixture", HP: 100, IsStatic: true},
		{DefID: objectHealthContainerTypeID, Key: "health-container-fixture", HP: 100, IsStatic: true,
			Behaviors: map[string]json.RawMessage{"container": json.RawMessage(`{}`)}, BehaviorOrder: []string{"container"},
			Components: &objectdefs.Components{Inventory: []objectdefs.InventoryDef{{Kind: "grid", W: 2, H: 2}}}},
	}))
}

func spawnObjectHealthLifecycleFixture(t *testing.T, w *ecs.World, chunk *core.Chunk, id types.EntityID, typeID int, hp float64) types.Handle {
	t.Helper()
	def, ok := objectdefs.Global().GetByID(typeID)
	require.True(t, ok)
	h := SpawnEntityFromDef(w, def, DefSpawnParams{EntityID: id, X: 10, Y: 10, Region: 1, Quality: 10})
	require.NotEqual(t, types.InvalidHandle, h)
	ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: chunk.Coord.X, CurrentChunkY: chunk.Coord.Y})
	require.NoError(t, SetObjectHP(w, h, hp))
	chunk.Spatial().AddStatic(h, 10, 10)
	return h
}

func TestEmbeddedObjectHealthSnapshotExactAndOwned(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	factory := cm.ObjectFactory()
	for index, hp := range []float64{.6, .49, float64(81) / 13, 0, float64(math.MaxInt32) + .5} {
		h := spawnObjectHealthLifecycleFixture(t, cm.world, chunk, types.EntityID(9820+index), objectHealthLifecycleTypeID, hp)
		snapshot, err := factory.CaptureWorldObjectSnapshot(cm.world, h)
		require.NoError(t, err)
		require.NotNil(t, snapshot.HP)
		require.Equal(t, hp, *snapshot.HP)
		require.NoError(t, SetObjectHP(cm.world, h, 42))
		require.Equal(t, hp, *snapshot.HP, "snapshot must not retain an ECS-backed pointer")
		data, err := SerializeSnapshotToJSON(snapshot)
		require.NoError(t, err)
		if hp == 0 {
			require.Contains(t, string(data), `"hp":0`)
		}
		decoded, err := DeserializeSnapshotFromJSON(data)
		require.NoError(t, err)
		require.Equal(t, hp, *decoded.HP)
		chunk.Spatial().RemoveStatic(h, 10, 10)
		cm.world.Despawn(h)
		restored, err := factory.SpawnWorldObjectFromSnapshot(cm.world, decoded, SnapshotSpawnOptions{
			X: 10, Y: 10, ChunkManager: cm, Logger: zap.NewNop(),
		})
		require.NoError(t, err)
		health, ok := ecs.GetComponent[components.ObjectInternalState](cm.world, restored)
		require.True(t, ok)
		require.True(t, health.HasHP)
		require.Equal(t, hp, health.HP)
	}
}

type objectHealthRestoreBehavior struct {
	t              *testing.T
	hp             float64
	initCalls      int
	recomputeCalls int
}

func (*objectHealthRestoreBehavior) Key() string { return "container" }

func (behavior *objectHealthRestoreBehavior) GetBehavior(key string) (contracts.Behavior, bool) {
	return behavior, key == behavior.Key()
}

func (behavior *objectHealthRestoreBehavior) Keys() []string { return []string{behavior.Key()} }

func (behavior *objectHealthRestoreBehavior) IsRegisteredBehaviorKey(key string) bool {
	return key == behavior.Key()
}

func (*objectHealthRestoreBehavior) ValidateBehaviorKeys([]string) error { return nil }

func (behavior *objectHealthRestoreBehavior) InitObjectBehaviors(ctx *contracts.BehaviorObjectInitContext, _ []string) error {
	state, ok := ecs.GetComponent[components.ObjectInternalState](ctx.World, ctx.Handle)
	require.True(behavior.t, ok)
	require.True(behavior.t, state.HasHP)
	require.Equal(behavior.t, behavior.hp, state.HP, "behavior init must see saved HP")
	require.Equal(behavior.t, contracts.ObjectBehaviorInitReasonRestore, ctx.Reason)
	runtime, ok := components.GetRuntimeObjectState(state)
	require.True(behavior.t, ok)
	require.Equal(behavior.t, 1, runtime.Behaviors["tree"].(*components.TreeBehaviorState).ChopPoints)
	behavior.initCalls++
	return nil
}

func (behavior *objectHealthRestoreBehavior) ApplyRuntime(*contracts.BehaviorRuntimeContext) contracts.BehaviorRuntimeResult {
	behavior.recomputeCalls++
	return contracts.BehaviorRuntimeResult{
		State:    &components.RuntimeObjectState{Behaviors: map[string]any{"tree": &components.TreeBehaviorState{ChopPoints: 2}}},
		HasState: true,
		Flags:    []string{"restored"},
	}
}

func TestEmbeddedObjectHealthSurvivesStateAndBehaviorRestore(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	for _, hp := range []float64{0, .49} {
		cm := newTestChunkManagerWithLoadWorkers(0)
		t.Cleanup(cm.Stop)
		chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
		chunk.SetState(types.ChunkStateActive)
		cm.chunks[chunk.Coord] = chunk
		h := spawnObjectHealthLifecycleFixture(t, cm.world, chunk, 9884, objectHealthContainerTypeID, hp)
		ecs.WithComponent(cm.world, h, func(state *components.ObjectInternalState) {
			components.SetBehaviorState(state, "tree", &components.TreeBehaviorState{ChopPoints: 1})
		})
		snapshot, err := cm.ObjectFactory().CaptureWorldObjectSnapshot(cm.world, h)
		require.NoError(t, err)
		chunk.Spatial().RemoveStatic(h, 10, 10)
		require.True(t, cm.world.Despawn(h))
		behavior := &objectHealthRestoreBehavior{t: t, hp: hp}
		restored, err := cm.ObjectFactory().SpawnWorldObjectFromSnapshot(cm.world, snapshot, SnapshotSpawnOptions{
			X: 10, Y: 10, ChunkManager: cm, BehaviorRegistry: behavior,
		})
		require.NoError(t, err)
		state, ok := ecs.GetComponent[components.ObjectInternalState](cm.world, restored)
		require.True(t, ok)
		require.True(t, state.HasHP)
		require.Equal(t, hp, state.HP)
		require.True(t, state.IsDirty)
		require.Equal(t, []string{"restored"}, state.Flags)
		runtime, ok := components.GetRuntimeObjectState(state)
		require.True(t, ok)
		require.Equal(t, 2, runtime.Behaviors["tree"].(*components.TreeBehaviorState).ChopPoints)
		require.Equal(t, 1, behavior.initCalls)
		require.Equal(t, 1, behavior.recomputeCalls)
	}
}

func TestEmbeddedObjectHealthRejectsInvalidBeforeCaptureAndSpawn(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	w := ecs.NewWorldForTesting()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	h := spawnObjectHealthLifecycleFixture(t, w, chunk, 9881, objectHealthLifecycleTypeID, .6)
	factory := NewObjectFactory(nil)
	for _, hp := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		ecs.WithComponent(w, h, func(health *components.ObjectInternalState) { health.HP = hp })
		// An invalid runtime payload must not be serialized ahead of health.
		ecs.AddComponent(w, h, components.StationState{Values: map[string]float64{"temperature": math.NaN()}})
		_, err := factory.CaptureWorldObjectSnapshot(w, h)
		require.ErrorIs(t, err, ErrInvalidObjectHP)
		snapshot := EmbeddedObjectSnapshotV1{Version: 1, EntityID: 9882, TypeID: objectHealthLifecycleTypeID, HP: &hp}
		_, err = SerializeSnapshotToJSON(snapshot)
		require.ErrorIs(t, err, ErrInvalidObjectHP)
		before := w.EntityCount()
		_, err = factory.SpawnWorldObjectFromSnapshot(w, snapshot, SnapshotSpawnOptions{})
		require.ErrorIs(t, err, ErrInvalidObjectHP, "reject HP before target-chunk/state/inventory work")
		require.Equal(t, before, w.EntityCount())
	}
	ecs.WithComponent(w, h, func(state *components.ObjectInternalState) { state.HasHP = false })
	_, err := factory.CaptureWorldObjectSnapshot(w, h)
	require.ErrorIs(t, err, ErrObjectHealthMissing)
	_, err = DeserializeSnapshotFromJSON([]byte(`{"version":1,"type_id":9811,"entity_id":9882}`))
	require.ErrorIs(t, err, ErrObjectHealthMissing)
	_, err = DeserializeSnapshotFromJSON([]byte(`{"version":1,"type_id":9811,"hp":null}`))
	require.ErrorIs(t, err, ErrObjectHealthMissing)
}

func TestEmbeddedDroppedItemHealthIsAbsent(t *testing.T) {
	snapshot := EmbeddedObjectSnapshotV1{Version: 1, EntityID: 9883, TypeID: constt.DroppedItemTypeID}
	data, err := SerializeSnapshotToJSON(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"hp"`)
	decoded, err := DeserializeSnapshotFromJSON(data)
	require.NoError(t, err)
	require.Nil(t, decoded.HP)
	hp := 0.0
	snapshot.HP = &hp
	_, err = SerializeSnapshotToJSON(snapshot)
	require.ErrorIs(t, err, ErrInvalidObjectHP)
	_, err = DeserializeSnapshotFromJSON([]byte(`{"version":1,"type_id":1000,"hp":0}`))
	require.ErrorIs(t, err, ErrInvalidObjectHP)
}
