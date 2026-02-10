package systems

import (
	"math/rand"
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
const visionMetricsLogInterval = 5 * time.Second

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
	skipDirty      bool
	computeStats   observerComputeStats
}

type deadObserverEntry struct {
	handle types.Handle
	known  map[types.Handle]types.EntityID
}

type observerComputeStats struct {
	findCandidatesDur time.Duration
	filterDur         time.Duration
	diffDur           time.Duration
	chunksQueried     int
	queryRadiusCalls  int
	candidates        int
	visible           int
}

type visionMetricsWindow struct {
	windowStart time.Time

	updates            uint64
	forceUpdates       uint64
	observersTotal     uint64
	observersDue       uint64
	observersSkipDirty uint64

	chunksQueriedTotal uint64
	queryRadiusCalls   uint64
	candidatesTotal    uint64
	visibleTotal       uint64
	spawnEvents        uint64
	despawnEvents      uint64

	collectDur        time.Duration
	computeDur        time.Duration
	commitDur         time.Duration
	publishDur        time.Duration
	findCandidatesDur time.Duration
	filterDur         time.Duration
	diffDur           time.Duration
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
	externalIDStorage *ecs.ComponentStorage[ecs.ExternalID]

	metrics visionMetricsWindow
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
		externalIDStorage: ecs.GetOrCreateStorage[ecs.ExternalID](world),
		metrics: visionMetricsWindow{
			windowStart: time.Now(),
		},
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
	computeStart := time.Now()
	result := s.computeObserver(w, scratch, observerJob{handle: observerHandle, observerVis: observerVis}, now)
	computeDur := time.Since(computeStart)

	// Commit single result
	commitStart := time.Now()
	visState.Mu.Lock()
	s.commitResult(visState, &result)
	visState.Mu.Unlock()
	commitDur := time.Since(commitStart)

	// Publish events
	publishStart := time.Now()
	s.publishResultEvents(&result)
	publishDur := time.Since(publishStart)

	s.accumulateForceMetrics(&result, computeDur, commitDur, publishDur)
	s.maybeLogMetrics()
}

