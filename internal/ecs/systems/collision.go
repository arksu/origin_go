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
				return
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

	// Check tile collisions first
	movement, hasMovement := s.movementStorage.Get(entityHandle)
	isSwimming := hasMovement && movement.Mode == constt.Swim
	tileCollisionX, tileCollisionY, hasTileCollision := s.checkTileCollision(
		transform.X, transform.Y, dx, dy, chunk, isSwimming,
	)
	if hasTileCollision {
		// Stop at tile collision point
		result.FinalX = tileCollisionX
		result.FinalY = tileCollisionY
		result.HasCollision = true
		return result
	}

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

	// Remaining movement
	remainingDX := dx
	remainingDY := dy
	currentX := transform.X
	currentY := transform.Y

	// Iteration limit for sliding
	const maxIterations = 3

	for iter := 0; iter < maxIterations; iter++ {
		if math.Abs(remainingDX) < 0.001 && math.Abs(remainingDY) < 0.001 {
			break
		}

		earliestT := 1.0
		var hitNormalX, hitNormalY float64
		var collidedWith types.EntityID

		for _, candidateHandle := range candidates {
			if candidateHandle == entityHandle {
				continue
			}
			if !w.Alive(candidateHandle) {
				continue
			}

			// Use cached storage for faster component access
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

			// Check if candidate is also moving (dynamic collision)
			candidateMovement, candidateMoving := s.movementStorage.Get(candidateHandle)
			if candidateMoving && candidateMovement.State == constt.StateMoving {
				// Both moving - stop, do not push back
				t, nx, ny, hit := s.sweptAABB(
					currentX, currentY, entityHalfW, entityHalfH,
					remainingDX, remainingDY,
					candidateTransform.X, candidateTransform.Y,
					candidateCollider.HalfWidth, candidateCollider.HalfHeight,
				)
				if hit && t < earliestT {
					earliestT = t
					hitNormalX = nx
					hitNormalY = ny
					if id, ok := w.GetExternalID(candidateHandle); ok {
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
				candidateTransform.X, candidateTransform.Y,
				candidateCollider.HalfWidth, candidateCollider.HalfHeight,
			)

			if hit && t < earliestT {
				earliestT = t
				hitNormalX = nx
				hitNormalY = ny
				if id, ok := w.GetExternalID(candidateHandle); ok {
					collidedWith = id
				}
			}
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

			// Normalize and apply original speed
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

			// Apply original speed to parallel direction
			remainingDX = (parallelX / parallelSpeed) * originalSpeed
			remainingDY = (parallelY / parallelSpeed) * originalSpeed
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

// checkTileCollision checks if movement path crosses impassable tiles
// Returns collision point (x, y) and whether collision occurred
func (s *CollisionSystem) checkTileCollision(
	startX, startY, dx, dy float64,
	chunk *core.Chunk,
	isSwimming bool,
) (float64, float64, bool) {
	tileSize := float64(constt.CoordPerTile)
	movementLength := math.Sqrt(dx*dx + dy*dy)

	// If movement is less than one tile, check only destination
	if movementLength <= tileSize {
		endX := startX + dx
		endY := startY + dy
		if !s.isTilePassableAt(endX, endY, chunk, isSwimming) {
			return startX, startY, true
		}
		return endX, endY, false
	}

	// Split movement into tile-sized steps
	numSteps := int(math.Ceil(movementLength / tileSize))
	stepX := dx / float64(numSteps)
	stepY := dy / float64(numSteps)

	currentX := startX
	currentY := startY

	for i := 1; i <= numSteps; i++ {
		nextX := startX + stepX*float64(i)
		nextY := startY + stepY*float64(i)

		if !s.isTilePassableAt(nextX, nextY, chunk, isSwimming) {
			// Return last valid position
			return currentX, currentY, true
		}

		currentX = nextX
		currentY = nextY
	}

	return currentX, currentY, false
}

// isTilePassableAt checks if a tile at world coordinates is passable
func (s *CollisionSystem) isTilePassableAt(
	worldX, worldY float64,
	chunk *core.Chunk,
	isSwimming bool,
) bool {
	// Convert world coordinates to tile coordinates
	tileSize := float64(constt.CoordPerTile)
	tileX := int(math.Floor(worldX / tileSize))
	tileY := int(math.Floor(worldY / tileSize))

	// Convert to chunk-local coordinates
	chunkWorldX := chunk.Coord.X * s.chunkSize
	chunkWorldY := chunk.Coord.Y * s.chunkSize
	localTileX := tileX - chunkWorldX
	localTileY := tileY - chunkWorldY

	// Check if coordinates are within current chunk
	if localTileX < 0 || localTileX >= s.chunkSize || localTileY < 0 || localTileY >= s.chunkSize {
		// Need to check neighboring chunk
		targetChunkX := chunkWorldX
		targetChunkY := chunkWorldY

		if localTileX < 0 {
			targetChunkX--
			localTileX += s.chunkSize
		} else if localTileX >= s.chunkSize {
			targetChunkX++
			localTileX -= s.chunkSize
		}

		if localTileY < 0 {
			targetChunkY--
			localTileY += s.chunkSize
		} else if localTileY >= s.chunkSize {
			targetChunkY++
			localTileY -= s.chunkSize
		}

		neighborCoord := types.ChunkCoord{X: targetChunkX, Y: targetChunkY}
		neighborChunk := s.chunkManager.GetChunk(neighborCoord)
		if neighborChunk == nil {
			// No chunk loaded - consider impassable
			return false
		}
		chunk = neighborChunk
	}

	// Check tile passability based on movement mode
	if isSwimming {
		return chunk.IsTileSwimmable(localTileX, localTileY, s.chunkSize)
	}
	return chunk.IsTilePassable(localTileX, localTileY, s.chunkSize)
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
