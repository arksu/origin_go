package ecs

import (
	"container/heap"
	"errors"
	"time"

	"origin/internal/types"
)

type DetachedEntities struct {
	// Map contains pending bodies only. Mutations must use the scheduler methods
	// below; assigning directly would leave the deadline queue inconsistent.
	Map            map[types.EntityID]DetachedEntity
	records        map[types.Handle]*detachedRecord
	byEntity       map[types.EntityID]*detachedRecord
	queue          detachedMinHeap
	mapReservation int
}

// DetachedEntity represents a player entity that has disconnected but remains in the world
type DetachedEntity struct {
	Handle         types.Handle
	ExpirationTime time.Time
	DetachedAt     time.Time
	SaveRetryAt    time.Time // Snapshot capture retry; independent of disconnect expiry.
}

var (
	ErrInvalidLogoutIdentity  = errors.New("logout: invalid identity")
	ErrLogoutIdentityConflict = errors.New("logout: identity already prepared")
)

// DetachedCandidate retains generational identity when a due entry is removed
// from the queue. The pending body remains registered until final cleanup.
type DetachedCandidate struct {
	EntityID types.EntityID
	Handle   types.Handle
}

type detachedRecord struct {
	DetachedCandidate
	NextCheckAt time.Time
	HeapIndex   int
}

type detachedMinHeap []*detachedRecord

func (h detachedMinHeap) less(i, j int) bool {
	a, b := h[i], h[j]
	if !a.NextCheckAt.Equal(b.NextCheckAt) {
		return a.NextCheckAt.Before(b.NextCheckAt)
	}
	if a.EntityID != b.EntityID {
		return a.EntityID < b.EntityID
	}
	return a.Handle < b.Handle
}

func (h detachedMinHeap) swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].HeapIndex = i
	h[j].HeapIndex = j
}

func (h detachedMinHeap) up(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if !h.less(index, parent) {
			return
		}
		h.swap(index, parent)
		index = parent
	}
}

func (h detachedMinHeap) down(index int) {
	for {
		left := 2*index + 1
		if left >= len(h) {
			return
		}
		child := left
		if right := left + 1; right < len(h) && h.less(right, left) {
			child = right
		}
		if !h.less(child, index) {
			return
		}
		h.swap(index, child)
		index = child
	}
}

func (h *detachedMinHeap) remove(index int) *detachedRecord {
	queue := *h
	record := queue[index]
	last := len(queue) - 1
	queue[index] = queue[last]
	queue[last] = nil
	queue = queue[:last]
	*h = queue
	if index < last {
		queue[index].HeapIndex = index
		if index > 0 && queue.less(index, (index-1)/2) {
			queue.up(index)
		} else {
			queue.down(index)
		}
	}
	record.HeapIndex = -1
	return record
}

// PreparePlayer reserves storage outside the tick and combat paths. One record
// can move between connected, queued and consumed states without allocation.
func (d *DetachedEntities) PreparePlayer(entityID types.EntityID, handle types.Handle) error {
	if entityID == 0 || handle == types.InvalidHandle {
		return ErrInvalidLogoutIdentity
	}
	if record := d.records[handle]; record != nil {
		if record.EntityID != entityID {
			return ErrLogoutIdentityConflict
		}
		return nil
	}
	if d.byEntity[entityID] != nil {
		return ErrLogoutIdentityConflict
	}
	if d.records == nil {
		d.records = make(map[types.Handle]*detachedRecord, 128)
		d.byEntity = make(map[types.EntityID]*detachedRecord, 128)
	}
	count := len(d.records) + 1
	if cap(d.queue) < count {
		queue := make(detachedMinHeap, len(d.queue), max(128, cap(d.queue)*2, count))
		copy(queue, d.queue)
		d.queue = queue
	}
	if d.mapReservation < count {
		d.mapReservation = max(128, d.mapReservation*2, count)
		pending := make(map[types.EntityID]DetachedEntity, d.mapReservation)
		for id, entity := range d.Map {
			pending[id] = entity
		}
		d.Map = pending
	}
	record := &detachedRecord{DetachedCandidate: DetachedCandidate{EntityID: entityID, Handle: handle}, HeapIndex: -1}
	d.records[handle] = record
	d.byEntity[entityID] = record
	return nil
}

func (d *DetachedEntities) IsPrepared(entityID types.EntityID, handle types.Handle) bool {
	record := d.byEntity[entityID]
	return handle != types.InvalidHandle && record != nil && record.Handle == handle
}

// AddDetachedEntity adds an entity to the detached entities map
func (d *DetachedEntities) AddDetachedEntity(entityID types.EntityID, handle types.Handle, expirationTime time.Time, detachedAt time.Time) {
	if err := d.PreparePlayer(entityID, handle); err != nil {
		return
	}
	if current, exists := d.Map[entityID]; exists && current.Handle == handle {
		return
	}
	d.Map[entityID] = DetachedEntity{
		Handle:         handle,
		ExpirationTime: expirationTime,
		DetachedAt:     detachedAt,
	}
	d.schedule(d.records[handle], expirationTime)
}

