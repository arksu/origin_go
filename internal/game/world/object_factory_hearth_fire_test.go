package world_test

import (
	"path/filepath"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHearthFireDefinitionSpawnsAndRestoresWithoutGameplayState(t *testing.T) {
	dataPath := filepath.Join("..", "..", "..", "data")
	items, err := itemdefs.LoadFromDirectory(filepath.Join(dataPath, "items"), zap.NewNop())
	require.NoError(t, err)
	previousItems := itemdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	objects, err := objectdefs.LoadFromDirectory(filepath.Join(dataPath, "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	previousObjects := objectdefs.Global()
	objectdefs.SetGlobalForTesting(objects)
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previousObjects) })
	definition, exists := objects.GetByKey("hearth_fire")
	require.True(t, exists)

	assertHearth := func(w *ecs.World, h types.Handle) {
		t.Helper()
		require.True(t, w.Alive(h))
		info, exists := ecs.GetComponent[components.EntityInfo](w, h)
		require.True(t, exists)
		require.Equal(t, uint32(36), info.TypeID)
		require.True(t, info.IsStatic)
		require.True(t, info.Indestructible)
		require.Empty(t, info.Behaviors)
		appearance, exists := ecs.GetComponent[components.Appearance](w, h)
		require.True(t, exists)
		require.Equal(t, "hearth_fire", appearance.Resource)
		collider, exists := ecs.GetComponent[components.Collider](w, h)
		require.True(t, exists)
		require.Equal(t, objectdefs.BuildColliderComponent(definition.Components.Collider), collider)
		require.InDelta(t, constt.CoordPerTile, definition.Components.Collider.W, 0)
		state, exists := ecs.GetComponent[components.ObjectInternalState](w, h)
		require.True(t, exists)
		require.True(t, state.HasHP)
		require.Equal(t, float64(100), state.HP)
		require.Nil(t, state.State)
		require.False(t, ecs.HasComponent[components.StationState](w, h))
		require.False(t, ecs.HasComponent[components.InventoryOwner](w, h))
	}

	w := ecs.NewWorldForTesting()
	h := world.SpawnEntityFromDef(w, definition, world.DefSpawnParams{EntityID: 42, X: 120, Y: 120, Region: 1})
	assertHearth(w, h)
	ecs.AddComponent(w, h, components.ChunkRef{})
	factory := world.NewObjectFactory(nil)
	raw, err := factory.Serialize(w, h)
	require.NoError(t, err)
	require.Equal(t, 36, raw.TypeID)
	require.False(t, raw.Data.Valid, "the preparation must not persist invented Hearth behavior state")
	for _, build := range []func(*ecs.World, *repository.Object, []repository.Inventory) (types.Handle, error){factory.Build, factory.BuildForChunk} {
		restoredWorld := ecs.NewWorldForTesting()
		restored, err := build(restoredWorld, raw, nil)
		require.NoError(t, err)
		assertHearth(restoredWorld, restored)
	}
}
