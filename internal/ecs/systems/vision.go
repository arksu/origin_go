package systems

import (
	"math/rand"
	"slices"
	"sync"
	"time"

	_const "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

const numVisionWorkers = 3

// ---------------- internal types ----------------

type visibleEntry struct {
	Handle   types.Handle
	EntityID types.EntityID
}

type spawnEventData struct {
	observerID   types.EntityID
	targetID     types.EntityID
	targetHandle types.Handle
	layer        int
}

type despawnEventData struct {
	observerID types.EntityID
	targetID   types.EntityID
	layer      int
}

type observerJob struct {
	handle      types.Handle
	observerVis ecs.ObserverVisibility // value copy; Known is a shared reference
}

type observerResult struct {
	handle         types.Handle
	newVis         ecs.ObserverVisibility // new state with fresh Known map
	spawnTargets   []types.Handle
	despawnTargets []types.Handle
	spawns         []spawnEventData
	despawns       []despawnEventData
	skipOnly       bool // canSkipUpdate was true — only NextUpdateTime changed
}

type deadObserverEntry struct {
	handle types.Handle
	known  map[types.Handle]types.EntityID
}

// visionWorkerScratch holds per-worker scratch buffers to avoid contention.
type visionWorkerScratch struct {
	candidatesBuffer []types.Handle
	newVisibleBuf    []visibleEntry
	queriedGens      []ecs.ChunkGen
}

func newWorkerScratch() visionWorkerScratch {
	return visionWorkerScratch{
		candidatesBuffer: make([]types.Handle, 0, 256),
		newVisibleBuf:    make([]visibleEntry, 0, 256),
		queriedGens:      make([]ecs.ChunkGen, 0, 9),
	}
}

// ---------------- VisionSystem ----------------

type VisionSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	eventBus     *eventbus.EventBus
	logger       *zap.Logger

	workers       [numVisionWorkers]visionWorkerScratch
	jobs          []observerJob
	results       []observerResult
	deadObservers []deadObserverEntry

	transformStorage  *ecs.ComponentStorage[components.Transform]
	visionStorage     *ecs.ComponentStorage[components.Vision]
	stealthStorage    *ecs.ComponentStorage[components.Stealth]
	chunkRefStorage   *ecs.ComponentStorage[components.ChunkRef]
	entityInfoStorage *ecs.ComponentStorage[components.EntityInfo]
}

func NewVisionSystem(
	world *ecs.World,
	chunkManager core.ChunkManager,
	eventBus *eventbus.EventBus,
	logger *zap.Logger,
) *VisionSystem {
	sys := &VisionSystem{
		BaseSystem:        ecs.NewBaseSystem("VisionSystem", 350),
		chunkManager:      chunkManager,
		eventBus:          eventBus,
		logger:            logger,
		jobs:              make([]observerJob, 0, 128),
		results:           make([]observerResult, 0, 128),
		deadObservers:     make([]deadObserverEntry, 0, 16),
		transformStorage:  ecs.GetOrCreateStorage[components.Transform](world),
		visionStorage:     ecs.GetOrCreateStorage[components.Vision](world),
		stealthStorage:    ecs.GetOrCreateStorage[components.Stealth](world),
		chunkRefStorage:   ecs.GetOrCreateStorage[components.ChunkRef](world),
		entityInfoStorage: ecs.GetOrCreateStorage[components.EntityInfo](world),
	}
	for i := range sys.workers {
		sys.workers[i] = newWorkerScratch()
	}
	return sys
}

