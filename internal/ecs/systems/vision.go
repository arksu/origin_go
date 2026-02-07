package systems

import (
	"math"
	_const "origin/internal/const"
	"time"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	visionUpdateInterval = 1 * time.Second
)

type VisionSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	eventBus     *eventbus.EventBus
	logger       *zap.Logger

	candidatesBuffer []types.Handle
	newVisibleSet    map[types.Handle]struct{}

	transformStorage *ecs.ComponentStorage[components.Transform]
	visionStorage    *ecs.ComponentStorage[components.Vision]
	stealthStorage   *ecs.ComponentStorage[components.Stealth]
	chunkRefStorage  *ecs.ComponentStorage[components.ChunkRef]
}

func NewVisionSystem(
	world *ecs.World,
	chunkManager core.ChunkManager,
	eventBus *eventbus.EventBus,
	logger *zap.Logger,
) *VisionSystem {
	return &VisionSystem{
		BaseSystem:       ecs.NewBaseSystem("VisionSystem", 350),
		chunkManager:     chunkManager,
		eventBus:         eventBus,
		logger:           logger,
		candidatesBuffer: make([]types.Handle, 0, 256),
		newVisibleSet:    make(map[types.Handle]struct{}, 128),
		transformStorage: ecs.GetOrCreateStorage[components.Transform](world),
		visionStorage:    ecs.GetOrCreateStorage[components.Vision](world),
		stealthStorage:   ecs.GetOrCreateStorage[components.Stealth](world),
		chunkRefStorage:  ecs.GetOrCreateStorage[components.ChunkRef](world),
	}
}

