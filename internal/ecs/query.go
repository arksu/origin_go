package ecs

// Query provides efficient iteration over entities with specific components
// Inspired by Bevy's Query<(&A, &B, &mut C)> pattern
// Zero-copy iteration via ForEach, pooled buffers for Handles()
type Query struct {
	world    *World
	required ComponentMask
	excluded ComponentMask
}

// NewQuery creates a new query builder for the given world
// For repeated queries, use PreparedQuery instead
func NewQuery(world *World) *Query {
	return &Query{
		world:    world,
		required: 0,
		excluded: 0,
	}
}

// Prepare converts this query to a PreparedQuery with cached archetypes
func (q *Query) Prepare() *PreparedQuery {
	return NewPreparedQuery(q.world, q.required, q.excluded)
}

// With adds a required component to the query
func (q *Query) With(componentID ComponentID) *Query {
	q.required.Set(componentID)
	return q
}

// Without adds an excluded component to the query
func (q *Query) Without(componentID ComponentID) *Query {
	q.excluded.Set(componentID)
	return q
}

// HandlesInto appends all matching handles to dst and returns the result
// Caller manages the buffer - zero allocations if dst has sufficient capacity
// This is the preferred method for performance-critical code
func (q *Query) HandlesInto(dst []Handle) []Handle {
	archetypes := q.world.archetypes.QueryArchetypes(q.required)

	for _, arch := range archetypes {
		if q.excluded != 0 && arch.Mask()&q.excluded != 0 {
			continue
		}
		// Direct append from archetype's internal slice
		arch.ForEachHandle(func(h Handle) {
			dst = append(dst, h)
		})
	}

	return dst
}

// Handles returns all handles matching the query
// Allocates a new slice - use HandlesInto for zero-allocation version
func (q *Query) Handles() []Handle {
	return q.HandlesInto(nil)
}

// Count returns the number of entities matching the query
func (q *Query) Count() int {
	archetypes := q.world.archetypes.QueryArchetypes(q.required)

	count := 0
	for _, arch := range archetypes {
		if q.excluded != 0 && arch.Mask()&q.excluded != 0 {
			continue
		}
		count += arch.Len()
	}
	return count
}

// ForEach iterates over all entities matching the query
// Zero-copy iteration - no allocations
func (q *Query) ForEach(fn func(Handle)) {
	archetypes := q.world.archetypes.QueryArchetypes(q.required)

	for _, arch := range archetypes {
		if q.excluded != 0 && arch.Mask()&q.excluded != 0 {
			continue
		}
		arch.ForEachHandle(fn)
	}
}

// QueryBuilder provides a fluent API for building queries with type safety
type QueryBuilder[T any] struct {
	query *Query
}

// BuildQuery creates a typed query builder
func BuildQuery[T any](world *World) *QueryBuilder[T] {
	return &QueryBuilder[T]{
		query: NewQuery(world),
	}
}

// With adds a required component type
func (qb *QueryBuilder[T]) With(componentID ComponentID) *QueryBuilder[T] {
	qb.query.With(componentID)
	return qb
}

// Without adds an excluded component type
func (qb *QueryBuilder[T]) Without(componentID ComponentID) *QueryBuilder[T] {
	qb.query.Without(componentID)
	return qb
}

// Build returns the underlying query
func (qb *QueryBuilder[T]) Build() *Query {
	return qb.query
}

// PreparedQuery caches archetype list for repeated queries
// Eliminates archetype scanning overhead on every tick
// Automatically refreshes when new archetypes are created (version tracking)
// Single-threaded per shard - no locks needed
type PreparedQuery struct {
	world       *World
	required    ComponentMask
	excluded    ComponentMask
	archetypes  []*Archetype
	seenVersion int64 // Last archetype version seen during Refresh
}

// NewPreparedQuery creates a prepared query with cached archetype list
func NewPreparedQuery(world *World, required, excluded ComponentMask) *PreparedQuery {
	pq := &PreparedQuery{
		world:    world,
		required: required,
		excluded: excluded,
	}
	pq.Refresh()
	return pq
}

// Refresh updates the cached archetype list
// Automatically called by ForEach when archetype version changes
// Single-threaded - no lock needed
func (pq *PreparedQuery) Refresh() {
	archetypes := pq.world.archetypes.QueryArchetypes(pq.required)

	filtered := make([]*Archetype, 0, len(archetypes))
	for _, arch := range archetypes {
		if pq.excluded != 0 && arch.Mask()&pq.excluded != 0 {
			continue
		}
		filtered = append(filtered, arch)
	}

	pq.archetypes = filtered
	pq.seenVersion = pq.world.archetypes.Version()
}

// ForEach iterates with zero allocations using cached archetype list
// Automatically refreshes if new archetypes were created since last refresh
// Single-threaded - no lock needed
func (pq *PreparedQuery) ForEach(fn func(Handle)) {
	// Auto-refresh if archetype version changed (new archetypes created)
	if pq.seenVersion != pq.world.archetypes.Version() {
		pq.Refresh()
	}

	for _, arch := range pq.archetypes {
		arch.ForEachHandle(fn)
	}
}

// Count returns entity count using cached archetype list
// Automatically refreshes if new archetypes were created since last refresh
// Single-threaded - no lock needed
func (pq *PreparedQuery) Count() int {
	// Auto-refresh if archetype version changed (new archetypes created)
	if pq.seenVersion != pq.world.archetypes.Version() {
		pq.Refresh()
	}

	count := 0
	for _, arch := range pq.archetypes {
		count += arch.Len()
	}
	return count
}
