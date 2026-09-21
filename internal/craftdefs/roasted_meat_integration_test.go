package craftdefs_test

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/craftdefs"
	"origin/internal/game/behaviors"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"path/filepath"
	"testing"
)

func TestLoadRoastedMeatCatalog(t *testing.T) {
	previousItems, previousObjects := itemdefs.Global(), objectdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems); objectdefs.SetGlobalForTesting(previousObjects) })
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	itemdefs.SetGlobalForTesting(items)
	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	crafts, err := craftdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "crafts"), zap.NewNop())
	require.NoError(t, err)
	recipe, ok := crafts.GetByKey("roasted_meat")
	require.True(t, ok)
	require.Equal(t, "Roasted meat", recipe.Name)
	require.Equal(t, []craftdefs.CraftInput{{ItemTag: "raw_meat", Count: 1, QualityWeight: 1}}, recipe.Inputs)
	require.Equal(t, []craftdefs.CraftOutput{{ItemKey: "roasted_meat", Count: 1}}, recipe.Outputs)
	require.Equal(t, map[string]string{"beef": "roasted_beef", "fox_meat": "roasted_fox_meat", "rabbit_meat": "roasted_rabbit_meat", "boar_meat": "roasted_boar_meat", "bear_meat": "roasted_bear_meat", "raw_deer_meat": "roasted_deer_meat", "raw_mutton": "roasted_mutton", "raw_pork": "roast_pork", "raw_chicken_meat": "roasted_chicken_meat"}, recipe.OutputByInputKey)
	require.Equal(t, []craftdefs.StationRequirement{{Capability: "cooking", State: "burning"}}, recipe.StationRequirements)
	require.Empty(t, recipe.RequiredLinkedObject)
	require.Empty(t, recipe.RequiredSkills)
	require.Empty(t, recipe.RequiredDiscovery)
	require.Equal(t, craftdefs.QualityFormulaWeightedAverageFloor, recipe.QualityFormula)
	require.Equal(t, uint32(10), recipe.TicksRequired)
	require.Equal(t, float64(10), recipe.StaminaCost)
	for sourceKey, targetKey := range recipe.OutputByInputKey {
		source, ok := items.GetByKey(sourceKey)
		require.True(t, ok)
		require.Contains(t, source.Tags, "raw_meat")
		_, ok = items.GetByKey(targetKey)
		require.True(t, ok)
	}
}
