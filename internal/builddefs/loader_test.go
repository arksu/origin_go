package builddefs

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func seedDependencyRegistries(t *testing.T) {
	t.Helper()

	prevItems := itemdefs.Global()
	prevObjects := objectdefs.Global()
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(prevItems)
		objectdefs.SetGlobalForTesting(prevObjects)
	})

	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1, Key: "stone"},
		{DefID: 2, Key: "branch"},
		{DefID: 3, Key: "ore_tin", Tags: []string{"ore"}},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 101, Key: "campfire_obj"},
		{DefID: 102, Key: "crate_obj"},
	}))
}

func writeBuildsFile(t *testing.T, dir string, fileName string, body string) string {
	t.Helper()
	path := filepath.Join(dir, fileName)
	require.NoError(t, os.WriteFile(path, []byte(body), 0644))
	return path
}

func wrapBuildsJSON(builds string) string {
	return fmt.Sprintf(`{
  "v": 1,
  "source": "test",
  "builds": [
    %s
  ]
}`, builds)
}

func TestLoadFromDirectory_SuccessNormalizationAndDefaults(t *testing.T) {
	seedDependencyRegistries(t)
	dir := t.TempDir()

	file := `{
  "v": 1,
  "source": "basic",
  "builds": [
    {
      "defId": 1001,
      "key": "campfire",
      "name": "Campfire",
      "inputs": [
        { "itemKey": "stone", "count": 8, "qualityWeight": 1 },
        { "itemKey": "branch", "count": 3, "qualityWeight": 2 }
      ],
      "staminaCost": 5,
      "ticksRequired": 10,
      "requiredSkills": ["masonry", "survival", "masonry", " "],
      "requiredDiscovery": ["campfire", " fire ", "campfire"],
      "allowedTiles": [` + fmt.Sprintf("%d, %d, %d", types.TileDirt, types.TileGrass, types.TileDirt) + `],
      "objectKey": "campfire_obj"
    },
    {
      "defId": 1002,
      "key": "ore_crate",
      "name": "   ",
      "inputs": [
        { "itemTag": "ore", "count": 4, "qualityWeight": 4 },
        { "itemKey": "branch", "count": 1, "qualityWeight": 1 }
      ],
      "staminaCost": 0,
      "ticksRequired": 20,
      "disallowedTiles": [` + fmt.Sprintf("%d, %d, %d", types.TileSwamp1, types.TileDeepWater, types.TileSwamp1) + `],
      "requiredSkills": [],
      "requiredDiscovery": [],
      "objectKey": "crate_obj"
    },
    {
      "defId": 1003,
      "key": "plain_build",
      "name": "Plain Build",
      "inputs": [
        { "itemKey": "stone", "count": 1, "qualityWeight": 1 }
      ],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "crate_obj"
    }
  ]
}`

	writeBuildsFile(t, dir, "basic.jsonc", file)

	registry, err := LoadFromDirectory(dir, testLogger())
	require.NoError(t, err)
	require.Equal(t, 3, registry.Count())

	campfire, ok := registry.GetByKey("campfire")
	require.True(t, ok)
	assert.Equal(t, []string{"campfire", "fire"}, campfire.RequiredDiscovery)
	assert.Equal(t, []string{"masonry", "survival"}, campfire.RequiredSkills)
	assert.Equal(t, []int{types.TileGrass, types.TileDirt}, campfire.AllowedTiles)
	assert.Empty(t, campfire.DisallowedTiles)

	oreCrate, ok := registry.GetByKey("ore_crate")
	require.True(t, ok)
	assert.Equal(t, "ore_crate", oreCrate.Name)
	assert.Empty(t, oreCrate.AllowedTiles)
	assert.Equal(t, []int{types.TileDeepWater, types.TileSwamp1}, oreCrate.DisallowedTiles)
	require.Len(t, oreCrate.Inputs, 2)
	assert.Equal(t, "ore", oreCrate.Inputs[0].ItemTag)
	assert.Equal(t, "branch", oreCrate.Inputs[1].ItemKey)

	plain, ok := registry.GetByKey("plain_build")
	require.True(t, ok)
	assert.Empty(t, plain.AllowedTiles)
	assert.Empty(t, plain.DisallowedTiles)
}

