package ecs

import "origin/internal/types"

// CorpseDecayEntry identifies a loaded corpse and its absolute server-runtime
// deadline. The full generational handle must be revalidated before execution.
type CorpseDecayEntry struct {
	Handle            types.Handle
	EntityID          types.EntityID
	DueRuntimeSeconds int64
}

// CorpseDecaySchedule is an indexed minimum heap owned by the shard. Updating
// or canceling a deadline removes its old entry instead of accumulating stale
// heap records. All calls run under the owning world's lock.
type CorpseDecaySchedule struct {
	queue             []CorpseDecayEntry
	positions         map[types.Handle]int
	cleanupRegistered bool
}

// EnsureCorpseDecaySchedule initializes the resource and despawn cleanup once.
// Unloaded corpses retain their persistent deadline and are rescheduled by the
// behavior lifecycle when restored.
func EnsureCorpseDecaySchedule(w *World) *CorpseDecaySchedule {
	if w == nil {
		return nil
	}
	schedule, exists := TryGetResource[CorpseDecaySchedule](w)
	if !exists {
		schedule = InitResource(w, CorpseDecaySchedule{})
	}
	if !schedule.cleanupRegistered {
		w.AddDespawnObserver(func(handle types.Handle) {
			schedule.Cancel(handle)
		})
		schedule.cleanupRegistered = true
	}
	return schedule
}

func (s *CorpseDecaySchedule) Schedule(handle types.Handle, entityID types.EntityID, dueRuntimeSeconds int64) bool {
	if s == nil || handle == types.InvalidHandle || entityID == 0 || dueRuntimeSeconds < 0 {
		return false
	}
	entry := CorpseDecayEntry{Handle: handle, EntityID: entityID, DueRuntimeSeconds: dueRuntimeSeconds}
	if index, exists := s.positions[handle]; exists {
		s.queue[index] = entry
		s.fix(index)
		return true
	}
	if s.positions == nil {
		s.positions = make(map[types.Handle]int)
	}
	index := len(s.queue)
	s.queue = append(s.queue, entry)
	s.positions[handle] = index
	s.up(index)
	return true
}

func (s *CorpseDecaySchedule) Cancel(handle types.Handle) bool {
	if s == nil {
		return false
	}
	index, exists := s.positions[handle]
	if !exists {
		return false
	}
	s.remove(index)
	return true
}

func (s *CorpseDecaySchedule) PopDue(nowRuntimeSeconds int64) (CorpseDecayEntry, bool) {
	if s == nil || len(s.queue) == 0 || s.queue[0].DueRuntimeSeconds > nowRuntimeSeconds {
		return CorpseDecayEntry{}, false
	}
	entry := s.queue[0]
	s.remove(0)
	return entry, true
}

func (s *CorpseDecaySchedule) PendingCount() int {
	if s == nil {
		return 0
	}
	return len(s.queue)
}

func (s *CorpseDecaySchedule) remove(index int) {
	removed := s.queue[index]
	last := len(s.queue) - 1
	if index != last {
		s.swap(index, last)
	}
	s.queue[last] = CorpseDecayEntry{}
	s.queue = s.queue[:last]
	delete(s.positions, removed.Handle)
	if index < last {
		s.fix(index)
	}
}

func (s *CorpseDecaySchedule) fix(index int) {
	if index > 0 && s.less(index, (index-1)/2) {
		s.up(index)
		return
	}
	s.down(index)
}

func (s *CorpseDecaySchedule) up(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if !s.less(index, parent) {
			return
		}
		s.swap(index, parent)
		index = parent
	}
}

func (s *CorpseDecaySchedule) down(index int) {
	for {
		child := index*2 + 1
		if child >= len(s.queue) {
			return
		}
		if right := child + 1; right < len(s.queue) && s.less(right, child) {
			child = right
		}
		if !s.less(child, index) {
			return
		}
		s.swap(index, child)
		index = child
	}
}

func (s *CorpseDecaySchedule) less(i, j int) bool {
	left, right := s.queue[i], s.queue[j]
	if left.DueRuntimeSeconds != right.DueRuntimeSeconds {
		return left.DueRuntimeSeconds < right.DueRuntimeSeconds
	}
	if left.EntityID != right.EntityID {
		return left.EntityID < right.EntityID
	}
	return left.Handle < right.Handle
}

func (s *CorpseDecaySchedule) swap(i, j int) {
	s.queue[i], s.queue[j] = s.queue[j], s.queue[i]
	s.positions[s.queue[i].Handle] = i
	s.positions[s.queue[j].Handle] = j
}
