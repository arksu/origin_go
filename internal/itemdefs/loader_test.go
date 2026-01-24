package itemdefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func TestLoadFromDirectory_Success(t *testing.T) {
	dir := t.TempDir()

	foodJSON := `{
		"v": 1,
		"source": "food",
		"items": [
			{
				"defId": 2001,
				"key": "food_bread",
				"name": "Bread",
				"tags": ["food"],
				"size": { "w": 1, "h": 1 },
				"stack": { "mode": "stack", "max": 20 },
				"allowed": { "hand": true, "grid": true, "equipmentSlots": [] }
			}
		]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "food.json"), []byte(foodJSON), 0644))

	toolsJSON := `{
		"v": 1,
		"source": "tools",
		"items": [
			{
				"defId": 1001,
				"key": "tool_pickaxe",
				"name": "Pickaxe",
				"tags": ["tool"],
				"size": { "w": 1, "h": 2 },
				"stack": null,
				"allowed": { "hand": true, "grid": true, "equipmentSlots": ["mainHand"] }
			}
		]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tools.json"), []byte(toolsJSON), 0644))

	registry, err := LoadFromDirectory(dir, testLogger())
	require.NoError(t, err)

	assert.Equal(t, 2, registry.Count())

	bread, ok := registry.GetByID(2001)
	require.True(t, ok)
	assert.Equal(t, "food_bread", bread.Key)
	assert.Equal(t, "Bread", bread.Name)
	assert.Equal(t, 1, bread.Size.W)
	assert.Equal(t, 1, bread.Size.H)
	assert.Equal(t, StackModeStack, bread.Stack.Mode)
	assert.Equal(t, 20, bread.Stack.Max)

	pickaxe, ok := registry.GetByKey("tool_pickaxe")
	require.True(t, ok)
	assert.Equal(t, 1001, pickaxe.DefID)
	assert.Nil(t, pickaxe.Stack)
}

func TestLoadFromDirectory_DuplicateDefID(t *testing.T) {
	dir := t.TempDir()

	file1 := `{
		"v": 1,
		"source": "file1",
		"items": [{ "defId": 1001, "key": "item_a", "name": "A", "tags": [], "size": { "w": 1, "h": 1 }, "allowed": { "hand": true, "grid": true, "equipmentSlots": [] } }]
	}`
	file2 := `{
		"v": 1,
		"source": "file2",
		"items": [{ "defId": 1001, "key": "item_b", "name": "B", "tags": [], "size": { "w": 1, "h": 1 }, "allowed": { "hand": true, "grid": true, "equipmentSlots": [] } }]
	}`

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(file1), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"), []byte(file2), 0644))

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate defId")
}

func TestLoadFromDirectory_DuplicateKey(t *testing.T) {
	dir := t.TempDir()

	file1 := `{
		"v": 1,
		"source": "file1",
		"items": [{ "defId": 1001, "key": "same_key", "name": "A", "tags": [], "size": { "w": 1, "h": 1 }, "allowed": { "hand": true, "grid": true, "equipmentSlots": [] } }]
	}`
	file2 := `{
		"v": 1,
		"source": "file2",
		"items": [{ "defId": 1002, "key": "same_key", "name": "B", "tags": [], "size": { "w": 1, "h": 1 }, "allowed": { "hand": true, "grid": true, "equipmentSlots": [] } }]
	}`

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(file1), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"), []byte(file2), 0644))

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key")
}

func TestLoadFromDirectory_InvalidVersion(t *testing.T) {
	dir := t.TempDir()

	json := `{
		"v": 99,
		"source": "test",
		"items": []
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.json"), []byte(json), 0644))

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported version")
}

func TestLoadFromDirectory_UnknownField(t *testing.T) {
	dir := t.TempDir()

	json := `{
		"v": 1,
		"source": "test",
		"unknownField": true,
		"items": []
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.json"), []byte(json), 0644))

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse JSON")
}

func TestLoadFromDirectory_InvalidDefID(t *testing.T) {
	dir := t.TempDir()

	json := `{
		"v": 1,
		"source": "test",
		"items": [{ "defId": 0, "key": "test", "name": "Test", "tags": [], "size": { "w": 1, "h": 1 }, "allowed": { "hand": true, "grid": true, "equipmentSlots": [] } }]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.json"), []byte(json), 0644))

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defId must be > 0")
}

func TestLoadFromDirectory_MissingKey(t *testing.T) {
	dir := t.TempDir()

	json := `{
		"v": 1,
		"source": "test",
		"items": [{ "defId": 1001, "key": "", "name": "Test", "tags": [], "size": { "w": 1, "h": 1 }, "allowed": { "hand": true, "grid": true, "equipmentSlots": [] } }]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.json"), []byte(json), 0644))

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key is required")
}

func TestLoadFromDirectory_InvalidSize(t *testing.T) {
	dir := t.TempDir()

	json := `{
		"v": 1,
		"source": "test",
		"items": [{ "defId": 1001, "key": "test", "name": "Test", "tags": [], "size": { "w": 0, "h": 1 }, "allowed": { "hand": true, "grid": true, "equipmentSlots": [] } }]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.json"), []byte(json), 0644))

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size.w must be >= 1")
}

func TestLoadFromDirectory_InvalidStackMax(t *testing.T) {
	dir := t.TempDir()

	json := `{
		"v": 1,
		"source": "test",
		"items": [{ "defId": 1001, "key": "test", "name": "Test", "tags": [], "size": { "w": 1, "h": 1 }, "stack": { "mode": "stack", "max": 1 }, "allowed": { "hand": true, "grid": true, "equipmentSlots": [] } }]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.json"), []byte(json), 0644))

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stack.max must be >= 2")
}

func TestLoadFromDirectory_InvalidStackMode(t *testing.T) {
	dir := t.TempDir()

	json := `{
		"v": 1,
		"source": "test",
		"items": [{ "defId": 1001, "key": "test", "name": "Test", "tags": [], "size": { "w": 1, "h": 1 }, "stack": { "mode": "invalid", "max": 10 }, "allowed": { "hand": true, "grid": true, "equipmentSlots": [] } }]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.json"), []byte(json), 0644))

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid stack.mode")
}

func TestLoadFromDirectory_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	registry, err := LoadFromDirectory(dir, testLogger())
	require.NoError(t, err)
	assert.Equal(t, 0, registry.Count())
}

func TestRegistry_GetByID_NotFound(t *testing.T) {
	registry := NewRegistry(nil)

	_, ok := registry.GetByID(9999)
	assert.False(t, ok)
}

func TestRegistry_GetByKey_NotFound(t *testing.T) {
	registry := NewRegistry(nil)

	_, ok := registry.GetByKey("nonexistent")
	assert.False(t, ok)
}

func TestLoadFromDirectory_DefaultValues(t *testing.T) {
	dir := t.TempDir()

	json := `{
		"v": 1,
		"source": "test",
		"items": [
			{
				"defId": 1001,
				"key": "test_item",
				"name": "Test Item",
				"tags": [],
				"size": { "w": 1, "h": 1 }
			}
		]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.json"), []byte(json), 0644))

	registry, err := LoadFromDirectory(dir, testLogger())
	require.NoError(t, err)

	item, ok := registry.GetByID(1001)
	require.True(t, ok)

	require.NotNil(t, item.Allowed.Hand)
	assert.True(t, *item.Allowed.Hand)

	require.NotNil(t, item.Allowed.Grid)
	assert.True(t, *item.Allowed.Grid)

	require.NotNil(t, item.Allowed.EquipmentSlots)
	assert.Equal(t, 0, len(item.Allowed.EquipmentSlots))
}

func TestLoadFromDirectory_ExplicitValues(t *testing.T) {
	dir := t.TempDir()

	json := `{
		"v": 1,
		"source": "test",
		"items": [
			{
				"defId": 1001,
				"key": "test_item",
				"name": "Test Item",
				"tags": [],
				"size": { "w": 1, "h": 1 },
				"allowed": {
					"hand": false,
					"grid": false,
					"equipmentSlots": ["head", "chest"]
				}
			}
		]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.json"), []byte(json), 0644))

	registry, err := LoadFromDirectory(dir, testLogger())
	require.NoError(t, err)

	item, ok := registry.GetByID(1001)
	require.True(t, ok)

	require.NotNil(t, item.Allowed.Hand)
	assert.False(t, *item.Allowed.Hand)

	require.NotNil(t, item.Allowed.Grid)
	assert.False(t, *item.Allowed.Grid)

	require.NotNil(t, item.Allowed.EquipmentSlots)
	assert.Equal(t, 2, len(item.Allowed.EquipmentSlots))
	assert.Equal(t, "head", item.Allowed.EquipmentSlots[0])
	assert.Equal(t, "chest", item.Allowed.EquipmentSlots[1])
}
