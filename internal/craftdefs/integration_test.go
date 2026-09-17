package craftdefs

import (
	"path/filepath"
	"testing"

	"origin/internal/itemdefs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadAllCrafts_RegistersNettleShirtRecipe(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)

	previousItems := itemdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })

	crafts, err := LoadFromDirectory(filepath.Join("..", "..", "data", "crafts"), zap.NewNop())
	require.NoError(t, err)

	nettleShirt, ok := crafts.GetByKey("nettle_shirt")
	require.True(t, ok, "nettle_shirt recipe should be loaded")
	require.Equal(t, []CraftInput{{ItemKey: "branch", Count: 1, QualityWeight: 1}}, nettleShirt.Inputs)
	require.Equal(t, []CraftOutput{{ItemKey: "nettle_shirt", Count: 1}}, nettleShirt.Outputs)
	assert.Equal(t, 100.0, nettleShirt.StaminaCost)
	assert.Equal(t, uint32(10), nettleShirt.TicksRequired)
}
