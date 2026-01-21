# ECS Systems Documentation

## Overview

This document describes all ECS (Entity Component System) systems in the game engine. Systems are responsible for processing entities with specific components in a defined order based on priority.

## World Architecture

The ECS World is the central component that manages entities, components, and systems for each shard. Each World instance has:

- **Event Bus**: For publishing events (despawn, spawn, movement, etc.)
- **Layer**: Shard layer identifier for multi-layer deployments
- **Visibility State**: Manages observer-target visibility relationships
- **Component Storage**: Type-erased storage for all component types
- **CharacterEntities**: Tracks player characters and their save state
- **DetachedEntities**: Tracks players who disconnected but entity remains in world

## System Execution Order

Systems are executed in ascending order of priority (lower priority numbers run first). This ensures deterministic behavior and proper system dependencies.

## System Registry

| Priority | System Name           | Description                                                  | Dependencies                  | Notes                                     |
|----------|-----------------------|--------------------------------------------------------------|-------------------------------|-------------------------------------------|
| 0        | ResetSystem           | Clears temporary data structures at frame start              | MovedEntities buffer          | Runs first, resets arrays                 |
| 50       | CharacterSaveSystem   | Periodically saves character data to database                | Transform, Character          | Batch saves character positions/stats     |
| 100      | MovementSystem        | Updates entity movement based on Movement components         | Transform, Movement           | Appends to MovedEntities buffer           |
| 200      | CollisionSystem       | Performs collision detection and resolution                  | Transform, Collider, ChunkRef | Reads from MovedEntities buffer           |
| 250      | ExpireDetachedSystem  | Handles delayed despawn of detached entities                 | Detached, Character           | Saves character data before despawn       |
| 300      | TransformUpdateSystem | Applies final position updates and publishes movement events | Transform, CollisionResult    | Processes moved entities                  |
| 350      | VisionSystem          | Calculates entity visibility and manages observer state      | Vision, Transform, ChunkRef   | Updates VisibilityState, publishes events |
| 400      | ChunkSystem           | Manages chunk lifecycle and entity migration                 | ChunkRef                      | Handles entity chunk transitions          |

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
- Clears movement target when destination is reached

**Performance Notes**:

- Appends to pre-allocated MovedEntities buffer
- Zero allocations during normal operation
- Direct buffer append for moved entities tracking

### CollisionSystem (Priority: 200)

**Purpose**: Detects and resolves collisions between moving entities and static/dynamic objects.

**Components Required**:

- `Transform` - Current position and movement intent
- `ChunkRef` - Current chunk for spatial queries
- `Collider` - Collision dimensions and layers
- `CollisionResult` - Output for collision resolution

**Behavior**:

- Reads from MovedEntities buffer (populated by MovementSystem)
- Performs swept AABB collision detection
- Handles sliding along walls and obstacle avoidance
- Supports collision layers and masks
- Processes phantom colliders for build boundaries
- Updates `CollisionResult` with final position and collision data

**Performance Optimizations**:

- Direct buffer iteration instead of PreparedQuery
- Pooled candidates buffer to avoid allocations
- Cached component storages for direct access
- Optimized spatial queries via chunk-based spatial hash

### TransformUpdateSystem (Priority: 300)

**Purpose**: Applies final position updates after collision resolution and publishes movement events.

**Components Required**:

- `Transform` - Current and final positions
- `CollisionResult` - Collision resolution data

**Behavior**:

- Reads from MovedEntities buffer (populated by MovementSystem)
- Updates entity positions from collision results
- Publishes movement events to event bus
- Manages collision state for oscillation detection
- Clears temporary collision data for next frame

**Event Publishing**:

- Movement events for network synchronization
- Position updates for client-side prediction

### VisionSystem (Priority: 350)

**Purpose**: Calculates entity visibility and manages observer state for vision-based gameplay mechanics.

**Components Required**:

- `Vision` - Observer's vision capabilities (radius and power)
- `Transform` - Current position for spatial queries
- `ChunkRef` - Current chunk for spatial hash access

**Behavior**:

- Iterates over entities with Vision components (observers)
- Uses time-based updates (3-second intervals) to throttle visibility calculations
- Performs spatial queries using neighboring chunks and `QueryRadius`
- Calculates visibility based on distance, vision power, and target stealth
- Maintains `VisibilityState` with `VisibleByObserver` and `ObserversByVisibleTarget` maps
- Stores EntityID in Known maps for reliable despawn events
- Publishes `ObjectSpawn` and `ObjectDespawn` events for visibility changes
- Automatically registers new Vision entities during entity spawning

**Visibility State Management**:

The VisionSystem maintains a bidirectional visibility relationship:

```go
type ObserverVisibility struct {
Known map[types.Handle]types.EntityID // Handle -> EntityID mapping
NextUpdateTime time.Time
}

type VisibilityState struct {
VisibleByObserver map[types.Handle]ObserverVisibility
ObserversByVisibleTarget map[types.Handle]map[types.Handle]struct{}
Mu sync.RWMutex
}
```

**Event Publishing**:

- `ObjectSpawnEvent` - When entities enter visibility range
- `ObjectDespawnEvent` - When entities leave visibility range

