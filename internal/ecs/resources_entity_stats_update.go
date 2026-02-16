package ecs

import (
	"container/heap"

	"origin/internal/types"
)

type entityStatsRegenState struct {
	DueTick uint64
	Seq     uint64
}

type entityStatsRegenHeapItem struct {
	Handle  types.Handle
	DueTick uint64
	Seq     uint64
}

type entityStatsRegenMinHeap []entityStatsRegenHeapItem

func (h entityStatsRegenMinHeap) Len() int { return len(h) }

func (h entityStatsRegenMinHeap) Less(i, j int) bool {
	if h[i].DueTick == h[j].DueTick {
		return h[i].Handle < h[j].Handle
	}
	return h[i].DueTick < h[j].DueTick
}

func (h entityStatsRegenMinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *entityStatsRegenMinHeap) Push(x any) {
	item, ok := x.(entityStatsRegenHeapItem)
	if !ok {
		return
	}
	*h = append(*h, item)
}

func (h *entityStatsRegenMinHeap) Pop() any {
	old := *h
	n := len(old)
	if n == 0 {
		return nil
	}
	item := old[n-1]
	*h = old[:n-1]
	return item
}

type playerStatsPushState struct {
	DueUnixMs int64
	Seq       uint64
}

type PlayerStatsNetSnapshot struct {
	Stamina    uint32
	Energy     uint32
	StaminaMax uint32
	EnergyMax  uint32
}

type playerStatsPushHeapItem struct {
	EntityID  types.EntityID
	DueUnixMs int64
	Seq       uint64
}

type playerStatsPushMinHeap []playerStatsPushHeapItem

func (h playerStatsPushMinHeap) Len() int { return len(h) }

func (h playerStatsPushMinHeap) Less(i, j int) bool {
	if h[i].DueUnixMs == h[j].DueUnixMs {
		return h[i].EntityID < h[j].EntityID
	}
	return h[i].DueUnixMs < h[j].DueUnixMs
}

func (h playerStatsPushMinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *playerStatsPushMinHeap) Push(x any) {
	item, ok := x.(playerStatsPushHeapItem)
	if !ok {
		return
	}
	*h = append(*h, item)
}

