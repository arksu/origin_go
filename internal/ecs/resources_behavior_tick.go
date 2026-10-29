package ecs

import (
	"strings"

	"origin/internal/types"
)

type BehaviorTickKey struct {
	EntityID    types.EntityID
	BehaviorKey string
}

type behaviorTickState struct {
	// A negative index retains registration while a due callback is in flight.
	index int
}

type behaviorTickHeapItem struct {
	Key     BehaviorTickKey
	DueTick uint64
}

type behaviorTickMinHeap []behaviorTickHeapItem

func (h behaviorTickMinHeap) less(i, j int) bool {
	if h[i].DueTick == h[j].DueTick {
		if h[i].Key.EntityID == h[j].Key.EntityID {
			return h[i].Key.BehaviorKey < h[j].Key.BehaviorKey
		}
		return h[i].Key.EntityID < h[j].Key.EntityID
	}
	return h[i].DueTick < h[j].DueTick
}

// BehaviorTickSchedule owns one indexed heap entry per scheduled entity/behavior
// pair. All access runs synchronously under the owning shard's lock.
type BehaviorTickSchedule struct {
	queue    behaviorTickMinHeap
	latest   map[BehaviorTickKey]behaviorTickState
	byEntity map[types.EntityID]map[string]struct{}
}

func (s *BehaviorTickSchedule) Schedule(entityID types.EntityID, behaviorKey string, dueTick uint64) bool {
	if entityID == 0 {
		return false
	}
	keyName := strings.TrimSpace(behaviorKey)
	if keyName == "" {
		return false
	}
	key := BehaviorTickKey{
		EntityID:    entityID,
		BehaviorKey: keyName,
	}
	current, registered := s.latest[key]
	if registered && current.index >= 0 {
		s.queue[current.index].DueTick = dueTick
		s.fix(current.index)
		return true
	}
	if !registered {
		if s.latest == nil {
			s.latest = make(map[BehaviorTickKey]behaviorTickState, 256)
		}
		if s.byEntity == nil {
			s.byEntity = make(map[types.EntityID]map[string]struct{}, 128)
		}
		entityBehaviors, exists := s.byEntity[entityID]
		if !exists {
			entityBehaviors = make(map[string]struct{}, 2)
			s.byEntity[entityID] = entityBehaviors
		}
		entityBehaviors[keyName] = struct{}{}
	}
	index := len(s.queue)
	s.queue = append(s.queue, behaviorTickHeapItem{
		Key:     key,
		DueTick: dueTick,
	})
	s.latest[key] = behaviorTickState{index: index}
	s.up(index)
	return true
}

func (s *BehaviorTickSchedule) Cancel(entityID types.EntityID, behaviorKey string) bool {
	if entityID == 0 {
		return false
	}
	keyName := strings.TrimSpace(behaviorKey)
	if keyName == "" || len(s.latest) == 0 {
		return false
	}
	key := BehaviorTickKey{
		EntityID:    entityID,
		BehaviorKey: keyName,
	}
	current, exists := s.latest[key]
	if !exists {
		return false
	}
	if current.index >= 0 {
		s.removeQueued(current.index)
	}
	delete(s.latest, key)
	s.removeEntityBehavior(entityID, keyName)
	// Popped callbacks are no longer pending, even while their registration is
	// retained for rearm. Preserve cancellation's pending-work return value.
	return current.index >= 0
}

func (s *BehaviorTickSchedule) CancelAll(entityID types.EntityID) int {
	if entityID == 0 || len(s.byEntity) == 0 {
		return 0
	}
	behaviorSet, exists := s.byEntity[entityID]
	if !exists || len(behaviorSet) == 0 {
		return 0
	}
	canceled := 0
	for behaviorKey := range behaviorSet {
		if s.Cancel(entityID, behaviorKey) {
			canceled++
		}
	}
	return canceled
}

func (s *BehaviorTickSchedule) PopDue(nowTick uint64, max int, dst []BehaviorTickKey) []BehaviorTickKey {
	return s.popDue(nowTick, max, dst, false)
}

// PopDueForExecution retains each due key's registration until CompleteTick.
// The caller must complete every returned key after its synchronous callback,
// including rejected or failed callbacks. Rearming during that callback reuses
// registration without allocating; unrearmed one-shot keys are then released.
func (s *BehaviorTickSchedule) PopDueForExecution(nowTick uint64, max int, dst []BehaviorTickKey) []BehaviorTickKey {
	return s.popDue(nowTick, max, dst, true)
}

