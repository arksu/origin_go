package objectdefs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

type testBehaviorRegistry struct {
	byKey map[string]contracts.Behavior
}

func (r *testBehaviorRegistry) GetBehavior(key string) (contracts.Behavior, bool) {
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

func (r *testBehaviorRegistry) InitObjectBehaviors(_ *contracts.BehaviorObjectInitContext, _ []string) error {
	return nil
}

type testPriorityOnlyBehavior struct {
	key string
}

func (b testPriorityOnlyBehavior) Key() string { return b.key }

func (b testPriorityOnlyBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
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

func (testTreeBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 100, nil
	}
	if ctx.Def == nil {
		return 0, fmt.Errorf("tree config target def is nil")
	}

	var cfg contracts.TreeBehaviorConfig
	if err := decodeStrictJSONForTest(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid tree config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = 100
	}
	if len(cfg.Stages) == 0 {
		return 0, fmt.Errorf("tree.stages is required")
	}
	for idx, stage := range cfg.Stages {
		if stage.ChopPointsTotal <= 0 {
			return 0, fmt.Errorf("tree.stages[%d].chopPointsTotal must be > 0", idx)
		}
		if idx < len(cfg.Stages)-1 && stage.StageDuration <= 0 {
			return 0, fmt.Errorf("tree.stages[%d].stageDurationTicks must be > 0 for non-final stage", idx)
		}
		for objectIdx, objectKey := range stage.SpawnChopObject {
			if objectKey == "" {
				return 0, fmt.Errorf("tree.stages[%d].spawnChopObject[%d] must not be empty", idx, objectIdx)
			}
		}
		for itemIdx, itemKey := range stage.SpawnChopItem {
			if itemKey == "" {
				return 0, fmt.Errorf("tree.stages[%d].spawnChopItem[%d] must not be empty", idx, itemIdx)
			}
		}
		seenTakeIDs := make(map[string]int, len(stage.Take))
		for takeIdx, take := range stage.Take {
			if err := validateTakeConfigForTest(idx, takeIdx, take); err != nil {
				return 0, err
			}
			takeID := strings.TrimSpace(take.ID)
			if first, exists := seenTakeIDs[takeID]; exists {
				return 0, fmt.Errorf("tree.stages[%d].take[%d].id duplicate %q (first at index %d)", idx, takeIdx, takeID, first)
			}
			seenTakeIDs[takeID] = takeIdx
		}
	}

	ctx.Def.SetTreeBehaviorConfig(cfg)
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

func validateTakeConfigForTest(stageIndex int, takeIndex int, cfg contracts.TreeTakeConfig) error {
	takeID := strings.TrimSpace(cfg.ID)
	if takeID == "" {
		return fmt.Errorf("tree.stages[%d].take[%d].id must not be empty", stageIndex, takeIndex)
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("tree.stages[%d].take[%d].name must not be empty", stageIndex, takeIndex)
	}
	itemKey := strings.TrimSpace(cfg.ItemDefKey)
	if itemKey == "" {
		return fmt.Errorf("tree.stages[%d].take[%d].itemDefKey must not be empty", stageIndex, takeIndex)
	}
	if cfg.Count <= 0 {
		return fmt.Errorf("tree.stages[%d].take[%d].count must be > 0", stageIndex, takeIndex)
	}
	registry := itemdefs.Global()
	if registry == nil {
		return fmt.Errorf("tree.stages[%d].take[%d].itemDefKey validation requires loaded item defs", stageIndex, takeIndex)
	}
	if _, ok := registry.GetByKey(itemKey); !ok {
		return fmt.Errorf("tree.stages[%d].take[%d].itemDefKey unknown item key %q", stageIndex, takeIndex, itemKey)
	}
	return nil
}

func testBehaviors(_ *testing.T) contracts.BehaviorRegistry {
	return &testBehaviorRegistry{
		byKey: map[string]contracts.Behavior{
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
				"name": "Box",
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
				"name": "Player",
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
				"name": "Tree",
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
				"name": "Rock",
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
				"name": "Stump",
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
		"objects": [{ "defId": 1, "key": "a", "name": "A", "resource": "a.png" }]
	}`)
	writeJSONC(t, dir, "b.jsonc", `{
		"v": 1, "source": "b",
		"objects": [{ "defId": 1, "key": "b", "name": "B", "resource": "b.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate defId")
}

func TestLoadFromDirectory_DuplicateKey(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "a.jsonc", `{
		"v": 1, "source": "a",
		"objects": [{ "defId": 1, "key": "same", "name": "Same A", "resource": "a.png" }]
	}`)
	writeJSONC(t, dir, "b.jsonc", `{
		"v": 1, "source": "b",
		"objects": [{ "defId": 2, "key": "same", "name": "Same B", "resource": "b.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key")
}

func TestLoadFromDirectory_InvalidDefID(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 0, "key": "bad", "name": "Bad", "resource": "bad.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defId must be > 0")
}

func TestLoadFromDirectory_MissingKey(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "", "name": "Missing Key", "resource": "x.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key is required")
}

func TestLoadFromDirectory_MissingName(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad", "name": "", "resource": "x.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestLoadFromDirectory_InvalidCollider(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad", "name": "Bad", "components": { "collider": { "w": 0, "h": 5 } }, "resource": "x.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "components.collider.w must be > 0")
}

func TestLoadFromDirectory_InvalidInventory(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad", "name": "Bad", "components": { "inventory": [{ "w": 0, "h": 5 }] }, "resource": "x.png" }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "components.inventory[0].w must be > 0")
}

func TestLoadFromDirectory_InvalidTreeBehaviorConfig(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad", "name": "Bad", "resource": "x.png", "behaviors": { "tree": { "unknown": 1 } } }]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tree config")
}

func TestLoadFromDirectory_TreeBehaviorRejectsLegacyFlatConfig(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [{
			"defId": 1,
			"key": "legacy_tree",
			"name": "Legacy Tree",
			"resource": "x.png",
			"behaviors": {
				"tree": {
					"chopPointsTotal": 6,
					"chopCycleDurationTicks": 20
				}
			}
		}]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tree config")
}

func TestLoadFromDirectory_TreeBehaviorRequiresStages(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [{
			"defId": 1,
			"key": "tree_no_stages",
			"name": "Tree",
			"resource": "x.png",
			"behaviors": {
				"tree": {
					"stages": []
				}
			}
		}]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tree.stages is required")
}

func TestLoadFromDirectory_TreeBehaviorRejectsInvalidStageDuration(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [{
			"defId": 1,
			"key": "tree_bad_duration",
			"name": "Tree",
			"resource": "x.png",
			"behaviors": {
				"tree": {
					"stages": [
						{
							"chopPointsTotal": 1,
							"stageDurationTicks": 0,
							"allowChop": true,
							"spawnChopObject": [],
							"spawnChopItem": []
						},
						{
							"chopPointsTotal": 1,
							"stageDurationTicks": 60,
							"allowChop": true,
							"spawnChopObject": [],
							"spawnChopItem": []
						}
					]
				}
			}
		}]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stageDurationTicks must be > 0 for non-final stage")
}

func TestLoadFromDirectory_TreeBehaviorTakeRequiresAllFields(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [{
			"defId": 1,
			"key": "tree_bad_take_pair",
			"name": "Tree",
			"resource": "x.png",
			"behaviors": {
				"tree": {
					"stages": [
						{
							"chopPointsTotal": 1,
							"stageDurationTicks": 60,
							"allowChop": true,
							"spawnChopObject": [],
							"spawnChopItem": [],
							"take": [
								{
									"id": "take_branch",
									"itemDefKey": "branch",
									"count": 1
								}
							]
						}
					]
				}
			}
		}]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "take[0].name must not be empty")
}

func TestLoadFromDirectory_TreeBehaviorTakeUnknownItemFails(t *testing.T) {
	dir := t.TempDir()
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1, Key: "known", Name: "Known", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	defer itemdefs.SetGlobalForTesting(previousRegistry)

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [{
			"defId": 1,
			"key": "tree_bad_take_key",
			"name": "Tree",
			"resource": "x.png",
			"behaviors": {
				"tree": {
					"stages": [
						{
							"chopPointsTotal": 1,
							"stageDurationTicks": 60,
							"allowChop": true,
							"spawnChopObject": [],
							"spawnChopItem": [],
							"take": [
								{
									"id": "take_branch",
									"name": "Take Branch",
									"itemDefKey": "unknown_item",
									"count": 1
								}
							]
						}
					]
				}
			}
		}]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "take[0].itemDefKey unknown item key")
}

func TestLoadFromDirectory_TreeBehaviorTakeDuplicateIDFails(t *testing.T) {
	dir := t.TempDir()
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1, Key: "branch", Name: "Branch", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	defer itemdefs.SetGlobalForTesting(previousRegistry)

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [{
			"defId": 1,
			"key": "tree_bad_take_duplicate",
			"name": "Tree",
			"resource": "x.png",
			"behaviors": {
				"tree": {
					"stages": [
						{
							"chopPointsTotal": 1,
							"stageDurationTicks": 60,
							"allowChop": true,
							"spawnChopObject": [],
							"spawnChopItem": [],
							"take": [
								{
									"id": "take_branch",
									"name": "Take Branch",
									"itemDefKey": "branch",
									"count": 1
								},
								{
									"id": "take_branch",
									"name": "Take Branch Again",
									"itemDefKey": "branch",
									"count": 1
								}
							]
						}
					]
				}
			}
		}]
	}`)

	_, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "take[1].id duplicate")
}

func TestLoadFromDirectory_TreeBehaviorStages(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1,
		"source": "test",
		"objects": [{
			"defId": 1,
			"key": "tree",
			"name": "Tree",
			"resource": "x.png",
			"behaviors": {
				"tree": {
					"stages": [
						{
							"chopPointsTotal": 2,
							"stageDurationTicks": 60,
							"allowChop": false,
							"spawnChopObject": [],
							"spawnChopItem": []
						},
						{
							"chopPointsTotal": 4,
							"stageDurationTicks": 120,
							"allowChop": true,
							"spawnChopObject": ["log"],
							"spawnChopItem": ["branch", "branch"],
							"transformToDefKey": "stump_birch"
						}
					]
				}
			}
		}]
	}`)

	registry, err := LoadFromDirectory(dir, testBehaviors(t), testLogger())
	require.NoError(t, err)

	def, ok := registry.GetByID(1)
	require.True(t, ok)
	require.NotNil(t, def.TreeConfig)
	require.Len(t, def.TreeConfig.Stages, 2)
	assert.Equal(t, 2, def.TreeConfig.Stages[0].ChopPointsTotal)
	assert.Equal(t, 120, def.TreeConfig.Stages[1].StageDuration)
	assert.True(t, def.TreeConfig.Stages[1].AllowChop)
	assert.Equal(t, "stump_birch", def.TreeConfig.Stages[1].TransformToDefKey)
}

func TestLoadFromDirectory_UnknownBehavior(t *testing.T) {
	dir := t.TempDir()

	writeJSONC(t, dir, "test.jsonc", `{
		"v": 1, "source": "test",
		"objects": [{ "defId": 1, "key": "bad", "name": "Bad", "resource": "x.png", "behaviors": { "nonexistent": {} } }]
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
			"defId": 1, "key": "bad", "name": "Bad", "resource": "x.png",
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
		"objects": [{ "defId": 1, "key": "bad", "name": "Bad" }]
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
			"name": "Ordered",
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
