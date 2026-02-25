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

| Priority | System Name           | Description                                                  | Dependencies                       | Notes                                                    |
|----------|-----------------------|--------------------------------------------------------------|------------------------------------|----------------------------------------------------------|
| 0        | NetworkCommandSystem  | Drains player/server inboxes and routes network commands     | PlayerCommandInbox, ServerJobInbox | Also handles login snapshot jobs + craft/window commands |
| 0        | ResetSystem           | Clears temporary data structures at frame start              | MovedEntities buffer               | Runs first, resets arrays                                |
| 50       | CharacterSaveSystem   | Periodically saves character data to database                | Transform, Character               | Batch saves character positions/stats                    |
| 100      | MovementSystem        | Updates entity movement based on Movement components         | Transform, Movement                | Appends to MovedEntities buffer                          |
| 200      | CollisionSystem       | Performs collision detection and resolution                  | Transform, Collider, ChunkRef      | Reads from MovedEntities buffer                          |
| 250      | ExpireDetachedSystem  | Handles delayed despawn of detached entities                 | Detached, Character                | Saves character data before despawn                      |
| 300      | TransformUpdateSystem | Applies final position updates and publishes movement events | Transform, CollisionResult         | Processes moved entities                                 |
| 320      | AutoInteractSystem    | Executes pending interactions when player reaches target     | Transform, PendingInteraction      | Auto-pickup dropped items on arrival                     |
| 350      | VisionSystem          | Calculates entity visibility and manages observer state      | Vision, Transform, ChunkRef        | Updates VisibilityState, publishes events                |
| 355      | BehaviorTickSystem    | Processes scheduled behavior ticks with global budget        | BehaviorTickSchedule, TimeState    | Dispatches to behavior scheduled-tick capability         |
| 360      | ObjectBehaviorSystem  | Recomputes object behavior flags/state/appearance            | ObjectBehaviorDirtyQueue           | Dirty-queue driven, budget-limited                       |
| 400      | ChunkSystem           | Manages chunk lifecycle and entity migration                 | ChunkRef                           | Handles entity chunk transitions                         |

## System Details

### NetworkCommandSystem (Priority: 0)

**Build integration notes**:
- Routes build commands (`BuildStart`, `BuildProgress`, `BuildTakeBack`) to `BuildService` on the ECS thread.
- After successful inventory move into a build-site (`dst.kind = INVENTORY_KIND_BUILD`), trigger immediate build-state snapshot refresh; hand inventory updates still flow through inventory result/update packets.
- Keep build-state sends on the ECS thread to preserve deterministic ordering with inventory/build mutations.

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
- Uses time-based updates (`const.VisionUpdateInterval`) to throttle visibility calculations
- Performs spatial queries using neighboring chunks and `QueryRadius`
- Calculates visibility based on distance, vision power, and target stealth
- Maintains `VisibilityState` with `VisibleByObserver` and `ObserversByVisibleTarget` maps
- Stores EntityID in Known maps for reliable despawn events
- Publishes `ObjectSpawn` and `ObjectDespawn` events for visibility changes
- Automatically registers new Vision entities during entity spawning
- Supports `ForceUpdateForObserver(...)` for immediate recompute after critical interactions (pickup/chop spawn/transform)

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
    ↓ (positions finalized)
AutoInteractSystem (320)
    ↓ (pickup may despawn entities)
VisionSystem (350)
    ↓ (dispatches due scheduled behavior ticks)
BehaviorTickSystem (355)
    ↓ (updates VisibilityState)
ObjectBehaviorSystem (360)
    ↓ (applies behavior flags/resource only for dirty objects)
