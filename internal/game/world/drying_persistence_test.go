package world_test

import (
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	ecssystems "origin/internal/ecs/systems"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/lifecycle"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const dryingPersistenceObjectID types.EntityID = 31001

type dryingPersistenceIDAllocator struct{ next types.EntityID }

func (a *dryingPersistenceIDAllocator) GetFreeID() types.EntityID {
	a.next++
	return a.next
}

func dryingPersistenceRegistry(t *testing.T) *behaviors.Registry {
	t.Helper()
	previousItems, previousObjects := itemdefs.Global(), objectdefs.Global()
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	items, err := itemdefs.LoadFromDirectory("../../../data/items", zap.NewNop())
	require.NoError(t, err)
	itemdefs.SetGlobalForTesting(items)
	registry := behaviors.MustDefaultRegistry()
	objects, err := objectdefs.LoadFromDirectory("../../../data/objects", registry, zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	return registry
}

func setDryingPersistenceTime(w *ecs.World, runtime int64, tick uint64, wall time.Time) {
	ecs.SetResource(w, ecs.TimeState{
		RuntimeSecondsTotal: runtime, Tick: tick, TickRate: 20, Delta: 0.05,
		Now: time.Unix(runtime, 0), WallNow: wall, UnixMs: wall.UnixMilli(),
	})
}

func restoreDryingPersistenceObject(t *testing.T, w *ecs.World, factory *gameworld.ObjectFactory, registry *behaviors.Registry, raw *repository.Object, rows []repository.Inventory) types.Handle {
	t.Helper()
	handle, err := factory.BuildForChunk(w, raw, rows)
	require.NoError(t, err)
	require.NotEqual(t, types.InvalidHandle, handle)
	ecs.AddComponent(w, handle, components.ChunkRef{CurrentChunkX: raw.ChunkX, CurrentChunkY: raw.ChunkY})
	restored, err := factory.DeserializeObjectState(raw)
	require.NoError(t, err)
	ecs.WithComponent(w, handle, func(internal *components.ObjectInternalState) {
		internal.State = restored
		internal.Flags = nil
		internal.IsDirty = false
	})
	factory.RestoreDerivedComponentsFromState(w, handle)
	info, found := ecs.GetComponent[components.EntityInfo](w, handle)
	require.True(t, found)
	require.NoError(t, registry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
		World: w, Handle: handle, EntityID: types.EntityID(raw.ID), EntityType: info.TypeID,
		Reason: contracts.ObjectBehaviorInitReasonRestore,
	}, info.Behaviors))
	ecssystems.RecomputeObjectBehaviorsNow(w, nil, zap.NewNop(), registry, []types.Handle{handle})
	return handle
}

func dryingPersistenceInventory(t *testing.T, w *ecs.World) (types.Handle, components.InventoryContainer) {
	t.Helper()
	handle, found := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, dryingPersistenceObjectID, 0)
	require.True(t, found)
	require.True(t, w.Alive(handle))
	container, found := ecs.GetComponent[components.InventoryContainer](w, handle)
	require.True(t, found)
	return handle, container
}

func dryingPersistenceEntries(t *testing.T, w *ecs.World, handle types.Handle) []components.DryingItemState {
	t.Helper()
	internal, found := ecs.GetComponent[components.ObjectInternalState](w, handle)
	require.True(t, found)
	state, found := components.GetBehaviorState[components.DryingBehaviorState](internal, "drying")
	require.True(t, found)
	return append([]components.DryingItemState(nil), state.Entries...)
}

func saveDryingPersistenceObject(t *testing.T, factory *gameworld.ObjectFactory, w *ecs.World, handle types.Handle) (*repository.Object, []repository.Inventory) {
	t.Helper()
	raw, err := factory.Serialize(w, handle)
	require.NoError(t, err)
	require.NotNil(t, raw)
	rows, err := factory.SerializeObjectInventories(w, handle)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return raw, rows
}

