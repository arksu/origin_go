package objectdefs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"origin/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

type testBehaviorRegistry struct {
	byKey map[string]types.Behavior
}

func (r *testBehaviorRegistry) GetBehavior(key string) (types.Behavior, bool) {
	if r == nil {
		return nil, false
	}
	behavior, ok := r.byKey[key]
	return behavior, ok
}

func (r *testBehaviorRegistry) Keys() []string {
	if r == nil || len(r.byKey) == 0 {
		return nil
	}
	keys := make([]string, 0, len(r.byKey))
	for key := range r.byKey {
		keys = append(keys, key)
	}
	return keys
}

func (r *testBehaviorRegistry) IsRegisteredBehaviorKey(key string) bool {
	if r == nil {
		return false
	}
	_, ok := r.byKey[key]
	return ok
}

func (r *testBehaviorRegistry) ValidateBehaviorKeys(keys []string) error {
	for _, key := range keys {
		if !r.IsRegisteredBehaviorKey(key) {
			return fmt.Errorf("unknown behavior %q", key)
		}
	}
	return nil
}

func (r *testBehaviorRegistry) InitObjectBehaviors(_ *types.BehaviorObjectInitContext, _ []string) error {
	return nil
}

type testPriorityOnlyBehavior struct {
	key string
}

func (b testPriorityOnlyBehavior) Key() string { return b.key }

func (b testPriorityOnlyBehavior) ValidateAndApplyDefConfig(ctx *types.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 100, nil
	}
	var cfg struct {
		Priority int `json:"priority,omitempty"`
	}
	if err := decodeStrictJSONForTest(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid %s config: %w", b.key, err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = 100
	}
	return cfg.Priority, nil
}

type testTreeBehavior struct{}

func (testTreeBehavior) Key() string { return "tree" }

func (testTreeBehavior) ValidateAndApplyDefConfig(ctx *types.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 100, nil
	}

	targetDef, ok := ctx.Def.(*ObjectDef)
	if !ok || targetDef == nil {
		return 0, fmt.Errorf("tree config target def must be *ObjectDef")
	}

	var cfg TreeBehaviorConfig
	if err := decodeStrictJSONForTest(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid tree config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = 100
	}
	if cfg.ChopPointsTotal <= 0 {
		return 0, fmt.Errorf("tree.chopPointsTotal must be > 0")
	}
	if cfg.ChopCycleDurationTicks <= 0 {
		return 0, fmt.Errorf("tree.chopCycleDurationTicks must be > 0")
	}
	if cfg.LogsSpawnDefKey == "" {
		return 0, fmt.Errorf("tree.logsSpawnDefKey is required")
	}
	if cfg.LogsSpawnCount <= 0 {
		return 0, fmt.Errorf("tree.logsSpawnCount must be > 0")
	}
	if cfg.LogsSpawnInitialOffset < 0 {
		return 0, fmt.Errorf("tree.logsSpawnInitialOffset must be >= 0")
	}
	if cfg.LogsSpawnStepOffset <= 0 {
		return 0, fmt.Errorf("tree.logsSpawnStepOffset must be > 0")
	}
	if cfg.TransformToDefKey == "" {
		return 0, fmt.Errorf("tree.transformToDefKey is required")
	}

	targetDef.TreeConfig = &cfg
	return cfg.Priority, nil
}

func decodeStrictJSONForTest(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected trailing JSON data")
	}
	return nil
}

func testBehaviors(_ *testing.T) types.BehaviorRegistry {
	return &testBehaviorRegistry{
		byKey: map[string]types.Behavior{
			"tree":      testTreeBehavior{},
			"container": testPriorityOnlyBehavior{key: "container"},
			"player":    testPriorityOnlyBehavior{key: "player"},
		},
	}
}

