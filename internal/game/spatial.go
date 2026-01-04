package game

import (
	"sync"

	"origin/internal/ecs"
)

type SpatialHashGrid struct {
	cellSize    float64
	invCellSize float64

	staticCells  map[int64][]ecs.Handle
	dynamicCells map[int64][]ecs.Handle

	mu sync.RWMutex
}

func NewSpatialHashGrid(cellSize float64) *SpatialHashGrid {
	return &SpatialHashGrid{
		cellSize:     cellSize,
		invCellSize:  1.0 / cellSize,
		staticCells:  make(map[int64][]ecs.Handle),
		dynamicCells: make(map[int64][]ecs.Handle),
	}
}

func (g *SpatialHashGrid) cellKey(x, y float64) int64 {
	cx := int32(x * g.invCellSize)
	cy := int32(y * g.invCellSize)
	return mortonEncode(cx, cy)
}

func mortonEncode(x, y int32) int64 {
	return int64(interleave(uint32(x))) | (int64(interleave(uint32(y))) << 1)
}

func interleave(x uint32) uint32 {
	x = (x | (x << 16)) & 0x0000FFFF
	x = (x | (x << 8)) & 0x00FF00FF
	x = (x | (x << 4)) & 0x0F0F0F0F
	x = (x | (x << 2)) & 0x33333333
	x = (x | (x << 1)) & 0x55555555
	return x
}

func (g *SpatialHashGrid) AddStatic(h ecs.Handle, x, y float64) {
	key := g.cellKey(x, y)

	g.mu.Lock()
	defer g.mu.Unlock()

	g.staticCells[key] = append(g.staticCells[key], h)
}

func (g *SpatialHashGrid) RemoveStatic(h ecs.Handle, x, y float64) {
	key := g.cellKey(x, y)

	g.mu.Lock()
	defer g.mu.Unlock()

	handles := g.staticCells[key]
	for i, handle := range handles {
		if handle == h {
			handles[i] = handles[len(handles)-1]
			g.staticCells[key] = handles[:len(handles)-1]
			return
		}
	}
}

func (g *SpatialHashGrid) AddDynamic(h ecs.Handle, x, y float64) {
	key := g.cellKey(x, y)

	g.mu.Lock()
	defer g.mu.Unlock()

	g.dynamicCells[key] = append(g.dynamicCells[key], h)
}

func (g *SpatialHashGrid) RemoveDynamic(h ecs.Handle, x, y float64) {
	key := g.cellKey(x, y)

	g.mu.Lock()
	defer g.mu.Unlock()

	handles := g.dynamicCells[key]
	for i, handle := range handles {
		if handle == h {
			handles[i] = handles[len(handles)-1]
			g.dynamicCells[key] = handles[:len(handles)-1]
			return
		}
	}
}

func (g *SpatialHashGrid) UpdateDynamic(h ecs.Handle, oldX, oldY, newX, newY float64) {
	oldKey := g.cellKey(oldX, oldY)
	newKey := g.cellKey(newX, newY)

	if oldKey == newKey {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	found := false
	handles := g.dynamicCells[oldKey]
	for i, handle := range handles {
		if handle == h {
			handles[i] = handles[len(handles)-1]
			g.dynamicCells[oldKey] = handles[:len(handles)-1]
			found = true
			break
		}
	}

	// Only add to new cell if we actually found and removed from old cell
	if found {
		g.dynamicCells[newKey] = append(g.dynamicCells[newKey], h)
	}
}

func (g *SpatialHashGrid) ClearDynamic() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for k := range g.dynamicCells {
		delete(g.dynamicCells, k)
	}
}

func (g *SpatialHashGrid) ClearStatic() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for k := range g.staticCells {
		delete(g.staticCells, k)
	}
}

func (g *SpatialHashGrid) QueryRadius(x, y, radius float64, result *[]ecs.Handle) {
	minCX := int32((x - radius) * g.invCellSize)
	maxCX := int32((x + radius) * g.invCellSize)
	minCY := int32((y - radius) * g.invCellSize)
	maxCY := int32((y + radius) * g.invCellSize)

	g.mu.RLock()
	defer g.mu.RUnlock()

	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			key := mortonEncode(cx, cy)

			if handles, ok := g.staticCells[key]; ok {
				*result = append(*result, handles...)
			}
			if handles, ok := g.dynamicCells[key]; ok {
				*result = append(*result, handles...)
			}
		}
	}
}

func (g *SpatialHashGrid) QueryAABB(minX, minY, maxX, maxY float64, result *[]ecs.Handle) {
	minCX := int32(minX * g.invCellSize)
	maxCX := int32(maxX * g.invCellSize)
	minCY := int32(minY * g.invCellSize)
	maxCY := int32(maxY * g.invCellSize)

	g.mu.RLock()
	defer g.mu.RUnlock()

	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			key := mortonEncode(cx, cy)

			if handles, ok := g.staticCells[key]; ok {
				*result = append(*result, handles...)
			}
			if handles, ok := g.dynamicCells[key]; ok {
				*result = append(*result, handles...)
			}
		}
	}
}

func (g *SpatialHashGrid) StaticCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	count := 0
	for _, handles := range g.staticCells {
		count += len(handles)
	}
	return count
}

func (g *SpatialHashGrid) GetAllHandles() []ecs.Handle {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]ecs.Handle, 0, 256)
	for _, handles := range g.staticCells {
		result = append(result, handles...)
	}
	for _, handles := range g.dynamicCells {
		result = append(result, handles...)
	}
	return result
}

func (g *SpatialHashGrid) GetDynamicHandles() []ecs.Handle {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]ecs.Handle, 0, 256)
	for _, handles := range g.dynamicCells {
		result = append(result, handles...)
	}
	return result
}

func (g *SpatialHashGrid) DynamicCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	count := 0
	for _, handles := range g.dynamicCells {
		count += len(handles)
	}
	return count
}
