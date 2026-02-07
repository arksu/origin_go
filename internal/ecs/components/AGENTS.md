# Component Registration Guide

## Overview

All ECS components must be registered with **explicit, stable IDs** during package initialization. This ensures:

- **Deterministic behavior** across builds and processes
- **Compatibility** with persistence and replication systems
- **Stable component masks** for serialization

## Component ID Allocation

Component IDs are `uint8` values from 0-63 (64 total slots).

### Reserved IDs

- **ID 0**: `ExternalID` (mandatory component for all entities)
- **IDs 1-9**: Reserved for core system components
- **IDs 10-63**: Available for game-specific components

## How to Define a Component

### 1. Define the Component Type

```go
package components

import "origin/internal/ecs"

// Transform represents an entity's position and orientation in 2D space
type Transform struct {
	// исходные координаты на начало тика
	X float64
	Y float64
	// то куда передвигаемся на текущем тике
	IntentX float64
	IntentY float64
	// направление вращения в градусах
	Direction float64
}

```

### 2. Register with Explicit ID

```go
package components

import "origin/internal/ecs"

const (
	TransformComponentID       = 10
	ChunkRefComponentID        = 11
	MovementComponentID        = 12
	EntityInfoComponentID      = 13
	ColliderComponentID        = 14
	CollisionResultComponentID = 15
)

func init() {
	ecs.RegisterComponent[Transform](TransformComponentID)

```

### 3. Usage in Game Code

```go
package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

func SpawnPlayer(world *ecs.World, entityID ecs.EntityID) ecs.Handle {
	h := world.Spawn(entityID)

	// Add components
	ecs.AddComponent(world, h, components.Position{X: 100, Y: 200, Z: 0})
	ecs.AddComponent(world, h, components.Velocity{DX: 0, DY: 0, DZ: 0})
	ecs.AddComponent(world, h, components.Health{Current: 100, Maximum: 100})

	return h
}

func UpdateMovement(world *ecs.World, h ecs.Handle, deltaTime float64) {
	// Get position and velocity
	pos, hasPos := ecs.GetComponent[components.Position](world, h)
	vel, hasVel := ecs.GetComponent[components.Velocity](world, h)

	if !hasPos || !hasVel {
		return
	}

	// Mutate position
	ecs.WithComponent(world, h, func(p *components.Position) {
		p.X += vel.DX * deltaTime
		p.Y += vel.DY * deltaTime
		p.Z += vel.DZ * deltaTime
	})
}

func TakeDamage(world *ecs.World, h ecs.Handle, damage int32) bool {
	// Conditional mutation with return value
	return ecs.MutateComponent(world, h, func(health *components.Health) bool {
		if health.Current <= 0 {
			return false // Already dead
		}
		health.Current -= damage
		if health.Current < 0 {
			health.Current = 0
		}
		return true
	})
}
```

## Important Rules

1. **Never reuse IDs**: Once an ID is assigned to a component type, it must never be changed or reassigned
2. **Document ID allocation**: Keep a central registry of all component IDs
3. **Register during init()**: All components must be registered before any game logic runs
4. **Panic on conflicts**: The system will panic if you try to register duplicate IDs or types
5. **Sequential allocation**: Assign IDs sequentially to avoid gaps and confusion

## Component ID Registry

Maintain this list as you add new components:

| ID | Component                     | Description                                |
|----|-------------------------------|--------------------------------------------|
| 0  | ExternalID                    | Maps Handle to global EntityID (mandatory) |
| 10 | TransformComponentID          | Entity world position                      |
| 11 | ChunkRefComponentID           | Reference to current chunk                 |
| 12 | MovementComponentID           | Entity movement velocity                   |
| 13 | EntityInfoComponentID         | Base Entity info (isStatic, region, layer) |
| 14 | ColliderComponentID           | Collider for collision system              |
| 15 | CollisionResultComponentID    | Result of collision calculations           |
| 16 | VisionComponentID             | Entity vision radius and power             |
| 17 | StealthComponentID            | Entity stealth value                       |
| 18 | AppearanceComponentID         | Entity visual appearance (name)            |
| 19 | InventoryOwnerComponentID     | Links entity to its inventory containers   |
| 20 | InventoryContainerComponentID | Inventory container (grid/hand/equip/drop) |
| 21 | DroppedItemComponentID        | Marks entity as a dropped item in world    |
| 22 | PendingInteractionComponentID | Pending auto-interaction intent (pickup)   |

## Migration from Auto-Assignment

If you have existing code using auto-assigned IDs:

1. Determine the current ID assignment order
2. Assign explicit IDs matching the current order
3. Add `RegisterComponent` calls in `init()`
4. Test thoroughly to ensure masks are preserved

## Error Messages

- `"component ID exceeds maximum (63)"` - ID must be 0-63
- `"component ID X already registered for type Y"` - ID conflict, choose different ID
- `"component type X already registered with ID Y"` - Type already registered
- `"component type X not registered"` - Missing `RegisterComponent` call
