package craftdefs

import (
	"os"
	"path/filepath"
	"testing"

	"origin/internal/itemdefs"
	"origin/internal/objectdefs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func craftDefsTestLogger() *zap.Logger {
	return zap.NewNop()
}

func setCraftDefsTestRegistries(t *testing.T) {
	t.Helper()

	prevItems := itemdefs.Global()
	prevObjects := objectdefs.Global()

	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1001, Key: "branch", Name: "Branch", Tags: []string{"wood"}},
		{DefID: 1002, Key: "stone", Name: "Stone", Tags: []string{"rock"}},
		{DefID: 1003, Key: "stone_axe", Name: "Stone Axe", Tags: []string{"tool"}},
		{DefID: 1004, Key: "wheat_seed", Name: "Wheat Seed", Tags: []string{"seed"}},
		{DefID: 1005, Key: "seed_pouch", Name: "Seed Pouch", Tags: []string{"container"}},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 2001, Key: "anvil", Name: "Anvil"},
	}))

	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(prevItems)
		objectdefs.SetGlobalForTesting(prevObjects)
	})
}

func writeCraftDefsTestFile(t *testing.T, dir string, name string, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0644))
	return path
}

func TestLoadFromDirectory_Success_ItemKeyAndItemTagInputs(t *testing.T) {
	setCraftDefsTestRegistries(t)
	dir := t.TempDir()

	writeCraftDefsTestFile(t, dir, "crafts.jsonc", `{
		"v": 1,
		"source": "test",
		"crafts": [
			{
				"defId": 1,
				"key": "stone_axe",
				"name": "Stone Axe",
				"inputs": [
					{ "itemKey": "branch", "count": 1, "qualityWeight": 1 },
					{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }
				],
				"outputs": [
					{ "itemKey": "stone_axe", "count": 1 }
				],
				"staminaCost": 10,
				"ticksRequired": 5
			},
			{
				"defId": 2,
				"key": "seed_sorting",
				"inputs": [
					{ "itemTag": "seed", "count": 2, "qualityWeight": 3 }
				],
				"outputs": [
					{ "itemKey": "seed_pouch", "count": 1 }
				],
				"requiredLinkedObjectKey": "anvil",
				"staminaCost": 1,
				"ticksRequired": 1
			}
		]
	}`)

	reg, err := LoadFromDirectory(dir, craftDefsTestLogger())
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.Equal(t, 2, reg.Count())

	axe, ok := reg.GetByKey("stone_axe")
	require.True(t, ok)
	require.Len(t, axe.Inputs, 2)
	assert.Equal(t, "branch", axe.Inputs[0].ItemKey)
	assert.Equal(t, "", axe.Inputs[0].ItemTag)
	assert.Equal(t, QualityFormulaWeightedAverageFloor, axe.QualityFormula)

	seedSorting, ok := reg.GetByKey("seed_sorting")
	require.True(t, ok)
	require.Len(t, seedSorting.Inputs, 1)
	assert.Equal(t, "", seedSorting.Inputs[0].ItemKey)
	assert.Equal(t, "seed", seedSorting.Inputs[0].ItemTag)
	assert.Equal(t, "seed_sorting", seedSorting.Name) // defaulted from key
	assert.Equal(t, "anvil", seedSorting.RequiredLinkedObject)
	assert.Equal(t, QualityFormulaWeightedAverageFloor, seedSorting.QualityFormula)
}

func TestLoadFromDirectory_AllowsFutureItemTag(t *testing.T) {
	setCraftDefsTestRegistries(t)
	dir := t.TempDir()

	writeCraftDefsTestFile(t, dir, "future_tag.json", `{
		"v": 1,
		"source": "test",
		"crafts": [
			{
				"defId": 1,
				"key": "future_seed_recipe",
				"inputs": [
					{ "itemTag": "future_seed", "count": 1, "qualityWeight": 1 }
				],
				"outputs": [
					{ "itemKey": "seed_pouch", "count": 1 }
				],
				"staminaCost": 0,
				"ticksRequired": 1
			}
		]
	}`)

	reg, err := LoadFromDirectory(dir, craftDefsTestLogger())
	require.NoError(t, err)
	require.NotNil(t, reg)

	recipe, ok := reg.GetByKey("future_seed_recipe")
	require.True(t, ok)
	require.Len(t, recipe.Inputs, 1)
	assert.Equal(t, "future_seed", recipe.Inputs[0].ItemTag)
}

func TestLoadFromDirectory_InputMissingItemKeyAndItemTag(t *testing.T) {
	setCraftDefsTestRegistries(t)
	dir := t.TempDir()

	writeCraftDefsTestFile(t, dir, "bad.json", `{
		"v": 1,
		"source": "test",
		"crafts": [
			{
				"defId": 1,
				"key": "bad_recipe",
				"inputs": [
					{ "count": 1, "qualityWeight": 1 }
				],
				"outputs": [
					{ "itemKey": "stone_axe", "count": 1 }
				],
				"staminaCost": 1,
				"ticksRequired": 1
			}
		]
	}`)

	_, err := LoadFromDirectory(dir, craftDefsTestLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of itemKey or itemTag")
}

func TestLoadFromDirectory_InputHasBothItemKeyAndItemTag(t *testing.T) {
	setCraftDefsTestRegistries(t)
	dir := t.TempDir()

	writeCraftDefsTestFile(t, dir, "bad.json", `{
		"v": 1,
		"source": "test",
		"crafts": [
			{
				"defId": 1,
				"key": "bad_recipe",
				"inputs": [
					{ "itemKey": "branch", "itemTag": "seed", "count": 1, "qualityWeight": 1 }
				],
				"outputs": [
					{ "itemKey": "stone_axe", "count": 1 }
				],
				"staminaCost": 1,
				"ticksRequired": 1
			}
		]
	}`)

	_, err := LoadFromDirectory(dir, craftDefsTestLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of itemKey or itemTag")
}

func TestLoadFromDirectory_UnknownExactItemKeyRejected(t *testing.T) {
	setCraftDefsTestRegistries(t)
	dir := t.TempDir()

	writeCraftDefsTestFile(t, dir, "bad.json", `{
		"v": 1,
		"source": "test",
		"crafts": [
			{
				"defId": 1,
				"key": "bad_recipe",
				"inputs": [
					{ "itemKey": "missing_item", "count": 1, "qualityWeight": 1 }
				],
				"outputs": [
					{ "itemKey": "stone_axe", "count": 1 }
				],
				"staminaCost": 1,
				"ticksRequired": 1
			}
		]
	}`)

	_, err := LoadFromDirectory(dir, craftDefsTestLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "itemKey unknown: missing_item")
}