func writeJSONC(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

func TestLoadFromDirectory_Success(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "objects.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [
			{
				"defId": 10,
				"key": "box",
				"hp": 1000,
				"components": {
					"collider": { "w": 10, "h": 10 },
					"inventory": [{ "w": 8, "h": 4 }]
				},
				"resource": "objects/box_empty.png",
				"appearance": [
					{
						"id": "full",
						"when": { "flags": ["container.has_items"] },
						"resource": "objects/box_full.png"
					}
				],
				"behaviors": {
					"container": {}
				}
			},
			{
				"defId": 11,
				"key": "player",
				"static": false,
				"components": {
					"collider": { "w": 9, "h": 9 }
				},
				"resource": "player",
				"behaviors": {
					"player": {}
				}
			}
		]
	}`)

	registry, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.NoError(t, err)
	assert.Equal(t, 2, registry.Count())

	box, ok := registry.GetByID(10)
	require.True(t, ok)
	assert.Equal(t, "box", box.Key)
	assert.True(t, box.IsStatic)
	assert.Equal(t, 1000, box.HP)
	require.NotNil(t, box.Components)
	require.NotNil(t, box.Components.Collider)
	assert.Equal(t, 10.0, box.Components.Collider.W)
	assert.Equal(t, 10.0, box.Components.Collider.H)
	assert.Equal(t, uint64(1), box.Components.Collider.Layer)
	assert.Equal(t, uint64(1), box.Components.Collider.Mask)
	require.Len(t, box.Components.Inventory, 1)
	assert.Equal(t, 8, box.Components.Inventory[0].W)
	assert.Equal(t, 4, box.Components.Inventory[0].H)
	assert.Equal(t, "grid", box.Components.Inventory[0].Kind)
	require.Len(t, box.Appearance, 1)
	assert.Equal(t, "full", box.Appearance[0].ID)
	assert.Equal(t, "objects/box_full.png", box.Appearance[0].Resource)
	require.Len(t, box.BehaviorOrder, 1)
	assert.Equal(t, "container", box.BehaviorOrder[0])
	assert.Equal(t, 100, box.PriorityForBehavior("container"))
	assert.True(t, box.ContextMenuEvenForOneItemValue)

	player, ok := registry.GetByKey("player")
	require.True(t, ok)
	assert.Equal(t, 11, player.DefID)
	assert.False(t, player.IsStatic)
}

func TestLoadFromDirectory_JSONCComments(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "objects.jsonc", `{
		// This is a comment
		"v": 1,
		"source": "test",
		"objects": [
			{
				"defId": 1,
				"key": "tree",
				/* block comment */
				"components": {
					"collider": { "w": 10, "h": 10 }
				},
				"resource": "tree.png",
				"behaviors": {
					"player": {}
				}
			}
		]
	}`)

	registry, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.NoError(t, err)
	assert.Equal(t, 1, registry.Count())
}

func TestLoadFromDirectory_StaticDefault(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "objects.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [
			{
				"defId": 1,
				"key": "rock",
				"components": { "collider": { "w": 5, "h": 5 } },
				"resource": "rock.png"
			}
		]
	}`)

	registry, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.NoError(t, err)

	rock, ok := registry.GetByID(1)
	require.True(t, ok)
	assert.True(t, rock.IsStatic, "static should default to true")
}