func (s *VisionSystem) Update(w *ecs.World, dt float64) {
	visState := ecs.GetResource[ecs.VisibilityState](w)
	now := ecs.GetResource[ecs.TimeState](w).Now
	observersTotal := len(visState.VisibleByObserver)

	var collectDur time.Duration
	var computeDur time.Duration
	var commitDur time.Duration
	var publishDur time.Duration
	var findCandidatesDur time.Duration
	var filterDur time.Duration
	var diffDur time.Duration
	var observersSkipDirty int
	var chunksQueriedTotal int
	var queryRadiusCalls int
	var candidatesTotal int
	var visibleTotal int
	var spawnEvents int
	var despawnEvents int

	// ---- Step A: collect jobs & dead observers (no lock — single writer in tick) ----
	collectStart := time.Now()
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
	collectDur = time.Since(collectStart)

	if len(s.jobs) == 0 {
		s.emitVisionTimings(w, collectDur, 0, 0, 0, 0, 0, 0)
		s.accumulateTickMetrics(
			observersTotal, 0, 0,
			0, 0, 0, 0,
			0, 0,
			collectDur, 0, 0, 0,
			0, 0, 0,
		)
		s.maybeLogMetrics()
		return
	}

	// ---- Step B: parallel compute (no locks, no shared writes) ----
	computeStart := time.Now()
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
	computeDur = time.Since(computeStart)

	for i := range s.results {
		r := &s.results[i]
		if r.skipDirty {
			observersSkipDirty++
		}
		findCandidatesDur += r.computeStats.findCandidatesDur
		filterDur += r.computeStats.filterDur
		diffDur += r.computeStats.diffDur
		chunksQueriedTotal += r.computeStats.chunksQueried
		queryRadiusCalls += r.computeStats.queryRadiusCalls
		candidatesTotal += r.computeStats.candidates
		visibleTotal += r.computeStats.visible
		spawnEvents += len(r.spawns)
		despawnEvents += len(r.despawns)
	}

	// ---- Step C: commit under lock, then publish events ----
	commitStart := time.Now()
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
	commitDur = time.Since(commitStart)

	// Publish events without lock (batch per observer)
	publishStart := time.Now()
	for i := range s.results {
		s.publishResultEvents(&s.results[i])
	}
	publishDur = time.Since(publishStart)

	s.emitVisionTimings(
		w,
		collectDur, computeDur, commitDur, publishDur,
		findCandidatesDur, filterDur, diffDur,
	)
	s.accumulateTickMetrics(
		observersTotal, len(s.jobs), observersSkipDirty,
		chunksQueriedTotal, queryRadiusCalls, candidatesTotal, visibleTotal,
		spawnEvents, despawnEvents,
		collectDur, computeDur, commitDur, publishDur,
		findCandidatesDur, filterDur, diffDur,
	)
	s.maybeLogMetrics()
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
		return observerResult{handle: observerHandle, newVis: observerVis, skipOnly: true, skipDirty: true}
	}

	observerExt, ok := s.externalIDStorage.Get(observerHandle)
	if !ok {
		return observerResult{handle: observerHandle, newVis: observerVis, skipOnly: true}
	}
	observerID := observerExt.ID
	var stats observerComputeStats

	visionRadius := CalcMaxVisionRadius(vision)
	visionRadiusSq := visionRadius * visionRadius

	// --- Spatial query (worker-local buffers) ---
	scratch.candidatesBuffer = scratch.candidatesBuffer[:0]
	scratch.queriedGens = scratch.queriedGens[:0]
	findStart := time.Now()
	s.findCandidatesW(scratch, observerTransform.X, observerTransform.Y, visionRadius, chunkRef)
	stats.findCandidatesDur = time.Since(findStart)
	stats.chunksQueried = len(scratch.queriedGens)
	stats.queryRadiusCalls = len(scratch.queriedGens)
	stats.candidates = len(scratch.candidatesBuffer)

	// --- Filter candidates into visible slice ---
	filterStart := time.Now()
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
			if ext, hasExt := s.externalIDStorage.Get(candidateHandle); hasExt {
				scratch.newVisibleBuf = append(scratch.newVisibleBuf, visibleEntry{candidateHandle, ext.ID})
			}
		}
	}
	stats.filterDur = time.Since(filterStart)
	stats.visible = len(scratch.newVisibleBuf)

	// --- Diff: spawn / despawn ---
	diffStart := time.Now()
	oldKnown := observerVis.Known
	if oldKnown == nil {
		oldKnown = make(map[types.Handle]types.EntityID, 32)
	}

	var res observerResult
	res.handle = observerHandle
	res.computeStats = stats

	newKnown := make(map[types.Handle]types.EntityID, len(scratch.newVisibleBuf))
	for _, entry := range scratch.newVisibleBuf {
		newKnown[entry.Handle] = entry.EntityID
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
		if _, stillVisible := newKnown[targetHandle]; !stillVisible {
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
	res.computeStats.diffDur = time.Since(diffStart)

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
		chunk := s.getChunk(cg.Coord)
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
	chunk := s.getChunk(chunkCoord)
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
			neighborChunk := s.getChunk(neighborCoord)
			if neighborChunk != nil {
				scratch.queriedGens = append(scratch.queriedGens, ecs.ChunkGen{Coord: neighborCoord, Gen: neighborChunk.Spatial().Generation()})
				neighborChunk.Spatial().QueryRadius(x, y, radius, &scratch.candidatesBuffer)
			}
		}
	}
}

func (s *VisionSystem) getChunk(coord types.ChunkCoord) *core.Chunk {
	return s.chunkManager.GetChunkFast(coord)
}

func (s *VisionSystem) emitVisionTimings(
	w *ecs.World,
	collectDur, computeDur, commitDur, publishDur time.Duration,
	findCandidatesDur, filterDur, diffDur time.Duration,
) {
	w.AddExternalTiming("VisionCollect", collectDur)
	w.AddExternalTiming("VisionCompute", computeDur)
	w.AddExternalTiming("VisionCommit", commitDur)
	w.AddExternalTiming("VisionPublish", publishDur)
	w.AddExternalTiming("VisionFindCandidates", findCandidatesDur)
	w.AddExternalTiming("VisionFilter", filterDur)
	w.AddExternalTiming("VisionDiff", diffDur)
}

