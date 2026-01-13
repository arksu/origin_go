package systems

import (
	"math"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

type CollisionSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	logger       *zap.Logger
}

func NewCollisionSystem(chunkManager core.ChunkManager, logger *zap.Logger) *CollisionSystem {
	return &CollisionSystem{
		BaseSystem:   ecs.NewBaseSystem("CollisionSystem", 110),
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

			// Get read-only copies of components
			collider, ok := ecs.GetComponent[components.Collider](w, h)
			if !ok {
				continue
			}

			// Ensure CollisionResult exists and get mutable access
			hasResult := ecs.WithComponent(w, h, func(result *components.CollisionResult) {
				// Reset result
				result.HasCollision = false
				result.CollidedWith = result.CollidedWith[:0]
				result.IsPhantom = false
				result.CollisionNormalX = 0
				result.CollisionNormalY = 0

				// Case 1: Phantom Collider
				if collider.Phantom != nil {
					result.IsPhantom = true
					s.checkPhantomCollision(w, chunk, h, collider, result)
					return
				}

				// Case 2: Normal Entity Movement
				transform, ok := ecs.GetComponent[components.Transform](w, h)
				if !ok {
					return
				}

				result.FinalX = transform.IntentX
				result.FinalY = transform.IntentY

				s.checkSweptCollision(w, chunk, h, transform, collider, result)
			})

			// If component didn't exist, create it and try again
			if !hasResult {
				ecs.AddComponent(w, h, components.CollisionResult{})
				ecs.WithComponent(w, h, func(result *components.CollisionResult) {
					result.HasCollision = false
					result.CollidedWith = make([]types.EntityID, 0)
					result.IsPhantom = false

					if collider.Phantom != nil {
						result.IsPhantom = true
						s.checkPhantomCollision(w, chunk, h, collider, result)
						return
					}

					transform, ok := ecs.GetComponent[components.Transform](w, h)
					if !ok {
						return
					}
					result.FinalX = transform.IntentX
					result.FinalY = transform.IntentY
					s.checkSweptCollision(w, chunk, h, transform, collider, result)
				})
			}
		}
	}
}

func (s *CollisionSystem) checkPhantomCollision(w *ecs.World, chunk *core.Chunk, entityH types.Handle, collider components.Collider, result *components.CollisionResult) {
	phantom := collider.Phantom

	// Phantom AABB
	minX := int(phantom.WorldX) - phantom.HalfWidth
	maxX := int(phantom.WorldX) + phantom.HalfWidth
	minY := int(phantom.WorldY) - phantom.HalfHeight
	maxY := int(phantom.WorldY) + phantom.HalfHeight

	var candidates []types.Handle
	chunk.Spatial().QueryAABB(minX, minY, maxX, maxY, &candidates)

	for _, otherH := range candidates {
		if otherH == entityH {
			continue
		}

		otherCollider, ok := ecs.GetComponent[components.Collider](w, otherH)
		if !ok || otherCollider.Phantom != nil { // Ignore other phantoms
			continue
		}

		otherTransform, ok := ecs.GetComponent[components.Transform](w, otherH)
		if !ok {
			continue
		}

		otherX, otherY := otherTransform.X, otherTransform.Y

		// Check overlap
		if rectsOverlap(phantom.WorldX, phantom.WorldY, float64(phantom.HalfWidth), float64(phantom.HalfHeight),
			otherX, otherY, float64(otherCollider.HalfWidth), float64(otherCollider.HalfHeight)) {

			result.HasCollision = true
			if id, ok := w.GetExternalID(otherH); ok {
				result.CollidedWith = append(result.CollidedWith, id)
			}
		}
	}
}

