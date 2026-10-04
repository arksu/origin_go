package itemdefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadAllItems(t *testing.T) {
	dataDir := filepath.Join("..", "..", "data", "items")
	logger, _ := zap.NewDevelopment()

	registry, err := LoadFromDirectory(dataDir, logger)
	require.NoError(t, err)

	seedBag, ok := registry.GetByKey("seed_bag")
	require.True(t, ok, "seed_bag should be loaded")
	assert.Equal(t, 4001, seedBag.DefID)
	assert.Equal(t, "Seed Bag", seedBag.Name)
	assert.Equal(t, 1, seedBag.Size.W)
	assert.Equal(t, 1, seedBag.Size.H)
	assert.Contains(t, seedBag.Tags, "container")

	require.NotNil(t, seedBag.Container, "seed_bag should have container definition")
	assert.Equal(t, 6, seedBag.Container.Size.W)
	assert.Equal(t, 7, seedBag.Container.Size.H)
	assert.Equal(t, 1, len(seedBag.Container.Rules.AllowTags))
	assert.Equal(t, "seed", seedBag.Container.Rules.AllowTags[0])

	branch, ok := registry.GetByKey("branch")
	require.True(t, ok, "branch should be loaded")
	assert.Equal(t, uint32(1), branch.Abilities["fuel"])

	stoneAxe, ok := registry.GetByKey("stone_axe")
	require.True(t, ok, "stone_axe should be loaded")
	require.NotNil(t, stoneAxe.Melee)
	assert.Equal(t, float64(6), stoneAxe.Melee.BaseDamage)
	assert.Nil(t, stoneAxe.Armor)
}

func TestLoadAllItems_RegistersNettleShirtForChestEquipment(t *testing.T) {
	dataDir := filepath.Join("..", "..", "data", "items")
	registry, err := LoadFromDirectory(dataDir, zap.NewNop())
	require.NoError(t, err)

	nettleShirt, ok := registry.GetByKey("nettle_shirt")
	require.True(t, ok, "nettle_shirt should be loaded")
	assert.Equal(t, []string{"chest"}, nettleShirt.Allowed.EquipmentSlots)
	assert.Nil(t, nettleShirt.Melee)
	assert.Nil(t, nettleShirt.Armor)
}

func TestLoadAllItems_DigResourcesExist(t *testing.T) {
	registry, err := LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)

	for _, key := range []string{"soil", "clay", "stone", "sand"} {
		definition, exists := registry.GetByKey(key)
		require.True(t, exists, "missing item %s", key)
		assert.Equal(t, "items/"+key+".png", definition.Resource)
		assert.Equal(t, 1, definition.Size.W)
		assert.Equal(t, 1, definition.Size.H)
		_, err := os.Stat(filepath.Join("..", "..", "web_new", "public", "assets", "game", definition.Resource))
		require.NoError(t, err, "missing image for %s", key)
	}
}
