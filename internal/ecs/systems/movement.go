package systems

import (
	"log"
	"math"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

const (
	defaultSpeed = 10.0
)

// MovementSystem updates entity positions based on velocity and movement targets
// Runs at priority 100 (after input processing, before collision)
type MovementSystem struct {
	ecs.BaseSystem
}

// NewMovementSystem creates a new movement system
func NewMovementSystem() *MovementSystem {
	return &MovementSystem{
		BaseSystem: ecs.NewBaseSystem("MovementSystem", 100),
	}
}

// Update processes all entities with Position and Velocity components
func (s *MovementSystem) Update(w *ecs.World, dt float64) {
	posStorage := ecs.GetOrCreateStorage[components.Position](w)
	velStorage := ecs.GetOrCreateStorage[components.Velocity](w)
	targetStorage := ecs.GetOrCreateStorage[components.MovementTarget](w)
	speedStorage := ecs.GetOrCreateStorage[components.Speed](w)
	colliderStorage := ecs.GetOrCreateStorage[components.Collider](w)

	// Use active lists if available (chunk-filtered), otherwise fallback to query
	var handles []ecs.Handle
	if activeLists := ecs.GetActiveLists(w); activeLists != nil && len(activeLists.Dynamic) > 0 {
		handles = activeLists.Dynamic
	} else {
		// Fallback: query all entities with Position + Velocity
		query := w.Query().
			With(components.PositionID).
			With(components.VelocityID)
		handles = query.Handles()
	}

	for _, h := range handles {
		pos := posStorage.GetPtr(h)
		vel := velStorage.GetPtr(h)
		if pos == nil || vel == nil {
			continue
		}

		// Check for movement target
		if target, ok := targetStorage.Get(h); ok {
			s.updateVelocityTowardsTarget(w, h, pos, vel, &target, speedStorage, dt)
		}

		// Apply velocity to position
		// IMPORTANT: Only apply simple movement if the entity DOES NOT have a collider.
		// If it has a collider, the CollisionSystem (priority 200) handles the movement via swept AABB.
		if !colliderStorage.Has(h) {
			pos.X += vel.X * dt
			pos.Y += vel.Y * dt
		}
	}
}

// updateVelocityTowardsTarget calculates velocity to move towards target
func (s *MovementSystem) updateVelocityTowardsTarget(w *ecs.World, h ecs.Handle, pos *components.Position, vel *components.Velocity, target *components.MovementTarget, speedStorage *ecs.ComponentStorage[components.Speed], dt float64) {
	dx := target.X - pos.X
	dy := target.Y - pos.Y
	dist := math.Sqrt(dx*dx + dy*dy)

	// Arrival threshold
	const arrivalThreshold = 0.5
	log.Printf("movement target: handle=%d target (%.2f %.2f) dx=%.2f dy=%.2f dist=%.2f", h, target.X, target.Y, dx, dy, dist)

	if dist < arrivalThreshold {
		// Arrived at target
		vel.X = 0
		vel.Y = 0
		ecs.RemoveComponent[components.MovementTarget](w, h)
		return
	}

	// Get desired speed (default to 1.0 if not set)
	desiredSpeed := defaultSpeed
	if sComp, ok := speedStorage.Get(h); ok {
		desiredSpeed = sComp.Value
	}

	// Clamp speed so that we don't move further than the target in this frame
	// maxSpeedThisFrame * dt <= dist  =>  maxSpeedThisFrame = dist / dt
	if dt > 0 {
		maxSpeedThisFrame := dist / dt
		if desiredSpeed > maxSpeedThisFrame {
			desiredSpeed = maxSpeedThisFrame
		}
	}

	// Normalize direction and apply (possibly clamped) speed
	invDist := 1.0 / dist
	vel.X = dx * invDist * desiredSpeed
	vel.Y = dy * invDist * desiredSpeed
}

// SetTarget sets a movement target for an entity
func SetTarget(w *ecs.World, h ecs.Handle, x, y float64) {
	ecs.AddComponent(w, h, components.MovementTarget{
		X: x,
		Y: y,
	})
}

// SetFollowTarget sets an entity to follow another entity
func SetFollowTarget(w *ecs.World, h ecs.Handle, targetHandle ecs.Handle, interact, attack bool) {
	ecs.AddComponent(w, h, components.MovementTarget{
		TargetHandle: targetHandle,
		Interact:     interact,
		Attack:       attack,
	})
}

// StopMovement removes velocity and movement target from an entity
func StopMovement(w *ecs.World, h ecs.Handle) {
	if vel := ecs.GetComponentPtr[components.Velocity](w, h); vel != nil {
		vel.X = 0
		vel.Y = 0
	}
	ecs.RemoveComponent[components.MovementTarget](w, h)
}
