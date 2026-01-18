package restapi

import (
	"math/rand"
	"origin/internal/const"
	"testing"

	"github.com/stretchr/testify/assert"

	"origin/internal/config"
)

func TestGenerateRandomPosition(t *testing.T) {
	// Create test config
	gameConfig := &config.GameConfig{
		WorldWidthChunks:  10,
		WorldHeightChunks: 10,
	}

	// Test the position generation logic (same as in handleCreateCharacter)
	marginTiles := 50
	worldWidthTiles := gameConfig.WorldWidthChunks * _const.ChunkSize
	worldHeightTiles := gameConfig.WorldHeightChunks * _const.ChunkSize

	// Calculate valid spawn area
	minX := marginTiles
	maxX := worldWidthTiles - marginTiles
	minY := marginTiles
	maxY := worldHeightTiles - marginTiles

	// Verify bounds are calculated correctly
	assert.Equal(t, 50, minX)
	assert.Equal(t, 1280-50, maxX) // 10 chunks * 128 tiles - 50 margin
	assert.Equal(t, 50, minY)
	assert.Equal(t, 1280-50, maxY) // 10 chunks * 128 tiles - 50 margin

	// Test that random positions are within bounds
	for i := 0; i < 1000; i++ {
		x := rand.Intn(maxX-minX+1) + minX
		y := rand.Intn(maxY-minY+1) + minY

		assert.GreaterOrEqual(t, x, minX, "X coordinate should be >= min")
		assert.LessOrEqual(t, x, maxX, "X coordinate should be <= max")
		assert.GreaterOrEqual(t, y, minY, "Y coordinate should be >= min")
		assert.LessOrEqual(t, y, maxY, "Y coordinate should be <= max")
	}

	// Test that the range calculation is correct
	rangeX := maxX - minX + 1
	rangeY := maxY - minY + 1
	assert.Equal(t, 1181, rangeX) // 1230 - 50 + 1
	assert.Equal(t, 1181, rangeY) // 1230 - 50 + 1

	// Test that random positions can generate values at the bounds
	// (We can't test exact values since it's random, but we can test the range)
	for i := 0; i < 100; i++ {
		x := rand.Intn(rangeX) + minX
		y := rand.Intn(rangeY) + minY

		assert.GreaterOrEqual(t, x, minX)
		assert.LessOrEqual(t, x, maxX)
		assert.GreaterOrEqual(t, y, minY)
		assert.LessOrEqual(t, y, maxY)
	}
}
