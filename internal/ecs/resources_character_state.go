package ecs

import (
	"container/heap"
	"time"

	"origin/internal/types"
)

type DetachedEntities struct {
	Map map[types.EntityID]DetachedEntity
}

// DetachedEntity represents a player entity that has disconnected but remains in the world
type DetachedEntity struct {
	Handle         types.Handle
	ExpirationTime time.Time
	DetachedAt     time.Time
}

// AddDetachedEntity adds an entity to the detached entities map
func (d *DetachedEntities) AddDetachedEntity(entityID types.EntityID, handle types.Handle, expirationTime time.Time, detachedAt time.Time) {
	d.Map[entityID] = DetachedEntity{
		Handle:         handle,
		ExpirationTime: expirationTime,
		DetachedAt:     detachedAt,
	}
}

// RemoveDetachedEntity removes an entity from the detached entities map
func (d *DetachedEntities) RemoveDetachedEntity(entityID types.EntityID) {
	delete(d.Map, entityID)
}

// GetDetachedEntity returns a detached entity by EntityID
func (d *DetachedEntities) GetDetachedEntity(entityID types.EntityID) (DetachedEntity, bool) {
	entity, ok := d.Map[entityID]
	return entity, ok
}

// IsDetached checks if an entity is in detached state
func (d *DetachedEntities) IsDetached(entityID types.EntityID) bool {
	_, ok := d.Map[entityID]
	return ok
}

type CharacterEntities struct {
	Map    map[types.EntityID]CharacterEntity
	queue  characterSaveMinHeap
	latest map[types.EntityID]characterSaveState
	seq    uint64
}

type CharacterEntity struct {
	Handle     types.Handle
	LastSaveAt time.Time
	NextSaveAt time.Time
	SavesCount int
}

type characterSaveState struct {
	DueAt time.Time
	Seq   uint64
}

type characterSaveHeapItem struct {
	EntityID types.EntityID
	DueAt    time.Time
	Seq      uint64
}

type characterSaveMinHeap []characterSaveHeapItem

func (h characterSaveMinHeap) Len() int { return len(h) }

func (h characterSaveMinHeap) Less(i, j int) bool {
	return h[i].DueAt.Before(h[j].DueAt)
}

func (h characterSaveMinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *characterSaveMinHeap) Push(x any) {
	item, ok := x.(characterSaveHeapItem)
	if !ok {
		return
	}
	*h = append(*h, item)
}

func (h *characterSaveMinHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	old[n-1] = characterSaveHeapItem{}
	return item
}

func (c *CharacterEntities) Add(entityID types.EntityID, handle types.Handle, nextSaveAt time.Time) {
	c.Map[entityID] = CharacterEntity{
		Handle:     handle,
		LastSaveAt: time.Time{},
		NextSaveAt: nextSaveAt,
		SavesCount: 0,
	}
	c.schedule(entityID, nextSaveAt)
}

func (c *CharacterEntities) Remove(entityID types.EntityID) {
	delete(c.Map, entityID)
	if c.latest != nil {
		delete(c.latest, entityID)
	}
}

func (c *CharacterEntities) UpdateSaveTime(entityID types.EntityID, lastSaveAt, nextSaveAt time.Time) {
	if entity, ok := c.Map[entityID]; ok {
		entity.LastSaveAt = lastSaveAt
		entity.NextSaveAt = nextSaveAt
		entity.SavesCount++
		c.Map[entityID] = entity
		c.schedule(entityID, nextSaveAt)
	}
}

func (c *CharacterEntities) PopDue(now time.Time, dst []types.EntityID) []types.EntityID {
	for len(c.queue) > 0 {
		next := c.queue[0]
		if next.DueAt.After(now) {
			break
		}

		popped := heap.Pop(&c.queue)
		item, ok := popped.(characterSaveHeapItem)
		if !ok {
			continue
		}

		current, exists := c.latest[item.EntityID]
		if !exists || current.Seq != item.Seq {
			continue
		}

		delete(c.latest, item.EntityID)
		dst = append(dst, item.EntityID)
	}
	return dst
}

func (c *CharacterEntities) PendingSaveCount() int {
	return len(c.latest)
}

func (c *CharacterEntities) schedule(entityID types.EntityID, dueAt time.Time) {
	if entityID == 0 {
		return
	}
	if c.latest == nil {
		c.latest = make(map[types.EntityID]characterSaveState, 128)
	}

	c.seq++
	c.latest[entityID] = characterSaveState{
		DueAt: dueAt,
		Seq:   c.seq,
	}
	heap.Push(&c.queue, characterSaveHeapItem{
		EntityID: entityID,
		DueAt:    dueAt,
		Seq:      c.seq,
	})
}

// GetAll returns all character entity IDs
func (c *CharacterEntities) GetAll() []types.EntityID {
	entityIDs := make([]types.EntityID, 0, len(c.Map))
	for entityID := range c.Map {
		entityIDs = append(entityIDs, entityID)
	}
	return entityIDs
}