// RemoveDetachedEntity removes an entity from the detached entities map
func (d *DetachedEntities) RemoveDetachedEntity(entityID types.EntityID) {
	if record := d.byEntity[entityID]; record != nil && record.HeapIndex >= 0 {
		d.queue.remove(record.HeapIndex)
	}
	delete(d.Map, entityID)
}

// Release retires only the exact generation, so delayed cleanup cannot remove
// a replacement body with the same persistence ID.
func (d *DetachedEntities) Release(handle types.Handle) bool {
	record := d.records[handle]
	if record == nil {
		return false
	}
	if pending, exists := d.Map[record.EntityID]; exists && pending.Handle == handle {
		d.RemoveDetachedEntity(record.EntityID)
	}
	if record.HeapIndex >= 0 {
		d.queue.remove(record.HeapIndex)
	}
	delete(d.byEntity, record.EntityID)
	delete(d.records, handle)
	return true
}

// RequestRecheck advances a pending check without bypassing the independent
// base delay or rejected-snapshot retry. Connected entries are untouched.
func (d *DetachedEntities) RequestRecheck(handle types.Handle, now time.Time) {
	record := d.records[handle]
	if record == nil {
		return
	}
	entity, exists := d.Map[record.EntityID]
	if !exists || entity.Handle != handle {
		return
	}
	due := detachedEarliestCheck(entity, now)
	if record.HeapIndex >= 0 && !due.Before(record.NextCheckAt) {
		return
	}
	d.schedule(record, due)
}

// ScheduleNext restores a consumed blocked entry. Hints are advisory, while
// minimum delay prevents repeatedly consuming one entry within a tick.
func (d *DetachedEntities) ScheduleNext(entityID types.EntityID, handle types.Handle, now time.Time, hint, minDelay time.Duration) {
	entity, exists := d.Map[entityID]
	if !exists || entity.Handle != handle || !d.IsPrepared(entityID, handle) {
		return
	}
	if hint <= 0 {
		hint = time.Second
	}
	if minDelay <= 0 {
		minDelay = time.Millisecond
	}
	d.schedule(d.records[handle], detachedEarliestCheck(entity, now.Add(max(hint, minDelay))))
}

// SetSaveRetryAt updates both state and its deadline; periodic and final
// capture must use this method instead of mutating Map directly.
func (d *DetachedEntities) SetSaveRetryAt(entityID types.EntityID, handle types.Handle, retryAt time.Time) bool {
	entity, exists := d.Map[entityID]
	if !exists || entity.Handle != handle || !d.IsPrepared(entityID, handle) {
		return false
	}
	entity.SaveRetryAt = retryAt
	d.Map[entityID] = entity
	record := d.records[handle]
	due := retryAt
	if record.HeapIndex >= 0 && record.NextCheckAt.After(due) {
		due = record.NextCheckAt
	}
	d.schedule(record, detachedEarliestCheck(entity, due))
	return true
}

// PopDueInto never grows dst. Its capacity is the caller's candidate budget;
// consumed records retain their preparation for retries and reconnects.
func (d *DetachedEntities) PopDueInto(now time.Time, dst []DetachedCandidate) []DetachedCandidate {
	for len(dst) < cap(dst) && len(d.queue) > 0 && !d.queue[0].NextCheckAt.After(now) {
		record := d.queue.remove(0)
		dst = append(dst, record.DetachedCandidate)
	}
	return dst
}

func (d *DetachedEntities) PendingCheckCount() int { return len(d.queue) }
func (d *DetachedEntities) PreparedCount() int     { return len(d.records) }

func detachedEarliestCheck(entity DetachedEntity, requested time.Time) time.Time {
	if entity.ExpirationTime.After(requested) {
		requested = entity.ExpirationTime
	}
	if entity.SaveRetryAt.After(requested) {
		requested = entity.SaveRetryAt
	}
	return requested
}

func (d *DetachedEntities) schedule(record *detachedRecord, due time.Time) {
	previous := record.NextCheckAt
	record.NextCheckAt = due
	if record.HeapIndex < 0 {
		record.HeapIndex = len(d.queue)
		d.queue = append(d.queue, record)
		d.queue.up(record.HeapIndex)
	} else if due.Before(previous) {
		d.queue.up(record.HeapIndex)
	} else if due.After(previous) {
		d.queue.down(record.HeapIndex)
	}
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

// RescheduleSave retries snapshot capture without counting the rejected attempt
// as a saved character or changing the last accepted snapshot time.
func (c *CharacterEntities) RescheduleSave(entityID types.EntityID, nextSaveAt time.Time) {
	if entity, ok := c.Map[entityID]; ok {
		entity.NextSaveAt = nextSaveAt
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