func (s *VisionSystem) accumulateTickMetrics(
	observersTotal, observersDue, observersSkipDirty int,
	chunksQueriedTotal, queryRadiusCalls, candidatesTotal, visibleTotal int,
	spawnEvents, despawnEvents int,
	collectDur, computeDur, commitDur, publishDur time.Duration,
	findCandidatesDur, filterDur, diffDur time.Duration,
) {
	m := &s.metrics
	m.updates++
	m.observersTotal += uint64(observersTotal)
	m.observersDue += uint64(observersDue)
	m.observersSkipDirty += uint64(observersSkipDirty)
	m.chunksQueriedTotal += uint64(chunksQueriedTotal)
	m.queryRadiusCalls += uint64(queryRadiusCalls)
	m.candidatesTotal += uint64(candidatesTotal)
	m.visibleTotal += uint64(visibleTotal)
	m.spawnEvents += uint64(spawnEvents)
	m.despawnEvents += uint64(despawnEvents)

	m.collectDur += collectDur
	m.computeDur += computeDur
	m.commitDur += commitDur
	m.publishDur += publishDur
	m.findCandidatesDur += findCandidatesDur
	m.filterDur += filterDur
	m.diffDur += diffDur
}

func (s *VisionSystem) accumulateForceMetrics(
	result *observerResult,
	computeDur, commitDur, publishDur time.Duration,
) {
	m := &s.metrics
	m.forceUpdates++
	m.chunksQueriedTotal += uint64(result.computeStats.chunksQueried)
	m.queryRadiusCalls += uint64(result.computeStats.queryRadiusCalls)
	m.candidatesTotal += uint64(result.computeStats.candidates)
	m.visibleTotal += uint64(result.computeStats.visible)
	m.spawnEvents += uint64(len(result.spawns))
	m.despawnEvents += uint64(len(result.despawns))

	m.computeDur += computeDur
	m.commitDur += commitDur
	m.publishDur += publishDur
	m.findCandidatesDur += result.computeStats.findCandidatesDur
	m.filterDur += result.computeStats.filterDur
	m.diffDur += result.computeStats.diffDur
}

func (s *VisionSystem) maybeLogMetrics() {
	now := time.Now()
	m := &s.metrics
	if m.windowStart.IsZero() {
		m.windowStart = now
	}
	if now.Sub(m.windowStart) < visionMetricsLogInterval {
		return
	}

	var duePerUpdate float64
	var candidatesPerDue float64
	var visiblePerDue float64
	var chunksPerDue float64
	var skipDirtyRate float64
	if m.updates > 0 {
		duePerUpdate = float64(m.observersDue) / float64(m.updates)
	}
	if m.observersDue > 0 {
		candidatesPerDue = float64(m.candidatesTotal) / float64(m.observersDue)
		visiblePerDue = float64(m.visibleTotal) / float64(m.observersDue)
		chunksPerDue = float64(m.chunksQueriedTotal) / float64(m.observersDue)
		skipDirtyRate = float64(m.observersSkipDirty) / float64(m.observersDue)
	}

	s.logger.Info("Vision metrics (5s)",
		zap.Uint64("updates", m.updates),
		zap.Uint64("observers_total", m.observersTotal),
		zap.Uint64("observers_due", m.observersDue),
		zap.Uint64("observers_skipped_dirty", m.observersSkipDirty),
		zap.Uint64("force_updates", m.forceUpdates),
		zap.Uint64("chunks_queried_total", m.chunksQueriedTotal),
		zap.Uint64("query_radius_calls", m.queryRadiusCalls),
		zap.Uint64("candidates_total", m.candidatesTotal),
		zap.Uint64("visible_total", m.visibleTotal),
		zap.Uint64("spawn_events", m.spawnEvents),
		zap.Uint64("despawn_events", m.despawnEvents),
		zap.Float64("due_per_update", duePerUpdate),
		zap.Float64("chunks_per_due", chunksPerDue),
		zap.Float64("candidates_per_due", candidatesPerDue),
		zap.Float64("visible_per_due", visiblePerDue),
		zap.Float64("skip_dirty_rate", skipDirtyRate),
		zap.Duration("collect", m.collectDur),
		zap.Duration("compute", m.computeDur),
		zap.Duration("commit", m.commitDur),
		zap.Duration("publish", m.publishDur),
		zap.Duration("find_candidates", m.findCandidatesDur),
		zap.Duration("filter", m.filterDur),
		zap.Duration("diff", m.diffDur),
	)

	s.metrics = visionMetricsWindow{windowStart: now}
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
