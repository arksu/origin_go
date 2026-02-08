package systems

import (
	"math/rand"
	_const "origin/internal/const"
	"slices"
	"time"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

type visibleEntry struct {
	Handle   types.Handle
	EntityID types.EntityID
}

type VisionSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	eventBus     *eventbus.EventBus
	logger       *zap.Logger

	candidatesBuffer []types.Handle
	newVisibleBuf    []visibleEntry
	spawnTargets     []types.Handle
	despawnTargets   []types.Handle
	queriedGens      []ecs.ChunkGen

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
	return &VisionSystem{
		BaseSystem:        ecs.NewBaseSystem("VisionSystem", 350),
		chunkManager:      chunkManager,
		eventBus:          eventBus,
		logger:            logger,
		candidatesBuffer:  make([]types.Handle, 0, 256),
		newVisibleBuf:     make([]visibleEntry, 0, 256),
		spawnTargets:      make([]types.Handle, 0, 64),
		despawnTargets:    make([]types.Handle, 0, 64),
		queriedGens:       make([]ecs.ChunkGen, 0, 9),
		transformStorage:  ecs.GetOrCreateStorage[components.Transform](world),
		visionStorage:     ecs.GetOrCreateStorage[components.Vision](world),
		stealthStorage:    ecs.GetOrCreateStorage[components.Stealth](world),
		chunkRefStorage:   ecs.GetOrCreateStorage[components.ChunkRef](world),
		entityInfoStorage: ecs.GetOrCreateStorage[components.EntityInfo](world),
	}
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

	// Update visibility immediately
	s.updateObserverVisibility(w, visState, observerHandle, &observerVis, now)
}

func (s *VisionSystem) Update(w *ecs.World, dt float64) {
	visState := ecs.GetResource[ecs.VisibilityState](w)

	now := ecs.GetResource[ecs.TimeState](w).Now

	for observerHandle, observerVis := range visState.VisibleByObserver {
		if !w.Alive(observerHandle) {
			s.cleanupDeadObserver(w, visState, observerHandle, observerVis.Known)
			continue
		}

		if now.Before(observerVis.NextUpdateTime) {
			continue
		}

		s.updateObserverVisibility(w, visState, observerHandle, &observerVis, now)
	}
}

