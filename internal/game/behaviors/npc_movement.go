package behaviors

import (
	"math"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/types"
)

// NPCMovementGuard examines only moved NPCs after collision, before transform.
// It enforces the controller's task lease, never a species/profile/activity enum.
type NPCMovementGuard struct {
	ecs.BaseSystem
	controller *NPCController
	brains     *ecs.ComponentStorage[components.NPCBrain]
	transforms *ecs.ComponentStorage[components.Transform]
	movements  *ecs.ComponentStorage[components.Movement]
	colliders  *ecs.ComponentStorage[components.Collider]
	results    *ecs.ComponentStorage[components.CollisionResult]
}

func NewNPCMovementGuard(c *NPCController) *NPCMovementGuard {
	if c == nil {
		panic("NPCMovementGuard requires NPCController")
	}
	w := c.world
	return &NPCMovementGuard{BaseSystem: ecs.NewBaseSystem("NPCMovementGuard", 240), controller: c,
		brains: ecs.GetOrCreateStorage[components.NPCBrain](w), transforms: ecs.GetOrCreateStorage[components.Transform](w),
		movements: ecs.GetOrCreateStorage[components.Movement](w), colliders: ecs.GetOrCreateStorage[components.Collider](w), results: ecs.GetOrCreateStorage[components.CollisionResult](w)}
}
func (s *NPCMovementGuard) Update(w *ecs.World, _ float64) {
	if w != s.controller.world {
		panic("NPCMovementGuard used with another World")
	}
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
		transform, ht := s.transforms.Get(h)
		movement, hm := s.movements.Get(h)
		collider, hc := s.colliders.Get(h)
		result, hr := s.results.Get(h)
		if !ht || !hm || !hc || !hr {
			continue
		}
		lease := brain.MovementLease
		// Movement also publishes stationary stop notifications. A retired lease is
		// expected here and must not turn an ordinary stop into a rejected task.
		if lease.Owner == components.NPCMovementUnowned && movement.State != constt.StateMoving && movement.TargetType == constt.TargetNone && result.FinalX == transform.X && result.FinalY == transform.Y {
			continue
		}
		status := contracts.NPCExecutionNone
		if lease.Owner == components.NPCMovementUnowned || ecs.ObjectDestructionPending(w, h) {
			status = contracts.NPCExecutionRejected
		} else if lease.DeadlineTick != 0 && tick >= lease.DeadlineTick {
			status = contracts.NPCExecutionTimeout
		} else if !npcWithinHome(result.FinalX, result.FinalY, brain.HomeX, brain.HomeY, lease.HomeRadius) || !npcTerrainAllowed(s.controller.chunks, transform.X, transform.Y, result.FinalX, result.FinalY, collider) {
			status = contracts.NPCExecutionBlocked
		} else if lease.Owner == components.NPCMovementAI && lease.Target != (contracts.EntityRef{}) && !s.controller.valid(lease.Target) {
			status = contracts.NPCExecutionRejected
		}
		terminalAbility := false
		if lease.Owner == components.NPCMovementAbility {
			id, exists := w.GetExternalID(h)
			actor := contracts.EntityRef{Handle: h, EntityID: id}
			if !exists || s.controller.abilities == nil || lease.Generation == 0 || lease.Generation != brain.Ability.Generation {
				status = contracts.NPCExecutionRejected
			} else {
				state := s.controller.abilities.State(actor, brain.Ability.Generation)
				if !npcMatchingAbility(state, actor, brain.Ability) {
					status = contracts.NPCExecutionRejected
				} else if !state.Running() {
					// The execution may have removed its target on completion.
					// A matching terminal result takes precedence over target validity.
					terminalAbility = true
					status = npcTerminalAbilityStatus(state)
				} else if !s.controller.targetValid(brain.Ability.Target) {
					status = contracts.NPCExecutionRejected
				}
			}
		}
		if status != contracts.NPCExecutionNone {
			result.FinalX, result.FinalY = transform.X, transform.Y
			s.results.Set(h, result)
			if lease.Owner == components.NPCMovementAbility && !terminalAbility {
				if id, exists := w.GetExternalID(h); exists {
					reason := contracts.NPCRetireMovementBlocked
					if status == contracts.NPCExecutionTimeout {
						reason = contracts.NPCRetireTimeout
					}
					s.controller.cancelAbility(h, &brain, contracts.EntityRef{Handle: h, EntityID: id}, reason)
				}
			}
			movement.ClearTarget()
			movement.Direction.UpdatePending = true
			s.movements.Set(h, movement)
			brain.MovementLease = components.NPCMovementLease{}
			brain.ActivityDeadlineTick = 0
			brain.LastResult = status
			s.brains.Set(h, brain)
			if id, exists := w.GetExternalID(h); exists {
				ecs.WakeBehaviorTick(w, id, npcAIKey, tick)
			}
		} else if lease.Owner == components.NPCMovementAI && (movement.State != constt.StateMoving || (result.PerpendicularOscillation && !movement.PointStopPending && movement.TargetType != constt.TargetDirection)) {
			brain.LastResult = contracts.NPCExecutionCompleted
			if result.PerpendicularOscillation {
				brain.LastResult = contracts.NPCExecutionBlocked
			}
			s.brains.Set(h, brain)
			if id, exists := w.GetExternalID(h); exists {
				ecs.WakeBehaviorTick(w, id, npcAIKey, tick)
			}
		}
	}
}

// Check the swept footprint, including the starting cell and shallow water.
// The fixed tile budget fails closed for malformed or unexpectedly large steps.
func npcTerrainAllowed(chunks contracts.TreeChunkProvider, fromX, fromY, toX, toY float64, collider components.Collider) bool {
	if chunks == nil || !npcFinitePositive(collider.HalfWidth) || !npcFinitePositive(collider.HalfHeight) || collider.HalfWidth > constt.CoordPerTile*2 || collider.HalfHeight > constt.CoordPerTile*2 {
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
