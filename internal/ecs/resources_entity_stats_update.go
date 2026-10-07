package ecs

import (
	"container/heap"

	constt "origin/internal/const"
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
	EntityID  types.EntityID
	Handle    types.Handle // InvalidHandle denotes a legacy, unprepared record.
	DueUnixMs int64
	HeapIndex int // -1 when no notification is pending.
}

type PlayerStatsNetSnapshot struct {
	Stamina      uint32
	Energy       uint32
	StaminaMax   uint32
	EnergyMax    uint32
	SHP          uint32
	HHP          uint32
	MHP          uint32
	IsKnockedOut bool
	KOUntilMs    int64
	IsLying      bool
	CanStandUp   bool
}

type movementModeState struct {
	pendingQueue []types.EntityID
	pendingSet   map[types.EntityID]struct{}
	lastSent     map[types.EntityID]constt.MoveMode
}

// Player notifications use a typed indexed heap: scheduling and removing a
// prepared record do not box values or leave superseded entries behind.
type playerStatsPushMinHeap []*playerStatsPushState

func (h playerStatsPushMinHeap) Less(i, j int) bool {
	if h[i].DueUnixMs == h[j].DueUnixMs {
		return h[i].EntityID < h[j].EntityID
	}
	return h[i].DueUnixMs < h[j].DueUnixMs
}

func (h playerStatsPushMinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].HeapIndex = i
	h[j].HeapIndex = j
}

func (h playerStatsPushMinHeap) up(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if !h.Less(index, parent) {
			return
		}
		h.Swap(index, parent)
		index = parent
	}
}

func (h playerStatsPushMinHeap) down(index int) {
	for {
		left := 2*index + 1
		if left >= len(h) {
			return
		}
		child := left
		if right := left + 1; right < len(h) && h.Less(right, left) {
			child = right
		}
		if !h.Less(child, index) {
			return
		}
		h.Swap(index, child)
		index = child
	}
}

func (h *playerStatsPushMinHeap) push(record *playerStatsPushState) {
	record.HeapIndex = len(*h)
	*h = append(*h, record)
	h.up(record.HeapIndex)
}

func (h *playerStatsPushMinHeap) remove(index int) *playerStatsPushState {
	queue := *h
	record := queue[index]
	lastIndex := len(queue) - 1
	last := queue[lastIndex]
	queue[lastIndex] = nil
	queue = queue[:lastIndex]
	*h = queue
	if index < lastIndex {
		queue[index] = last
		last.HeapIndex = index
		if index > 0 && queue.Less(index, (index-1)/2) {
			queue.up(index)
		} else {
			queue.down(index)
		}
	}
	record.HeapIndex = -1
	return record
}

// EntityStatsUpdateState tracks scheduled regen ticks and throttled player stats pushes.
// It is queue-driven and avoids full ECS scans for high-load runtime updates.
type EntityStatsUpdateState struct {
	regenQueue  entityStatsRegenMinHeap
	regenLatest map[types.Handle]entityStatsRegenState
	regenSeq    uint64

	pushQueue  playerStatsPushMinHeap
	pushLatest map[types.EntityID]*playerStatsPushState

	lastSentUnixMs map[types.EntityID]int64
	lastSentNet    map[types.EntityID]PlayerStatsNetSnapshot
	movementMode   movementModeState
}

