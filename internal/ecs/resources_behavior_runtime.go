package ecs

import (
	"strings"

	"origin/internal/types"
)

// BehaviorRuntimeKey identifies one deadline for an exact entity generation.
type BehaviorRuntimeKey struct {
	Handle      types.Handle
	BehaviorKey string
}

// BehaviorRuntimeEntry records an absolute server-runtime deadline. EntityID
// must still match the generational Handle before its callback is executed.
type BehaviorRuntimeEntry struct {
	Handle            types.Handle
	EntityID          types.EntityID
	BehaviorKey       string
	DueRuntimeSeconds int64
}

func (e BehaviorRuntimeEntry) key() BehaviorRuntimeKey {
	return BehaviorRuntimeKey{Handle: e.Handle, BehaviorKey: e.BehaviorKey}
}

// BehaviorRuntimeSchedule is an indexed minimum heap owned by a shard. Each
// handle/behavior pair has one physical record, including after rescheduling.
// All access must run under the owning world's lock.
type BehaviorRuntimeSchedule struct {
	queue             []BehaviorRuntimeEntry
	positions         map[BehaviorRuntimeKey]int
	byHandle          map[types.Handle][]string
	cleanupRegistered bool
}

// EnsureBehaviorRuntimeSchedule lazily initializes runtime scheduling and its
// despawn cleanup. Unloaded objects are rescheduled by their restore lifecycle.
func EnsureBehaviorRuntimeSchedule(w *World) *BehaviorRuntimeSchedule {
	if w == nil {
		return nil
	}
	schedule, exists := TryGetResource[BehaviorRuntimeSchedule](w)
	if !exists {
		schedule = InitResource(w, BehaviorRuntimeSchedule{})
	}
	if !schedule.cleanupRegistered {
		w.AddDespawnObserver(func(handle types.Handle) {
			schedule.CancelAll(handle)
		})
		schedule.cleanupRegistered = true
	}
	return schedule
}

func (s *BehaviorRuntimeSchedule) Schedule(handle types.Handle, entityID types.EntityID, behaviorKey string, dueRuntimeSeconds int64) bool {
	keyName := strings.TrimSpace(behaviorKey)
	if s == nil || handle == types.InvalidHandle || entityID == 0 || keyName == "" || dueRuntimeSeconds < 0 {
		return false
	}
	entry := BehaviorRuntimeEntry{
		Handle: handle, EntityID: entityID, BehaviorKey: keyName, DueRuntimeSeconds: dueRuntimeSeconds,
	}
	key := entry.key()
	if index, exists := s.positions[key]; exists {
		s.queue[index] = entry
		s.fix(index)
		return true
	}
	if s.positions == nil {
		s.positions = make(map[BehaviorRuntimeKey]int)
		s.byHandle = make(map[types.Handle][]string)
	}
	index := len(s.queue)
	s.queue = append(s.queue, entry)
	s.positions[key] = index
	s.byHandle[handle] = append(s.byHandle[handle], keyName)
	s.up(index)
	return true
}

func (s *BehaviorRuntimeSchedule) Cancel(handle types.Handle, behaviorKey string) bool {
	if s == nil {
		return false
	}
	key := BehaviorRuntimeKey{Handle: handle, BehaviorKey: strings.TrimSpace(behaviorKey)}
	index, exists := s.positions[key]
	if !exists {
		return false
	}
	s.remove(index)
	return true
}

// CancelAll removes only deadlines for this exact generation, without scanning
// the heap or interfering with entities that reused the same handle index.
func (s *BehaviorRuntimeSchedule) CancelAll(handle types.Handle) int {
	if s == nil {
		return 0
	}
	canceled := 0
	for len(s.byHandle[handle]) > 0 {
		keys := s.byHandle[handle]
		s.Cancel(handle, keys[len(keys)-1])
		canceled++
	}
	return canceled
}

func (s *BehaviorRuntimeSchedule) PopDue(nowRuntimeSeconds int64) (BehaviorRuntimeEntry, bool) {
	if s == nil || len(s.queue) == 0 || s.queue[0].DueRuntimeSeconds > nowRuntimeSeconds {
		return BehaviorRuntimeEntry{}, false
	}
	entry := s.queue[0]
	s.remove(0)
	return entry, true
}

func (s *BehaviorRuntimeSchedule) PendingCount() int {
	if s == nil {
		return 0
	}
	return len(s.queue)
}

func (s *BehaviorRuntimeSchedule) remove(index int) {
	removed := s.queue[index]
	last := len(s.queue) - 1
	if index != last {
		s.swap(index, last)
	}
	s.queue[last] = BehaviorRuntimeEntry{}
	s.queue = s.queue[:last]
	delete(s.positions, removed.key())
	keys := s.byHandle[removed.Handle]
	for i, keyName := range keys {
		if keyName == removed.BehaviorKey {
			keys[i] = keys[len(keys)-1]
			keys[len(keys)-1] = ""
			keys = keys[:len(keys)-1]
			break
		}
	}
	if len(keys) == 0 {
		delete(s.byHandle, removed.Handle)
	} else {
		s.byHandle[removed.Handle] = keys
	}
	if index < last {
		s.fix(index)
	}
}

func (s *BehaviorRuntimeSchedule) fix(index int) {
	if index > 0 && s.less(index, (index-1)/2) {
		s.up(index)
		return
	}
	s.down(index)
}

func (s *BehaviorRuntimeSchedule) up(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if !s.less(index, parent) {
			return
		}
		s.swap(index, parent)
		index = parent
	}
}

func (s *BehaviorRuntimeSchedule) down(index int) {
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

func (s *BehaviorRuntimeSchedule) less(i, j int) bool {
	left, right := s.queue[i], s.queue[j]
	if left.DueRuntimeSeconds != right.DueRuntimeSeconds {
		return left.DueRuntimeSeconds < right.DueRuntimeSeconds
	}
	if left.EntityID != right.EntityID {
		return left.EntityID < right.EntityID
	}
	if left.BehaviorKey != right.BehaviorKey {
		return left.BehaviorKey < right.BehaviorKey
	}
	return left.Handle < right.Handle
}

func (s *BehaviorRuntimeSchedule) swap(i, j int) {
	s.queue[i], s.queue[j] = s.queue[j], s.queue[i]
	s.positions[s.queue[i].key()] = i
	s.positions[s.queue[j].key()] = j
}

func ScheduleBehaviorRuntime(w *World, handle types.Handle, entityID types.EntityID, behaviorKey string, dueRuntimeSeconds int64) bool {
	return EnsureBehaviorRuntimeSchedule(w).Schedule(handle, entityID, behaviorKey, dueRuntimeSeconds)
}

func CancelBehaviorRuntime(w *World, handle types.Handle, behaviorKey string) bool {
	if w == nil {
		return false
	}
	schedule, exists := TryGetResource[BehaviorRuntimeSchedule](w)
	return exists && schedule.Cancel(handle, behaviorKey)
}

func CancelBehaviorRuntimeByHandle(w *World, handle types.Handle) int {
	if w == nil {
		return 0
	}
	schedule, exists := TryGetResource[BehaviorRuntimeSchedule](w)
	if !exists {
		return 0
	}
	return schedule.CancelAll(handle)
}
