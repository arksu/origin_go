package game

import (
	constt "origin/internal/const"
	"testing"
)

func TestWorldCoordToChunkIndex(t *testing.T) {
	chunkWorldSize := float64(constt.ChunkWorldSize)
	testCases := []struct {
		name     string
		coord    float64
		expected int
	}{
		{name: "zero", coord: 0, expected: 0},
		{name: "positive within first chunk", coord: chunkWorldSize - 0.1, expected: 0},
		{name: "positive boundary", coord: chunkWorldSize, expected: 1},
		{name: "small negative", coord: -0.1, expected: -1},
		{name: "negative within first chunk", coord: -(chunkWorldSize - 0.1), expected: -1},
		{name: "negative boundary", coord: -chunkWorldSize, expected: -1},
		{name: "negative next chunk", coord: -(chunkWorldSize + 0.1), expected: -2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := worldCoordToChunkIndex(testCase.coord)
			if got != testCase.expected {
				t.Fatalf("worldCoordToChunkIndex(%f): got %d, want %d", testCase.coord, got, testCase.expected)
			}
		})
	}
}

func TestResolveLogFallAxisDirection(t *testing.T) {
	testCases := []struct {
		name         string
		treeX        float64
		treeY        float64
		playerX      float64
		playerY      float64
		expectedDirX float64
		expectedDirY float64
	}{
		{name: "player east", treeX: 0, treeY: 0, playerX: 10, playerY: 1, expectedDirX: -1, expectedDirY: 0},
		{name: "player west", treeX: 0, treeY: 0, playerX: -10, playerY: 1, expectedDirX: 1, expectedDirY: 0},
		{name: "player north", treeX: 0, treeY: 0, playerX: 1, playerY: 10, expectedDirX: 0, expectedDirY: -1},
		{name: "player south", treeX: 0, treeY: 0, playerX: 1, playerY: -10, expectedDirX: 0, expectedDirY: 1},
		{name: "axis tie prefers x", treeX: 0, treeY: 0, playerX: 10, playerY: 10, expectedDirX: -1, expectedDirY: 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gotDirX, gotDirY := resolveLogFallAxisDirection(testCase.treeX, testCase.treeY, testCase.playerX, testCase.playerY)
			if gotDirX != testCase.expectedDirX || gotDirY != testCase.expectedDirY {
				t.Fatalf("resolveLogFallAxisDirection() got (%f,%f), want (%f,%f)", gotDirX, gotDirY, testCase.expectedDirX, testCase.expectedDirY)
			}
		})
	}
}

func TestLogSpawnPosition_UsesInitialAndStepOffsets(t *testing.T) {
	treeX := 100.0
	treeY := 200.0
	dirX := 1.0
	dirY := 0.0
	initialOffset := 12
	stepOffset := 10

	x0, y0 := logSpawnPosition(treeX, treeY, dirX, dirY, initialOffset, stepOffset, 0)
	x1, y1 := logSpawnPosition(treeX, treeY, dirX, dirY, initialOffset, stepOffset, 1)
	x2, y2 := logSpawnPosition(treeX, treeY, dirX, dirY, initialOffset, stepOffset, 2)

	if x0 != 112 || y0 != 200 {
		t.Fatalf("index0: got (%f,%f), want (112,200)", x0, y0)
	}
	if x1 != 122 || y1 != 200 {
		t.Fatalf("index1: got (%f,%f), want (122,200)", x1, y1)
	}
	if x2 != 132 || y2 != 200 {
		t.Fatalf("index2: got (%f,%f), want (132,200)", x2, y2)
	}
}
