# ECS Systems Documentation

## Overview

This document describes all ECS (Entity Component System) systems in the game engine. Systems are responsible for processing entities with specific components in a defined order based on priority.

## System Execution Order

Systems are executed in ascending order of priority (lower priority numbers run first). This ensures deterministic behavior and proper system dependencies.

## System Registry

| Priority | System Name           | Description                                                  | Dependencies                  | Notes                                   |
|----------|-----------------------|--------------------------------------------------------------|-------------------------------|-----------------------------------------|
| 100      | MovementSystem        | Updates entity movement based on Movement components         | Transform, Movement           | Sets MoveTag on real movement           |
| 200      | CollisionSystem       | Performs collision detection and resolution                  | Transform, MoveTag, Collider, ChunkRef | Uses optimized component storage access |
| 300      | TransformUpdateSystem | Applies final position updates and publishes movement events | Transform, MoveTag, CollisionResult    | Processes moved entities and removes MoveTag |
| 400      | ChunkSystem           | Manages chunk lifecycle and entity migration                 | ChunkRef                      | Handles entity chunk transitions        |

## System Details

### MovementSystem (Priority: 100)

**Purpose**: Processes entity movement intent and calculates velocity-based movement.

**Components Required**:

- `Transform` - Current position and movement intent
- `Movement` - Movement state, target, and speed

**Behavior**:

- Iterates over entities with Transform and Movement components
- Handles both point targets and entity targets
- Calculates velocity based on movement mode and speed
- Sets `Transform.IntentX/Y` for movement intent
- Adds `MoveTag` component when real movement occurs
- Clears movement target when destination is reached

**Performance Notes**:

- Uses PreparedQuery for efficient entity iteration
- Zero allocations during normal operation

### CollisionSystem (Priority: 200)

**Purpose**: Detects and resolves collisions between moving entities and static/dynamic objects.

**Components Required**:

- `Transform` - Current position and movement intent
- `MoveTag` - Marker for entities that moved this frame
- `ChunkRef` - Current chunk for spatial queries
- `Collider` - Collision dimensions and layers
- `CollisionResult` - Output for collision resolution

**Behavior**:

- Processes entities with MoveTag (moved this frame)
- Performs swept AABB collision detection
- Handles sliding along walls and obstacle avoidance
- Supports collision layers and masks
- Processes phantom colliders for build boundaries
- Updates `CollisionResult` with final position and collision data

**Performance Optimizations**:

- Pooled candidates buffer to avoid allocations
- Cached component storages for direct access
- Optimized spatial queries via chunk-based spatial hash

### TransformUpdateSystem (Priority: 300)

**Purpose**: Applies final position updates after collision resolution and publishes movement events.

**Components Required**:

- `Transform` - Current and final positions
- `MoveTag` - Marker for entities that moved this frame
- `CollisionResult` - Collision resolution data

**Behavior**:

- Processes entities with MoveTag using PreparedQuery
- Updates entity positions from collision results
- Publishes movement events to event bus
- Manages collision state for oscillation detection
- Removes MoveTag component after processing
- Clears temporary collision data for next frame

**Event Publishing**:

- Movement events for network synchronization
- Position updates for client-side prediction

### ChunkSystem (Priority: 400)

**Purpose**: Manages entity migration between chunks and chunk lifecycle.

**Components Required**:

- `ChunkRef` - Current and previous chunk references

**Behavior**:

- Detects when entities cross chunk boundaries
- Migrates entities between chunks
- Updates spatial hash positions
- Manages chunk loading/unloading based on entity presence

**Chunk Management**:

- AOI (Area of Interest) calculations
- Chunk state transitions
- Entity spatial indexing

### MoveTag Component

**Purpose**: Temporal marker for entities that moved during the current frame.

**Lifecycle**:

- Added by MovementSystem when real movement occurs
- Used by CollisionSystem and TransformUpdateSystem for efficient queries
- Removed by TransformUpdateSystem after processing
- Only exists for one frame, ensuring temporal accuracy

**Benefits**:

- Efficient queries for recently moved entities
- Temporal movement detection without persistent flags
- Performance optimization for movement-dependent systems
- Clean separation of movement intent from actual movement

## System Dependencies

```
MovementSystem (100)
    ↓ (adds MoveTag)
CollisionSystem (200) 
    ↓ (uses MoveTag)
TransformUpdateSystem (300)
    ↓ (removes MoveTag)
ChunkSystem (400)
```

## Performance Considerations

### Hot Path Optimizations

1. **PreparedQuery Caching**: All systems use PreparedQuery with cached archetype lists
2. **Component Storage Caching**: CollisionSystem caches component storages for direct access
3. **Buffer Pooling**: CollisionSystem uses pooled candidates buffer to avoid allocations
4. **Zero-Allocation Iteration**: Systems minimize allocations during hot path execution

### Memory Management

1. **MoveTag Lifecycle**: Temporary tags are created by MovementSystem and removed by TransformUpdateSystem
2. **Spatial Hash**: Chunk-based spatial indexing for efficient collision queries
3. **Component Storage**: Columnar storage for cache-friendly access patterns
4. **PreparedQuery Caching**: All systems use cached archetype lists for zero-allocation iteration

## Adding New Systems

When adding a new system:

1. **Choose Priority**: Place it appropriately in the execution order
2. **Define Dependencies**: Ensure required components are available
3. **Use PreparedQuery**: For efficient entity iteration
4. **Update Registry**: Add to the system registry table above
5. **Register in Shard**: Add to system initialization in shard.go

### Example System Registration

```go
// In shard.go
s.world.AddSystem(systems.NewYourSystem(s.world, logger))
```

## System Best Practices

1. **Single Responsibility**: Each system should have one clear purpose
2. **Component Isolation**: Avoid modifying components outside system scope
3. **Deterministic Order**: System dependencies should be clear via priority
4. **Performance First**: Use PreparedQuery and minimize allocations
5. **Event-Driven**: Use event bus for cross-system communication when appropriate

## Debugging and Monitoring

- System execution can be monitored via logging
- Component counts can be tracked via ECS world queries
- Performance profiling should focus on hot path systems (Movement, Collision)
- Event bus provides visibility into system interactions