// ForceUpdateForObserver immediately updates vision for a specific observer,
// bypassing the normal 1-second throttle. Used for dropped/picked items.
func (s *VisionSystem) ForceUpdateForObserver(w *ecs.World, observerHandle types.Handle) {
	visState := ecs.GetResource[ecs.VisibilityState](w)

	// Check if observer exists and is alive
	observerVis, exists := visState.VisibleByObserver[observerHandle]
	if !exists || !w.Alive(observerHandle) {
		return
	}

	now := ecs.GetResource[ecs.TimeState](w).Now

	// Force immediate update by setting NextUpdateTime to now
	observerVis.NextUpdateTime = now

	scratch := &s.workers[0]
	result := s.computeObserver(w, scratch, observerJob{handle: observerHandle, observerVis: observerVis}, now)

	// Commit single result
	visState.Mu.Lock()
	s.commitResult(visState, &result)
	visState.Mu.Unlock()

	// Publish events
	s.publishResultEvents(&result)
}

func (s *VisionSystem) Update(w *ecs.World, dt float64) {
	visState := ecs.GetResource[ecs.VisibilityState](w)
	now := ecs.GetResource[ecs.TimeState](w).Now

	// ---- Step A: collect jobs & dead observers (no lock — single writer in tick) ----
	s.jobs = s.jobs[:0]
	s.deadObservers = s.deadObservers[:0]

	for observerHandle, observerVis := range visState.VisibleByObserver {
		if !w.Alive(observerHandle) {
			s.deadObservers = append(s.deadObservers, deadObserverEntry{
				handle: observerHandle,
				known:  observerVis.Known,
			})
			continue
		}
		if now.Before(observerVis.NextUpdateTime) {
			continue
		}
		s.jobs = append(s.jobs, observerJob{
			handle:      observerHandle,
			observerVis: observerVis,
		})
	}

	// Cleanup dead observers under lock
	if len(s.deadObservers) > 0 {
		visState.Mu.Lock()
		for _, dead := range s.deadObservers {
			for targetHandle := range dead.known {
				if observers := visState.ObserversByVisibleTarget[targetHandle]; observers != nil {
					delete(observers, dead.handle)
					if len(observers) == 0 {
						delete(visState.ObserversByVisibleTarget, targetHandle)
					}
				}
			}
			delete(visState.VisibleByObserver, dead.handle)
		}
		visState.Mu.Unlock()
	}

	if len(s.jobs) == 0 {
		return
	}

	// ---- Step B: parallel compute (no locks, no shared writes) ----
	if cap(s.results) < len(s.jobs) {
		s.results = make([]observerResult, len(s.jobs))
	}
	s.results = s.results[:len(s.jobs)]

	if len(s.jobs) == 1 {
		// Single observer — run inline, no goroutine overhead
		s.results[0] = s.computeObserver(w, &s.workers[0], s.jobs[0], now)
	} else {
		var wg sync.WaitGroup
		wg.Add(numVisionWorkers)

		// Split jobs among workers
		jobsPerWorker := len(s.jobs) / numVisionWorkers
		remainder := len(s.jobs) % numVisionWorkers

		start := 0
		for workerIdx := 0; workerIdx < numVisionWorkers; workerIdx++ {
			end := start + jobsPerWorker
			if workerIdx < remainder {
				end++ // Distribute remainder to first workers
			}
			if end > len(s.jobs) {
				end = len(s.jobs)
			}

			go func(wIdx, sIdx, eIdx int) {
				defer wg.Done()
				scratch := &s.workers[wIdx]
				for i := sIdx; i < eIdx; i++ {
					s.results[i] = s.computeObserver(w, scratch, s.jobs[i], now)
				}
			}(workerIdx, start, end)

			start = end
		}

		wg.Wait()
	}

	// ---- Step C: commit under lock, then publish events ----
	visState.Mu.Lock()
	for i := range s.results {
		r := &s.results[i]
		// Observer may have died during computation
		if !w.Alive(r.handle) {
			if r.newVis.Known != nil {
				for targetHandle := range r.newVis.Known {
					if observers := visState.ObserversByVisibleTarget[targetHandle]; observers != nil {
						delete(observers, r.handle)
						if len(observers) == 0 {
							delete(visState.ObserversByVisibleTarget, targetHandle)
						}
					}
				}
			}
			delete(visState.VisibleByObserver, r.handle)
			continue
		}
		s.commitResult(visState, r)
	}
	visState.Mu.Unlock()

	// Publish events without lock (batch per observer)
	for i := range s.results {
		s.publishResultEvents(&s.results[i])
	}
}