func TestDryingPersistenceRestoresRuntimeDeadlinesAndContents(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		restart bool
	}{
		{"online unload and overdue reload", false},
		{"offline pause and fresh world restart", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			registry := dryingPersistenceRegistry(t)
			factory := gameworld.NewObjectFactory(nil)
			w := ecs.NewWorldForTesting()
			wall := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
			setDryingPersistenceTime(w, 100, 5000, wall)
			frame := restoreDryingPersistenceObject(t, w, factory, registry, &repository.Object{
				ID: int64(dryingPersistenceObjectID), TypeID: 19, Quality: 80, Region: 1, Layer: 0,
				X: 10, Y: 20, ChunkX: 0, ChunkY: 0,
			}, nil)
			station, found := ecs.GetComponent[components.StationState](w, frame)
			require.True(t, found, "legacy frame without a station payload uses the current definition")
			require.Equal(t, []string{"drying"}, station.Capabilities)
			require.Equal(t, "ready", station.CurrentState)
			inputDef, found := itemdefs.Global().GetByKey("wild_windsown_weed")
			require.True(t, found)
			outputKeys := []string{"seed_wheat", "seed_carrot", "seed_beet", "seed_potato"}
			behavior, found := registry.GetBehavior("drying")
			require.True(t, found)
			listener, supported := behavior.(contracts.RootInventoryMutationBehavior)
			require.True(t, supported)
			inputs := []components.InvItem{
				{ItemID: 41001, TypeID: uint32(inputDef.DefID), Resource: inputDef.ResolveResource(false), Quality: 37, Quantity: 1, W: 1, H: 1, X: 0, Y: 0},
				{ItemID: 41002, TypeID: uint32(inputDef.DefID), Resource: inputDef.ResolveResource(false), Quality: 8, Quantity: 1, W: 1, H: 1, X: 1, Y: 1},
			}
			root, _ := dryingPersistenceInventory(t, w)
			for i, input := range inputs {
				if i > 0 {
					setDryingPersistenceTime(w, 125, 5001, wall.Add(25*time.Second))
				}
				ecs.WithComponent(w, root, func(container *components.InventoryContainer) {
					container.Items = append(container.Items, input)
					container.Version++
				})
				_, err := listener.OnRootInventoryMutation(&contracts.RootInventoryMutationContext{
					World: w, Handle: frame, EntityID: dryingPersistenceObjectID, EntityType: 19,
				})
				require.NoError(t, err)
			}
			deadlines := dryingPersistenceEntries(t, w, frame)
			require.Len(t, deadlines, 2)
			require.Equal(t, int64(57700), deadlines[0].CompletionRuntimeSeconds)
			require.Equal(t, int64(57725), deadlines[1].CompletionRuntimeSeconds)
			require.Equal(t, uint8(1), deadlines[1].InputX)
			require.Equal(t, uint8(1), deadlines[1].InputY)
			require.Equal(t, 1, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
			setDryingPersistenceTime(w, 200, 5002, wall.Add(100*time.Second))
			saved, rows := saveDryingPersistenceObject(t, factory, w, frame)
			require.True(t, saved.Data.Valid)
			require.EqualValues(t, 80, saved.Quality)
			require.EqualValues(t, 3, rows[0].Version)

			// The same teardown used for owned object inventories must remove the
			// frame's physical scheduler entry as well as its inventory references.
			require.True(t, lifecycle.DeleteObject(w, dryingPersistenceObjectID, frame, lifecycle.DeleteObjectOptions{DeleteOwnedInventories: true}))
			require.False(t, w.Alive(frame))
			require.False(t, w.Alive(root))
			require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
			_, found = ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, dryingPersistenceObjectID, 0)
			require.False(t, found)

			updates := 0
			deps := &contracts.ExecutionDeps{
				IDAllocator: &dryingPersistenceIDAllocator{next: 50000},
				RootInventoryUpdate: func(_ *ecs.World, rootID types.EntityID) {
					require.Equal(t, dryingPersistenceObjectID, rootID)
					updates++
				},
			}
			system := ecssystems.NewBehaviorTickSystem(zap.NewNop(), ecssystems.BehaviorTickSystemConfig{BehaviorRegistry: registry, ExecutionDeps: deps})
			if scenario.restart {
				w = ecs.NewWorldForTesting()
				// A year offline and a reset global/local tick cannot consume
				// any of the saved server-runtime deadline.
				setDryingPersistenceTime(w, 200, 0, wall.AddDate(1, 0, 0))
			} else {
				setDryingPersistenceTime(w, 57750, 5003, wall.Add(57650*time.Second))
				system.Update(w, 0.05)
				require.Zero(t, updates, "unloaded frames have no live work")
			}
			frame = restoreDryingPersistenceObject(t, w, factory, registry, saved, rows)
			_, container := dryingPersistenceInventory(t, w)
			require.Equal(t, inputs, container.Items)
			require.Equal(t, deadlines, dryingPersistenceEntries(t, w, frame))
			station, found = ecs.GetComponent[components.StationState](w, frame)
			require.True(t, found)
			require.Equal(t, "ready", station.CurrentState)
			require.EqualValues(t, 3, container.Version)
			require.Equal(t, 1, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
			require.Zero(t, updates, "restore schedules overdue work after activation")

			if scenario.restart {
				system.Update(w, 0.05)
				require.Zero(t, updates)
				setDryingPersistenceTime(w, 57699, 999999999, wall.AddDate(2, 0, 0))
				system.Update(w, 0.05)
				_, container = dryingPersistenceInventory(t, w)
				require.Equal(t, inputs, container.Items, "ticks and wall time cannot finish the process")
				require.Equal(t, deadlines, dryingPersistenceEntries(t, w, frame))
				setDryingPersistenceTime(w, 57700, 1, wall.Add(-time.Hour))
				system.Update(w, 0.05)
				_, container = dryingPersistenceInventory(t, w)
				output, found := itemdefs.Global().GetByID(int(container.Items[0].TypeID))
				require.True(t, found)
				require.Contains(t, outputKeys, output.Key)
				require.Equal(t, inputs[1], container.Items[1], "second independent deadline remains pending")
				require.Equal(t, deadlines[1:], dryingPersistenceEntries(t, w, frame))
				require.Equal(t, 1, updates)
				setDryingPersistenceTime(w, 57725, 2, wall.Add(-time.Hour))
			}
			system.Update(w, 0.05)
			_, container = dryingPersistenceInventory(t, w)
			require.Len(t, container.Items, 2)
			for i, result := range container.Items {
				output, found := itemdefs.Global().GetByID(int(result.TypeID))
				require.True(t, found)
				require.Contains(t, outputKeys, output.Key)
				require.Equal(t, inputs[i].Quality, result.Quality)
				require.Equal(t, inputs[i].X, result.X)
				require.Equal(t, inputs[i].Y, result.Y)
				require.EqualValues(t, 1, result.Quantity)
				require.Equal(t, output.ResolveResource(false), result.Resource)
				require.Equal(t, types.EntityID(50001+i), result.ItemID)
				require.NotEqual(t, inputs[i].ItemID, result.ItemID)
			}
			require.Empty(t, dryingPersistenceEntries(t, w, frame))
			require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
			if scenario.restart {
				require.Equal(t, 2, updates)
				require.EqualValues(t, 5, container.Version)
			} else {
				require.Equal(t, 1, updates)
				require.EqualValues(t, 4, container.Version)
			}

			// Save the completed output and pass it through a second fresh-world
			// restore, proving new identity and cleared processing state persist.
			completed, completedRows := saveDryingPersistenceObject(t, factory, w, frame)
			restoredWorld := ecs.NewWorldForTesting()
			setDryingPersistenceTime(restoredWorld, ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal, 0, wall.AddDate(3, 0, 0))
			restoredFrame := restoreDryingPersistenceObject(t, restoredWorld, factory, registry, completed, completedRows)
			_, restoredContainer := dryingPersistenceInventory(t, restoredWorld)
			require.Equal(t, container.Items, restoredContainer.Items)
			require.Equal(t, container.Version, restoredContainer.Version)
			require.Empty(t, dryingPersistenceEntries(t, restoredWorld, restoredFrame))
			require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(restoredWorld).PendingCount())
		})
	}
}