**Performance Optimizations**:

- Time-throttled updates (3-second intervals per observer)
- Pre-allocated buffers for candidates and visible sets
- Cached component storages for direct access
- Spatial queries limited to vision radius
- Zero-allocation iteration patterns
- EntityID caching prevents despawn failures

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

### ResetSystem (Priority: 0)

**Purpose**: Clears temporary data structures at the beginning of each frame.

**Components Required**:

- `MovedEntities` buffer - Shared tracking of moved entities

**Behavior**:

- Clears `Handles` array (sets length to 0, keeps capacity)
- Clears `IntentX` array (sets length to 0, keeps capacity)
- Clears `IntentY` array (sets length to 0, keeps capacity)
- Runs first in system execution order

**Performance Notes**:

- Zero allocations - reuses pre-allocated arrays
- O(1) operation - only resets slice lengths

## System Dependencies

```
ResetSystem (0)
    ↓ (clears buffers)
CharacterSaveSystem (50)
    ↓ (periodic saves)
MovementSystem (100)
    ↓ (appends to MovedEntities)
CollisionSystem (200) 
    ↓ (reads from MovedEntities)
ExpireDetachedSystem (250)
    ↓ (saves before despawn)
TransformUpdateSystem (300)
VisionSystem (350)
    ↓ (updates VisibilityState)
ChunkSystem (400)
```

## Performance Considerations

### Hot Path Optimizations

1. **MovedEntities Buffer**: Direct array iteration for moved entities (no query overhead)
2. **Component Storage Caching**: CollisionSystem and VisionSystem cache component storages for direct access
3. **Buffer Pooling**: CollisionSystem and VisionSystem use pooled candidates buffer to avoid allocations
4. **Zero-Allocation Iteration**: Systems minimize allocations during hot path execution
5. **Pre-allocated Arrays**: MovedEntities arrays allocated once with capacity 256
6. **Time-Throttled Updates**: VisionSystem uses 3-second intervals per observer to reduce computational load
7. **Batch Database Operations**: CharacterSaveSystem uses PostgreSQL unnest for efficient bulk updates
8. **Async Processing**: CharacterSaveSystem worker pool prevents database operations from blocking game loop
9. **Context Isolation**: Separate contexts for shutdown operations prevent cancellation during critical saves

### Memory Management

1. **MovedEntities Buffer**: Pre-allocated arrays (capacity 256) reused each frame
2. **Spatial Hash**: Chunk-based spatial indexing for efficient collision queries
3. **Component Storage**: Columnar storage for cache-friendly access patterns
4. **Array Reuse**: ResetSystem clears arrays without deallocating, maintaining capacity

## Adding New Systems

When adding a new system:

1. **Choose Priority**: Place it appropriately in the execution order (0-400 reserved)
2. **Define Dependencies**: Ensure required components are available
3. **Data Access Pattern**: Use MovedEntities buffer for moved entities, PreparedQuery for other queries
4. **Event Bus Access**: Systems can access event bus through World for event publishing
5. **Update Registry**: Add to the system registry table above
6. **Register in Shard**: Add to system initialization in shard.go

### Example System Registration

```go
// In shard.go
s.world.AddSystem(systems.NewYourSystem(s.world, s.eventBus, logger))
```

### System Constructor Pattern

```go
func NewYourSystem(world *ecs.World, eventBus *eventbus.EventBus, logger *zap.Logger) *YourSystem {
return &YourSystem{
BaseSystem: ecs.NewBaseSystem("YourSystem", 250), // Between Collision and Transform
world:      world,
eventBus:   eventBus,
logger:     logger,
}
}
```

### Accessing World Resources

Systems can access World resources:

```go
func (s *YourSystem) Update(world *ecs.World, dt float64) {
// Access visibility state
visState := world.VisibilityState()

// Access moved entities buffer
movedEntities := world.MovedEntities()

// Access character entities
characterEntities := world.CharacterEntities()

// Access detached entities
detachedEntities := world.DetachedEntities()

// Get world layer
layer := world.Layer

// Publish events via world's event bus
if world.eventBus != nil {
world.eventBus.PublishAsync(yourEvent, eventbus.PriorityMedium)
}
}
```

## System Best Practices

1. **Single Responsibility**: Each system should have one clear purpose
2. **Component Isolation**: Avoid modifying components outside system scope
3. **Deterministic Order**: System dependencies should be clear via priority
4. **Performance First**: Use MovedEntities buffer for moved entities, PreparedQuery for other queries
5. **Event-Driven**: Use event bus for cross-system communication when appropriate
6. **World Integration**: Leverage World's event bus and layer information
7. **Visibility Awareness**: Consider visibility state when modifying entities
8. **Despawn Safety**: Rely on World.Despawn() for automatic cleanup

## Debugging and Monitoring

- System execution can be monitored via logging
- Component counts can be tracked via ECS world queries
- Performance profiling should focus on hot path systems (Movement, Collision)
- Event bus provides visibility into system interactions
- Visibility state can be inspected for debugging observer relationships
- Despawn events can be monitored for proper cleanup
