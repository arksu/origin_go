package game

import (
	"math"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// Small rectangles use direct probes even after the sparse map has retained a
// large allocation from old listeners. This is a work-selection threshold, not
// a recipient limit; larger queries examine occupied cells without truncation.
const localChatDirectCellThreshold = 1024

type localChatRecipientStats struct {
	cellVisits       uint64
	candidates       uint64
	occupiedFallback bool
}

// AppendLocalChatRecipients appends current connected characters within the
// inclusive chat radius. The caller must hold this shard's world lock; dst
// remains owned by the caller and no mutable world references escape.
func (s *Shard) AppendLocalChatRecipients(w *ecs.World, senderX, senderY, radius, radiusSq float64, dst []types.EntityID) []types.EntityID {
	dst, _ = s.appendLocalChatRecipients(w, senderX, senderY, radius, radiusSq, dst)
	return dst
}

// The private counters describe the actual query traversal for tests and
// benchmarks. They are local to this call and never affect audio budgets.
func (s *Shard) appendLocalChatRecipients(w *ecs.World, senderX, senderY, radius, radiusSq float64, dst []types.EntityID) ([]types.EntityID, localChatRecipientStats) {
	var stats localChatRecipientStats
	if s == nil || w == nil || w != s.world || s.soundEvents == nil {
		return dst, stats
	}
	index := &s.soundEvents.listeners
	if len(index.cells) == 0 {
		return dst, stats
	}
	positions, ok := w.GetStorage(ecs.GetComponentID[components.Transform]()).(*ecs.ComponentStorage[components.Transform])
	if !ok {
		return dst, stats
	}
	externalIDs, ok := w.GetStorage(ecs.ExternalIDComponentID).(*ecs.ComponentStorage[ecs.ExternalID])
	if !ok {
		return dst, stats
	}
	query := localChatRecipientQuery{
		world: w, positions: positions, externalIDs: externalIDs,
		characters: ecs.GetResource[ecs.CharacterEntities](w).Map, listeners: index,
		senderX: senderX, senderY: senderY, radiusSq: radiusSq,
	}
	// The exact predicate below uses rounded subtraction and multiplication.
	// Its accepted points can extend slightly beyond sender +/- radius (and a
	// zero squared radius can accept tiny nonzero distances by underflow).
	// Expand the query only; final eligibility keeps the original predicate.
	queryRadius := math.Nextafter(math.Sqrt(math.Nextafter(radiusSq, math.Inf(1))), math.Inf(1))
	queryRadius = math.Max(math.Abs(radius), queryRadius)
	minimum, maximum, bounded := localChatCellBounds(senderX, senderY, queryRadius, index.cellSize)
	count, counted := localChatRectangleCellCount(minimum, maximum)
	if bounded && counted && count <= uint64(max(localChatDirectCellThreshold, len(index.cells))) {
		for cellX := minimum.x; ; cellX++ {
			for cellY := minimum.y; ; cellY++ {
				handles := index.cells[soundCell{cellX, cellY}]
				stats.cellVisits++
				stats.candidates += uint64(len(handles))
				dst = query.appendBucket(handles, dst)
				if cellY == maximum.y {
					break
				}
			}
			if cellX == maximum.x {
				break
			}
		}
		return dst, stats
	}

	stats.occupiedFallback = true
	for cell, handles := range index.cells {
		stats.cellVisits++
		if bounded && (cell.x < minimum.x || cell.x > maximum.x || cell.y < minimum.y || cell.y > maximum.y) {
			continue
		}
		stats.candidates += uint64(len(handles))
		dst = query.appendBucket(handles, dst)
	}
	return dst, stats
}

type localChatRecipientQuery struct {
	world       *ecs.World
	positions   *ecs.ComponentStorage[components.Transform]
	externalIDs *ecs.ComponentStorage[ecs.ExternalID]
	characters  map[types.EntityID]ecs.CharacterEntity
	listeners   *soundListenerIndex
	senderX     float64
	senderY     float64
	radiusSq    float64
}

func (query *localChatRecipientQuery) appendBucket(handles []types.Handle, dst []types.EntityID) []types.EntityID {
	for _, handle := range handles {
		if !query.world.Alive(handle) {
			continue
		}
		position, exists := query.positions.Get(handle)
		if !exists {
			continue
		}
		dx, dy := position.X-query.senderX, position.Y-query.senderY
		if !(dx*dx+dy*dy <= query.radiusSq) {
			continue
		}
		listener := query.listeners.members[handle]
		if listener == nil {
			continue
		}
		character, exists := query.characters[listener.entityID]
		if !exists || character.Handle != handle {
			continue
		}
		externalID, exists := query.externalIDs.Get(handle)
		if !exists || externalID.ID != listener.entityID {
			continue
		}
		dst = append(dst, listener.entityID)
	}
	return dst
}

func localChatCellBounds(x, y, radius, cellSize float64) (minimum, maximum soundCell, bounded bool) {
	coordinates := [4]float64{
		math.Floor(math.Nextafter(x-radius, math.Inf(-1)) / cellSize),
		math.Floor(math.Nextafter(y-radius, math.Inf(-1)) / cellSize),
		math.Floor(math.Nextafter(x+radius, math.Inf(1)) / cellSize),
		math.Floor(math.Nextafter(y+radius, math.Inf(1)) / cellSize),
	}
	for _, coordinate := range coordinates {
		// float64(math.MaxInt64) rounds up to 2^63, so that endpoint must
		// be excluded before conversion. Comparisons also reject NaN/Inf.
		if !(coordinate >= -0x1p63 && coordinate < 0x1p63) {
			return minimum, maximum, false
		}
	}
	minimum = soundCell{int64(coordinates[0]), int64(coordinates[1])}
	maximum = soundCell{int64(coordinates[2]), int64(coordinates[3])}
	return minimum, maximum, minimum.x <= maximum.x && minimum.y <= maximum.y
}

func localChatRectangleCellCount(minimum, maximum soundCell) (uint64, bool) {
	if minimum.x > maximum.x || minimum.y > maximum.y {
		return 0, false
	}
	// Unsigned subtraction gives the exact nonnegative span even when
	// the signed coordinates cross zero. Check before adding endpoints.
	width, height := uint64(maximum.x)-uint64(minimum.x), uint64(maximum.y)-uint64(minimum.y)
	if width == math.MaxUint64 || height == math.MaxUint64 {
		return 0, false
	}
	width++
	height++
	if width > math.MaxUint64/height {
		return 0, false
	}
	return width * height, true
}