func (s *CollisionSystem) checkSweptCollision(w *ecs.World, chunk *core.Chunk, entityH types.Handle, transform components.Transform, collider components.Collider, result *components.CollisionResult) {
	startX, startY := transform.X, transform.Y
	targetX, targetY := transform.IntentX, transform.IntentY

	hw, hh := float64(collider.HalfWidth), float64(collider.HalfHeight)

	// Sliding loop
	for i := 0; i < 3; i++ {
		// Calculate delta for this iteration
		dx := targetX - startX
		dy := targetY - startY

		if math.Abs(dx) < 0.001 && math.Abs(dy) < 0.001 {
			break
		}

		// Broadphase box
		minX := int(math.Min(startX, targetX)-hw) - 16
		minY := int(math.Min(startY, targetY)-hh) - 16
		maxX := int(math.Max(startX, targetX)+hw) + 16
		maxY := int(math.Max(startY, targetY)+hh) + 16

		var candidates []types.Handle
		chunk.Spatial().QueryAABB(minX, minY, maxX, maxY, &candidates)

		earliestTime := 1.0
		hitNormalX, hitNormalY := 0.0, 0.0
		hitSomething := false
		var hitEntity types.EntityID

		for _, otherH := range candidates {
			if otherH == entityH {
				continue
			}

			otherCollider, ok := ecs.GetComponent[components.Collider](w, otherH)
			if !ok || otherCollider.Phantom != nil {
				continue
			}

			otherTransform, ok := ecs.GetComponent[components.Transform](w, otherH)
			if !ok {
				continue
			}

			// Minkowski Difference AABB
			// Expand other box by our size
			expandedMinX := otherTransform.X - float64(otherCollider.HalfWidth) - hw
			expandedMaxX := otherTransform.X + float64(otherCollider.HalfWidth) + hw
			expandedMinY := otherTransform.Y - float64(otherCollider.HalfHeight) - hh
			expandedMaxY := otherTransform.Y + float64(otherCollider.HalfHeight) + hh

			// Check ray intersection against expanded box
			t, nx, ny := rayAABB(startX, startY, dx, dy, expandedMinX, expandedMinY, expandedMaxX, expandedMaxY)

			if t >= 0 && t < earliestTime {
				earliestTime = t
				hitNormalX, hitNormalY = nx, ny
				hitSomething = true
				if id, ok := w.GetExternalID(otherH); ok {
					hitEntity = id
				}
			}
		}

		if hitSomething {
			result.HasCollision = true
			result.CollisionNormalX = hitNormalX
			result.CollisionNormalY = hitNormalY
			if hitEntity != 0 {
				result.CollidedWith = append(result.CollidedWith, hitEntity)
			}

			// Move to collision point (slightly before)
			epsilon := 0.001
			startX += dx * (earliestTime - epsilon)
			startY += dy * (earliestTime - epsilon)

			// Slide
			// Project remaining delta onto normal
			remainingT := 1.0 - earliestTime
			remainingDx := dx * remainingT
			remainingDy := dy * remainingT

			dot := remainingDx*hitNormalX + remainingDy*hitNormalY
			slideDx := remainingDx - dot*hitNormalX
			slideDy := remainingDy - dot*hitNormalY

			// New target is current pos + slide
			targetX = startX + slideDx
			targetY = startY + slideDy

		} else {
			// No collision, move to target
			startX = targetX
			startY = targetY
			break
		}
	}

	result.FinalX = startX
	result.FinalY = startY
}

func rectsOverlap(x1, y1, hw1, hh1, x2, y2, hw2, hh2 float64) bool {
	if math.Abs(x1-x2) > (hw1 + hw2) {
		return false
	}
	if math.Abs(y1-y2) > (hh1 + hh2) {
		return false
	}
	return true
}

func rayAABB(ox, oy, dx, dy, minX, minY, maxX, maxY float64) (float64, float64, float64) {
	tMin := 0.0
	tMax := 1.0

	nx, ny := 0.0, 0.0

	// X axis
	if math.Abs(dx) < 1e-9 {
		if ox < minX || ox > maxX {
			return -1, 0, 0
		}
	} else {
		ood := 1.0 / dx
		t1 := (minX - ox) * ood
		t2 := (maxX - ox) * ood

		if t1 > t2 {
			t1, t2 = t2, t1
		}

		if t1 > tMin {
			tMin = t1
			nx = -1.0
			if dx > 0 {
				nx = -1.0
			} else {
				nx = 1.0
			}
			ny = 0
		}

		if t2 < tMax {
			tMax = t2
		}

		if tMin > tMax {
			return -1, 0, 0
		}
	}

	// Y axis
	if math.Abs(dy) < 1e-9 {
		if oy < minY || oy > maxY {
			return -1, 0, 0
		}
	} else {
		ood := 1.0 / dy
		t1 := (minY - oy) * ood
		t2 := (maxY - oy) * ood

		if t1 > t2 {
			t1, t2 = t2, t1
		}

		if t1 > tMin {
			tMin = t1
			ny = -1.0
			nx = 0 // Override normal if Y axis is the limiting factor
			if dy > 0 {
				ny = -1.0
			} else {
				ny = 1.0
			}
		}

		if t2 < tMax {
			tMax = t2
		}

		if tMin > tMax {
			return -1, 0, 0
		}
	}

	return tMin, nx, ny
}
