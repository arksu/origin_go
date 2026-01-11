package core

import (
	"testing"

	"origin/internal/types"
)

func TestSpatialHashGrid(t *testing.T) {
	grid := NewSpatialHashGrid(16.0)

	// Test adding and retrieving static handles
	h1 := types.MakeHandle(1, 0)
	h2 := types.MakeHandle(2, 0)
	h3 := types.MakeHandle(3, 0)

	grid.AddStatic(h1, 10.0, 10.0)
	grid.AddStatic(h2, 10.0, 10.0)
	grid.AddStatic(h3, 50.0, 50.0)

	// Test query
	var result []types.Handle
	grid.QueryAABB(0, 0, 20, 20, &result)

	if len(result) != 2 {
		t.Errorf("expected 2 handles in AABB query, got %d", len(result))
	}

	// Test radius query
	result = result[:0] // clear slice
	grid.QueryRadius(10, 10, 20, &result)

	if len(result) != 2 {
		t.Errorf("expected 2 handles in radius query, got %d", len(result))
	}

	// Test removal
	grid.RemoveStatic(h1, 10.0, 10.0)
	result = result[:0] // clear slice
	grid.QueryAABB(0, 0, 20, 20, &result)

	if len(result) != 1 {
		t.Errorf("expected 1 handle after removal, got %d", len(result))
	}

	if result[0] != h2 {
		t.Errorf("expected remaining handle to be h2, got %v", result[0])
	}

	// Test dynamic handles
	grid.AddDynamic(h1, 30.0, 30.0)
	grid.AddDynamic(h2, 30.0, 30.0)

	result = result[:0] // clear slice
	grid.QueryAABB(25, 25, 35, 35, &result)

	if len(result) != 2 {
		t.Errorf("expected 2 dynamic handles, got %d", len(result))
	}

	// Test clear dynamic
	grid.ClearDynamic()
	result = result[:0] // clear slice
	grid.QueryAABB(25, 25, 35, 35, &result)

	if len(result) != 0 {
		t.Errorf("expected 0 handles after clear dynamic, got %d", len(result))
	}

	// Test counts
	if grid.StaticCount() != 2 {
		t.Errorf("expected 2 static handles, got %d", grid.StaticCount())
	}

	if grid.DynamicCount() != 0 {
		t.Errorf("expected 0 dynamic handles, got %d", grid.DynamicCount())
	}
}

func TestSpatialHashGridUpdateDynamic(t *testing.T) {
	grid := NewSpatialHashGrid(16.0)

	h := types.MakeHandle(1, 0)

	// Add dynamic handle
	grid.AddDynamic(h, 10.0, 10.0)

	// Verify it's in the old location
	var result []types.Handle
	grid.QueryAABB(5, 5, 15, 15, &result)

	if len(result) != 1 {
		t.Fatalf("expected 1 handle initially, got %d", len(result))
	}

	// Update to new location
	grid.UpdateDynamic(h, 10.0, 10.0, 50.0, 50.0)

	// Verify it's no longer in old location
	result = result[:0] // clear slice
	grid.QueryAABB(5, 5, 15, 15, &result)

	if len(result) != 0 {
		t.Errorf("expected 0 handles in old location after update, got %d", len(result))
	}

	// Verify it's in new location
	result = result[:0] // clear slice
	grid.QueryAABB(45, 45, 55, 55, &result)

	if len(result) != 1 {
		t.Errorf("expected 1 handle in new location after update, got %d", len(result))
	}

	if result[0] != h {
		t.Errorf("expected handle to be %v, got %v", h, result[0])
	}
}

func TestSpatialHashGridGetAllHandles(t *testing.T) {
	grid := NewSpatialHashGrid(16.0)

	h1 := types.MakeHandle(1, 0)
	h2 := types.MakeHandle(2, 0)
	h3 := types.MakeHandle(3, 0)

	grid.AddStatic(h1, 10.0, 10.0)
	grid.AddStatic(h2, 20.0, 20.0)
	grid.AddDynamic(h3, 30.0, 30.0)

	allHandles := grid.GetAllHandles()
	if len(allHandles) != 3 {
		t.Errorf("expected 3 total handles, got %d", len(allHandles))
	}

	dynamicHandles := grid.GetDynamicHandles()
	if len(dynamicHandles) != 1 {
		t.Errorf("expected 1 dynamic handle, got %d", len(dynamicHandles))
	}

	if dynamicHandles[0] != h3 {
		t.Errorf("expected dynamic handle to be %v, got %v", h3, dynamicHandles[0])
	}
}
