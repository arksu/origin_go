package ecs

// Query provides efficient iteration over entities with specific components
// Inspired by Bevy's Query<(&A, &B, &mut C)> pattern
// Works with Handle (uint32) for compact iteration
type Query struct {
	world    *World
	required ComponentMask
	excluded ComponentMask
}

// NewQuery creates a new query builder for the given world
func NewQuery(world *World) *Query {
	return &Query{
		world:    world,
		required: 0,
		excluded: 0,
	}
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

// Handles returns all handles matching the query
func (q *Query) Handles() []Handle {
	archetypes := q.world.archetypes.QueryArchetypes(q.required)

	var result []Handle
	for _, arch := range archetypes {
		// Skip if archetype has any excluded components
		if q.excluded != 0 && arch.Mask()&q.excluded != 0 {
			continue
		}
		result = append(result, arch.Handles()...)
	}
	return result
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
func (q *Query) ForEach(fn func(Handle)) {
	archetypes := q.world.archetypes.QueryArchetypes(q.required)

	for _, arch := range archetypes {
		if q.excluded != 0 && arch.Mask()&q.excluded != 0 {
			continue
		}
		for _, h := range arch.Handles() {
			fn(h)
		}
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
