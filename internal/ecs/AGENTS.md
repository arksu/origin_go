# ECS Agents (Systems) Guide

High-performance archetype-based ECS implementation for MMO game servers with focus on agent-based systems architecture.

## Overview

This guide covers the ECS (Entity Component System) implementation with emphasis on building efficient game systems (agents) that process entities in a deterministic, high-performance manner.

## Key Features

### Generational Handles

**Problem**: Handle reuse creates "stale handle" bugs - a classic MMO issue where:
1. Entity A dies, handle is freed
2. Handle is reused for new Entity B
3. Stale references (in spatial grids, events, queues) now point to wrong entity

**Solution**: Generational handles pack `(index: uint32, generation: uint32)` into `uint64`:
- Lower 32 bits = index (for sparse array lookup)
- Upper 32 bits = generation (incremented on each reuse)
- All component access validates generation before returning data
- Stale handles fail validation and return false/nil

```go
// Example: Stale handle protection
h1 := world.Spawn(EntityID(1))
spatialGrid.Add(h1, x, y)  // h1 stored in spatial grid

world.Despawn(h1)           // h1 becomes stale

h2 := world.Spawn(EntityID(2))  // Reuses same index, new generation

// Critical: h1 in spatial grid won't access h2's data
if world.Alive(h1) {  // Returns false - generation mismatch
    // Never executed
}

comp, ok := GetComponent[Position](world, h1)  // ok = false
```

### System (Agent) Architecture

**Systems** are the primary way to process entities in the ECS. Each System has:

- **Priority**: Determines execution order (lower numbers run first)
- **Components**: Required components for entities to process
- **Update Method**: Called each tick with delta time
- **Zero-Allocation Design**: Optimized for hot path performance


### Performance Optimizations for Systems

#### Zero-Copy Iteration

**ForEach**: Iterates without allocations
- `Archetype.ForEachHandle()` iterates internal slice directly
- No handle copying, no temporary slices
- **0 allocs/op** for PreparedQuery

**Caller-Managed Buffers**: `HandlesInto(dst []Handle)` for zero allocations
- Caller manages buffer lifecycle - reuse across ticks
- **0 allocs/op** with preallocated buffer
- **6.6x faster** than old pooled approach


#### PreparedQuery for Repeated Operations

**Auto-Refresh**: Caches archetype list and automatically updates when new archetypes created
- Eliminates archetype scanning on every tick
- Prevents silent bugs from missing entities in new archetypes
- ~1 ns overhead for version check

```go
type AIAgent struct {
    ecs.BaseSystem
    movementQuery *ecs.PreparedQuery
    combatQuery   *ecs.PreparedQuery
}

func (a *AIAgent) Update(w *ecs.World, dt float64) {
    // Initialize queries once
    if a.movementQuery == nil {
        a.movementQuery = w.Query().With(PositionID).With(VelocityID).Prepare()
        a.combatQuery = w.Query().With(PositionID).With(CombatStatsID).Prepare()
    }
    
    // Process moving entities (0 allocs per tick)
    a.movementQuery.ForEach(func(h ecs.Handle) {
        // AI movement logic...
    })
    
    // Process combat entities (0 allocs per tick)
    a.combatQuery.ForEach(func(h ecs.Handle) {
        // Combat AI logic...
    })
}
```

### Agent Communication Patterns

#### Event-Driven Communication

Agents communicate through the World's event bus:

```go
type TransformAgent struct {
    ecs.BaseSystem
    eventBus *eventbus.EventBus
}

func (a *TransformAgent) Update(w *ecs.World, dt float64) {
    // Process moved entities
    movedEntities := w.MovedEntities()
    
    for i := 0; i < movedEntities.Count; i++ {
        h := movedEntities.Handles[i]
        
        // Get new position
        newPos := getNewPosition(w, h)
        
        // Publish movement event
        a.eventBus.PublishAsync(
            &MovementEvent{
                EntityID: getEntityID(w, h),
                Position: newPos,
            },
            eventbus.PriorityMedium,
        )
    }
}
```

#### Direct Component Access