// commitResult applies a single observer result to visState. Caller must hold visState.Mu.
func (s *VisionSystem) commitResult(visState *ecs.VisibilityState, r *observerResult) {
	if visState.ObserversByVisibleTarget == nil {
		visState.ObserversByVisibleTarget = make(map[types.Handle]map[types.Handle]struct{})
	}
	for _, targetHandle := range r.spawnTargets {
		observers := visState.ObserversByVisibleTarget[targetHandle]
		if observers == nil {
			observers = make(map[types.Handle]struct{}, 8)
			visState.ObserversByVisibleTarget[targetHandle] = observers
		}
		observers[r.handle] = struct{}{}
	}
	for _, targetHandle := range r.despawnTargets {
		if observers := visState.ObserversByVisibleTarget[targetHandle]; observers != nil {
			delete(observers, r.handle)
			if len(observers) == 0 {
				delete(visState.ObserversByVisibleTarget, targetHandle)
			}
		}
	}
	visState.VisibleByObserver[r.handle] = r.newVis
}

// publishResultEvents publishes spawn/despawn events for one observer result.
func (s *VisionSystem) publishResultEvents(r *observerResult) {
	for i := range r.spawns {
		sp := &r.spawns[i]
		s.eventBus.PublishAsync(
			ecs.NewEntitySpawnEvent(sp.observerID, sp.targetID, sp.targetHandle, sp.layer),
			eventbus.PriorityMedium,
		)
	}
	for i := range r.despawns {
		dp := &r.despawns[i]
		s.eventBus.PublishAsync(
			ecs.NewEntityDespawnEvent(dp.observerID, dp.targetID, dp.layer),
			eventbus.PriorityMedium,
		)
	}
}