func (s *VisionSystem) updateObserverVisibility(
	w *ecs.World,
	visState *ecs.VisibilityState,
	observerHandle types.Handle,
	observerVis *ecs.ObserverVisibility,
	now time.Time,
) {
	observerTransform, ok := s.transformStorage.Get(observerHandle)
	if !ok {
		return
	}

	vision, ok := s.visionStorage.Get(observerHandle)
	if !ok {
		return
	}

	chunkRef, ok := s.chunkRefStorage.Get(observerHandle)
	if !ok {
		return
	}

	// --- Dirty-flag skip (2.1) ---
	if s.canSkipUpdate(observerVis, observerTransform) {
		observerVis.NextUpdateTime = now.Add(_const.VisionUpdateInterval + jitterDuration())
		visState.Mu.Lock()
		visState.VisibleByObserver[observerHandle] = *observerVis
		visState.Mu.Unlock()
		return
	}

	observerID, _ := w.GetExternalID(observerHandle)

	visionRadius := CalcMaxVisionRadius(vision)
	visionRadiusSq := visionRadius * visionRadius

	// --- Spatial query ---
	s.candidatesBuffer = s.candidatesBuffer[:0]
	s.queriedGens = s.queriedGens[:0]
	s.findCandidates(observerTransform.X, observerTransform.Y, visionRadius, chunkRef)

	// --- Filter candidates into sorted visible slice (1.1 + 1.2) ---
	s.newVisibleBuf = s.newVisibleBuf[:0]
	for _, candidateHandle := range s.candidatesBuffer {
		if !w.Alive(candidateHandle) {
			continue
		}

		// Always include self in visible set for proper ObserversByVisibleTarget mapping
		if candidateHandle == observerHandle {
			s.newVisibleBuf = append(s.newVisibleBuf, visibleEntry{candidateHandle, observerID})
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
				s.newVisibleBuf = append(s.newVisibleBuf, visibleEntry{candidateHandle, entityID})
			}
		}
	}

	// Sort by Handle for O(log n) binary search in despawn diff
	slices.SortFunc(s.newVisibleBuf, func(a, b visibleEntry) int {
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

	s.spawnTargets = s.spawnTargets[:0]
	for _, entry := range s.newVisibleBuf {
		if _, wasKnown := oldKnown[entry.Handle]; !wasKnown {
			layer := 0
			if info, ok := s.entityInfoStorage.Get(entry.Handle); ok {
				layer = info.Layer
			}
			s.eventBus.PublishAsync(
				ecs.NewEntitySpawnEvent(observerID, entry.EntityID, entry.Handle, layer),
				eventbus.PriorityMedium,
			)
			s.spawnTargets = append(s.spawnTargets, entry.Handle)
		}
	}

	s.despawnTargets = s.despawnTargets[:0]
	for targetHandle, targetID := range oldKnown {
		if !s.isInNewVisible(targetHandle) {
			layer := 0
			if info, ok := s.entityInfoStorage.Get(targetHandle); ok {
				layer = info.Layer
			}
			s.eventBus.PublishAsync(
				ecs.NewEntityDespawnEvent(observerID, targetID, layer),
				eventbus.PriorityMedium,
			)
			s.despawnTargets = append(s.despawnTargets, targetHandle)
		}
	}

	// --- Rebuild Known from cached EntityIDs (1.2) ---
	if observerVis.Known == nil {
		observerVis.Known = make(map[types.Handle]types.EntityID, len(s.newVisibleBuf))
	} else {
		for k := range observerVis.Known {
			delete(observerVis.Known, k)
		}
	}
	for _, entry := range s.newVisibleBuf {
		observerVis.Known[entry.Handle] = entry.EntityID
	}

	observerVis.LastX = observerTransform.X
	observerVis.LastY = observerTransform.Y
	observerVis.LastChunkX = chunkRef.CurrentChunkX
	observerVis.LastChunkY = chunkRef.CurrentChunkY
	observerVis.LastChunkGens = append(observerVis.LastChunkGens[:0], s.queriedGens...)
	observerVis.NextUpdateTime = now.Add(_const.VisionUpdateInterval + jitterDuration())

	// --- Batch VisibilityState mutations (1.3) ---
	visState.Mu.Lock()
	if visState.ObserversByVisibleTarget == nil {
		visState.ObserversByVisibleTarget = make(map[types.Handle]map[types.Handle]struct{})
	}
	for _, targetHandle := range s.spawnTargets {
		observers := visState.ObserversByVisibleTarget[targetHandle]
		if observers == nil {
			observers = make(map[types.Handle]struct{}, 8)
			visState.ObserversByVisibleTarget[targetHandle] = observers
		}
		observers[observerHandle] = struct{}{}
	}
	for _, targetHandle := range s.despawnTargets {
		if observers := visState.ObserversByVisibleTarget[targetHandle]; observers != nil {
			delete(observers, observerHandle)
			if len(observers) == 0 {
				delete(visState.ObserversByVisibleTarget, targetHandle)
			}
		}
	}
	visState.VisibleByObserver[observerHandle] = *observerVis
	visState.Mu.Unlock()
}

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

func (s *VisionSystem) findCandidates(x, y, radius float64, chunkRef components.ChunkRef) {
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

	s.queriedGens = append(s.queriedGens, ecs.ChunkGen{Coord: chunkCoord, Gen: chunk.Spatial().Generation()})
	chunk.Spatial().QueryRadius(x, y, radius, &s.candidatesBuffer)
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
				s.queriedGens = append(s.queriedGens, ecs.ChunkGen{Coord: neighborCoord, Gen: neighborChunk.Spatial().Generation()})
				neighborChunk.Spatial().QueryRadius(x, y, radius, &s.candidatesBuffer)
			}
		}
	}
}

func (s *VisionSystem) cleanupDeadObserver(
	w *ecs.World,
	visState *ecs.VisibilityState,
	observerHandle types.Handle,
	known map[types.Handle]types.EntityID,
) {
	visState.Mu.Lock()
	for targetHandle := range known {
		if observers := visState.ObserversByVisibleTarget[targetHandle]; observers != nil {
			delete(observers, observerHandle)
			if len(observers) == 0 {
				delete(visState.ObserversByVisibleTarget, targetHandle)
			}
		}
	}
	delete(visState.VisibleByObserver, observerHandle)
	visState.Mu.Unlock()
}

func (s *VisionSystem) isInNewVisible(h types.Handle) bool {
	lo, hi := 0, len(s.newVisibleBuf)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if s.newVisibleBuf[mid].Handle < h {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(s.newVisibleBuf) && s.newVisibleBuf[lo].Handle == h
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
