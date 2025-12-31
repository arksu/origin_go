package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"testing"
)

func TestActiveChunksSystem_CollectsPlayerChunks(t *testing.T) {
	w := ecs.NewWorld()

	// Initialize chunk index and active lists
	chunkIndex := ecs.NewChunkIndex()
	ecs.SetChunkIndex(w, chunkIndex)

	activeLists := ecs.NewActiveLists()
	ecs.SetActiveLists(w, activeLists)

	// Create a player at position (1000, 1000)
	player := w.Spawn(1)
	ecs.AddComponent(w, player, components.Position{X: 1000, Y: 1000})
	ecs.AddComponent(w, player, components.Player{CharacterID: 1, Name: "Test"})

	// Add ChunkRef and register in index
	chunkRef := ChunkRefFromPosition(0, 0, 1000, 1000)
	ecs.AddComponent(w, player, chunkRef)
	chunkIndex.Add(player, chunkRef.Key())

	// Run ActiveChunksSystem
	sys := NewActiveChunksSystem()
	sys.Update(w, 0.016)

	// Verify active chunks were collected
	activeLists = ecs.GetActiveLists(w)
	if activeLists == nil {
		t.Fatal("ActiveLists should not be nil")
	}

	// Should have (2*ChunkInterestRadius+1)^2 = 7*7 = 49 chunks
	expectedChunks := (2*ChunkInterestRadius + 1) * (2*ChunkInterestRadius + 1)
	if activeLists.ActiveChunkCount() != expectedChunks {
		t.Errorf("expected %d active chunks, got %d", expectedChunks, activeLists.ActiveChunkCount())
	}
}

func TestActiveListsBuilder_CategorizeEntities(t *testing.T) {
	w := ecs.NewWorld()

	// Initialize chunk index and active lists
	chunkIndex := ecs.NewChunkIndex()
	ecs.SetChunkIndex(w, chunkIndex)

	activeLists := ecs.NewActiveLists()
	ecs.SetActiveLists(w, activeLists)

	// Create entities with different component combinations
	chunkKey := components.ChunkKeyFromCoords(0, 0, 0, 0)
	activeLists.AddActiveChunk(chunkKey)

	// Dynamic entity (Position + Velocity)
	dynamic := w.Spawn(1)
	ecs.AddComponent(w, dynamic, components.Position{X: 100, Y: 100})
	ecs.AddComponent(w, dynamic, components.Velocity{X: 1, Y: 0})
	ecs.AddComponent(w, dynamic, components.ChunkRef{Region: 0, Layer: 0, ChunkX: 0, ChunkY: 0})
	chunkIndex.Add(dynamic, chunkKey)

	// Static entity (Position + Static + Collider)
	static := w.Spawn(2)
	ecs.AddComponent(w, static, components.Position{X: 200, Y: 200})
	ecs.AddComponent(w, static, components.Static{})
	ecs.AddComponent(w, static, components.Collider{Width: 10, Height: 10})
	ecs.AddComponent(w, static, components.ChunkRef{Region: 0, Layer: 0, ChunkX: 0, ChunkY: 0})
	chunkIndex.Add(static, chunkKey)

	// Observer entity (Position + Perception + VisibilityState)
	observer := w.Spawn(3)
	ecs.AddComponent(w, observer, components.Position{X: 300, Y: 300})
	ecs.AddComponent(w, observer, components.Perception{Range: 100})
	ecs.AddComponent(w, observer, components.VisibilityState{})
	ecs.AddComponent(w, observer, components.ChunkRef{Region: 0, Layer: 0, ChunkX: 0, ChunkY: 0})
	chunkIndex.Add(observer, chunkKey)

	// Visible entity (Position + EntityMeta)
	visible := w.Spawn(4)
	ecs.AddComponent(w, visible, components.Position{X: 400, Y: 400})
	ecs.AddComponent(w, visible, components.EntityMeta{EntityID: 4, EntityType: 1})
	ecs.AddComponent(w, visible, components.ChunkRef{Region: 0, Layer: 0, ChunkX: 0, ChunkY: 0})
	chunkIndex.Add(visible, chunkKey)

	// Entity outside active chunk (should not be included)
	outsideKey := components.ChunkKeyFromCoords(0, 0, 100, 100)
	outside := w.Spawn(5)
	ecs.AddComponent(w, outside, components.Position{X: 10000, Y: 10000})
	ecs.AddComponent(w, outside, components.Velocity{X: 1, Y: 0})
	ecs.AddComponent(w, outside, components.ChunkRef{Region: 0, Layer: 0, ChunkX: 100, ChunkY: 100})
	chunkIndex.Add(outside, outsideKey)

	// Run ActiveListsBuilder
	builder := NewActiveListsBuilderSystem()
	builder.Update(w, 0.016)

	// Verify categorization
	activeLists = ecs.GetActiveLists(w)

	if len(activeLists.All) != 4 {
		t.Errorf("expected 4 entities in All, got %d", len(activeLists.All))
	}

	if len(activeLists.Dynamic) != 1 {
		t.Errorf("expected 1 dynamic entity, got %d", len(activeLists.Dynamic))
	}

	if len(activeLists.Static) != 1 {
		t.Errorf("expected 1 static entity, got %d", len(activeLists.Static))
	}

	if len(activeLists.Vision) != 1 {
		t.Errorf("expected 1 vision entity, got %d", len(activeLists.Vision))
	}

	if len(activeLists.Visible) != 1 {
		t.Errorf("expected 1 visible entity, got %d", len(activeLists.Visible))
	}

	// Verify outside entity is not included
	for _, h := range activeLists.All {
		if h == outside {
			t.Error("outside entity should not be in active lists")
		}
	}
}

