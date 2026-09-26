package systems

import (
	"math"
	constt "origin/internal/const"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

const epsilon = 0.001

type CollisionSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	logger       *zap.Logger
	// Pooled buffer for candidates to avoid allocations
	candidatesBuffer []types.Handle
	// Pooled per-sweep prefetch of candidate data (invariant across slide
	// iterations)
	sweepCandidates []sweepCandidate
	// Cached component storages for hot path
	colliderStorage  *ecs.ComponentStorage[components.Collider]
	transformStorage *ecs.ComponentStorage[components.Transform]
	movementStorage  *ecs.ComponentStorage[components.Movement]
	// World boundary configuration
	worldMinX   float64
	worldMaxX   float64
	worldMinY   float64
	worldMaxY   float64
	marginTiles int
	chunkSize   int
}

func NewCollisionSystem(world *ecs.World, chunkManager core.ChunkManager, logger *zap.Logger, worldMinX, worldMaxX, worldMinY, worldMaxY float64, marginTiles int) *CollisionSystem {
	// Cache component storages for hot path optimization
	colliderStorage := ecs.GetOrCreateStorage[components.Collider](world)
	transformStorage := ecs.GetOrCreateStorage[components.Transform](world)
	movementStorage := ecs.GetOrCreateStorage[components.Movement](world)

	marginPixels := float64(marginTiles) * float64(constt.CoordPerTile)

	return &CollisionSystem{
		BaseSystem:       ecs.NewBaseSystem("CollisionSystem", 200),
		chunkManager:     chunkManager,
		logger:           logger,
		candidatesBuffer: make([]types.Handle, 0, 128),
		sweepCandidates:  make([]sweepCandidate, 0, 128),
		colliderStorage:  colliderStorage,
		transformStorage: transformStorage,
		movementStorage:  movementStorage,
		worldMinX:        worldMinX + marginPixels,
		worldMaxX:        worldMaxX - marginPixels,
		worldMinY:        worldMinY + marginPixels,
		worldMaxY:        worldMaxY - marginPixels,
		marginTiles:      marginTiles,
		chunkSize:        constt.ChunkSize,
	}
}

func (s *CollisionSystem) Update(w *ecs.World, dt float64) {
	movedEntities := ecs.GetResource[ecs.MovedEntities](w)
	// Iterate through moved entities from the buffer
	for i := 0; i < movedEntities.Count; i++ {
		h := movedEntities.Handles[i]
		if !w.Alive(h) {
			continue
		}

		transform, ok := ecs.GetComponent[components.Transform](w, h)
		if !ok {
			continue
		}

		intentX := movedEntities.IntentX[i]
		intentY := movedEntities.IntentY[i]

		collider, hasCollider := ecs.GetComponent[components.Collider](w, h)
		if !hasCollider {
			// No collider - just allow movement
			ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
				cr.FinalX = intentX
				cr.FinalY = intentY
				cr.HasCollision = false
				cr.CollidedWith = 0
			})
			continue
		}

		// Get chunk reference
		chunkRef, ok := ecs.GetComponent[components.ChunkRef](w, h)
		if !ok {
			// No chunk ref - allow movement without collision
			ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
				cr.FinalX = intentX
				cr.FinalY = intentY
				cr.HasCollision = false
				cr.CollidedWith = 0
			})
			continue
		}

		// Calculate movement delta
		dx := intentX - transform.X
		dy := intentY - transform.Y

		// Get chunk for spatial queries using ChunkRef
		chunkCoord := types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}
		chunk := s.chunkManager.GetChunk(chunkCoord)
		if chunk == nil {
			// Entity outside valid chunks - allow movement without collision
			ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
				cr.FinalX = intentX
				cr.FinalY = intentY
				cr.HasCollision = false
				cr.CollidedWith = 0
			})
			continue
		}

		// Check phantom collider first (owner's build intent)
		if collider.Phantom != nil {
			phantomResult := s.checkPhantomCollision(w, h, transform, collider, dx, dy, chunk)
			if phantomResult.HasCollision {
				// Hard stop at phantom border - do not slide
				ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
					*cr = phantomResult
				})
				continue
			}
		}

		// Perform swept AABB collision
		result := s.sweepCollision(w, h, transform, collider, dx, dy, chunk)

		// Store collision result
		ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
			*cr = result
		})
	}
}

