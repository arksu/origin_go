package world

import (
	"database/sql"
	"encoding/json"
	"math"
	"testing"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
)

func installObjectHealthTestDefinitions(t *testing.T) {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 1, Key: "player"},
		{DefID: 910, Key: "health_rock", HP: 100},
		{DefID: 911, Key: "health_box", HP: 100, BehaviorOrder: []string{"container"}, Components: &objectdefs.Components{Inventory: []objectdefs.InventoryDef{{Kind: "grid", W: 2, H: 2}}}},
	}))
}

func TestObjectFactoryHealthRoundTrip(t *testing.T) {
	installObjectHealthTestDefinitions(t)
	factory := NewObjectFactory(nil)
	for _, hp := range []float64{0.6, 0.49, 81.0 / 13, 0, float64(math.MaxInt32) + 100} {
		w := ecs.NewWorldForTesting()
		healthWrites := 0
		w.AddComponentObserver(components.ObjectInternalStateComponentID, func(observed types.Handle) {
			health, ok := ecs.GetComponent[components.ObjectInternalState](w, observed)
			require.True(t, ok)
			require.True(t, health.HasHP)
			require.Equal(t, hp, health.HP, "component observers must see the saved HP at first publication")
			healthWrites++
		})
		raw := &repository.Object{ID: 1, TypeID: 910, Hp: sql.NullFloat64{Float64: hp, Valid: true}}
		h, err := factory.Build(w, raw, nil)
		require.NoError(t, err)
		health, ok := ecs.GetComponent[components.ObjectInternalState](w, h)
		require.True(t, ok)
		require.True(t, health.HasHP)
		require.Equal(t, hp, health.HP, "restore must preserve saved HP without a def clamp")
		require.Equal(t, 1, healthWrites)
		ecs.AddComponent(w, h, components.ChunkRef{})
		serialized, err := factory.Serialize(w, h)
		require.NoError(t, err)
		require.NotNil(t, serialized)
		require.Equal(t, raw.Hp, serialized.Hp)
	}
}

func TestObjectFactoryHealthFailureBeforeSpawnOrInventory(t *testing.T) {
	installObjectHealthTestDefinitions(t)
	w := ecs.NewWorldForTesting()
	factory := NewObjectFactory(nil)
	rows := []repository.Inventory{{OwnerID: 1, Data: json.RawMessage(`{"width":2,"height":2,"items":[]}`)}}
	for _, hp := range []sql.NullFloat64{{}, {Float64: -1, Valid: true}, {Float64: math.NaN(), Valid: true}, {Float64: math.Inf(1), Valid: true}, {Float64: math.Inf(-1), Valid: true}} {
		raw := &repository.Object{ID: 1, TypeID: 911, Hp: hp}
		h, err := factory.Build(w, raw, rows)
		require.Error(t, err)
		if hp.Valid {
			require.ErrorIs(t, err, ErrInvalidObjectHP)
		} else {
			require.ErrorIs(t, err, ErrObjectHealthMissing)
		}
		require.Equal(t, types.InvalidHandle, h)
		require.Zero(t, w.EntityCount())
		require.Zero(t, testing.AllocsPerRun(1000, func() { _, _ = factory.Build(w, raw, rows) }), "health validation must precede inventory JSON allocations")
	}
}

type objectHealthMarshalProbe struct{ calls *int }

func (probe objectHealthMarshalProbe) MarshalJSON() ([]byte, error) {
	*probe.calls++
	return []byte(`{}`), nil
}

func TestObjectFactoryInvalidHealthSkipsSerialization(t *testing.T) {
	installObjectHealthTestDefinitions(t)
	w := ecs.NewWorldForTesting()
	factory := NewObjectFactory(nil)
	h := SpawnEntityFromDef(w, &objectdefs.ObjectDef{DefID: 910, Key: "health_rock", HP: 100}, DefSpawnParams{EntityID: 1})
	ecs.AddComponent(w, h, components.ChunkRef{})
	marshalCalls := 0
	ecs.WithComponent(w, h, func(state *components.ObjectInternalState) {
		state.State = &components.RuntimeObjectState{Behaviors: map[string]any{"probe": objectHealthMarshalProbe{calls: &marshalCalls}}}
	})
	for _, hp := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		ecs.WithComponent(w, h, func(health *components.ObjectInternalState) { health.HP = hp })
		raw, err := factory.Serialize(w, h)
		require.ErrorIs(t, err, ErrInvalidObjectHP)
		require.Nil(t, raw)
		require.Zero(t, marshalCalls)
		require.Zero(t, testing.AllocsPerRun(1000, func() { _, _ = factory.Serialize(w, h) }))
	}
	ecs.WithComponent(w, h, func(state *components.ObjectInternalState) { state.HasHP = false })
	raw, err := factory.Serialize(w, h)
	require.ErrorIs(t, err, ErrObjectHealthMissing)
	require.Nil(t, raw)
	require.Zero(t, marshalCalls)
	require.Zero(t, testing.AllocsPerRun(1000, func() { _, _ = factory.Serialize(w, h) }))

	// Even a transient empty build site cannot turn invalid HP into deletion intent.
	ecs.WithComponent(w, h, func(info *components.EntityInfo) { info.TypeID = constt.BuildObjectTypeID })
	ecs.WithComponent(w, h, func(state *components.ObjectInternalState) {
		state.State = nil
		components.SetBehaviorState(state, "build", &components.BuildBehaviorState{})
	})
	raw, err = factory.Serialize(w, h)
	require.ErrorIs(t, err, ErrObjectHealthMissing)
	require.Nil(t, raw)
}

func TestObjectFactoryDroppedItemHasNoHealth(t *testing.T) {
	installObjectHealthTestDefinitions(t)
	previousItems := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 900, Key: "health_stone", Resource: "stone", Size: itemdefs.Size{W: 1, H: 1}}}))
	w := ecs.NewWorldForTesting()
	factory := NewObjectFactory(nil)
	raw := &repository.Object{ID: 1, TypeID: constt.DroppedItemTypeID, Data: pqtype.NullRawMessage{Valid: true, RawMessage: []byte(`{"has_inventory":true,"contained_item_id":1,"drop_time":0,"dropper_id":0}`)}}
	inventories := []repository.Inventory{{OwnerID: 1, Kind: int16(constt.InventoryDroppedItem), Version: 1, Data: json.RawMessage(`{"items":[{"item_id":1,"type_id":900,"quantity":1}]}`)}}
	h, err := factory.Build(w, raw, inventories)
	require.NoError(t, err)
	state, ok := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.False(t, ok && state.HasHP)
	ecs.AddComponent(w, h, components.ChunkRef{})
	serialized, err := factory.Serialize(w, h)
	require.NoError(t, err)
	require.NotNil(t, serialized)
	require.False(t, serialized.Hp.Valid)
	snapshot, err := factory.CaptureWorldObjectSnapshot(w, h)
	require.NoError(t, err)
	require.Nil(t, snapshot.HP)
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	restored, err := factory.SpawnWorldObjectFromSnapshot(cm.world, snapshot, SnapshotSpawnOptions{ChunkManager: cm})
	require.NoError(t, err)
	state, ok = ecs.GetComponent[components.ObjectInternalState](cm.world, restored)
	require.True(t, ok, "restore must retain dropped items' existing internal state")
	require.False(t, state.HasHP)
	require.ErrorIs(t, SetObjectHP(cm.world, restored, 1), ErrObjectHealthMissing)
}