// computeObserver performs the full vision computation for one observer using
// worker-local scratch. It reads components (read-only) and returns a result
// that will be committed later. No locks, no shared writes.
func (s *VisionSystem) computeObserver(
	w *ecs.World,
	scratch *visionWorkerScratch,
	job observerJob,
	now time.Time,
) observerResult {
	observerHandle := job.handle
	observerVis := job.observerVis

	observerTransform, ok := s.transformStorage.Get(observerHandle)
	if !ok {
		return observerResult{handle: observerHandle, newVis: observerVis, skipOnly: true}
	}

	vision, ok := s.visionStorage.Get(observerHandle)
	if !ok {
		return observerResult{handle: observerHandle, newVis: observerVis, skipOnly: true}
	}

	chunkRef, ok := s.chunkRefStorage.Get(observerHandle)
	if !ok {
		return observerResult{handle: observerHandle, newVis: observerVis, skipOnly: true}
	}

	// --- Dirty-flag skip ---
	if s.canSkipUpdate(&observerVis, observerTransform) {
		observerVis.NextUpdateTime = now.Add(_const.VisionUpdateInterval + jitterDuration())
		return observerResult{handle: observerHandle, newVis: observerVis, skipOnly: true}
	}

	observerID, _ := w.GetExternalID(observerHandle)

	visionRadius := CalcMaxVisionRadius(vision)
	visionRadiusSq := visionRadius * visionRadius

	// --- Spatial query (worker-local buffers) ---
	scratch.candidatesBuffer = scratch.candidatesBuffer[:0]
	scratch.queriedGens = scratch.queriedGens[:0]
	s.findCandidatesW(scratch, observerTransform.X, observerTransform.Y, visionRadius, chunkRef)

	// --- Filter candidates into sorted visible slice ---
	scratch.newVisibleBuf = scratch.newVisibleBuf[:0]
	for _, candidateHandle := range scratch.candidatesBuffer {
		if !w.Alive(candidateHandle) {
			continue
		}

		// Always include self in visible set for proper ObserversByVisibleTarget mapping
		if candidateHandle == observerHandle {
			scratch.newVisibleBuf = append(scratch.newVisibleBuf, visibleEntry{candidateHandle, observerID})
			continue
		}

		candidateTransform, ok := s.transformStorage.Get(candidateHandle)
		if !ok {
			continue
		}

		dx := candidateTransform.X - observerTransform.X
		dy := candidateTransform.Y - observerTransform.Y
		distSq := dx*dx + dy*dy

		if distSq > visionRadiusSq {
			continue
		}

		var targetStealth float64 = 0
		if stealth, hasStealth := s.stealthStorage.Get(candidateHandle); hasStealth {
			targetStealth = stealth.Value
		}

		if CalcVision(distSq, visionRadius, vision.Power, targetStealth) {
			entityID, ok := w.GetExternalID(candidateHandle)
			if ok {
				scratch.newVisibleBuf = append(scratch.newVisibleBuf, visibleEntry{candidateHandle, entityID})
			}
		}
	}

	// Sort by Handle for O(log n) binary search in despawn diff
	slices.SortFunc(scratch.newVisibleBuf, func(a, b visibleEntry) int {
		if a.Handle < b.Handle {
			return -1
		}
		if a.Handle > b.Handle {
			return 1
		}
		return 0
	})

	// --- Diff: spawn / despawn ---
	oldKnown := observerVis.Known
	if oldKnown == nil {
		oldKnown = make(map[types.Handle]types.EntityID, 32)
	}

	var res observerResult
	res.handle = observerHandle

	for _, entry := range scratch.newVisibleBuf {
		if _, wasKnown := oldKnown[entry.Handle]; !wasKnown {
			layer := 0
			if info, ok := s.entityInfoStorage.Get(entry.Handle); ok {
				layer = info.Layer
			}
			res.spawns = append(res.spawns, spawnEventData{
				observerID:   observerID,
				targetID:     entry.EntityID,
				targetHandle: entry.Handle,
				layer:        layer,
			})
			res.spawnTargets = append(res.spawnTargets, entry.Handle)
		}
	}

	for targetHandle, targetID := range oldKnown {
		if !isInSortedVisible(scratch.newVisibleBuf, targetHandle) {
			layer := 0
			if info, ok := s.entityInfoStorage.Get(targetHandle); ok {
				layer = info.Layer
			}
			res.despawns = append(res.despawns, despawnEventData{
				observerID: observerID,
				targetID:   targetID,
				layer:      layer,
			})
			res.despawnTargets = append(res.despawnTargets, targetHandle)
		}
	}

	// --- Build new Known map (don't mutate old one) ---
	newKnown := make(map[types.Handle]types.EntityID, len(scratch.newVisibleBuf))
	for _, entry := range scratch.newVisibleBuf {
		newKnown[entry.Handle] = entry.EntityID
	}

	res.newVis = ecs.ObserverVisibility{
		Known:          newKnown,
		LastX:          observerTransform.X,
		LastY:          observerTransform.Y,
		LastChunkX:     chunkRef.CurrentChunkX,
		LastChunkY:     chunkRef.CurrentChunkY,
		LastChunkGens:  append([]ecs.ChunkGen(nil), scratch.queriedGens...),
		NextUpdateTime: now.Add(_const.VisionUpdateInterval + jitterDuration()),
	}

	return res
}

// ---------------- helpers ----------------

func (s *VisionSystem) canSkipUpdate(
	observerVis *ecs.ObserverVisibility,
	transform components.Transform,
) bool {
	if len(observerVis.LastChunkGens) == 0 {
		return false
	}

	dx := transform.X - observerVis.LastX
	dy := transform.Y - observerVis.LastY
	if dx*dx+dy*dy > _const.VisionPosEpsilon*_const.VisionPosEpsilon {
		return false
	}

	for _, cg := range observerVis.LastChunkGens {
		chunk := s.chunkManager.GetChunk(cg.Coord)
		if chunk == nil {
			return false
		}
		if chunk.Spatial().Generation() != cg.Gen {
			return false
		}
	}

	return true
}