func (s *VisionSystem) Update(w *ecs.World, dt float64) {
	visState := w.VisibilityState()
	if visState == nil {
		return
	}

	now := time.Now()

	for observerHandle, observerVis := range visState.VisibleByObserver {
		if !w.Alive(observerHandle) {
			s.cleanupDeadObserver(w, visState, observerHandle, observerVis.Known)
			continue
		}

		if now.Before(observerVis.NextUpdateTime) {
			continue
		}

		s.updateObserverVisibility(w, visState, observerHandle, &observerVis, now)
		visState.Mu.Lock()
		visState.VisibleByObserver[observerHandle] = observerVis
		visState.Mu.Unlock()
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

	observerID, _ := w.GetExternalID(observerHandle)

	visionRadius := CalcMaxVisionRadius(vision)

	s.candidatesBuffer = s.candidatesBuffer[:0]
	s.findCandidates(observerTransform.X, observerTransform.Y, visionRadius, chunkRef)

	for k := range s.newVisibleSet {
		delete(s.newVisibleSet, k)
	}

	for _, candidateHandle := range s.candidatesBuffer {
		if !w.Alive(candidateHandle) {
			continue
		}

		// Always include self in visible set for proper ObserversByVisibleTarget mapping
		if candidateHandle == observerHandle {
			s.newVisibleSet[candidateHandle] = struct{}{}
			continue
		}

		candidateTransform, ok := s.transformStorage.Get(candidateHandle)
		if !ok {
			continue
		}

		dx := candidateTransform.X - observerTransform.X
		dy := candidateTransform.Y - observerTransform.Y
		distSq := dx*dx + dy*dy
		distance := math.Sqrt(distSq)

		if distance > visionRadius {
			continue
		}

		var targetStealth float64 = 0
		if stealth, hasStealth := s.stealthStorage.Get(candidateHandle); hasStealth {
			targetStealth = stealth.Value
		}

		if CalcVision(distance, vision.Power, targetStealth) {
			s.newVisibleSet[candidateHandle] = struct{}{}
		}
	}

	oldKnown := observerVis.Known
	if oldKnown == nil {
		oldKnown = make(map[types.Handle]types.EntityID, 32)
	}

	for targetHandle := range s.newVisibleSet {
		if _, wasKnown := oldKnown[targetHandle]; !wasKnown {
			targetID, ok := w.GetExternalID(targetHandle)
			if !ok {
				continue
			}

			// Get target entity layer
			targetEntityInfo, hasTargetEntityInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
			layer := 0 // default layer
			if hasTargetEntityInfo {
				layer = targetEntityInfo.Layer
			}

			s.addToObserversByTarget(visState, targetHandle, observerHandle)

			s.eventBus.PublishAsync(
				ecs.NewEntitySpawnEvent(observerID, targetID, targetHandle, layer),
				eventbus.PriorityMedium,
			)
		}
	}

	for targetHandle, targetID := range oldKnown {
		if _, stillVisible := s.newVisibleSet[targetHandle]; !stillVisible {
			// Use saved EntityID from Known map (works even if entity is already despawned)

			// Get target entity layer
			targetEntityInfo, hasTargetEntityInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
			layer := 0 // default layer
			if hasTargetEntityInfo {
				layer = targetEntityInfo.Layer
			}

			s.removeFromObserversByTarget(visState, targetHandle, observerHandle)

			s.eventBus.PublishAsync(
				ecs.NewEntityDespawnEvent(observerID, targetID, layer),
				eventbus.PriorityMedium,
			)
		}
	}

	if observerVis.Known == nil {
		observerVis.Known = make(map[types.Handle]types.EntityID, len(s.newVisibleSet))
	} else {
		for k := range observerVis.Known {
			delete(observerVis.Known, k)
		}
	}
	for h := range s.newVisibleSet {
		// Save EntityID for later despawn events
		entityID, ok := w.GetExternalID(h)
		if ok {
			observerVis.Known[h] = entityID
		}
	}
	//s.logger.Debug("VisionSystem updated", zap.Any("newVisibleSet", s.newVisibleSet), zap.Any("observerVis", observerVis))

	observerVis.NextUpdateTime = now.Add(visionUpdateInterval)
}

func (s *VisionSystem) findCandidates(x, y, radius float64, chunkRef components.ChunkRef) {
	chunkCoord := types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}
	chunk := s.chunkManager.GetChunk(chunkCoord)
	if chunk == nil {
		return
	}

	chunk.Spatial().QueryRadius(x, y, radius, &s.candidatesBuffer)

	chunkWorldSize := float64(_const.ChunkWorldSize)

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
	for targetHandle := range known {
		s.removeFromObserversByTarget(visState, targetHandle, observerHandle)
	}
	delete(visState.VisibleByObserver, observerHandle)
}

func (s *VisionSystem) addToObserversByTarget(
	visState *ecs.VisibilityState,
	targetHandle, observerHandle types.Handle,
) {
	visState.Mu.Lock()
	defer visState.Mu.Unlock()

	if visState.ObserversByVisibleTarget == nil {
		visState.ObserversByVisibleTarget = make(map[types.Handle]map[types.Handle]struct{})
	}
	observers := visState.ObserversByVisibleTarget[targetHandle]
	if observers == nil {
		observers = make(map[types.Handle]struct{}, 8)
		visState.ObserversByVisibleTarget[targetHandle] = observers
	}
	observers[observerHandle] = struct{}{}
}

func (s *VisionSystem) removeFromObserversByTarget(
	visState *ecs.VisibilityState,
	targetHandle, observerHandle types.Handle,
) {
	visState.Mu.Lock()
	defer visState.Mu.Unlock()

	if visState.ObserversByVisibleTarget == nil {
		return
	}
	observers := visState.ObserversByVisibleTarget[targetHandle]
	if observers == nil {
		return
	}
	delete(observers, observerHandle)
	if len(observers) == 0 {
		delete(visState.ObserversByVisibleTarget, targetHandle)
	}
}

func CalcMaxVisionRadius(vision components.Vision) float64 {
	// TODO formulae
	return vision.Radius
}

func CalcVision(distance float64, power float64, targetStealth float64) bool {
	effectiveRange := power - targetStealth
	if effectiveRange <= 0 {
		return false
	}
	return distance <= effectiveRange
}
