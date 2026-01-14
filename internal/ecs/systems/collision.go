package systems

import (
	"math"

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
}

func NewCollisionSystem(chunkManager core.ChunkManager, logger *zap.Logger) *CollisionSystem {
	return &CollisionSystem{
		BaseSystem:   ecs.NewBaseSystem("CollisionSystem", 200),
		chunkManager: chunkManager,
		logger:       logger,
	}
}

func (s *CollisionSystem) Update(w *ecs.World, dt float64) {
	activeChunks := s.chunkManager.ActiveChunks()
	for _, chunk := range activeChunks {
		dynamicHandles := chunk.GetDynamicHandles()

		for _, h := range dynamicHandles {
			if !w.Alive(h) {
				continue
			}

			transform, ok := ecs.GetComponent[components.Transform](w, h)
			if !ok {
				continue
			}

			// Skip if no movement intent
			if !transform.WasMoved {
				continue
			}

			collider, hasCollider := ecs.GetComponent[components.Collider](w, h)
			if !hasCollider {
				// No collider - just allow movement
				ecs.WithComponent(w, h, func(cr *components.CollisionResult) {
					cr.FinalX = transform.IntentX
					cr.FinalY = transform.IntentY
					cr.HasCollision = false
					cr.CollidedWith = nil
				})
				continue
			}

			// Calculate movement delta
			dx := transform.IntentX - transform.X
			dy := transform.IntentY - transform.Y

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
				cr.HasCollision = result.HasCollision
				cr.CollidedWith = result.CollidedWith
				cr.IsPhantom = result.IsPhantom
				cr.FinalX = result.FinalX
				cr.FinalY = result.FinalY
				cr.CollisionNormalX = result.CollisionNormalX
				cr.CollisionNormalY = result.CollisionNormalY
			})
		}
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

	// Entity AABB
	entityHalfW := collider.HalfWidth
	entityHalfH := collider.HalfHeight

	// Query potential colliders from spatial hash
	candidates := make([]types.Handle, 0, 64)
	queryRadius := math.Max(math.Abs(dx), math.Abs(dy)) + math.Max(entityHalfW, entityHalfH) + 64
	chunk.Spatial().QueryRadius(transform.X, transform.Y, queryRadius, &candidates)

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

			candidateCollider, ok := ecs.GetComponent[components.Collider](w, candidateHandle)
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

			candidateTransform, ok := ecs.GetComponent[components.Transform](w, candidateHandle)
			if !ok {
				continue
			}

			// Check if candidate is also moving (dynamic collision)
			candidateMovement, candidateMoving := ecs.GetComponent[components.Movement](w, candidateHandle)
			if candidateMoving && candidateMovement.State == components.StateMoving {
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
				float64(candidateCollider.HalfWidth), float64(candidateCollider.HalfHeight),
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
			if collidedWith != 0 {
				result.CollidedWith = append(result.CollidedWith, collidedWith)
			}

			// Slide along wall
			remainingTime := 1.0 - earliestT
			dotProduct := (remainingDX*hitNormalY + remainingDY*(-hitNormalX)) * remainingTime
			remainingDX = dotProduct * hitNormalY
			remainingDY = dotProduct * (-hitNormalX)
		} else {
			// No collision - move full distance
			currentX += remainingDX
			currentY += remainingDY
			break
		}
	}

	result.FinalX = currentX
	result.FinalY = currentY
	if result.HasCollision {
		s.logger.Debug("Collision",
			zap.Uint64("handle", uint64(entityHandle)),
			zap.Any("CollidedWith", result.CollidedWith),
			zap.Float64("finalX", result.FinalX),
			zap.Float64("finalY", result.FinalY),
		)
	}
	return result
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
			result.CollidedWith = []types.EntityID{extID}
		}
	}

	return result
}