For high-frequency communication between agents:

```go
// MovementAgent writes intent
ecs.MutateComponent[Transform](w, h, func(t *Transform) bool {
    t.IntentX = targetX
    t.IntentY = targetY
    return true
})

// CollisionAgent reads intent
ecs.GetComponent[Transform](w, h) // Returns current position with intent
```

### System Best Practices

#### 1. Single Responsibility

Each System should have one clear purpose:

```go
// Good: Single responsibility
type MovementAgent struct { /* Only handles movement */ }
type CollisionAgent struct { /* Only handles collision */ }

// Bad: Multiple responsibilities
type PhysicsAgent struct { /* Handles movement + collision + physics */ }
```

#### 2. Component Isolation

Only modify components that belong to your agent's domain:

```go
func (a *MovementAgent) Update(w *ecs.World, dt float64) {
    a.query.ForEach(func(h ecs.Handle) {
        // ✅ OK: Modify Movement and Transform
        ecs.MutateComponent[Movement](w, h, updateMovement)
        ecs.MutateComponent[Transform](w, h, updateIntent)
        
        // ❌ BAD: Modify unrelated components
        // ecs.MutateComponent[Health](w, h, damage) // Wrong agent
    })
}
```

#### 3. Priority-Based Dependencies

Use priority to ensure correct execution order:

```go
// Movement (100) → Collision (200) → Transform (300)
// Each agent depends on the previous one's output
```

#### 4. Memory Efficiency

Reuse buffers and avoid allocations:

```go
type EfficientAgent struct {
    ecs.BaseSystem
    query      *ecs.PreparedQuery
    buffer     []ecs.Handle  // Reused across ticks
    tempVec3   [100]Vec3     // Stack allocation for temp data
}

func (a *EfficientAgent) Update(w *ecs.World, dt float64) {
    a.buffer = a.buffer[:0]  // Reset, keep capacity
    a.buffer = a.query.HandlesInto(a.buffer)
    
    for i, h := range a.buffer {
        // Use stack-allocated temp array
        pos := &a.tempVec3[i%100]
        // Process...
    }
}
```

### System Testing

#### Unit Testing

Use `NewWorldForTesting()` for isolated agent tests:

```go
func TestMovementAgent(t *testing.T) {
    world := ecs.NewWorldForTesting()
    agent := agents.NewMovementAgent()
    
    // Setup test entities
    h1 := world.Spawn(EntityID(1))
    ecs.AddComponent(world, h1, Position{X: 0, Y: 0})
    ecs.AddComponent(world, h1, Velocity{X: 1, Y: 0})
    
    // Run agent
    agent.Update(world, 1.0)
    
    // Verify results
    pos, _ := ecs.GetComponent[Position](world, h1)
    assert.Equal(t, 1.0, pos.X)
}
```

#### Integration Testing

Test agent interactions with full World:

```go
func TestAgentIntegration(t *testing.T) {
    eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
    world := ecs.NewWorldWithCapacity(1000, eventBus, 0)
    
    // Register all agents
    world.AddAgent(agents.NewMovementAgent())
    world.AddAgent(agents.NewCollisionAgent())
    world.AddAgent(agents.NewTransformAgent())
    
    // Setup complex scenario
    // ... create entities with multiple components
    
    // Run full tick
    world.Update(0.016) // 60 FPS tick
    
    // Verify cross-agent interactions
    // ... check movement, collision, transforms
}
```

### Advanced Agent Patterns

#### Stateful Agents

Agents that maintain state across ticks:

```go
type PathfindingAgent struct {
    ecs.BaseSystem
    pathCache map[ecs.Handle]*Path  // Persistent cache
    lastUpdate float64              // Time-based throttling
}

func (a *PathfindingAgent) Update(w *ecs.World, dt float64) {
    a.lastUpdate += dt
    
    // Throttle expensive operations
    if a.lastUpdate < 0.1 { // 10 Hz update
        return
    }
    a.lastUpdate = 0
    
    // Process pathfinding...
}
```

#### Conditional Agents

Agents that only run under certain conditions:

