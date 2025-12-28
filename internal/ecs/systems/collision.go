package systems

import (
	"log"
	"math"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

// Collision system constants
const (
	// DefaultCollisionCellSize is the default cell size for spatial hashing
	DefaultCollisionCellSize = 64.0

	// maxCollisionIterations limits narrow-phase iterations to avoid pathological loops
	maxCollisionIterations = 4

	// collisionEpsilon is the minimum separation after collision resolution
	// Tuned for determinism - small enough to avoid visible gaps, large enough to prevent sticking
	collisionEpsilon = 0.001

	// sweepEpsilon is used for swept AABB time calculations
	sweepEpsilon = 1e-8
)

// packKey generates a 64-bit key from two 32-bit coordinates
// It preserves the bit pattern of the negative numbers correctly by casting to uint32 first
func packKey(x, y int32) int64 {
	return (int64(x) << 32) | int64(uint32(y))
}

// SpatialHashGrid provides O(1) broad-phase collision detection
type SpatialHashGrid struct {
	cellSize    float64
	invCellSize float64 // Precomputed 1/cellSize for faster division
	cells       map[int64][]ecs.Handle
	// Preallocated slice pools to avoid per-tick allocations
	cellPool [][]ecs.Handle
	poolIdx  int
}

// NewSpatialHash creates a new spatial hash with the given cell size
func NewSpatialHash(cellSize float64) *SpatialHashGrid {
	return &SpatialHashGrid{
		cellSize:    cellSize,
		invCellSize: 1.0 / cellSize,
		cells:       make(map[int64][]ecs.Handle, 1024),
		cellPool:    make([][]ecs.Handle, 256),
		poolIdx:     0,
	}
}

// Clear removes all entities from the spatial hash, reusing slices
func (sh *SpatialHashGrid) Clear() {
	// Return slices to pool and clear map
	sh.poolIdx = 0
	for k, v := range sh.cells {
		if sh.poolIdx < len(sh.cellPool) {
			sh.cellPool[sh.poolIdx] = v[:0] // Reset length, keep capacity
			sh.poolIdx++
		}
		delete(sh.cells, k)
	}
}

// getSlice returns a slice from the pool or creates a new one
func (sh *SpatialHashGrid) getSlice() []ecs.Handle {
	if sh.poolIdx > 0 {
		sh.poolIdx--
		return sh.cellPool[sh.poolIdx]
	}
	return make([]ecs.Handle, 0, 8)
}

// Insert adds an entity to all cells it overlaps
func (sh *SpatialHashGrid) Insert(h ecs.Handle, minX, minY, maxX, maxY float64) {
	x0 := int32(math.Floor(minX * sh.invCellSize))
	y0 := int32(math.Floor(minY * sh.invCellSize))
	x1 := int32(math.Floor(maxX * sh.invCellSize))
	y1 := int32(math.Floor(maxY * sh.invCellSize))

	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			key := packKey(x, y)
			cell, ok := sh.cells[key]
			if !ok {
				cell = sh.getSlice()
			}
			sh.cells[key] = append(cell, h)
		}
	}
}

// Query returns all entities in cells overlapping the given AABB
// Results are appended to the provided slice to avoid allocations
func (sh *SpatialHashGrid) Query(minX, minY, maxX, maxY float64, result []ecs.Handle) []ecs.Handle {
	x0 := int32(math.Floor(minX * sh.invCellSize))
	y0 := int32(math.Floor(minY * sh.invCellSize))
	x1 := int32(math.Floor(maxX * sh.invCellSize))
	y1 := int32(math.Floor(maxY * sh.invCellSize))

	// Use a simple seen map to deduplicate
	// For small query areas this is faster than sorting
	seen := make(map[ecs.Handle]struct{}, 16)

	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			key := packKey(x, y)
			if cell, ok := sh.cells[key]; ok {
				for _, h := range cell {
					if _, exists := seen[h]; !exists {
						seen[h] = struct{}{}
						result = append(result, h)
					}
				}
			}
		}
	}
	return result
}

// AABB represents an axis-aligned bounding box
type AABB struct {
	MinX, MinY, MaxX, MaxY float64
}

