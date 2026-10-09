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
	for _, definition := range objects.All() {
		require.Equal(t, definition.Key == "player_dead", definition.Indestructible,
			"only player_dead is indestructible in the object catalog: %s", definition.Key)
		if definition.Key == "player" {
			require.Zero(t, definition.HP)
			continue
		}
		require.Positive(t, definition.HP, "object %s (defId=%d)", definition.Key, definition.DefID)
	}

	for _, key := range []string{"boulder", "kiln", "campfire", "player_dead", "build"} {
		definition, exists := objects.GetByKey(key)
		require.True(t, exists, "object %s", key)
		require.Equal(t, 100, definition.HP, "object %s", key)
	}
	for key, hp := range map[string]int{"log_x": 1, "log_y": 1, "tree_birch": 100} {
		definition, exists := objects.GetByKey(key)
		require.True(t, exists, "object %s", key)
		require.Equal(t, hp, definition.HP, "existing HP for %s", key)
	}
}