func (h *playerStatsPushMinHeap) Pop() any {
	old := *h
	n := len(old)
	if n == 0 {
		return nil
	}
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// EntityStatsUpdateState tracks scheduled regen ticks and throttled player stats pushes.
// It is queue-driven and avoids full ECS scans for high-load runtime updates.
type EntityStatsUpdateState struct {
	regenQueue  entityStatsRegenMinHeap
	regenLatest map[types.Handle]entityStatsRegenState
	regenSeq    uint64

	pushQueue  playerStatsPushMinHeap
	pushLatest map[types.EntityID]playerStatsPushState
	pushSeq    uint64

	lastSentUnixMs map[types.EntityID]int64
	lastSentNet    map[types.EntityID]PlayerStatsNetSnapshot
}

func (s *EntityStatsUpdateState) ScheduleRegen(handle types.Handle, dueTick uint64) bool {
	if handle == types.InvalidHandle {
		return false
	}
	if s.regenLatest == nil {
		s.regenLatest = make(map[types.Handle]entityStatsRegenState, 128)
	}

	s.regenSeq++
	s.regenLatest[handle] = entityStatsRegenState{
		DueTick: dueTick,
		Seq:     s.regenSeq,
	}
	heap.Push(&s.regenQueue, entityStatsRegenHeapItem{
		Handle:  handle,
		DueTick: dueTick,
		Seq:     s.regenSeq,
	})
	return true
}

func (s *EntityStatsUpdateState) CancelRegen(handle types.Handle) bool {
	if handle == types.InvalidHandle || len(s.regenLatest) == 0 {
		return false
	}
	if _, exists := s.regenLatest[handle]; !exists {
		return false
	}
	delete(s.regenLatest, handle)
	return true
}

func (s *EntityStatsUpdateState) PopDueRegen(nowTick uint64, dst []types.Handle) []types.Handle {
	for len(s.regenQueue) > 0 {
		next := s.regenQueue[0]
		if next.DueTick > nowTick {
			break
		}

		popped := heap.Pop(&s.regenQueue)
		item, ok := popped.(entityStatsRegenHeapItem)
		if !ok {
			continue
		}

		current, exists := s.regenLatest[item.Handle]
		if !exists || current.Seq != item.Seq {
			continue
		}

		delete(s.regenLatest, item.Handle)
		dst = append(dst, item.Handle)
	}
	return dst
}

func (s *EntityStatsUpdateState) PendingRegenCount() int {
	return len(s.regenLatest)
}

func (s *EntityStatsUpdateState) NextAllowedSendUnixMs(entityID types.EntityID, nowUnixMs int64, ttlMs uint32) int64 {
	if entityID == 0 {
		return nowUnixMs
	}
	nextAllowed := nowUnixMs
	if lastSentAt, ok := s.lastSentUnixMs[entityID]; ok {
		ttlBoundary := lastSentAt + int64(ttlMs)
		if ttlBoundary > nextAllowed {
			nextAllowed = ttlBoundary
		}
	}
	return nextAllowed
}

func (s *EntityStatsUpdateState) MarkPlayerDirty(entityID types.EntityID, nowUnixMs int64, ttlMs uint32) bool {
	if entityID == 0 {
		return false
	}
	if s.pushLatest == nil {
		s.pushLatest = make(map[types.EntityID]playerStatsPushState, 256)
	}
	if s.lastSentUnixMs == nil {
		s.lastSentUnixMs = make(map[types.EntityID]int64, 256)
	}

	dueUnixMs := s.NextAllowedSendUnixMs(entityID, nowUnixMs, ttlMs)
	s.pushSeq++
	s.pushLatest[entityID] = playerStatsPushState{
		DueUnixMs: dueUnixMs,
		Seq:       s.pushSeq,
	}
	heap.Push(&s.pushQueue, playerStatsPushHeapItem{
		EntityID:  entityID,
		DueUnixMs: dueUnixMs,
		Seq:       s.pushSeq,
	})
	return true
}

func (s *EntityStatsUpdateState) PopDuePlayerStatsPush(nowUnixMs int64, dst []types.EntityID) []types.EntityID {
	for len(s.pushQueue) > 0 {
		next := s.pushQueue[0]
		if next.DueUnixMs > nowUnixMs {
			break
		}

		popped := heap.Pop(&s.pushQueue)
		item, ok := popped.(playerStatsPushHeapItem)
		if !ok {
			continue
		}

		current, exists := s.pushLatest[item.EntityID]
		if !exists || current.Seq != item.Seq {
			continue
		}

		delete(s.pushLatest, item.EntityID)
		dst = append(dst, item.EntityID)
	}
	return dst
}

func (s *EntityStatsUpdateState) PendingPlayerPushCount() int {
	return len(s.pushLatest)
}

func (s *EntityStatsUpdateState) MarkPlayerSent(entityID types.EntityID, nowUnixMs int64) bool {
	if entityID == 0 {
		return false
	}
	if s.lastSentUnixMs == nil {
		s.lastSentUnixMs = make(map[types.EntityID]int64, 256)
	}
	s.lastSentUnixMs[entityID] = nowUnixMs
	return true
}

func (s *EntityStatsUpdateState) GetLastSentPlayerStats(entityID types.EntityID) (PlayerStatsNetSnapshot, bool) {
	if entityID == 0 || len(s.lastSentNet) == 0 {
		return PlayerStatsNetSnapshot{}, false
	}
	snapshot, exists := s.lastSentNet[entityID]
	return snapshot, exists
}

func (s *EntityStatsUpdateState) ShouldSendPlayerStats(entityID types.EntityID, next PlayerStatsNetSnapshot, force bool) bool {
	if entityID == 0 {
		return false
	}
	if force {
		return true
	}
	last, exists := s.GetLastSentPlayerStats(entityID)
	if !exists {
		return true
	}
	return last.Stamina != next.Stamina ||
		last.Energy != next.Energy ||
		last.StaminaMax != next.StaminaMax ||
		last.EnergyMax != next.EnergyMax
}

func (s *EntityStatsUpdateState) MarkPlayerStatsSent(entityID types.EntityID, snapshot PlayerStatsNetSnapshot, nowUnixMs int64) bool {
	if entityID == 0 {
		return false
	}
	if s.lastSentUnixMs == nil {
		s.lastSentUnixMs = make(map[types.EntityID]int64, 256)
	}
	if s.lastSentNet == nil {
		s.lastSentNet = make(map[types.EntityID]PlayerStatsNetSnapshot, 256)
	}
	s.lastSentUnixMs[entityID] = nowUnixMs
	s.lastSentNet[entityID] = snapshot
	return true
}

func (s *EntityStatsUpdateState) ForgetPlayer(entityID types.EntityID) bool {
	if entityID == 0 {
		return false
	}
	removed := false
	if len(s.pushLatest) > 0 {
		if _, exists := s.pushLatest[entityID]; exists {
			delete(s.pushLatest, entityID)
			removed = true
		}
	}
	if len(s.lastSentUnixMs) > 0 {
		if _, exists := s.lastSentUnixMs[entityID]; exists {
			delete(s.lastSentUnixMs, entityID)
			removed = true
		}
	}
	if len(s.lastSentNet) > 0 {
		if _, exists := s.lastSentNet[entityID]; exists {
			delete(s.lastSentNet, entityID)
			removed = true
		}
	}
	return removed
}

func (s *EntityStatsUpdateState) ForgetEntity(entityID types.EntityID, handle types.Handle) bool {
	removed := s.ForgetPlayer(entityID)
	if s.CancelRegen(handle) {
		removed = true
	}
	return removed
}

func MarkPlayerStatsDirty(w *World, entityID types.EntityID, ttlMs uint32) bool {
	if w == nil || entityID == 0 {
		return false
	}
	state := GetResource[EntityStatsUpdateState](w)
	nowUnixMs := GetResource[TimeState](w).UnixMs
	return state.MarkPlayerDirty(entityID, nowUnixMs, ttlMs)
}

func MarkPlayerStatsDirtyByHandle(w *World, handle types.Handle, ttlMs uint32) bool {
	if w == nil || handle == types.InvalidHandle || !w.Alive(handle) {
		return false
	}
	entityID, ok := w.GetExternalID(handle)
	if !ok || entityID == 0 {
		return false
	}
	return MarkPlayerStatsDirty(w, entityID, ttlMs)
}

func ForgetPlayerStatsState(w *World, entityID types.EntityID) bool {
	if w == nil || entityID == 0 {
		return false
	}
	state := GetResource[EntityStatsUpdateState](w)
	return state.ForgetPlayer(entityID)
}

func ForgetEntityStatsState(w *World, entityID types.EntityID, handle types.Handle) bool {
	if w == nil {
		return false
	}
	state := GetResource[EntityStatsUpdateState](w)
	return state.ForgetEntity(entityID, handle)
}