// SweptAABBResult contains the result of a swept AABB test
type SweptAABBResult struct {
	Hit     bool
	Time    float64 // Time of first contact [0, 1]
	NormalX float64 // Collision normal X
	NormalY float64 // Collision normal Y
}

// sweptAABB performs continuous collision detection between a moving AABB and a static AABB
// Returns the time of first contact and collision normal
// Uses Minkowski difference approach for robust swept collision
func sweptAABB(moving AABB, vx, vy float64, static AABB) SweptAABBResult {
	result := SweptAABBResult{Hit: false, Time: 1.0}

	// If not moving, no swept collision (discrete check should handle this)
	if math.Abs(vx) < sweepEpsilon && math.Abs(vy) < sweepEpsilon {
		return result
	}

	// Expand static box by moving box dimensions (Minkowski sum)
	expandedMinX := static.MinX - (moving.MaxX - moving.MinX)
	expandedMinY := static.MinY - (moving.MaxY - moving.MinY)
	expandedMaxX := static.MaxX
	expandedMaxY := static.MaxY

	// Ray from moving box center to center + velocity
	// We use the min corner of moving box as the ray origin
	rayX := moving.MinX
	rayY := moving.MinY

	// Calculate entry and exit times for each axis
	var tEntryX, tEntryY, tExitX, tExitY float64

	if vx == 0 {
		if rayX < expandedMinX || rayX > expandedMaxX {
			return result // No collision possible on X axis
		}
		tEntryX = math.Inf(-1)
		tExitX = math.Inf(1)
	} else {
		invVX := 1.0 / vx
		t1 := (expandedMinX - rayX) * invVX
		t2 := (expandedMaxX - rayX) * invVX
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tEntryX = t1
		tExitX = t2
	}

	if vy == 0 {
		if rayY < expandedMinY || rayY > expandedMaxY {
			return result // No collision possible on Y axis
		}
		tEntryY = math.Inf(-1)
		tExitY = math.Inf(1)
	} else {
		invVY := 1.0 / vy
		t1 := (expandedMinY - rayY) * invVY
		t2 := (expandedMaxY - rayY) * invVY
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tEntryY = t1
		tExitY = t2
	}

	// Find the latest entry time and earliest exit time
	tEntry := math.Max(tEntryX, tEntryY)
	tExit := math.Min(tExitX, tExitY)

	// No collision if entry is after exit, or entry is after the movement, or exit is before start
	if tEntry > tExit || tEntry > 1.0 || tExit < 0 {
		return result
	}

	// Collision occurs
	if tEntry < 0 {
		tEntry = 0 // Already overlapping at start
	}

	result.Hit = true
	result.Time = tEntry

	// Determine collision normal based on which axis was entered last
	if tEntryX > tEntryY {
		if vx > 0 {
			result.NormalX = -1
		} else {
			result.NormalX = 1
		}
		result.NormalY = 0
	} else {
		result.NormalX = 0
		if vy > 0 {
			result.NormalY = -1
		} else {
			result.NormalY = 1
		}
	}

	return result
}

// CollisionSystem detects and resolves collisions between entities
// Runs at priority 200 (after movement)
// Handles both static and dynamic-dynamic collisions
type CollisionSystem struct {
	ecs.BaseSystem
	staticSpatialHash  *SpatialHashGrid
	dynamicSpatialHash *SpatialHashGrid
	events             []components.CollisionEvent

	// Preallocated buffer to avoid per-tick allocations
	candidates     []ecs.Handle
	dynamicHandles []ecs.Handle
}

// NewCollisionSystem creates a new collision system
// cellSize should be roughly 2x the largest expected collider radius
func NewCollisionSystem() *CollisionSystem {
	return &CollisionSystem{
		BaseSystem:         ecs.NewBaseSystem("CollisionSystem", 200),
		staticSpatialHash:  NewSpatialHash(DefaultCollisionCellSize),
		dynamicSpatialHash: NewSpatialHash(DefaultCollisionCellSize),
		events:             make([]components.CollisionEvent, 0, 64),
		candidates:         make([]ecs.Handle, 0, 64),
		dynamicHandles:     make([]ecs.Handle, 0, 128),
	}
}

// Events returns collision events from the last update (for other systems to consume)
func (s *CollisionSystem) Events() []components.CollisionEvent {
	return s.events
}