// CompleteTick releases a popped key unless it has since been rearmed.
func (s *BehaviorTickSchedule) CompleteTick(key BehaviorTickKey) {
	if current, exists := s.latest[key]; exists && current.index < 0 {
		delete(s.latest, key)
		s.removeEntityBehavior(key.EntityID, key.BehaviorKey)
	}
}

// ShouldExecuteTick rechecks a pre-popped callback after earlier callbacks may
// have canceled it or replaced its deadline. Examined keys still consume budget.
func (s *BehaviorTickSchedule) ShouldExecuteTick(key BehaviorTickKey) bool {
	current, exists := s.latest[key]
	return exists && current.index < 0
}

func (s *BehaviorTickSchedule) popDue(nowTick uint64, max int, dst []BehaviorTickKey, retain bool) []BehaviorTickKey {
	if max <= 0 {
		max = len(s.queue)
	}
	if max <= 0 || len(s.queue) == 0 {
		return dst
	}

	drained := 0
	for drained < max && len(s.queue) > 0 {
		next := s.queue[0]
		if next.DueTick > nowTick {
			break
		}

		s.removeQueued(0)
		if !retain {
			s.CompleteTick(next.Key)
		}
		dst = append(dst, next.Key)
		drained++
	}
	return dst
}

func (s *BehaviorTickSchedule) PendingCount() int {
	return len(s.queue)
}

// removeQueued physically removes a deadline but keeps its registration for a
// callback to reuse. The caller either rearms it or releases it with CompleteTick.
func (s *BehaviorTickSchedule) removeQueued(index int) {
	removed := s.queue[index]
	last := len(s.queue) - 1
	if index != last {
		s.swap(index, last)
	}
	s.queue[last] = behaviorTickHeapItem{}
	s.queue = s.queue[:last]
	s.latest[removed.Key] = behaviorTickState{index: -1}
	if index < last {
		s.fix(index)
	}
}

func (s *BehaviorTickSchedule) fix(index int) {
	if index > 0 && s.queue.less(index, (index-1)/2) {
		s.up(index)
		return
	}
	s.down(index)
}

func (s *BehaviorTickSchedule) up(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if !s.queue.less(index, parent) {
			return
		}
		s.swap(index, parent)
		index = parent
	}
}

func (s *BehaviorTickSchedule) down(index int) {
	for {
		child := index*2 + 1
		if child >= len(s.queue) {
			return
		}
		if right := child + 1; right < len(s.queue) && s.queue.less(right, child) {
			child = right
		}
		if !s.queue.less(child, index) {
			return
		}
		s.swap(index, child)
		index = child
	}
}

func (s *BehaviorTickSchedule) swap(i, j int) {
	s.queue[i], s.queue[j] = s.queue[j], s.queue[i]
	s.latest[s.queue[i].Key] = behaviorTickState{index: i}
	s.latest[s.queue[j].Key] = behaviorTickState{index: j}
}

func (s *BehaviorTickSchedule) removeEntityBehavior(entityID types.EntityID, behaviorKey string) {
	if len(s.byEntity) == 0 {
		return
	}
	behaviorSet, exists := s.byEntity[entityID]
	if !exists {
		return
	}
	delete(behaviorSet, behaviorKey)
	if len(behaviorSet) == 0 {
		delete(s.byEntity, entityID)
	}
}

func ScheduleBehaviorTick(w *World, entityID types.EntityID, behaviorKey string, dueTick uint64) bool {
	if w == nil {
		return false
	}
	schedule := GetResource[BehaviorTickSchedule](w)
	return schedule.Schedule(entityID, behaviorKey, dueTick)
}

func CancelBehaviorTick(w *World, entityID types.EntityID, behaviorKey string) bool {
	if w == nil {
		return false
	}
	schedule := GetResource[BehaviorTickSchedule](w)
	return schedule.Cancel(entityID, behaviorKey)
}

func CancelBehaviorTicksByEntityID(w *World, entityID types.EntityID) int {
	if w == nil {
		return 0
	}
	schedule := GetResource[BehaviorTickSchedule](w)
	return schedule.CancelAll(entityID)
}
