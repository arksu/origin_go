package behaviors

import (
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

// NPCMovementGuard runs after collision and before Transform publishes movement.
// It visits only this tick's movement buffer, never all animals or all chunks.
type NPCMovementGuard struct {
	ecs.BaseSystem
	chunks     contracts.TreeChunkProvider
	brains     *ecs.ComponentStorage[components.NPCBrain]
	transforms *ecs.ComponentStorage[components.Transform]
	movements  *ecs.ComponentStorage[components.Movement]
	colliders  *ecs.ComponentStorage[components.Collider]
	results    *ecs.ComponentStorage[components.CollisionResult]
	infos      *ecs.ComponentStorage[components.EntityInfo]
}

func NewNPCMovementGuard(w *ecs.World, chunks contracts.TreeChunkProvider) *NPCMovementGuard {
	return &NPCMovementGuard{ecs.NewBaseSystem("NPCMovementGuard", 240), chunks,
		ecs.GetOrCreateStorage[components.NPCBrain](w), ecs.GetOrCreateStorage[components.Transform](w),
		ecs.GetOrCreateStorage[components.Movement](w), ecs.GetOrCreateStorage[components.Collider](w), ecs.GetOrCreateStorage[components.CollisionResult](w), ecs.GetOrCreateStorage[components.EntityInfo](w)}
}

func (s *NPCMovementGuard) Update(w *ecs.World, _ float64) {
	moved := ecs.GetResource[ecs.MovedEntities](w)
	tick := ecs.GetResource[ecs.TimeState](w).Tick
	for i := 0; i < moved.Count; i++ {
		h := moved.Handles[i]
		if !w.Alive(h) {
			continue
		}
		brain, ok := s.brains.Get(h)
		if !ok {
			continue
		}
		transform, hasTransform := s.transforms.Get(h)
		movement, hasMovement := s.movements.Get(h)
		collider, hasCollider := s.colliders.Get(h)
		result, hasResult := s.results.Get(h)
		if !hasTransform || !hasMovement || !hasCollider || !hasResult {
			continue
		}
		withinHome := false
		if info, exists := s.infos.Get(h); exists && objectdefs.Global() != nil {
			if def, exists := objectdefs.Global().GetByID(int(info.TypeID)); exists && def.NPCConfig != nil {
				dx, dy := result.FinalX-brain.HomeX, result.FinalY-brain.HomeY
				radius := def.NPCConfig.HomeRadiusTiles * constt.CoordPerTile
				withinHome = dx*dx+dy*dy <= radius*radius
			}
		}
		if (brain.Activity == components.NPCWander && tick >= brain.DeadlineTick) || !withinHome || !npcTerrainAllowed(s.chunks, transform.X, transform.Y, result.FinalX, result.FinalY, collider) {
			result.FinalX, result.FinalY = transform.X, transform.Y
			s.results.Set(h, result)
			movement.ClearTarget()
			movement.Direction.UpdatePending = true
			s.movements.Set(h, movement)
			if id, exists := w.GetExternalID(h); exists {
				ecs.ScheduleBehaviorTick(w, id, npcAIKey, tick)
			}
		} else if brain.Activity == components.NPCWander && (movement.State != constt.StateMoving || (result.PerpendicularOscillation && !movement.PointStopPending && movement.TargetType != constt.TargetDirection)) {
			// Arrival is already settled by the movement/collision pipeline.
			// Start the rest interval now instead of waiting for the hard timeout.
			if id, exists := w.GetExternalID(h); exists {
				ecs.ScheduleBehaviorTick(w, id, npcAIKey, tick)
			}
		}
	}
}

// Check the swept footprint, including the starting cell and shallow water.
// The fixed tile budget fails closed for malformed or unexpectedly large steps.
func npcTerrainAllowed(chunks contracts.TreeChunkProvider, fromX, fromY, toX, toY float64, collider components.Collider) bool {
	if chunks == nil {
		return false
	}
	minX := math.Floor((math.Min(fromX, toX) - collider.HalfWidth) / constt.CoordPerTile)
	maxX := math.Floor((math.Max(fromX, toX) + collider.HalfWidth) / constt.CoordPerTile)
	minY := math.Floor((math.Min(fromY, toY) - collider.HalfHeight) / constt.CoordPerTile)
	maxY := math.Floor((math.Max(fromY, toY) + collider.HalfHeight) / constt.CoordPerTile)
	if math.IsNaN(minX+maxX+minY+maxY) || math.IsInf(minX+maxX+minY+maxY, 0) || math.Abs(minX) > 1<<30 || math.Abs(maxX) > 1<<30 || math.Abs(minY) > 1<<30 || math.Abs(maxY) > 1<<30 || (maxX-minX+1)*(maxY-minY+1) > 64 {
		return false
	}
	for y := int(minY); y <= int(maxY); y++ {
		for x := int(minX); x <= int(maxX); x++ {
			cx, cy := int(math.Floor(float64(x)/constt.ChunkSize)), int(math.Floor(float64(y)/constt.ChunkSize))
			chunk := chunks.GetChunkFast(types.ChunkCoord{X: cx, Y: cy})
			if chunk == nil || chunk.GetState() != types.ChunkStateActive {
				return false
			}
			lx, ly := x-cx*constt.ChunkSize, y-cy*constt.ChunkSize
			if !chunk.IsTilePassable(lx, ly, constt.ChunkSize) || chunk.IsTileSwimmable(lx, ly, constt.ChunkSize) {
				return false
			}
		}
	}
	return true
}