// Update performs broad-phase and narrow-phase collision detection
// Uses swept AABB for continuous collision to prevent tunneling
func (s *CollisionSystem) Update(w *ecs.World, dt float64) {
	// Clear previous frame data
	s.events = s.events[:0]
	s.staticSpatialHash.Clear()
	s.dynamicSpatialHash.Clear()
	s.dynamicHandles = s.dynamicHandles[:0]

	// Get component storages
	posStorage := ecs.GetOrCreateStorage[components.Position](w)
	velStorage := ecs.GetOrCreateStorage[components.Velocity](w)
	colliderStorage := ecs.GetOrCreateStorage[components.Collider](w)
	staticStorage := ecs.GetOrCreateStorage[components.Static](w)

	// Phase 1: Insert all static solid entities into spatial hash
	staticQuery := w.Query().
		With(components.PositionID).
		With(components.ColliderID).
		With(components.StaticID)

	for _, h := range staticQuery.Handles() {
		pos, ok := posStorage.Get(h)
		if !ok {
			continue
		}
		col, ok := colliderStorage.Get(h)
		if !ok {
			continue
		}

		halfW := col.Width * 0.5
		halfH := col.Height * 0.5
		s.staticSpatialHash.Insert(h, pos.X-halfW, pos.Y-halfH, pos.X+halfW, pos.Y+halfH)
	}

	// Phase 2: Process dynamic entities (entities with velocity, not static)
	dynamicQuery := w.Query().
		With(components.PositionID).
		With(components.VelocityID).
		With(components.ColliderID)

	for _, h := range dynamicQuery.Handles() {
		// Skip static entities
		if staticStorage.Has(h) {
			continue
		}

		pos := posStorage.GetPtr(h)
		vel := velStorage.GetPtr(h)
		col, ok := colliderStorage.Get(h)
		if pos == nil || vel == nil || !ok {
			continue
		}

		// Track dynamic entity for phase 3
		s.dynamicHandles = append(s.dynamicHandles, h)

		// Skip static collision resolution if not moving
		if math.Abs(vel.X) < sweepEpsilon && math.Abs(vel.Y) < sweepEpsilon {
			continue
		}

		// Resolve collisions with static geometry
		s.resolveStaticCollisions(w, h, pos, vel, &col, dt, posStorage, colliderStorage)
	}

	// Phase 3: Insert all dynamic entities into dynamic spatial hash (after static resolution)
	for _, h := range s.dynamicHandles {
		pos, ok := posStorage.Get(h)
		if !ok {
			continue
		}
		col, ok := colliderStorage.Get(h)
		if !ok {
			continue
		}
		halfW := col.Width * 0.5
		halfH := col.Height * 0.5
		s.dynamicSpatialHash.Insert(h, pos.X-halfW, pos.Y-halfH, pos.X+halfW, pos.Y+halfH)
	}

	// Phase 4: Resolve dynamic-dynamic collisions (push-back)
	s.resolveDynamicCollisions(posStorage, colliderStorage)
}

