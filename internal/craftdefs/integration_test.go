package craftdefs_test

import (
	"path/filepath"
	"testing"

	"origin/internal/craftdefs"
	"origin/internal/game/behaviors"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadAllCrafts_RegistersNettleShirtRecipe(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)

	previousItems := itemdefs.Global()
	previousObjects := objectdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
	})

	crafts, err := craftdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "crafts"), zap.NewNop())
	require.NoError(t, err)

	nettleShirt, ok := crafts.GetByKey("nettle_shirt")
	require.True(t, ok, "nettle_shirt recipe should be loaded")
	require.Equal(t, []craftdefs.CraftInput{{ItemKey: "branch", Count: 1, QualityWeight: 1}}, nettleShirt.Inputs)
	require.Equal(t, []craftdefs.CraftOutput{{ItemKey: "nettle_shirt", Count: 1}}, nettleShirt.Outputs)
	assert.Equal(t, 100.0, nettleShirt.StaminaCost)
	assert.Equal(t, uint32(10), nettleShirt.TicksRequired)
}

func TestLoadAllCrafts_RegistersStationRequirementFixtures(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)

	previousItems := itemdefs.Global()
	previousObjects := objectdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
	})

	crafts, err := craftdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "crafts"), zap.NewNop())
	require.NoError(t, err)

	campfire, ok := objects.GetByKey("campfire")
	require.True(t, ok)
	require.NotNil(t, campfire.Station)
	require.Equal(t, []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}}, campfireRecipeConsumption(t, crafts))

	burningRecipe, ok := crafts.GetByKey("campfire_dried_branch")
	require.True(t, ok)
	require.Equal(t, "campfire", burningRecipe.RequiredLinkedObject)
	require.Equal(t, "burning", burningRecipe.StationRequirements[0].State)

	noStationRecipe, ok := crafts.GetByKey("stone_axe")
	require.True(t, ok, "existing starter recipes must remain available")
	assert.Empty(t, noStationRecipe.StationRequirements)
}

func campfireRecipeConsumption(t *testing.T, crafts *craftdefs.Registry) []craftdefs.StationResourceConsumption {
	t.Helper()
	threadedRecipe, ok := crafts.GetByKey("campfire_threaded_branch")
	require.True(t, ok)
	require.Len(t, threadedRecipe.StationRequirements, 1)
	return threadedRecipe.StationRequirements[0].Consume
}
