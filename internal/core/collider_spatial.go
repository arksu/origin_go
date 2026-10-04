package core

import (
	"fmt"
	"math"
	"math/bits"

	"origin/internal/ecs"
	"origin/internal/types"
)

const colliderCellSize = 16.0

// ColliderBounds uses closed world-space bounds, so touching is a contact.
type ColliderBounds struct {
	MinX, MinY, MaxX, MaxY float64
}

func (bounds ColliderBounds) valid() bool {
	return finiteCoordinate(bounds.MinX) && finiteCoordinate(bounds.MinY) &&
		finiteCoordinate(bounds.MaxX) && finiteCoordinate(bounds.MaxY) &&
		bounds.MinX <= bounds.MaxX && bounds.MinY <= bounds.MaxY
}

func finiteCoordinate(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func (bounds ColliderBounds) intersects(other ColliderBounds) bool {
	return bounds.MinX <= other.MaxX && bounds.MaxX >= other.MinX &&
		bounds.MinY <= other.MaxY && bounds.MaxY >= other.MinY
}

type colliderCell struct{ X, Y int64 }
type colliderMembership struct {
	cell colliderCell
	slot int
}
type colliderEntry struct {
	minimum, maximum colliderCell
	handle           types.Handle
	bounds           ColliderBounds
	stamp            uint64
	level            uint8
	count            uint8
	members          [4]colliderMembership
}
type colliderLevel struct {
	cells map[colliderCell][]types.Handle
}

type ColliderQueryStats struct {
	Cells, Candidates int
}

// ColliderSpatialIndex is owned by the shard world lock. Every collider occupies
// at most four cells; large objects never enlarge other objects' search radius.
type ColliderSpatialIndex struct {
	levels      [64]*colliderLevel
	occupied    uint64
	entries     []*colliderEntry
	entryCount  int
	freeBuckets [][]types.Handle
	stamp       uint64
	lastQuery   ColliderQueryStats
}

func NewColliderSpatialIndex() *ColliderSpatialIndex {
	return &ColliderSpatialIndex{}
}

func colliderRange(bounds ColliderBounds, size float64) (minimum, maximum colliderCell, ok bool) {
	minX, minY := math.Floor(bounds.MinX/size), math.Floor(bounds.MinY/size)
	maxX, maxY := math.Floor(bounds.MaxX/size), math.Floor(bounds.MaxY/size)
	const limit = float64(1 << 62)
	if minX < -limit || minY < -limit || maxX >= limit || maxY >= limit {
		return minimum, maximum, false
	}
	return colliderCell{int64(minX), int64(minY)}, colliderCell{int64(maxX), int64(maxY)}, true
}

func (index *ColliderSpatialIndex) entry(handle types.Handle) *colliderEntry {
	if int(handle.Index()) >= len(index.entries) {
		return nil
	}
	entry := index.entries[handle.Index()]
	if entry == nil || entry.handle != handle {
		return nil
	}
	return entry
}

func (index *ColliderSpatialIndex) Upsert(handle types.Handle, bounds ColliderBounds) error {
	return index.upsert(handle, bounds, 0)
}

func (index *ColliderSpatialIndex) upsert(handle types.Handle, bounds ColliderBounds, margin float64) error {
	if handle == types.InvalidHandle || int(handle.Index()) >= ecs.MaxSparseSize || !bounds.valid() {
		return fmt.Errorf("invalid collider bounds for handle %d", handle)
	}
	entry := index.entry(handle)
	if entry != nil && entry.bounds == bounds {
		return nil
	}
	if entry != nil && bounds.MaxX-bounds.MinX == entry.bounds.MaxX-entry.bounds.MinX &&
		bounds.MaxY-bounds.MinY == entry.bounds.MaxY-entry.bounds.MinY {
		size := math.Ldexp(colliderCellSize, int(entry.level))
		if bounds.MinX >= float64(entry.minimum.X)*size && bounds.MinY >= float64(entry.minimum.Y)*size &&
			bounds.MaxX < float64(entry.maximum.X+1)*size && bounds.MaxY < float64(entry.maximum.Y+1)*size {
			entry.bounds = bounds
			return nil
		}
	}
	// The membership envelope may be loose, but candidate filtering always uses
	// the exact, current bounds. This amortizes moving-body cell transitions.
	coverage := ColliderBounds{bounds.MinX - margin, bounds.MinY - margin, bounds.MaxX + margin, bounds.MaxY + margin}
	var minimum, maximum colliderCell
	var level int
	for ; level < len(index.levels); level++ {
		var ok bool
		minimum, maximum, ok = colliderRange(coverage, math.Ldexp(colliderCellSize, level))
		spanX, spanY := maximum.X-minimum.X, maximum.Y-minimum.Y
		if ok && spanX <= 3 && spanY <= 3 && (spanX+1)*(spanY+1) <= 4 {
			break
		}
	}
	if level == len(index.levels) {
		return fmt.Errorf("collider bounds exceed spatial coordinate range for handle %d", handle)
	}
	if entry != nil && int(entry.level) == level && entry.minimum == minimum && entry.maximum == maximum {
		entry.bounds = bounds
		return nil
	}
	if entry == nil {
		position := int(handle.Index())
		if position >= len(index.entries) {
			size := max(1024, max(position+1, len(index.entries)*2))
			index.entries = append(index.entries, make([]*colliderEntry, size-len(index.entries))...)
		}
		// A generational replacement must never leave the old membership behind.
		if old := index.entries[position]; old != nil {
			index.Remove(old.handle)
		}
		entry = &colliderEntry{handle: handle, level: uint8(level)}
		index.entries[position] = entry
		index.entryCount++
	} else if int(entry.level) != level {
		index.removeMemberships(entry)
		index.retireLevel(entry.level)
		entry.level = uint8(level)
	} else {
		// Preserve common cells instead of removing and inserting all four.
		for position := uint8(0); position < entry.count; {
			cell := entry.members[position].cell
			if cell.X >= minimum.X && cell.X <= maximum.X && cell.Y >= minimum.Y && cell.Y <= maximum.Y {
				position++
				continue
			}
			index.removeMember(entry, position)
		}
	}
	grid := index.levels[level]
	if grid == nil {
		grid = &colliderLevel{cells: make(map[colliderCell][]types.Handle)}
		index.levels[level] = grid
		index.occupied |= uint64(1) << level
	}
	entry.level, entry.bounds, entry.minimum, entry.maximum = uint8(level), bounds, minimum, maximum
	for cellY := minimum.Y; cellY <= maximum.Y; cellY++ {
		for cellX := minimum.X; cellX <= maximum.X; cellX++ {
			cell := colliderCell{cellX, cellY}
			present := false
			for _, member := range entry.members[:entry.count] {
				if member.cell == cell {
					present = true
					break
				}
			}
			if present {
				continue
			}
			handles := grid.cells[cell]
			if handles == nil && len(index.freeBuckets) > 0 {
				last := len(index.freeBuckets) - 1
				handles = index.freeBuckets[last]
				index.freeBuckets[last] = nil
				index.freeBuckets = index.freeBuckets[:last]
			}
			entry.members[entry.count] = colliderMembership{cell, len(handles)}
			entry.count++
			grid.cells[cell] = append(handles, handle)
		}
	}
	return nil
}

func (index *ColliderSpatialIndex) removeMember(entry *colliderEntry, position uint8) {
	grid := index.levels[entry.level]
	member := entry.members[position]
	handles := grid.cells[member.cell]
	last := len(handles) - 1
	if member.slot != last {
		moved := handles[last]
		handles[member.slot] = moved
		movedEntry := index.entry(moved)
		for slot := uint8(0); slot < movedEntry.count; slot++ {
			if movedEntry.members[slot].cell == member.cell {
				movedEntry.members[slot].slot = member.slot
				break
			}
		}
	}
	if last == 0 {
		delete(grid.cells, member.cell)
		// A bounded pool avoids allocation churn without retaining dense peak buckets.
		if len(index.freeBuckets) < 1024 && cap(handles) <= 16 {
			index.freeBuckets = append(index.freeBuckets, handles[:0])
		}
	} else {
		grid.cells[member.cell] = handles[:last]
	}
	entry.count--
	entry.members[position] = entry.members[entry.count]
	entry.members[entry.count] = colliderMembership{}
}

func (index *ColliderSpatialIndex) removeMemberships(entry *colliderEntry) {
	for entry.count > 0 {
		index.removeMember(entry, entry.count-1)
	}
}

func (index *ColliderSpatialIndex) retireLevel(level uint8) {
	if len(index.levels[level].cells) == 0 {
		index.levels[level] = nil
		index.occupied &^= uint64(1) << level
	}
}

func (index *ColliderSpatialIndex) Remove(handle types.Handle) {
	if entry := index.entry(handle); entry != nil {
		index.removeMemberships(entry)
		index.retireLevel(entry.level)
		index.entries[handle.Index()] = nil
		index.entryCount--
	}
}

func (index *ColliderSpatialIndex) QueryInto(bounds ColliderBounds, destination []types.Handle) ([]types.Handle, error) {
	if !bounds.valid() {
		return destination, fmt.Errorf("invalid collider query bounds")
	}
	index.lastQuery = ColliderQueryStats{}
	index.stamp++
	if index.stamp == 0 {
		for _, entry := range index.entries {
			if entry != nil {
				entry.stamp = 0
			}
		}
		index.stamp = 1
	}
	for occupied := index.occupied; occupied != 0; occupied &= occupied - 1 {
		level := bits.TrailingZeros64(occupied)
		grid := index.levels[level]
		size := math.Ldexp(colliderCellSize, level)
		minimum, maximum, representable := colliderRange(bounds, size)
		if !representable || (float64(maximum.X)-float64(minimum.X)+1)*(float64(maximum.Y)-float64(minimum.Y)+1) > float64(len(grid.cells)) {
			minCellX, minCellY := math.Floor(bounds.MinX/size), math.Floor(bounds.MinY/size)
			maxCellX, maxCellY := math.Floor(bounds.MaxX/size), math.Floor(bounds.MaxY/size)
			for cell, handles := range grid.cells {
				index.lastQuery.Cells++
				if float64(cell.X) < minCellX || float64(cell.X) > maxCellX ||
					float64(cell.Y) < minCellY || float64(cell.Y) > maxCellY {
					continue
				}
				destination = index.appendCandidates(bounds, handles, destination)
			}
		} else {
			for cellY := minimum.Y; cellY <= maximum.Y; cellY++ {
				for cellX := minimum.X; cellX <= maximum.X; cellX++ {
					index.lastQuery.Cells++
					destination = index.appendCandidates(bounds, grid.cells[colliderCell{cellX, cellY}], destination)
				}
			}
		}
	}
	return destination, nil
}

func (index *ColliderSpatialIndex) appendCandidates(bounds ColliderBounds, handles, destination []types.Handle) []types.Handle {
	for _, handle := range handles {
		entry := index.entry(handle)
		if entry.stamp == index.stamp {
			continue
		}
		entry.stamp = index.stamp
		index.lastQuery.Candidates++
		if entry.bounds.intersects(bounds) {
			destination = append(destination, handle)
		}
	}
	return destination
}

func (index *ColliderSpatialIndex) LastQueryStats() ColliderQueryStats { return index.lastQuery }
