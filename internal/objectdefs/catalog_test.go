package objectdefs_test

import (
	"path/filepath"
	"testing"

	"origin/internal/game/behaviors"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestObjectCatalogInitialHP(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	previousItems := itemdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })

	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	require.NotEmpty(t, objects.All())
	corpse, exists := objects.GetByID(15)
	require.True(t, exists)
	require.Equal(t, "player_dead", corpse.Key)
	require.Equal(t, "Dead Player", corpse.Name)
	protected := map[string]bool{"player_dead": true, "player_skeleton": true, "player_skeleton_without_skull": true}
	for _, definition := range objects.All() {
		require.Equal(t, protected[definition.Key], definition.Indestructible,
			"only player remains are indestructible in the object catalog: %s", definition.Key)
		if definition.Key == "player" {
			require.Zero(t, definition.HP)
			continue
		}
		require.Positive(t, definition.HP, "object %s (defId=%d)", definition.Key, definition.DefID)
	}

	for _, key := range []string{"boulder", "kiln", "campfire", "player_dead", "player_skeleton", "player_skeleton_without_skull", "build"} {
		definition, exists := objects.GetByKey(key)
		require.True(t, exists, "object %s", key)
		require.Equal(t, 100, definition.HP, "object %s", key)
	}
	for key, id := range map[string]int{"player_skeleton": 17, "player_skeleton_without_skull": 18} {
		definition, exists := objects.GetByKey(key)
		require.True(t, exists)
		require.Equal(t, id, definition.DefID)
		require.True(t, definition.IsStatic)
		require.Equal(t, key, definition.Resource)
		require.InDelta(t, 9, definition.Components.Collider.W, 0)
		require.InDelta(t, 9, definition.Components.Collider.H, 0)
		require.Empty(t, definition.Components.Inventory)
		if key == "player_skeleton" {
			require.Len(t, definition.Behaviors, 2)
			require.Contains(t, definition.Behaviors, "player_skeleton")
		} else {
			require.Len(t, definition.Behaviors, 1)
			require.NotContains(t, definition.Behaviors, "player_skeleton")
		}
		require.Contains(t, definition.Behaviors, "lift")
	}
	for key, hp := range map[string]int{"log_x": 1, "log_y": 1, "tree_birch": 100} {
		definition, exists := objects.GetByKey(key)
		require.True(t, exists, "object %s", key)
		require.Equal(t, hp, definition.HP, "existing HP for %s", key)
	}
}
