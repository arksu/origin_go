package ecs

import "origin/internal/types"

// CharacterVisualDirtyQueue tracks visual changes in FIFO order. Prepare reserves
// a persistent record so subsequent Mark/Drain cycles do not allocate. Callers
// must hold the owning world's lock, as with other ECS resources.
type CharacterVisualDirtyQueue struct {
	records map[types.Handle]*characterVisualDirtyRecord
	head    *characterVisualDirtyRecord
	tail    *characterVisualDirtyRecord
	pending int
}

type characterVisualDirtyRecord struct {
	handle types.Handle
	prev   *characterVisualDirtyRecord
	next   *characterVisualDirtyRecord
	queued bool
}

// Prepare registers a handle without marking it dirty. Repeated preparation is
// idempotent; records remain registered after Drain until Forget is called.
func (q *CharacterVisualDirtyQueue) Prepare(handle types.Handle) bool {
	if handle == types.InvalidHandle {
		return false
	}
	if _, exists := q.records[handle]; exists {
		return true
	}
	if q.records == nil {
		q.records = make(map[types.Handle]*characterVisualDirtyRecord)
	}
	q.records[handle] = &characterVisualDirtyRecord{handle: handle}
	return true
}

func (q *CharacterVisualDirtyQueue) IsPrepared(handle types.Handle) bool {
	_, exists := q.records[handle]
	return exists
}

// Mark preserves the existing lazy-registration API for non-combat producers.
// Prepared handles require neither a map insertion nor queue growth.
func (q *CharacterVisualDirtyQueue) Mark(handle types.Handle) bool {
	record := q.records[handle]
	if record != nil && record.queued {
		return false
	}
	return q.enqueue(handle, record)
}

func (q *CharacterVisualDirtyQueue) enqueue(handle types.Handle, record *characterVisualDirtyRecord) bool {
	if handle == types.InvalidHandle {
		return false
	}
	if record == nil {
		q.Prepare(handle)
		record = q.records[handle]
	}
	record.prev = q.tail
	if q.tail != nil {
		q.tail.next = record
	} else {
		q.head = record
	}
	q.tail = record
	record.queued = true
	q.pending++
	return true
}

// Drain appends at most max handles in FIFO order. As before, nonpositive max
// drains all pending handles. A sufficiently sized dst keeps draining allocation
// free; registrations are retained for the next dirty update.
func (q *CharacterVisualDirtyQueue) Drain(max int, dst []types.Handle) []types.Handle {
	if max <= 0 || max > q.pending {
		max = q.pending
	}
	for i := 0; i < max; i++ {
		record := q.head
		q.unlink(record)
		dst = append(dst, record.handle)
	}
	return dst
}

func (q *CharacterVisualDirtyQueue) PendingCount() int {
	return q.pending
}

// Forget removes both a pending update and its persistent registration in O(1).
// World.Despawn calls this for every handle, including legacy lazy registrations.
func (q *CharacterVisualDirtyQueue) Forget(handle types.Handle) bool {
	record := q.records[handle]
	if record == nil {
		return false
	}
	if record.queued {
		q.unlink(record)
	}
	delete(q.records, handle)
	return true
}

func (q *CharacterVisualDirtyQueue) unlink(record *characterVisualDirtyRecord) {
	if record.prev != nil {
		record.prev.next = record.next
	} else {
		q.head = record.next
	}
	if record.next != nil {
		record.next.prev = record.prev
	} else {
		q.tail = record.prev
	}
	record.prev = nil
	record.next = nil
	record.queued = false
	q.pending--
}

func MarkCharacterVisualDirty(w *World, ownerID types.EntityID) {
	handle := w.GetHandleByEntityID(ownerID)
	if handle != types.InvalidHandle && w.Alive(handle) {
		GetResource[CharacterVisualDirtyQueue](w).Mark(handle)
	}
}