```go
type VisibilityAgent struct {
    ecs.BaseSystem
    lastUpdateTime map[ecs.Handle]time.Time
}

func (a *VisibilityAgent) Update(w *ecs.World, dt float64) {
    // Only update observers that haven't been updated recently
    now := time.Now()
    
    w.Query().With(VisionID).ForEach(func(h ecs.Handle) {
        lastUpdate, exists := a.lastUpdateTime[h]
        if !exists || now.Sub(lastUpdate) > 3*time.Second {
            // Update visibility for this observer
            a.updateObserverVisibility(w, h)
            a.lastUpdateTime[h] = now
        }
    })
}
```

### Performance Characteristics

#### Agent Execution Performance

```
Operation                    Time/op     Allocs/op
Agent Registration           45 ns       0
Agent Update (empty)         12 ns       0
Agent Update (10k entities)  115 µs     0
Agent Update (100k entities) 1.15 ms    0
```

#### Memory Efficiency

- **Per-agent overhead**: ~64 bytes (system struct + query)
- **Query caching**: Minimal memory, massive performance gains
- **Buffer reuse**: Zero allocations per tick
- **Component access**: O(1) with generation validation

#### Scalability

- **Linear scaling** with entity count
- **No lock contention** between agents
- **Predictable tick times** regardless of entity count
- **Suitable for 10k+ entities per agent**

### Migration from Old Systems

When converting existing systems to agents:

```go
// Old system approach
type OldSystem struct {
    world *ecs.World
}

func (s *OldSystem) Update(dt float64) {
    // Manual iteration with allocations
    handles := s.world.Query().With(PositionID).Handles()
    for _, h := range handles {
        // Process...
    }
}

// New agent approach
type NewAgent struct {
    ecs.BaseSystem
    query *ecs.PreparedQuery
}

func (a *NewAgent) Update(w *ecs.World, dt float64) {
    if a.query == nil {
        a.query = w.Query().With(PositionID).Prepare()
    }
    
    // Zero allocations, better performance
    a.query.ForEach(func(h ecs.Handle) {
        // Process...
    })
}
```

## Conclusion

The ECS agent architecture provides:

- **High Performance**: Zero allocations on hot paths
- **Deterministic Order**: Priority-based execution
- **Scalability**: Linear performance scaling
- **Maintainability**: Clear separation of concerns
- **Testability**: Easy unit and integration testing

By following these patterns and best practices, you can build efficient, scalable game systems that handle thousands of entities with predictable sub-millisecond tick times.

## Runtime Behavior Recompute

For object runtime behaviors (flags/state/appearance), avoid full-world polling every tick.

- Use resource `ObjectBehaviorDirtyQueue` as the primary invalidation channel.
- Mark dirty explicitly from mutation points:
  - `ecs.MarkObjectBehaviorDirty(world, handle)`
  - `ecs.MarkObjectBehaviorDirtyByEntityID(world, entityID)`
- `ObjectBehaviorSystem` consumes this queue with a per-tick budget (`game.object_behavior_budget_per_tick`).
- Debug fallback sweep is allowed only in non-production env for safety checks.

This pattern is mandatory for behavior-heavy worlds (e.g. thousands of containers/trees).

## Scheduled Behavior Ticks

For long-lived behavior state updates (tree growth, machines, etc.), use the scheduler resources instead of scanning all entities.

- `BehaviorTickSchedule` is the authoritative queue (`entity_id + behavior_key -> due_tick`) with heap-backed due ordering.
- `ScheduleBehaviorTick(...)` replaces existing due tick for the same key; stale heap entries are ignored by sequence check.
- `CancelBehaviorTick(...)` and `CancelBehaviorTicksByEntityID(...)` must be used on terminal states/despawn.
- `World.Despawn(...)` already calls `CancelBehaviorTicksByEntityID(...)`; keep this invariant intact.
- `BehaviorTickPolicy` controls:
  - global processing budget per tick,
  - per-object restore catch-up cap.
- Always read current game tick from `TimeState.Tick`; do not call wall clock directly inside ECS behavior systems.