func TestChunkUpdateSystem_UpdatesChunkOnMove(t *testing.T) {
	w := ecs.NewWorld()

	// Initialize chunk index
	chunkIndex := ecs.NewChunkIndex()
	ecs.SetChunkIndex(w, chunkIndex)

	// Create entity at origin
	entity := w.Spawn(1)
	ecs.AddComponent(w, entity, components.Position{X: 0, Y: 0})
	ecs.AddComponent(w, entity, components.Velocity{X: 1, Y: 0})

	chunkRef := ChunkRefFromPosition(0, 0, 0, 0)
	ecs.AddComponent(w, entity, chunkRef)
	chunkIndex.Add(entity, chunkRef.Key())

	originalKey := chunkRef.Key()

	// Move entity to a different chunk
	pos := ecs.GetComponentPtr[components.Position](w, entity)
	pos.X = float64(ChunkSize) + 100 // Move to next chunk

	// Run ChunkUpdateSystem
	sys := NewChunkUpdateSystem()
	sys.Update(w, 0.016)

	// Verify chunk changed
	newKey, ok := chunkIndex.GetChunkKey(entity)
	if !ok {
		t.Fatal("entity should still be in chunk index")
	}

	if newKey == originalKey {
		t.Error("chunk key should have changed after moving to new chunk")
	}

	// Verify ChunkRef component was updated
	newChunkRef, ok := ecs.GetComponent[components.ChunkRef](w, entity)
	if !ok {
		t.Fatal("entity should have ChunkRef component")
	}

	if newChunkRef.ChunkX == 0 {
		t.Error("ChunkRef.ChunkX should have changed")
	}
}

func TestWorldToChunkCoords(t *testing.T) {
	tests := []struct {
		worldX, worldY   float64
		expectX, expectY int32
	}{
		{0, 0, 0, 0},
		{float64(ChunkSize) - 1, float64(ChunkSize) - 1, 0, 0},
		{float64(ChunkSize), float64(ChunkSize), 1, 1},
		{float64(ChunkSize) * 2, float64(ChunkSize) * 3, 2, 3},
		{-1, -1, -1, -1},
		{float64(-ChunkSize), float64(-ChunkSize), -1, -1},
		{float64(-ChunkSize) - 1, float64(-ChunkSize) - 1, -2, -2},
	}

	for _, tt := range tests {
		x, y := WorldToChunkCoords(tt.worldX, tt.worldY)
		if x != tt.expectX || y != tt.expectY {
			t.Errorf("WorldToChunkCoords(%.0f, %.0f) = (%d, %d), want (%d, %d)",
				tt.worldX, tt.worldY, x, y, tt.expectX, tt.expectY)
		}
	}
}