// sweepCandidate is one spatial-query candidate with its invariant data
// prefetched once per sweep instead of on every slide iteration.
type sweepCandidate struct {
	handle    types.Handle
	collider  components.Collider
	transform components.Transform
	isMoving  bool
}

// sweepCollision performs swept AABB collision using Minkowski difference
func (s *CollisionSystem) sweepCollision(
	w *ecs.World,
	entityHandle types.Handle,
	transform components.Transform,
	collider components.Collider,
	dx, dy float64,
	chunk *core.Chunk,
) components.CollisionResult {
	result := components.CollisionResult{
		FinalX:       transform.X + dx,
		FinalY:       transform.Y + dy,
		HasCollision: false,
	}

	if math.Abs(dx) < 0.001 && math.Abs(dy) < 0.001 {
		result.FinalX = transform.X
		result.FinalY = transform.Y
		return result
	}

	// Find the terrain contact before comparing it with object contacts.
	movement, hasMovement := s.movementStorage.Get(entityHandle)
	isSwimming := hasMovement && movement.Mode == constt.Swim
	tileStopX, tileStopY, tileNormalX, tileNormalY, tileHitT, tileBlocked := s.checkTileCollision(
		transform.X, transform.Y, dx, dy,
		collider.HalfWidth, collider.HalfHeight, chunk, isSwimming,
	)

	// Entity AABB
	entityHalfW := collider.HalfWidth
	entityHalfH := collider.HalfHeight

	// Original velocity magnitude
	originalSpeed := math.Sqrt(dx*dx + dy*dy)

	// сколько в мировых координатах запрашиваем вокруг объекта для коллизий
	spatialRequestSize := 20.0

	// Query potential colliders from spatial hash using pooled buffer
	s.candidatesBuffer = s.candidatesBuffer[:0] // Reset buffer, keep capacity
	queryRadius := math.Max(math.Abs(dx), math.Abs(dy)) + math.Max(entityHalfW, entityHalfH) + spatialRequestSize

	// Query current chunk
	chunk.Spatial().QueryRadius(transform.X, transform.Y, queryRadius, &s.candidatesBuffer)

	// Check if query rectangle intersects neighboring chunks
	chunkWorldSize := float64(constt.ChunkWorldSize)

	// Calculate query rectangle bounds
	queryMinX := transform.X - queryRadius
	queryMaxX := transform.X + queryRadius
	queryMinY := transform.Y - queryRadius
	queryMaxY := transform.Y + queryRadius

	// Check each neighboring chunk (8 directions)
	neighborOffsets := []struct{ dx, dy int }{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	for _, offset := range neighborOffsets {
		neighborChunkX := chunk.Coord.X + offset.dx
		neighborChunkY := chunk.Coord.Y + offset.dy

		// Calculate neighbor chunk boundaries
		neighborMinX := float64(neighborChunkX) * chunkWorldSize
		neighborMaxX := float64(neighborChunkX+1) * chunkWorldSize
		neighborMinY := float64(neighborChunkY) * chunkWorldSize
		neighborMaxY := float64(neighborChunkY+1) * chunkWorldSize

		// Check if query rectangle intersects neighbor chunk
		intersects := !(queryMaxX < neighborMinX || queryMinX > neighborMaxX ||
			queryMaxY < neighborMinY || queryMinY > neighborMaxY)

		if intersects {
			neighborCoord := types.ChunkCoord{X: neighborChunkX, Y: neighborChunkY}
			neighborChunk := s.chunkManager.GetChunk(neighborCoord)
			if neighborChunk != nil {
				neighborChunk.Spatial().QueryRadius(transform.X, transform.Y, queryRadius, &s.candidatesBuffer)
			}
		}
	}

	candidates := s.candidatesBuffer

	// Prefetch the candidate set once for all slide iterations: nothing can
	// mutate these components mid-sweep (single-threaded tick), and the slide
	// budget bounds the whole path within the intended distance of the start,
	// so a candidate farther than intent + both half extents on any axis can
	// never be touched, not even after a slide redirect.
	pathBound := originalSpeed*(1+epsilon) + 1.0
	s.sweepCandidates = s.sweepCandidates[:0]
	for _, candidateHandle := range candidates {
		if candidateHandle == entityHandle || !w.Alive(candidateHandle) {
			continue
		}

		candidateCollider, ok := s.colliderStorage.Get(candidateHandle)
		if !ok {
			continue
		}

		// Check collision layer mask
		if collider.Layer&candidateCollider.Mask == 0 && candidateCollider.Layer&collider.Mask == 0 {
			continue
		}

		// Skip phantom colliders - they don't block other entities
		if candidateCollider.Phantom != nil {
			continue
		}

		candidateTransform, ok := s.transformStorage.Get(candidateHandle)
		if !ok {
			continue
		}

		if math.Abs(candidateTransform.X-transform.X) > pathBound+entityHalfW+candidateCollider.HalfWidth ||
			math.Abs(candidateTransform.Y-transform.Y) > pathBound+entityHalfH+candidateCollider.HalfHeight {
			continue
		}

		candidateMovement, candidateMoving := s.movementStorage.Get(candidateHandle)
		s.sweepCandidates = append(s.sweepCandidates, sweepCandidate{
			handle:    candidateHandle,
			collider:  candidateCollider,
			transform: candidateTransform,
			isMoving:  candidateMoving && candidateMovement.State == constt.StateMoving,
		})
	}

	// Remaining movement
	remainingDX := dx
	remainingDY := dy
	currentX := transform.X
	currentY := transform.Y

	// Fraction of the original per-tick distance still available for sliding.
	// Each slide hit consumes the traveled share so total displacement can
	// never exceed the intended per-tick distance.
	remainingBudget := 1.0

	// Iteration limit for sliding
	const maxIterations = 3

	for iter := 0; iter < maxIterations; iter++ {
		if math.Abs(remainingDX) < 0.001 && math.Abs(remainingDY) < 0.001 {
			break
		}

		// The first segment uses the intent's tile sweep above. Slides need
		// their own sweep because they leave that original path.
		if iter > 0 {
			tileStopX, tileStopY, tileNormalX, tileNormalY, tileHitT, tileBlocked = s.checkTileCollision(
				currentX, currentY, remainingDX, remainingDY,
				entityHalfW, entityHalfH, chunk, isSwimming,
			)
		}

		earliestT := 1.0
		var hitNormalX, hitNormalY float64
		var collidedWith types.EntityID

		for i := range s.sweepCandidates {
			candidate := &s.sweepCandidates[i]

			// An already-overlapping pair is a miss for sweptAABB (entryTime
			// < 0) and would pass through. Resolve it here: deepening movement
			// hits at t=0, while escaping or tangential movement stays free.
			if overlapNX, overlapNY, deepening, overlapped := startOverlapNormal(
				currentX, currentY, entityHalfW, entityHalfH,
				remainingDX, remainingDY,
				candidate.transform.X, candidate.transform.Y,
				candidate.collider.HalfWidth, candidate.collider.HalfHeight,
			); overlapped {
				if deepening && earliestT > 0 {
					earliestT = 0
					hitNormalX = overlapNX
					hitNormalY = overlapNY
					if id, ok := w.GetExternalID(candidate.handle); ok {
						collidedWith = id
					}
					if candidate.isMoving {
						// For dynamic-dynamic collision: stop completely
						remainingDX = 0
						remainingDY = 0
					}
				}
				continue
			}

			if candidate.isMoving {
				// Both moving - stop, do not push back
				t, nx, ny, hit := s.sweptAABB(
					currentX, currentY, entityHalfW, entityHalfH,
					remainingDX, remainingDY,
					candidate.transform.X, candidate.transform.Y,
					candidate.collider.HalfWidth, candidate.collider.HalfHeight,
				)
				if hit && t < earliestT {
					earliestT = t
					hitNormalX = nx
					hitNormalY = ny
					if id, ok := w.GetExternalID(candidate.handle); ok {
						collidedWith = id
					}
					// For dynamic-dynamic collision: stop completely
					remainingDX = 0
					remainingDY = 0
				}
				continue
			}

			// Static collision - use swept AABB with sliding
			t, nx, ny, hit := s.sweptAABB(
				currentX, currentY, entityHalfW, entityHalfH,
				remainingDX, remainingDY,
				candidate.transform.X, candidate.transform.Y,
				candidate.collider.HalfWidth, candidate.collider.HalfHeight,
			)

			if hit && t < earliestT {
				earliestT = t
				hitNormalX = nx
				hitNormalY = ny
				if id, ok := w.GetExternalID(candidate.handle); ok {
					collidedWith = id
				}
			}
		}

		// A tile can stop this segment only if it is closer than every object.
		// Otherwise resolve the object first and recheck the redirected slide.
		if tileBlocked && tileHitT < earliestT {
			result.HasCollision = true
			result.CollisionNormalX = tileNormalX
			result.CollisionNormalY = tileNormalY
			if iter == 0 {
				result.FinalX = tileStopX
				result.FinalY = tileStopY
				return result
			}
			currentX = tileStopX
			currentY = tileStopY
			break
		}

		if earliestT < 1.0 {
			// Move to collision point (with small epsilon)
			currentX += remainingDX * (earliestT - epsilon)
			currentY += remainingDY * (earliestT - epsilon)

			result.HasCollision = true
			result.CollisionNormalX = hitNormalX
			result.CollisionNormalY = hitNormalY
			result.CollidedWith = collidedWith

			// Slide along wall: maintain original speed in slide direction
			// Calculate parallel component from current movement direction
			dotNormal := remainingDX*hitNormalX + remainingDY*hitNormalY
			parallelX := remainingDX - dotNormal*hitNormalX
			parallelY := remainingDY - dotNormal*hitNormalY

			// Normalize and apply remaining budget in slide direction
			parallelSpeed := math.Sqrt(parallelX*parallelX + parallelY*parallelY)
			if parallelSpeed < epsilon {
				// Moving perpendicular to wall - stop
				result.PerpendicularOscillation = true
				if debugEnabled {
					s.logger.Debug("Perpendicular collision - stopping",
						zap.Uint64("handle", uint64(entityHandle)),
						zap.Float64("normalX", hitNormalX),
						zap.Float64("normalY", hitNormalY),
						zap.Float64("remainingDX", remainingDX),
						zap.Float64("remainingDY", remainingDY),
						zap.Float64("parallelSpeed", parallelSpeed),
					)
				}
				break
			}

			// The sweep moved (earliestT - epsilon) of the current remaining
			// vector; shrink the budget by that share before redirecting.
			spent := earliestT - epsilon
			if spent < 0 {
				spent = 0
			}
			remainingBudget *= 1 - spent

			// Apply original speed scaled by the remaining budget
			remainingDX = (parallelX / parallelSpeed) * originalSpeed * remainingBudget
			remainingDY = (parallelY / parallelSpeed) * originalSpeed * remainingBudget
		} else {
			// No collision - move full distance
			currentX += remainingDX
			currentY += remainingDY
			break
		}
	}

	result.FinalX = currentX
	result.FinalY = currentY

	// Clamp to world boundaries with margin
	result.FinalX = math.Max(s.worldMinX, math.Min(s.worldMaxX, result.FinalX))
	result.FinalY = math.Max(s.worldMinY, math.Min(s.worldMaxY, result.FinalY))

	// Detect oscillation: if object didn't move in intended direction
	if result.HasCollision && originalSpeed > 0.1 {
		// Check if object moved in the direction of original intent
		dotProduct := (result.FinalX-transform.X)*dx + (result.FinalY-transform.Y)*dy

		// If dot product is negative or very small, object moved opposite or perpendicular to intent
		if dotProduct < originalSpeed*0.1 {
			result.PerpendicularOscillation = true
		}

		// Also detect oscillation by checking if object is bouncing between two positions
		// Get previous collision result to compare
		prevCollisionResult, hasPrevCollision := ecs.GetComponent[components.CollisionResult](w, entityHandle)
		if hasPrevCollision && prevCollisionResult.HasCollision && result.CollidedWith != 0 && prevCollisionResult.CollidedWith != 0 {
			// Check if colliding with same object
			if result.CollidedWith == prevCollisionResult.CollidedWith {
				// Check if positions are very close (bouncing between two spots)
				distToPrev := math.Sqrt(
					(result.FinalX-prevCollisionResult.FinalX)*(result.FinalX-prevCollisionResult.FinalX) +
						(result.FinalY-prevCollisionResult.FinalY)*(result.FinalY-prevCollisionResult.FinalY),
				)
				// If distance to previous collision position is small, it's oscillation
				if distToPrev < 2.0 && distToPrev > 0.5 {
					result.PerpendicularOscillation = true
				}
			}
		}

		if debugEnabled {
			s.logger.Debug("Collision",
				zap.Uint64("handle", uint64(entityHandle)),
				zap.Any("CollidedWith", result.CollidedWith),
				zap.Float64("finalX", result.FinalX),
				zap.Float64("finalY", result.FinalY),
				zap.Bool("perpendicular", result.PerpendicularOscillation),
			)
		}
	}
	return result
}

// maxSweepTiles caps the tile range of one sweep as a defensive bound;
// per-tick movement stays far below it, so exceeding it signals a pathological dt.
const maxSweepTiles = 32

// checkTileCollision sweeps the entity's AABB (halfW/halfH) from start along
// (dx, dy) against impassable tiles. Returns the center position at the
// earliest contact (or the untouched destination when clear), the contact
// normal, hit time, and whether movement is blocked. Initial overlaps block
// deepening moves while allowing escape and movement along the terrain edge.
func (s *CollisionSystem) checkTileCollision(
	startX, startY, dx, dy, halfW, halfH float64,
	chunk *core.Chunk,
	isSwimming bool,
) (stopX, stopY, normalX, normalY, hitT float64, blocked bool) {
	if math.Abs(dx) < 0.001 && math.Abs(dy) < 0.001 {
		return startX, startY, 0, 0, 1, false
	}

	tileSize := float64(constt.CoordPerTile)
	tileHalf := tileSize / 2

	// Range of tiles the swept box can touch.
	minTileX := int(math.Floor((math.Min(startX, startX+dx) - halfW) / tileSize))
	maxTileX := int(math.Floor((math.Max(startX, startX+dx) + halfW) / tileSize))
	minTileY := int(math.Floor((math.Min(startY, startY+dy) - halfH) / tileSize))
	maxTileY := int(math.Floor((math.Max(startY, startY+dy) + halfH) / tileSize))

	if (maxTileX-minTileX+1)*(maxTileY-minTileY+1) > maxSweepTiles*maxSweepTiles {
		// Sweep too large to enumerate - skip rather than stall the tick.
		return startX + dx, startY + dy, 0, 0, 1, false
	}

	earliestT := 1.0
	var hitNormalX, hitNormalY float64

	for tileY := minTileY; tileY <= maxTileY; tileY++ {
		for tileX := minTileX; tileX <= maxTileX; tileX++ {
			if s.tilePassableAtCoords(tileX, tileY, chunk, isSwimming) {
				continue
			}

			tileCenterX := float64(tileX)*tileSize + tileHalf
			tileCenterY := float64(tileY)*tileSize + tileHalf
			nx, ny, deepening, overlapped := s.startTileOverlapNormal(
				startX, startY, halfW, halfH, dx, dy,
				tileX, tileY, chunk, isSwimming,
			)
			t, hit := 0.0, deepening
			if !overlapped {
				t, nx, ny, hit = s.sweptAABB(
					startX, startY, halfW, halfH, dx, dy,
					tileCenterX, tileCenterY, tileHalf, tileHalf,
				)
			}
			// Adjacent blocked tiles form one obstacle. Their internal faces
			// must not stop an overlapped body moving along the shoreline.
			if hit && t < earliestT && s.tilePassableAtCoords(tileX+int(nx), tileY+int(ny), chunk, isSwimming) {
				earliestT = t
				hitNormalX = nx
				hitNormalY = ny
			}
		}
	}

	if earliestT < 1.0 {
		travelT := math.Max(0, earliestT-epsilon)
		return startX + dx*travelT,
			startY + dy*travelT,
			hitNormalX, hitNormalY, earliestT, true
	}
	return startX + dx, startY + dy, 0, 0, 1, false
}

// Initial terrain overlaps use the nearest exposed face. Internal tile faces
// cannot define an escape direction because they lead into the same obstacle.
func (s *CollisionSystem) startTileOverlapNormal(
	startX, startY, halfW, halfH, dx, dy float64,
	tileX, tileY int, chunk *core.Chunk, isSwimming bool,
) (normalX, normalY float64, deepening, overlapped bool) {
	tileSize := float64(constt.CoordPerTile)
	tileHalf := tileSize / 2
	tileCenterX := float64(tileX)*tileSize + tileHalf
	tileCenterY := float64(tileY)*tileSize + tileHalf
	_, _, _, overlapped = startOverlapNormal(startX, startY, halfW, halfH, dx, dy,
		tileCenterX, tileCenterY, tileHalf, tileHalf)
	if !overlapped {
		return 0, 0, false, false
	}

	faces := [...]struct {
		normalX, normalY int
		exitDistance     float64
	}{
		{-1, 0, startX - (tileCenterX - tileHalf - halfW)},
		{1, 0, tileCenterX + tileHalf + halfW - startX},
		{0, -1, startY - (tileCenterY - tileHalf - halfH)},
		{0, 1, tileCenterY + tileHalf + halfH - startY},
	}
	nearestDistance := math.Inf(1)
	for _, face := range faces {
		if face.exitDistance < nearestDistance && s.tilePassableAtCoords(
			tileX+face.normalX, tileY+face.normalY, chunk, isSwimming,
		) {
			nearestDistance = face.exitDistance
			normalX = float64(face.normalX)
			normalY = float64(face.normalY)
		}
	}
	return normalX, normalY, dx*normalX+dy*normalY < 0, true
}

// tilePassableAtCoords reports whether the tile at integer tile coordinates is
// passable for the given movement mode, resolving the owning chunk (including
// neighbors of the base chunk) with floor division so negative coordinates work.
func (s *CollisionSystem) tilePassableAtCoords(
	tileX, tileY int,
	chunk *core.Chunk,
	isSwimming bool,
) bool {
	chunkCoord := types.ChunkCoord{
		X: floorDiv(tileX, s.chunkSize),
		Y: floorDiv(tileY, s.chunkSize),
	}
	if chunkCoord != chunk.Coord {
		resolved := s.chunkManager.GetChunk(chunkCoord)
		if resolved == nil {
			// No chunk loaded - consider impassable
			return false
		}
		chunk = resolved
	}
	localTileX := tileX - chunkCoord.X*s.chunkSize
	localTileY := tileY - chunkCoord.Y*s.chunkSize

	if isSwimming {
		return chunk.IsTileSwimmable(localTileX, localTileY, s.chunkSize)
	}
	return chunk.IsTilePassable(localTileX, localTileY, s.chunkSize)
}

// startOverlapNormal reports whether the mover's box already overlaps the
// candidate at sweep start. sweptAABB rejects such pairs (entryTime < 0), so
// the candidate loop uses this to resolve overlaps explicitly: deepening
// movement counts as a hit at t=0, escaping or tangential movement stays free
// so overlapped entities can always walk out. The normal is the
// minimum-penetration axis pointing from candidate toward mover; exact
// contact (penetration 0) is left to the normal sweep, which blocks it at
// entryTime 0.
func startOverlapNormal(
	ax, ay, aHalfW, aHalfH float64,
	dx, dy float64,
	bx, by, bHalfW, bHalfH float64,
) (normalX, normalY float64, deepening, overlapped bool) {
	penX := aHalfW + bHalfW - math.Abs(ax-bx)
	penY := aHalfH + bHalfH - math.Abs(ay-by)
	if penX <= 0 || penY <= 0 {
		return 0, 0, false, false
	}

	if penX < penY {
		normalX = -1
		if ax > bx {
			normalX = 1
		}
		return normalX, 0, dx*normalX < 0, true
	}
	normalY = -1
	if ay > by {
		normalY = 1
	}
	return 0, normalY, dy*normalY < 0, true
}

// sweptAABB performs swept AABB collision using Minkowski difference
// Returns: t (collision time 0-1), normalX, normalY, hit
func (s *CollisionSystem) sweptAABB(
	ax, ay, aHalfW, aHalfH float64,
	dx, dy float64,
	bx, by, bHalfW, bHalfH float64,
) (float64, float64, float64, bool) {
	// Minkowski sum: expand B by A's size
	mHalfW := aHalfW + bHalfW
	mHalfH := aHalfH + bHalfH

	// Check if sweep path can possibly intersect B
	// If no movement on an axis, objects must overlap on that axis
	if dx == 0 {
		if math.Abs(ax-bx) >= mHalfW {
			return 1.0, 0, 0, false
		}
	}
	if dy == 0 {
		if math.Abs(ay-by) >= mHalfH {
			return 1.0, 0, 0, false
		}
	}

	// Ray from A's center against expanded B
	// Entry and exit times for each axis
	var xInvEntry, yInvEntry float64
	var xInvExit, yInvExit float64

	if dx > 0 {
		xInvEntry = (bx - mHalfW) - ax
		xInvExit = (bx + mHalfW) - ax
	} else if dx < 0 {
		xInvEntry = (bx + mHalfW) - ax
		xInvExit = (bx - mHalfW) - ax
	} else {
		xInvEntry = math.Inf(-1)
		xInvExit = math.Inf(1)
	}

	if dy > 0 {
		yInvEntry = (by - mHalfH) - ay
		yInvExit = (by + mHalfH) - ay
	} else if dy < 0 {
		yInvEntry = (by + mHalfH) - ay
		yInvExit = (by - mHalfH) - ay
	} else {
		yInvEntry = math.Inf(-1)
		yInvExit = math.Inf(1)
	}

	// Calculate entry/exit times
	var xEntry, yEntry float64
	var xExit, yExit float64

	if dx == 0 {
		xEntry = math.Inf(-1)
		xExit = math.Inf(1)
	} else {
		xEntry = xInvEntry / dx
		xExit = xInvExit / dx
	}

	if dy == 0 {
		yEntry = math.Inf(-1)
		yExit = math.Inf(1)
	} else {
		yEntry = yInvEntry / dy
		yExit = yInvExit / dy
	}

	entryTime := math.Max(xEntry, yEntry)
	exitTime := math.Min(xExit, yExit)

	// No collision
	if entryTime > exitTime || (xEntry < 0 && yEntry < 0) || entryTime > 1 || entryTime < 0 {
		return 1.0, 0, 0, false
	}

	// Calculate normal
	var normalX, normalY float64
	if xEntry > yEntry {
		if dx < 0 {
			normalX = 1
		} else {
			normalX = -1
		}
		normalY = 0
	} else {
		normalX = 0
		if dy < 0 {
			normalY = 1
		} else {
			normalY = -1
		}
	}

	return entryTime, normalX, normalY, true
}

// checkPhantomCollision checks if entity's collider collides with its phantom
// Uses swept AABB to find touch point, then stops (no sliding, single iteration)
func (s *CollisionSystem) checkPhantomCollision(
	w *ecs.World,
	entityHandle types.Handle,
	transform components.Transform,
	collider components.Collider,
	dx, dy float64,
	chunk *core.Chunk,
) components.CollisionResult {
	result := components.CollisionResult{
		FinalX:       transform.X + dx,
		FinalY:       transform.Y + dy,
		HasCollision: false,
		IsPhantom:    false,
	}

	phantom := collider.Phantom
	if phantom == nil {
		return result
	}

	// Perform swept AABB collision with phantom
	entityHalfW := collider.HalfWidth
	entityHalfH := collider.HalfHeight
	phantomHalfW := phantom.HalfWidth
	phantomHalfH := phantom.HalfHeight

	t, normalX, normalY, hit := s.sweptAABB(
		transform.X, transform.Y, entityHalfW, entityHalfH,
		dx, dy,
		phantom.WorldX, phantom.WorldY, phantomHalfW, phantomHalfH,
	)

	if hit && t < 1.0 {
		// Collision detected - move to touch point (with epsilon)
		result.FinalX = transform.X + dx*(t-epsilon)
		result.FinalY = transform.Y + dy*(t-epsilon)
		result.HasCollision = true
		result.IsPhantom = true
		result.CollisionNormalX = normalX
		result.CollisionNormalY = normalY

		if extID, ok := w.GetExternalID(entityHandle); ok {
			result.CollidedWith = extID
		}
	}

	return result
}
