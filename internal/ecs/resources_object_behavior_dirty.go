package ecs

import "origin/internal/types"

// ObjectBehaviorDirtyQueue tracks object handles that require behavior recomputation.
// It is deduplicated and drained by ObjectBehaviorSystem with a per-tick budget.
type ObjectBehaviorDirtyQueue struct {
	pending []types.Handle
	inQueue map[types.Handle]struct{}
	head    int
}

func (q *ObjectBehaviorDirtyQueue) Mark(handle types.Handle) bool {
	if handle == types.InvalidHandle {
		return false
	}
	if _, exists := q.inQueue[handle]; exists {
		return false
	}
	q.inQueue[handle] = struct{}{}
	q.pending = append(q.pending, handle)
	return true
}

func (q *ObjectBehaviorDirtyQueue) Drain(max int, dst []types.Handle) []types.Handle {
	if max <= 0 {
		max = len(q.pending) - q.head
	}
	if max <= 0 {
		return dst
	}

	drained := 0
	for q.head < len(q.pending) && drained < max {
		h := q.pending[q.head]
		q.head++
		delete(q.inQueue, h)
		dst = append(dst, h)
		drained++
	}

	q.compact()
	return dst
}

func (q *ObjectBehaviorDirtyQueue) PendingCount() int {
	return len(q.pending) - q.head
}

func (q *ObjectBehaviorDirtyQueue) compact() {
	if q.head == 0 {
		return
	}
	if q.head == len(q.pending) {
		q.pending = q.pending[:0]
		q.head = 0
		return
	}
	if q.head > 1024 || q.head*2 >= len(q.pending) {
		copy(q.pending, q.pending[q.head:])
		q.pending = q.pending[:len(q.pending)-q.head]
		q.head = 0
	}
}

func MarkObjectBehaviorDirty(w *World, handle types.Handle) bool {
	if handle == types.InvalidHandle || !w.Alive(handle) {
		return false
	}
	queue := GetResource[ObjectBehaviorDirtyQueue](w)
	return queue.Mark(handle)
}

func MarkObjectBehaviorDirtyByEntityID(w *World, entityID types.EntityID) bool {
	if entityID == 0 {
		return false
	}
	handle := w.GetHandleByEntityID(entityID)
	if handle == types.InvalidHandle {
		return false
	}
	return MarkObjectBehaviorDirty(w, handle)
}
