package ecs

import (
	"container/heap"
	"strings"

	"origin/internal/types"
)

type BehaviorTickKey struct {
	EntityID    types.EntityID
	BehaviorKey string
}

type behaviorTickState struct {
	DueTick uint64
	Seq     uint64
}

type behaviorTickHeapItem struct {
	Key     BehaviorTickKey
	DueTick uint64
	Seq     uint64
}

type behaviorTickMinHeap []behaviorTickHeapItem

func (h behaviorTickMinHeap) Len() int { return len(h) }

func (h behaviorTickMinHeap) Less(i, j int) bool {
	if h[i].DueTick == h[j].DueTick {
		if h[i].Key.EntityID == h[j].Key.EntityID {
			return h[i].Key.BehaviorKey < h[j].Key.BehaviorKey
		}
		return h[i].Key.EntityID < h[j].Key.EntityID
	}
	return h[i].DueTick < h[j].DueTick
}

func (h behaviorTickMinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *behaviorTickMinHeap) Push(x any) {
	item, ok := x.(behaviorTickHeapItem)
	if !ok {
		return
	}
	*h = append(*h, item)
}

func (h *behaviorTickMinHeap) Pop() any {
	old := *h
	n := len(old)
	if n == 0 {
		return nil
	}
	item := old[n-1]
	*h = old[:n-1]
	return item
}

type BehaviorTickSchedule struct {
	queue    behaviorTickMinHeap
	latest   map[BehaviorTickKey]behaviorTickState
	byEntity map[types.EntityID]map[string]struct{}
	seq      uint64
}

func (s *BehaviorTickSchedule) Schedule(entityID types.EntityID, behaviorKey string, dueTick uint64) bool {
	if entityID == 0 {
		return false
	}
	keyName := strings.TrimSpace(behaviorKey)
	if keyName == "" {
		return false
	}
	if s.latest == nil {
		s.latest = make(map[BehaviorTickKey]behaviorTickState, 256)
	}
	if s.byEntity == nil {
		s.byEntity = make(map[types.EntityID]map[string]struct{}, 128)
	}

	s.seq++
	key := BehaviorTickKey{
		EntityID:    entityID,
		BehaviorKey: keyName,
	}
	s.latest[key] = behaviorTickState{
		DueTick: dueTick,
		Seq:     s.seq,
	}
	entityBehaviors, exists := s.byEntity[entityID]
	if !exists {
		entityBehaviors = make(map[string]struct{}, 2)
		s.byEntity[entityID] = entityBehaviors
	}
	entityBehaviors[keyName] = struct{}{}
	heap.Push(&s.queue, behaviorTickHeapItem{
		Key:     key,
		DueTick: dueTick,
		Seq:     s.seq,
	})
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
	if _, exists := s.latest[key]; !exists {
		return false
	}
	delete(s.latest, key)
	s.removeEntityBehavior(entityID, keyName)
	return true
}

func (s *BehaviorTickSchedule) CancelAll(entityID types.EntityID) int {
	if entityID == 0 || len(s.byEntity) == 0 {
		return 0
	}
	behaviorSet, exists := s.byEntity[entityID]
	if !exists || len(behaviorSet) == 0 {
		return 0
	}
	delete(s.byEntity, entityID)

	canceled := 0
	for behaviorKey := range behaviorSet {
		key := BehaviorTickKey{
			EntityID:    entityID,
			BehaviorKey: behaviorKey,
		}
		if _, has := s.latest[key]; has {
			delete(s.latest, key)
			canceled++
		}
	}
	return canceled
}

func (s *BehaviorTickSchedule) PopDue(nowTick uint64, max int, dst []BehaviorTickKey) []BehaviorTickKey {
	if max <= 0 {
		max = len(s.latest)
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

		popped := heap.Pop(&s.queue)
		item, ok := popped.(behaviorTickHeapItem)
		if !ok {
			continue
		}

		current, exists := s.latest[item.Key]
		if !exists || current.Seq != item.Seq {
			continue
		}

		delete(s.latest, item.Key)
		s.removeEntityBehavior(item.Key.EntityID, item.Key.BehaviorKey)
		dst = append(dst, item.Key)
		drained++
	}
	return dst
}

func (s *BehaviorTickSchedule) PendingCount() int {
	return len(s.latest)
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