ChunkSystem (400)
```

## ObjectBehaviorSystem (Priority: 360)

**Purpose**: Applies runtime behavior capability from unified behavior registry and updates:

- `ObjectInternalState.Flags`
- `ObjectInternalState.State`
- `Appearance.Resource` (with appearance-changed event)

**Registry/Contract**:

- Behavior contracts are defined in `internal/game/behaviors/contracts/behavior.go`.
- Runtime implementations live in `internal/game/behaviors`.
- System wiring receives unified `contracts.BehaviorRegistry` through `ObjectBehaviorConfig.BehaviorRegistry`.
- Registry is fail-fast validated on startup (invalid action/cyclic capabilities do not boot).

**Execution Model (important)**:

- No full-world scan in normal operation.
- Processes only handles from `ecs.ObjectBehaviorDirtyQueue`.
- Per-tick budget is controlled by config: `game.object_behavior_budget_per_tick` (default `512`).
- Debug-only fallback sweep can be enabled (dev environment) to catch missed dirty marks.

**Dirty Marking Contract**:

- Systems that mutate behavior-relevant data must call `ecs.MarkObjectBehaviorDirty(...)`.
- Current primary producer is inventory flow for object root containers.
- Chunk activation runs behavior lifecycle init for restored objects and then forces behavior recompute for all behavior-bearing objects (no lazy init).

## BehaviorTickSystem (Priority: 355)

**Purpose**: Executes due entries from `BehaviorTickSchedule` and delegates to behavior `OnScheduledTick`.

**Execution Model**:

- Reads current tick from `TimeState.Tick`.
- Pops due keys up to the global per-tick budget.
- Resolves handle + behavior at execution time (despawned/missing entries are skipped).
- Calls scheduled-tick behavior capability.
- Marks object dirty only when `BehaviorTickResult.StateChanged=true`.

**Safety/Performance**:

- Replaces world-wide polling for "live" behavior objects.
- Works with scheduler lazy dedupe (stale heap entries are ignored).
- Depends on `World.Despawn(...)` cancel-all cleanup for entity tick keys.

## Context Interaction Flow (RMB)

Context interactions are now behavior-driven and server-authoritative:

1. Client sends `Interact(entity_id)` on RMB.
2. `NetworkCommandSystem` computes actions via `ContextActionResolver` using behavior order from object `def`.
3. Branching:
    - `0` actions: silent ignore.
    - `1` action: either auto-select/start pending execution OR open `S2C_ContextMenu` when object def has `contextMenuEvenForOneItem=true`.
    - `2+` actions: send `S2C_ContextMenu`.
4. Execution is triggered only by `LinkCreated` (not direct distance checks in command handler).

### Cyclic Action UI Events

- During active cycle server sends `S2C_CyclicActionProgress`.
- On terminal state server sends `S2C_CyclicActionFinished`:
    - `COMPLETED` (no `reason_code`)
    - `CANCELED` (optional `reason_code`)

### Pending Action Rules

- Pending state is `components.PendingContextAction`.
- Cleanup paths:
    - timeout (`game.interaction_pending_timeout`, default `15s`)
    - movement stop without link
    - target/player despawn
    - new movement command (`MoveTo`/`MoveToEntity`)
    - `LinkBroken`

### Duplicate Action IDs

- Duplicate `action_id` between behaviors uses **first wins**.
- Runtime observability:
    - `WARN` log
    - metric `context_action_duplicate_total{entity_def,action_id,winner_behavior,loser_behavior}`

## Login Snapshot Jobs

`NetworkCommandSystem` also processes internal server jobs used by login/reattach bootstrap:

- `JobSendInventorySnapshot` → sends full inventory snapshot.
- `JobSendCharacterProfileSnapshot` → sends `S2C_CharacterProfile` snapshot.
- `JobSendPlayerStatsSnapshot` → sends `S2C_PlayerStats` snapshot.
- `JobSendCraftListSnapshot` → triggers craft-list snapshot send (`S2C_CraftList`), which is server-gated by opened `"craft"` window state.

These jobs run on ECS tick thread to avoid concurrent world/component access from network goroutines.

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

Systems access World resources via the typed generic API `ecs.GetResource[T](w)`:

```go
func (s *YourSystem) Update(w *ecs.World, dt float64) {
// Access resources via typed generic API (returns *T, panics if not initialised)
visState := ecs.GetResource[ecs.VisibilityState](w)
movedEntities := ecs.GetResource[ecs.MovedEntities](w)
characterEntities := ecs.GetResource[ecs.CharacterEntities](w)
detachedEntities := ecs.GetResource[ecs.DetachedEntities](w)
refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)

// Get world layer
layer := w.Layer
}
```

Resource API functions:

- `ecs.InitResource[T](w, value)` — store initial value (world constructor)
- `ecs.GetResource[T](w) *T` — get pointer, panics if missing
- `ecs.TryGetResource[T](w) (*T, bool)` — get pointer with existence check
- `ecs.SetResource[T](w, value)` — replace value
- `ecs.HasResource[T](w) bool` — check existence

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
