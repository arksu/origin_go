# ECS (Entity Component System)

High-performance archetype-based ECS implementation for MMO game servers.

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

### Architecture

- **Archetype-based storage**: Entities grouped by component signature for cache-friendly iteration
- **Entity location tracking**: O(1) archetype removal via index tracking (critical for large archetypes)
- **Sparse-dense arrays**: O(1) component access using handle index
- **Type-safe generics**: Compile-time component type safety
- **Lock-free reads**: RWMutex for concurrent system execution

### O(1) Archetype Removal

**Problem**: Naive archetype removal is O(n) - linear search through all entities in archetype
- Large archetypes (Transform+ChunkRef) can have 100k+ entities
- Every despawn, component add/remove triggers archetype transition
- O(n) removal creates performance cliff with entity count

**Solution**: Location tracking for O(1) removal
- `World.locations` maps Handle → (archetype, index)
- `Archetype.RemoveEntityAt(index)` uses swap-remove in O(1)
- When entity swapped, update its location to new index

**Benchmark Results** (archetype removal):
```
Size      O(n) Linear    O(1) Indexed    Speedup
100       39 ns/op       17 ns/op        2.3x
1,000     294 ns/op      16 ns/op        18x
10,000    2,739 ns/op    17 ns/op        161x
100,000   27,618 ns/op   16 ns/op        1,726x ✓
```

**Real-world impact**:
- Component changes: ~170 ns/op (constant, regardless of archetype size)
- Despawn: ~300-400 ns/op (constant, regardless of archetype size)
- No performance degradation with entity count scaling

### Components

Register components with explicit IDs during `init()`:

```go
type Position struct {
    X, Y, Z float64
}

const PositionComponentID ecs.ComponentID = 1

func init() {
    ecs.RegisterComponent[Position](PositionComponentID)
}
```

### Query Optimization

**Zero-Copy Iteration**: `ForEach` iterates without allocations
- `Archetype.ForEachHandle()` iterates internal slice directly
- No handle copying, no temporary slices
- **0 allocs/op** for PreparedQuery

**Caller-Managed Buffers**: `HandlesInto(dst []Handle)` for zero allocations
- Caller manages buffer lifecycle - reuse across ticks
- **0 allocs/op** with preallocated buffer
- **6.6x faster** than old pooled approach
- `Handles()` still available for convenience (allocates)

**PreparedQuery**: Caches archetype list for repeated queries
- Eliminates archetype scanning on every tick
- **Automatic refresh** when new archetypes created (version tracking)
- Prevents silent bugs from missing entities in new archetypes
- ~1 ns overhead for version check (int64 comparison)

**Auto-Refresh Mechanism**:
```go
// ArchetypeGraph increments version when new archetype created
g.version++ // In GetOrCreate()

// PreparedQuery auto-refreshes if version changed
if pq.seenVersion != pq.world.archetypes.Version() {
    pq.Refresh() // Automatic - no manual tracking needed
}
```

**Why Auto-Refresh?**
- Temporary entities/effects create new archetypes
- Dynamic combinations: InCombat, Swimming, Stealth, Dragging
- Active/Inactive chunks (if via component)
- Forgetting manual refresh → silent bugs (entities missed)
- Auto-refresh prevents "sometimes works" bugs

```go
// Zero-copy iteration (preferred for most cases)
world.Query().With(PositionID).ForEach(func(h Handle) { ... })

// Repeated query - use PreparedQuery (0 allocs)
pq := world.Query().With(PositionID).With(VelocityID).Prepare()
pq.ForEach(func(h Handle) { ... })  // Every tick, zero allocations

// Need handle slice? Use HandlesInto with caller-managed buffer
type MySystem struct {
    handleBuf []Handle  // Reused across ticks
}

func (s *MySystem) Update(w *World, dt float64) {
    s.handleBuf = s.handleBuf[:0]  // Reset length, keep capacity
    s.handleBuf = w.Query().With(PositionID).HandlesInto(s.handleBuf)
    // Process handles... 0 allocs/op
}

// Or allocate fresh (convenience)
handles := world.Query().With(PositionID).Handles()  // Allocates
```

**Benchmark Results** (10k entities):
```
Method                      Time         Memory      Allocs
HandlesInto-Preallocated    3,104 ns/op      0 B/op   0 allocs/op  ✓ Best
HandlesInto-Nil            20,461 ns/op  357 KB/op  19 allocs/op
Handles-Legacy             19,978 ns/op  357 KB/op  19 allocs/op
ForEach-ZeroCopy              889 ns/op      0 B/op   0 allocs/op  ✓ Preferred
PreparedQuery-ZeroCopy        827 ns/op      0 B/op   0 allocs/op  ✓ Fastest
```

### Usage