// resolveStaticCollisions handles collision between a dynamic entity and static geometry
// Uses swept AABB with iterative resolution
func (s *CollisionSystem) resolveStaticCollisions(
	w *ecs.World,
	h ecs.Handle,
	pos *components.Position,
	vel *components.Velocity,
	col *components.Collider,
	dt float64,
	posStorage *ecs.ComponentStorage[components.Position],
	colliderStorage *ecs.ComponentStorage[components.Collider],
) {
	halfW := col.Width * 0.5
	halfH := col.Height * 0.5

	// Calculate movement for this frame
	moveX := vel.X * dt
	moveY := vel.Y * dt

	// Remaining movement to process
	remainingX := moveX
	remainingY := moveY

	log.Printf("[collision] start entity=%d pos=(%.3f,%.3f) vel=(%.3f,%.3f) move=(%.3f,%.3f)", h, pos.X, pos.Y, vel.X, vel.Y, moveX, moveY)

	// Iterative collision resolution
	for iter := 0; iter < maxCollisionIterations; iter++ {
		if math.Abs(remainingX) < sweepEpsilon && math.Abs(remainingY) < sweepEpsilon {
			break
		}

		// Current AABB
		movingAABB := AABB{
			MinX: pos.X - halfW,
			MinY: pos.Y - halfH,
			MaxX: pos.X + halfW,
			MaxY: pos.Y + halfH,
		}

		// Expanded query region to include full movement path
		queryMinX := math.Min(movingAABB.MinX, movingAABB.MinX+remainingX)
		queryMinY := math.Min(movingAABB.MinY, movingAABB.MinY+remainingY)
		queryMaxX := math.Max(movingAABB.MaxX, movingAABB.MaxX+remainingX)
		queryMaxY := math.Max(movingAABB.MaxY, movingAABB.MaxY+remainingY)

		// Query spatial hash for candidates
		s.candidates = s.candidates[:0]
		s.candidates = s.staticSpatialHash.Query(queryMinX, queryMinY, queryMaxX, queryMaxY, s.candidates)

		// Find earliest collision
		earliestTime := 1.0
		var earliestNormalX, earliestNormalY float64
		var collidedHandle ecs.Handle

		for _, candidate := range s.candidates {
			if candidate == h {
				continue
			}

			// Layer mask check
			candidateCol, ok := colliderStorage.Get(candidate)
			if !ok {
				continue
			}
			if col.Layer&candidateCol.Mask == 0 && candidateCol.Layer&col.Mask == 0 {
				continue
			}

			candidatePos, ok := posStorage.Get(candidate)
			if !ok {
				continue
			}

			// Build static AABB
			candHalfW := candidateCol.Width * 0.5
			candHalfH := candidateCol.Height * 0.5
			staticAABB := AABB{
				MinX: candidatePos.X - candHalfW,
				MinY: candidatePos.Y - candHalfH,
				MaxX: candidatePos.X + candHalfW,
				MaxY: candidatePos.Y + candHalfH,
			}

			// Swept AABB test
			result := sweptAABB(movingAABB, remainingX, remainingY, staticAABB)
			if result.Hit && result.Time < earliestTime {
				earliestTime = result.Time
				earliestNormalX = result.NormalX
				earliestNormalY = result.NormalY
				collidedHandle = candidate
			}
		}

		if earliestTime >= 1.0 {
			// No collision, apply full remaining movement
			pos.X += remainingX
			pos.Y += remainingY
			log.Printf("[collision] no hit entity=%d delta=(%.3f,%.3f) newPos=(%.3f,%.3f)", h, remainingX, remainingY, pos.X, pos.Y)
			break
		}

		// Move to collision point (slightly before to avoid penetration)
		safeTime := math.Max(0, earliestTime-collisionEpsilon)
		pos.X += remainingX * safeTime
		pos.Y += remainingY * safeTime

		// Record collision event
		if collidedHandle.IsValid() {
			// Calculate overlap for event (approximate)
			overlapX := math.Abs(remainingX) * (1.0 - earliestTime)
			overlapY := math.Abs(remainingY) * (1.0 - earliestTime)
			s.events = append(s.events, components.CollisionEvent{
				HandleA:  h,
				HandleB:  collidedHandle,
				OverlapX: overlapX,
				OverlapY: overlapY,
			})
			log.Printf("[collision] hit entity=%d with=%d time=%.4f normal=(%.0f,%.0f) overlap=(%.3f,%.3f) pos=(%.3f,%.3f)", h, collidedHandle, earliestTime, earliestNormalX, earliestNormalY, overlapX, overlapY, pos.X, pos.Y)
		}

		// Slide along the collision surface
		// Remove the component of velocity in the normal direction
		remainingX = remainingX * (1.0 - earliestTime)
		remainingY = remainingY * (1.0 - earliestTime)

		// Project remaining movement onto the collision surface (slide)
		dotProduct := remainingX*earliestNormalX + remainingY*earliestNormalY
		remainingX -= dotProduct * earliestNormalX
		remainingY -= dotProduct * earliestNormalY
		log.Printf("[collision] slide entity=%d normal=(%.0f,%.0f) dot=%.4f remaining=(%.3f,%.3f)", h, earliestNormalX, earliestNormalY, dotProduct, remainingX, remainingY)

		// Also update velocity to reflect the collision (for next frame)
		velDot := vel.X*earliestNormalX + vel.Y*earliestNormalY
		if velDot < 0 { // Only if moving into the surface
			vel.X -= velDot * earliestNormalX
			vel.Y -= velDot * earliestNormalY
		}

		// Small epsilon push away from collision surface to prevent sticking
		pos.X += earliestNormalX * collisionEpsilon
		pos.Y += earliestNormalY * collisionEpsilon
	}
}

