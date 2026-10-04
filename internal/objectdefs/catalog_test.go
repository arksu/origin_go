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
	for _, definition := range objects.All() {
		if definition.Key == "player" {
			require.Zero(t, definition.HP)
			continue
		}
		require.Positive(t, definition.HP, "object %s (defId=%d)", definition.Key, definition.DefID)
	}

	for _, key := range []string{"boulder", "kiln", "campfire", "player_death", "build"} {
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
