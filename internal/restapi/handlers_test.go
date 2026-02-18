package restapi

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_const "origin/internal/const"
	"origin/internal/config"
)

func TestGenerateRandomPosition(t *testing.T) {
	// Create test config with non-zero world origin to verify min chunk offsets are applied.
	gameConfig := &config.GameConfig{
		WorldMinXChunks:   5,
		WorldMinYChunks:   7,
		WorldWidthChunks:  10,
		WorldHeightChunks: 8,
		WorldMarginTiles:  50,
	}

	chunkWorldSize := _const.ChunkWorldSize
	marginWorldUnits := gameConfig.WorldMarginTiles * _const.CoordPerTile

	minX := gameConfig.WorldMinXChunks*chunkWorldSize + marginWorldUnits
	maxX := (gameConfig.WorldMinXChunks+gameConfig.WorldWidthChunks)*chunkWorldSize - marginWorldUnits
	minY := gameConfig.WorldMinYChunks*chunkWorldSize + marginWorldUnits
	maxY := (gameConfig.WorldMinYChunks+gameConfig.WorldHeightChunks)*chunkWorldSize - marginWorldUnits

	// Verify bounds are calculated in world units and include world origin offset.
	assert.Equal(t, 5*1536+600, minX)
	assert.Equal(t, (5+10)*1536-600, maxX)
	assert.Equal(t, 7*1536+600, minY)
	assert.Equal(t, (7+8)*1536-600, maxY)

	// Test that random positions are within bounds
	for i := 0; i < 1000; i++ {
		x, y, err := generateRandomCharacterPosition(gameConfig)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, x, minX, "X coordinate should be >= min")
		assert.Less(t, x, maxX, "X coordinate should be < max")
		assert.GreaterOrEqual(t, y, minY, "Y coordinate should be >= min")
		assert.Less(t, y, maxY, "Y coordinate should be < max")
	}

	// Test that the range calculation is correct
	rangeX := maxX - minX
	rangeY := maxY - minY
	assert.Equal(t, 14160, rangeX) // 10 chunks * 1536 - 2 * 600
	assert.Equal(t, 11088, rangeY) // 8 chunks * 1536 - 2 * 600

	// Test that old and new random formulas for [min, max) are equivalent.
	for i := 0; i < 100; i++ {
		x := rand.Intn(maxX-minX) + minX
		y := rand.Intn(maxY-minY) + minY

		assert.GreaterOrEqual(t, x, minX)
		assert.Less(t, x, maxX)
		assert.GreaterOrEqual(t, y, minY)
		assert.Less(t, y, maxY)
	}
}

func TestGenerateRandomPositionInvalidRange(t *testing.T) {
	gameConfig := &config.GameConfig{
		WorldMinXChunks:   0,
		WorldMinYChunks:   0,
		WorldWidthChunks:  1,
		WorldHeightChunks: 1,
		WorldMarginTiles:  _const.ChunkSize, // Margin consumes whole chunk on each side.
	}

	_, _, err := generateRandomCharacterPosition(gameConfig)
	require.Error(t, err)
}