// resolveDynamicCollisions handles collision between dynamic entities
// Uses discrete overlap detection and push-back resolution
func (s *CollisionSystem) resolveDynamicCollisions(
	posStorage *ecs.ComponentStorage[components.Position],
	colliderStorage *ecs.ComponentStorage[components.Collider],
) {
	// Process each dynamic entity pair (only once per pair)
	for i, hA := range s.dynamicHandles {
		posA := posStorage.GetPtr(hA)
		colA, okA := colliderStorage.Get(hA)
		if posA == nil || !okA {
			continue
		}

		halfWA := colA.Width * 0.5
		halfHA := colA.Height * 0.5

		// Query for nearby dynamic entities
		s.candidates = s.candidates[:0]
		s.candidates = s.dynamicSpatialHash.Query(
			posA.X-halfWA, posA.Y-halfHA,
			posA.X+halfWA, posA.Y+halfHA,
			s.candidates,
		)

		for _, hB := range s.candidates {
			// Skip self
			if hA == hB {
				continue
			}

			// Only process each pair once (hA < hB ensures this)
			// Compare raw handle values to ensure consistent ordering
			if hA.IsValid() && hB.IsValid() && hA >= hB {
				continue
			}

			posB := posStorage.GetPtr(hB)
			colB, okB := colliderStorage.Get(hB)
			if posB == nil || !okB {
				continue
			}

			// Layer mask check
			if colA.Layer&colB.Mask == 0 && colB.Layer&colA.Mask == 0 {
				continue
			}

			halfWB := colB.Width * 0.5
			halfHB := colB.Height * 0.5

			// Calculate AABB overlap
			overlapX := (halfWA + halfWB) - math.Abs(posA.X-posB.X)
			overlapY := (halfHA + halfHB) - math.Abs(posA.Y-posB.Y)

			// No overlap
			if overlapX <= 0 || overlapY <= 0 {
				continue
			}

			// Record collision event
			s.events = append(s.events, components.CollisionEvent{
				HandleA:  hA,
				HandleB:  hB,
				OverlapX: overlapX,
				OverlapY: overlapY,
			})

			// Push-back: separate along the axis of minimum penetration
			// Each entity moves half the overlap distance
			if overlapX < overlapY {
				// Separate along X axis
				pushX := overlapX * 0.5
				if posA.X < posB.X {
					posA.X -= pushX + collisionEpsilon
					posB.X += pushX + collisionEpsilon
				} else {
					posA.X += pushX + collisionEpsilon
					posB.X -= pushX + collisionEpsilon
				}
				log.Printf("[collision] dynamic-dynamic hA=%d hB=%d overlapX=%.3f pushX=%.3f", hA, hB, overlapX, pushX)
				log.Printf("[collision] positions after pushX hA=%d posA=(%.3f,%.3f) hB=%d posB=(%.3f,%.3f)", hA, posA.X, posA.Y, hB, posB.X, posB.Y)
			} else {
				// Separate along Y axis
				pushY := overlapY * 0.5
				if posA.Y < posB.Y {
					posA.Y -= pushY + collisionEpsilon
					posB.Y += pushY + collisionEpsilon
				} else {
					posA.Y += pushY + collisionEpsilon
					posB.Y -= pushY + collisionEpsilon
				}
				log.Printf("[collision] dynamic-dynamic hA=%d hB=%d overlapY=%.3f pushY=%.3f", hA, hB, overlapY, pushY)
				log.Printf("[collision] positions after pushY hA=%d posA=(%.3f,%.3f) hB=%d posB=(%.3f,%.3f)", hA, posA.X, posA.Y, hB, posB.X, posB.Y)
			}
		}

		// Re-insert entity A into spatial hash with updated position (for subsequent queries)
		// This helps with multi-body pile-ups
		_ = i // i is available if we need to track processed entities
	}
}
