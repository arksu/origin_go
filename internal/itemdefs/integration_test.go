package itemdefs

import (
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
}