func TestLoadFromDirectory_MultiFileMerge(t *testing.T) {
	seedDependencyRegistries(t)
	dir := t.TempDir()

	writeBuildsFile(t, dir, "a.jsonc", `{
  "v": 1,
  "source": "a",
  "builds": [
    {
      "defId": 1001,
      "key": "build_a",
      "name": "A",
      "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }
  ]
}`)
	writeBuildsFile(t, dir, "b.jsonc", `{
  "v": 1,
  "source": "b",
  "builds": [
    {
      "defId": 1002,
      "key": "build_b",
      "name": "B",
      "inputs": [{ "itemTag": "ore", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 2,
      "ticksRequired": 2,
      "objectKey": "crate_obj"
    }
  ]
}`)

	registry, err := LoadFromDirectory(dir, testLogger())
	require.NoError(t, err)
	assert.Equal(t, 2, registry.Count())
	_, okA := registry.GetByKey("build_a")
	_, okB := registry.GetByKey("build_b")
	assert.True(t, okA)
	assert.True(t, okB)
}

func TestLoadFromDirectory_DuplicateDefID(t *testing.T) {
	seedDependencyRegistries(t)
	dir := t.TempDir()

	writeBuildsFile(t, dir, "a.json", `{
  "v": 1,
  "source": "a",
  "builds": [{
    "defId": 1001, "key": "a", "name": "A",
    "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
    "staminaCost": 1, "ticksRequired": 1, "objectKey": "campfire_obj"
  }]
}`)
	writeBuildsFile(t, dir, "b.json", `{
  "v": 1,
  "source": "b",
  "builds": [{
    "defId": 1001, "key": "b", "name": "B",
    "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
    "staminaCost": 1, "ticksRequired": 1, "objectKey": "campfire_obj"
  }]
}`)

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate defId")
}

func TestLoadFromDirectory_DuplicateKey(t *testing.T) {
	seedDependencyRegistries(t)
	dir := t.TempDir()

	writeBuildsFile(t, dir, "a.json", `{
  "v": 1,
  "source": "a",
  "builds": [{
    "defId": 1001, "key": "same", "name": "A",
    "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
    "staminaCost": 1, "ticksRequired": 1, "objectKey": "campfire_obj"
  }]
}`)
	writeBuildsFile(t, dir, "b.json", `{
  "v": 1,
  "source": "b",
  "builds": [{
    "defId": 1002, "key": "same", "name": "B",
    "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
    "staminaCost": 1, "ticksRequired": 1, "objectKey": "campfire_obj"
  }]
}`)

	_, err := LoadFromDirectory(dir, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key")
}

func TestLoadFromDirectory_ValidationErrors(t *testing.T) {
	seedDependencyRegistries(t)

	tests := []struct {
		name    string
		build   string
		wantErr string
	}{
		{
			name: "missing key",
			build: `{
      "defId": 1001,
      "key": "",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "key is required",
		},
		{
			name: "invalid def id",
			build: `{
      "defId": 0,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "defId must be > 0",
		},
		{
			name: "empty inputs",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "inputs must not be empty",
		},
		{
			name: "ticks required zero",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 0,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "ticksRequired must be > 0",
		},
		{
			name: "negative stamina",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
      "staminaCost": -1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "staminaCost must be >=",
		},
		{
			name: "missing object key",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": " "
    }`,
			wantErr: "objectKey is required",
		},
		{
			name: "unknown object key",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "unknown"
    }`,
			wantErr: "objectKey unknown",
		},
		{
			name: "input both selectors",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "itemTag": "ore", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "exactly one of itemKey or itemTag",
		},
		{
			name: "input neither selector",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "exactly one of itemKey or itemTag",
		},
		{
			name: "input count zero",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "count": 0, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "inputs[0].count must be > 0",
		},
		{
			name: "unknown item key",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "unknown", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "itemKey unknown",
		},
		{
			name: "zero total quality weight",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [
        { "itemKey": "stone", "count": 1, "qualityWeight": 0 },
        { "itemTag": "ore", "count": 1, "qualityWeight": 0 }
      ],
      "staminaCost": 1,
      "ticksRequired": 1,
      "objectKey": "campfire_obj"
    }`,
			wantErr: "qualityWeight must be > 0",
		},
		{
			name: "both tile lists set",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "allowedTiles": [` + fmt.Sprintf("%d", types.TileGrass) + `],
      "disallowedTiles": [` + fmt.Sprintf("%d", types.TileSwamp1) + `],
      "objectKey": "campfire_obj"
    }`,
			wantErr: "only one of allowedTiles or disallowedTiles",
		},
		{
			name: "unknown tile id in range",
			build: `{
      "defId": 1001,
      "key": "x",
      "name": "X",
      "inputs": [{ "itemKey": "stone", "count": 1, "qualityWeight": 1 }],
      "staminaCost": 1,
      "ticksRequired": 1,
      "allowedTiles": [2],
      "objectKey": "campfire_obj"
    }`,
			wantErr: "unknown tile id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeBuildsFile(t, dir, "test.jsonc", wrapBuildsJSON(tc.build))
			_, err := LoadFromDirectory(dir, testLogger())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