```go
world := ecs.NewWorld()

// Spawn entity
h := world.Spawn(EntityID(12345))

// Add components
ecs.AddComponent(world, h, Position{X: 10, Y: 20, Z: 0})
ecs.AddComponent(world, h, Velocity{X: 1, Y: 0, Z: 0})

// System with PreparedQuery (recommended for repeated queries)
type MovementSystem struct {
    ecs.BaseSystem
    query *ecs.PreparedQuery
}

func (s *MovementSystem) Update(w *ecs.World, dt float64) {
    if s.query == nil {
        s.query = w.Query().
            With(PositionComponentID).
            With(VelocityComponentID).
            Prepare()
    }
    
    // Zero allocations per tick
    s.query.ForEach(func(h ecs.Handle) {
        ecs.MutateComponent[Position](w, h, func(pos *Position) bool {
            vel, _ := ecs.GetComponent[Velocity](w, h)
            pos.X += vel.X * dt
            pos.Y += vel.Y * dt
            return true
        })
    })
}

// Despawn
world.Despawn(h)  // All stale references become invalid
```

### Hot Path Optimization

**Problem**: Original implementation had expensive operations on hot paths
- `HandleAllocator.allocated` map: ~40 bytes overhead per handle + map lookup cost
- `World.Alive()`: Double map lookup + double lock (entities map + IsValid call)
- Hot path operations (query iteration, component access) called Alive() frequently

**Solution**: Array-based validation and single lookup
- Replace `allocated map[Handle]bool` with generation array validation
- `IsValid()`: O(1) array lookup, no map overhead
- `World.Alive()`: Single map lookup, no additional lock
- Memory: 4 bytes per handle (uint32 generation) vs ~40 bytes (map entry)

**Benchmark Results** (hot path operations):
```
Operation                    Time/op     Allocs/op
HandleAllocator.IsValid      3.9 ns      0
World.Alive (valid)          9.5 ns      0
World.Alive (stale)          11.4 ns     0
Query + Alive check (10k)    115 µs      0
```

**Memory Savings**:
- HandleAllocator (1M capacity): 4 MB (array) vs ~40 MB (map)
- **~90% memory reduction** for handle validation

### Performance Characteristics

**Lock-Free Single-Threaded Design**:
- Handle allocation: O(1), no lock overhead
- Handle validation: O(1) array lookup, 3.9 ns/op
- Component access: O(1) with generation validation, **no locks**
- Entity alive check: O(1) single map lookup, 9.5 ns/op, **no locks**
- Archetype removal: O(1) with location tracking, **no locks**
- Component add/remove: O(1) archetype transition, **no locks**
- Entity despawn: O(1) regardless of archetype size, **no locks**
- Query iteration: 0 allocs/op with PreparedQuery, **no locks**
- Archetype iteration: Cache-friendly, ~100M entities/sec, **no locks**

**Memory**:
- ~16 bytes per entity (handle + location) + component data
- No lock overhead, no mutex memory
- GC pressure: Minimal with zero-copy iteration and pooled buffers

**Scalability**:
- Linear scaling with entity count
- No lock contention bottlenecks
- Predictable sub-millisecond tick times
- Suitable for 10k+ entities per shard

### Single-Threaded ECS Model

**Design Philosophy**: One ECS World per shard, single-threaded execution
- **No internal locks**: Zero lock contention on hot paths
- **Massive performance**: No mutex overhead for movement/collision/AI systems
- **Predictable tick time**: No lock stalls or contention spikes

**Why Single-Threaded?**

Lock contention kills MMO tick time with thousands of entities:
```
Before (multi-threaded with locks):
- World.mu: RWMutex on every entity operation
- Archetype.mu: RWMutex per archetype (blocks during ForEach)
- ComponentStorage.mu: RWMutex per component type
- HandleAllocator.mu: Mutex on every alloc/free

With 1000 players + 5000 bots:
- Movement system: 6000 × Get/Mutate = 12000 lock/unlock ops
- Collision system: 6000 × spatial queries = massive contention
- AI system: 5000 × component access = lock storms
- Archetype.ForEach holds RLock → blocks component add/remove

Result: Unpredictable tick time, stalls, poor scaling
```

**After (single-threaded per shard)**:
```
- Zero locks on hot paths
- No contention, no stalls
- Cache-friendly sequential access
- Predictable sub-millisecond tick times
- Scales linearly with entity count
```

**Cross-Thread Communication**

For external threads (network I/O, persistence):
```go
// Command queue pattern (external → ECS)
type SpawnCommand struct {
    EntityID EntityID
    Position Vec3
}

// Network thread enqueues commands
commandQueue.Push(SpawnCommand{...})

// ECS thread processes commands each tick
func (w *World) ProcessCommands(queue *CommandQueue) {
    for cmd := range queue.Drain() {
        switch c := cmd.(type) {
        case SpawnCommand:
            h := w.Spawn(c.EntityID)
            AddComponent(w, h, Position{X: c.Position.X, ...})
        }
    }
}

// Tick loop
for {
    world.ProcessCommands(commandQueue)
    world.Update(dt)
}
```

**Multi-Shard Architecture**

For horizontal scaling:
```
Shard 1 (Thread 1): World with entities 0-999
Shard 2 (Thread 2): World with entities 1000-1999
...

Each shard:
- Independent ECS World
- Single-threaded execution
- No cross-shard locks
- Command queue for external input
```

### Thread Safety

- **ECS operations**: Single-threaded per shard - no locks
- **Systems**: Run sequentially by priority - no parallelism needed
- **External access**: Use command queue pattern
- **World.mu**: Available for optional external synchronization (not used internally)