func (s *EntityStatsUpdateState) ScheduleRegen(handle types.Handle, dueTick uint64) bool {
	if handle == types.InvalidHandle {
		return false
	}
	if s.regenLatest == nil {
		s.regenLatest = make(map[types.Handle]entityStatsRegenState, 128)
	}
	if current, exists := s.regenLatest[handle]; exists && current.DueTick == dueTick {
		return true
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

// PreparePlayer binds notification storage to a live generational handle. Call
// during target setup under the owning shard lock, outside the damage path.
// Every record reserves one possible heap entry; hits only mutate existing
// records, including after ForgetPlayer clears disconnected client state.
func (s *EntityStatsUpdateState) PreparePlayer(entityID types.EntityID, handle types.Handle) bool {
	if entityID == 0 || handle == types.InvalidHandle {
		return false
	}
	if current := s.pushLatest[entityID]; current != nil && current.Handle != types.InvalidHandle && current.Handle != handle {
		return false
	}
	record := s.ensurePlayerPushRecord(entityID)
	record.Handle = handle
	return true
}

func (s *EntityStatsUpdateState) IsPlayerPrepared(entityID types.EntityID, handle types.Handle) bool {
	record := s.pushLatest[entityID]
	return handle != types.InvalidHandle && record != nil && record.Handle == handle
}

// ReleasePlayer retires only the exact prepared binding, so delayed cleanup
// cannot release a replacement entity with the same persistence ID.
func (s *EntityStatsUpdateState) ReleasePlayer(entityID types.EntityID, handle types.Handle) bool {
	if !s.IsPlayerPrepared(entityID, handle) {
		return false
	}
	s.ForgetPlayer(entityID)
	delete(s.pushLatest, entityID)
	return true
}

func (s *EntityStatsUpdateState) ensurePlayerPushRecord(entityID types.EntityID) *playerStatsPushState {
	if record := s.pushLatest[entityID]; record != nil {
		return record
	}
	if s.pushLatest == nil {
		s.pushLatest = make(map[types.EntityID]*playerStatsPushState, 256)
	}
	record := &playerStatsPushState{EntityID: entityID, HeapIndex: -1}
	s.pushLatest[entityID] = record
	if capacity := cap(s.pushQueue); capacity < len(s.pushLatest) {
		nextCapacity := max(128, capacity*2, len(s.pushLatest))
		queue := make(playerStatsPushMinHeap, len(s.pushQueue), nextCapacity)
		copy(queue, s.pushQueue)
		s.pushQueue = queue
	}
	return record
}

func (s *EntityStatsUpdateState) MarkPlayerDirty(entityID types.EntityID, nowUnixMs int64, ttlMs uint32) bool {
	record := s.pushLatest[entityID]
	if record != nil && record.HeapIndex >= 0 && record.DueUnixMs <= nowUnixMs {
		// No newly computed deadline can precede now. Keep an already due
		// notification without reading the sent-state map or touching the heap.
		return true
	}
	return s.markPlayerDirty(entityID, record, nowUnixMs, ttlMs)
}

func (s *EntityStatsUpdateState) markPlayerDirty(entityID types.EntityID, record *playerStatsPushState, nowUnixMs int64, ttlMs uint32) bool {
	if entityID == 0 {
		return false
	}
	if record == nil {
		record = s.ensurePlayerPushRecord(entityID)
	}
	dueUnixMs := s.NextAllowedSendUnixMs(entityID, nowUnixMs, ttlMs)
	if record.HeapIndex >= 0 {
		// Ordinary changes must not postpone an already urgent KO/pose push.
		if record.DueUnixMs > dueUnixMs {
			record.DueUnixMs = dueUnixMs
			s.pushQueue.up(record.HeapIndex)
		}
	} else {
		record.DueUnixMs = dueUnixMs
		s.pushQueue.push(record)
	}
	return true
}

func (s *EntityStatsUpdateState) PopDuePlayerStatsPush(nowUnixMs int64, dst []types.EntityID) []types.EntityID {
	for len(s.pushQueue) > 0 {
		next := s.pushQueue[0]
		if next.DueUnixMs > nowUnixMs {
			break
		}

		item := s.pushQueue.remove(0)
		dst = append(dst, item.EntityID)
	}
	return dst
}

func (s *EntityStatsUpdateState) PendingPlayerPushCount() int {
	return len(s.pushQueue)
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
		last.EnergyMax != next.EnergyMax ||
		last.SHP != next.SHP ||
		last.HHP != next.HHP ||
		last.MHP != next.MHP ||
		last.IsKnockedOut != next.IsKnockedOut ||
		last.KOUntilMs != next.KOUntilMs || last.IsLying != next.IsLying || last.CanStandUp != next.CanStandUp
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
	if record := s.pushLatest[entityID]; record != nil {
		if record.HeapIndex >= 0 {
			s.pushQueue.remove(record.HeapIndex)
			removed = true
		}
		if record.Handle == types.InvalidHandle {
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
	if len(s.movementMode.pendingSet) > 0 {
		if _, exists := s.movementMode.pendingSet[entityID]; exists {
			delete(s.movementMode.pendingSet, entityID)
			removed = true
		}
	}
	if len(s.movementMode.lastSent) > 0 {
		if _, exists := s.movementMode.lastSent[entityID]; exists {
			delete(s.movementMode.lastSent, entityID)
			removed = true
		}
	}
	return removed
}

func (s *EntityStatsUpdateState) ForgetEntity(entityID types.EntityID, handle types.Handle) bool {
	removed := false
	if record := s.pushLatest[entityID]; record == nil || record.Handle == types.InvalidHandle || record.Handle == handle {
		removed = s.ForgetPlayer(entityID)
		if record != nil {
			delete(s.pushLatest, entityID)
			removed = true
		}
	}
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

func UpdateEntityStatsRegenSchedule(
	w *World,
	handle types.Handle,
	stamina float64,
	energy float64,
	maxStamina float64,
) bool {
	if w == nil || handle == types.InvalidHandle {
		return false
	}

	if maxStamina < 0 {
		maxStamina = 0
	}
	if stamina < 0 {
		stamina = 0
	}
	if stamina > maxStamina {
		stamina = maxStamina
	}
	if energy < 0 {
		energy = 0
	}

	state := GetResource[EntityStatsUpdateState](w)
	if energy > 0 && stamina < maxStamina {
		nowTick := GetResource[TimeState](w).Tick
		dueTick := nowTick + ResolveStaminaRegenIntervalTicks(w)
		return state.ScheduleRegen(handle, dueTick)
	}
	return state.CancelRegen(handle)
}

func (s *EntityStatsUpdateState) MarkMovementModeDirty(entityID types.EntityID) bool {
	if entityID == 0 {
		return false
	}
	if s.movementMode.pendingSet == nil {
		s.movementMode.pendingSet = make(map[types.EntityID]struct{}, 256)
	}
	if _, exists := s.movementMode.pendingSet[entityID]; exists {
		return true
	}
	s.movementMode.pendingSet[entityID] = struct{}{}
	s.movementMode.pendingQueue = append(s.movementMode.pendingQueue, entityID)
	return true
}

func (s *EntityStatsUpdateState) PopDueMovementModePush(dst []types.EntityID) []types.EntityID {
	if len(s.movementMode.pendingQueue) == 0 {
		return dst
	}
	for _, entityID := range s.movementMode.pendingQueue {
		if _, exists := s.movementMode.pendingSet[entityID]; !exists {
			continue
		}
		delete(s.movementMode.pendingSet, entityID)
		dst = append(dst, entityID)
	}
	s.movementMode.pendingQueue = s.movementMode.pendingQueue[:0]
	return dst
}

func (s *EntityStatsUpdateState) ShouldSendMovementMode(entityID types.EntityID, next constt.MoveMode, force bool) bool {
	if entityID == 0 {
		return false
	}
	if force {
		return true
	}
	if len(s.movementMode.lastSent) == 0 {
		return true
	}
	last, exists := s.movementMode.lastSent[entityID]
	if !exists {
		return true
	}
	return last != next
}

func (s *EntityStatsUpdateState) MarkMovementModeSent(entityID types.EntityID, mode constt.MoveMode) bool {
	if entityID == 0 {
		return false
	}
	if s.movementMode.lastSent == nil {
		s.movementMode.lastSent = make(map[types.EntityID]constt.MoveMode, 256)
	}
	s.movementMode.lastSent[entityID] = mode
	return true
}

func MarkMovementModeDirty(w *World, entityID types.EntityID) bool {
	if w == nil || entityID == 0 {
		return false
	}
	state := GetResource[EntityStatsUpdateState](w)
	return state.MarkMovementModeDirty(entityID)
}

func MarkMovementModeDirtyByHandle(w *World, handle types.Handle) bool {
	if w == nil || handle == types.InvalidHandle || !w.Alive(handle) {
		return false
	}
	entityID, ok := w.GetExternalID(handle)
	if !ok || entityID == 0 {
		return false
	}
	return MarkMovementModeDirty(w, entityID)
}