func TestLoadFromDirectory_ContextMenuEvenForOneItemFalse(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "objects.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [
			{
				"defId": 1,
				"key": "stump",
				"contextMenuEvenForOneItem": false,
				"components": { "collider": { "w": 5, "h": 5 } },
				"resource": "stump.png"
			}
		]
	}`)

	registry, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.NoError(t, err)

	stump, ok := registry.GetByID(1)
	require.True(t, ok)
	assert.False(t, stump.ContextMenuEvenForOneItemValue)
}

func TestLoadFromDirectory_DuplicateDefID(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "a.jsonc", `{
		"v": 1, "source": "a",
		"objects": [{ "defId": 1, "key": "a", "resource": "a.png" }]
	}`)
	writeJSONC(t, dir, "b.jsonc", `{
		"v": 1, "source": "b",
		"objects": [{ "defId": 1, "key": "b", "resource": "b.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate defId")
}

func TestLoadFromDirectory_DuplicateKey(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "a.jsonc", `{
		"v": 1, "source": "a",
		"objects": [{ "defId": 1, "key": "same", "resource": "a.png" }]
	}`)
	writeJSONC(t, dir, "b.jsonc", `{
		"v": 1, "source": "b",
		"objects": [{ "defId": 2, "key": "same", "resource": "b.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key")
}

func TestLoadFromDirectory_InvalidDefID(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 0, "key": "bad", "resource": "bad.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defId must be > 0")
}

func TestLoadFromDirectory_MissingKey(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "", "resource": "x.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key is required")
}

func TestLoadFromDirectory_InvalidCollider(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad", "components": { "collider": { "w": 0, "h": 5 } }, "resource": "x.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "components.collider.w must be > 0")
}

func TestLoadFromDirectory_InvalidInventory(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad", "components": { "inventory": [{ "w": 0, "h": 5 }] }, "resource": "x.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "components.inventory[0].w must be > 0")
}

func TestLoadFromDirectory_InvalidTreeBehaviorConfig(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad", "resource": "x.png", "behaviors": { "tree": { "unknown": 1 } } }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tree config")
}

func TestLoadFromDirectory_TreeBehaviorActionSound(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [{
			"defId": 1,
			"key": "tree",
			"resource": "x.png",
			"behaviors": {
				"tree": {
					"chopPointsTotal": 6,
					"chopCycleDurationTicks": 20,
					"action_sound": "chop",
					"finish_sound": "tree_fall",
					"logsSpawnDefKey": "log_y",
					"logsSpawnCount": 3,
					"logsSpawnInitialOffset": 16,
					"logsSpawnStepOffset": 20,
					"transformToDefKey": "stump_birch"
				}
			}
		}]
	}`)

	registry, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.NoError(t, err)

	def, ok := registry.GetByID(1)
	require.True(t, ok)
	require.NotNil(t, def.TreeConfig)
	assert.Equal(t, "chop", def.TreeConfig.ActionSound)
	assert.Equal(t, "tree_fall", def.TreeConfig.FinishSound)
}

func TestLoadFromDirectory_UnknownBehavior(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad", "resource": "x.png", "behaviors": { "nonexistent": {} } }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown behavior")
}

func TestLoadFromDirectory_DuplicateAppearanceID(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{
			"defId": 1, "key": "bad", "resource": "x.png",
			"appearance": [
				{ "id": "a", "resource": "a.png" },
				{ "id": "a", "resource": "b.png" }
			]
		}]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate appearance.id")
}

func TestLoadFromDirectory_MissingResourceNoAppearance(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resource is required when appearance is empty")
}

func TestLoadFromDirectory_UnknownField(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test", "unknownField": true,
		"objects": []
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse JSON")
}

func TestLoadFromDirectory_InvalidVersion(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 99, "source": "test", "objects": []
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported version")
}

func TestLoadFromDirectory_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	registry, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.NoError(t, err)
	assert.Equal(t, 0, registry.Count())
}

func TestLoadFromDirectory_BehaviorOrderByPriority(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [{
			"defId": 1,
			"key": "ordered",
			"resource": "ordered.png",
			"behaviors": {
				"player": { "priority": 200 },
				"container": { "priority": 50 }
			}
		}]
	}`)

	registry, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.NoError(t, err)

	def, ok := registry.GetByID(1)
	require.True(t, ok)
	require.Equal(t, []string{"container", "player"}, def.BehaviorOrder)
	assert.Equal(t, 50, def.PriorityForBehavior("container"))
	assert.Equal(t, 200, def.PriorityForBehavior("player"))
}