func (s *VisionSystem) findCandidatesW(scratch *visionWorkerScratch, x, y, radius float64, chunkRef components.ChunkRef) {
	chunkCoord := types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}
	chunk := s.chunkManager.GetChunk(chunkCoord)
	if chunk == nil {
		return
	}

	chunkWorldSize := float64(_const.ChunkWorldSize)
	chunkMinX := float64(chunkCoord.X) * chunkWorldSize
	chunkMaxX := float64(chunkCoord.X+1) * chunkWorldSize
	chunkMinY := float64(chunkCoord.Y) * chunkWorldSize
	chunkMaxY := float64(chunkCoord.Y+1) * chunkWorldSize

	// Если круг целиком внутри чанка — соседей не трогаем
	localOnly := x-radius >= chunkMinX && x+radius <= chunkMaxX && y-radius >= chunkMinY && y+radius <= chunkMaxY

	scratch.queriedGens = append(scratch.queriedGens, ecs.ChunkGen{Coord: chunkCoord, Gen: chunk.Spatial().Generation()})
	chunk.Spatial().QueryRadius(x, y, radius, &scratch.candidatesBuffer)
	if localOnly {
		return
	}

	queryMinX := x - radius
	queryMaxX := x + radius
	queryMinY := y - radius
	queryMaxY := y + radius

	neighborOffsets := [8]struct{ dx, dy int }{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	for _, offset := range neighborOffsets {
		neighborChunkX := chunkCoord.X + offset.dx
		neighborChunkY := chunkCoord.Y + offset.dy

		neighborMinX := float64(neighborChunkX) * chunkWorldSize
		neighborMaxX := float64(neighborChunkX+1) * chunkWorldSize
		neighborMinY := float64(neighborChunkY) * chunkWorldSize
		neighborMaxY := float64(neighborChunkY+1) * chunkWorldSize

		intersects := !(queryMaxX < neighborMinX || queryMinX > neighborMaxX ||
			queryMaxY < neighborMinY || queryMinY > neighborMaxY)

		if intersects {
			neighborCoord := types.ChunkCoord{X: neighborChunkX, Y: neighborChunkY}
			neighborChunk := s.chunkManager.GetChunk(neighborCoord)
			if neighborChunk != nil {
				scratch.queriedGens = append(scratch.queriedGens, ecs.ChunkGen{Coord: neighborCoord, Gen: neighborChunk.Spatial().Generation()})
				neighborChunk.Spatial().QueryRadius(x, y, radius, &scratch.candidatesBuffer)
			}
		}
	}
}

func isInSortedVisible(buf []visibleEntry, h types.Handle) bool {
	lo, hi := 0, len(buf)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if buf[mid].Handle < h {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(buf) && buf[lo].Handle == h
}

func CalcMaxVisionRadius(vision components.Vision) float64 {
	// TODO formulae
	return vision.Radius
}

func CalcVision(distSq float64, maxRadius float64, power float64, targetStealth float64) bool {
	effectiveRange := power - targetStealth
	if effectiveRange <= 0 {
		return false
	}
	effectiveRangeSq := effectiveRange * effectiveRange
	if effectiveRange > maxRadius {
		effectiveRangeSq = maxRadius * maxRadius
	}
	return distSq <= effectiveRangeSq
}

func jitterDuration() time.Duration {
	if _const.VisionUpdateJitter <= 0 {
		return 0
	}
	// Uniform [-jitter, +jitter]
	rangeNs := int64(_const.VisionUpdateJitter * 2)
	offset := rand.Int63n(rangeNs) - int64(_const.VisionUpdateJitter)
	return time.Duration(offset)
}
