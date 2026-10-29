package game

import (
	"fmt"
	"math"
	"slices"

	"origin/internal/actiondefs"
	"origin/internal/combatpresentation"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/playerstate"
	"origin/internal/types"
)

type npcAbilityRecord struct {
	state  contracts.NPCAbilityState
	typeID uint32
}

// NPCAbilityExecutor adapts server intentions to the ordinary action lifecycle.
// At most one result is retained per live actor, independent of its target's
// lifetime. All calls and observers run under the owning shard lock.
type NPCAbilityExecutor struct {
	world     *ecs.World
	actions   *ActionService
	chunks    contracts.TreeChunkProvider
	spatial   *core.ColliderSpatialIndex
	records   map[types.Handle]npcAbilityRecord
	obstacles [256]types.Handle
}

func NewNPCAbilityExecutor(w *ecs.World, actions *ActionService, chunks contracts.TreeChunkProvider) (*NPCAbilityExecutor, error) {
	if w == nil || actions == nil || actions.world != w || actions.npcAbilities != nil || chunks == nil {
		return nil, fmt.Errorf("NPC abilities require world, actions and chunks")
	}
	spatial, ok := ecs.TryGetResource[*core.WorldColliderSpatial](w)
	if !ok || *spatial == nil {
		return nil, fmt.Errorf("NPC abilities require collider spatial index")
	}
	executor := &NPCAbilityExecutor{world: w, actions: actions, chunks: chunks, spatial: (*spatial).Index, records: make(map[types.Handle]npcAbilityRecord)}
	actions.npcAbilities = executor
	for _, handler := range actions.handlers {
		if melee, ok := handler.(*MeleeActionHandler); ok && melee.definition.Audience == actiondefs.AudienceNPC {
			melee.npcAbilities = executor
			melee.execution.npcAbilities = executor
		}
	}
	w.AddDespawnObserver(func(h types.Handle) { executor.retire(h) })
	w.AddComponentObserver(components.NPCBrainComponentID, func(h types.Handle) {
		if !ecs.HasComponent[components.NPCBrain](w, h) {
			executor.retire(h)
		}
	})
	return executor, nil
}

func (e *NPCAbilityExecutor) isNPCAction(id string) bool {
	definition, ok := e.actions.definitions.Get(id)
	return ok && definition.Audience == actiondefs.AudienceNPC
}

func (e *NPCAbilityExecutor) validRef(ref contracts.EntityRef) bool {
	if ref.EntityID == 0 || !e.world.Alive(ref.Handle) || e.world.GetHandleByEntityID(ref.EntityID) != ref.Handle || ecs.ObjectDestructionPending(e.world, ref.Handle) {
		return false
	}
	id, ok := e.world.GetExternalID(ref.Handle)
	return ok && id == ref.EntityID
}

func (e *NPCAbilityExecutor) actorAllowed(actor contracts.EntityRef, id string) bool {
	if !e.validRef(actor) || directionActionUnavailable(e.world, actor.Handle) {
		return false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](e.world, actor.Handle)
	if !ok || info.Layer != e.world.Layer {
		return false
	}
	registry := objectdefs.Global()
	if registry == nil {
		return false
	}
	def, ok := registry.GetByID(int(info.TypeID))
	if !ok || def.NPCConfig == nil || !def.HasBehavior("npc_ai") || !slices.Contains(def.NPCConfig.Abilities, id) || def.HP <= 0 {
		return false
	}
	health, ok := ecs.GetComponent[components.ObjectInternalState](e.world, actor.Handle)
	minimum := 0.0
	if policy, ok := def.NPCConfig.Parameters.(contracts.NPCCombatHealthPolicy); ok {
		minimum = policy.MinimumCombatHealthFraction()
	}
	return ok && health.HasHP && positiveCombatParameter(health.HP) && health.HP > float64(def.HP)*minimum && ecs.HasComponent[components.NPCBrain](e.world, actor.Handle)
}

func (e *NPCAbilityExecutor) targetAllowed(actor contracts.EntityRef, target contracts.EntityRef) bool {
	if actor == target || !e.validRef(target) {
		return false
	}
	// Only a living, standing creature can be the bite's confirmed aggressor.
	health, ok := ecs.GetComponent[components.EntityHealth](e.world, target.Handle)
	return ok && positiveCombatParameter(health.SHP) && positiveCombatParameter(health.HHP) && health.SHP <= health.HHP && !playerstate.IsIncapacitated(e.world, target.Handle)
}

func (e *NPCAbilityExecutor) confirmedTarget(actor, target contracts.EntityRef) bool {
	brain, ok := ecs.GetComponent[components.NPCBrain](e.world, actor.Handle)
	return ok && brain.DamageSource == target && e.targetAllowed(actor, target)
}

func (e *NPCAbilityExecutor) Availability(actor contracts.EntityRef, id string, target contracts.NPCTarget) contracts.NPCAbilityAvailabilityState {
	definition, ok := e.actions.definitions.Get(id)
	if !ok || definition.Audience != actiondefs.AudienceNPC || definition.Sector == nil {
		return contracts.NPCAbilityAvailabilityState{}
	}
	return contracts.NPCAbilityAvailabilityState{Range: definition.Sector.Range, Ready: e.actorAllowed(actor, id) && target.Kind == contracts.NPCTargetEntity && e.confirmedTarget(actor, target.Entity) && !e.actions.isOnCooldown(e.world, actor.Handle, id) && !ecs.HasComponent[components.ActiveGameAction](e.world, actor.Handle) && !ecs.HasComponent[components.ActiveCyclicAction](e.world, actor.Handle)}
}

func (e *NPCAbilityExecutor) TryStart(request contracts.NPCAbilityRequest) contracts.NPCAbilityState {
	rejected := contracts.NPCAbilityState{Actor: request.Actor, AbilityID: request.AbilityID, Generation: request.Generation, Phase: contracts.NPCAbilityFinished, Status: contracts.NPCExecutionRejected}
	if request.Generation == 0 {
		return rejected
	}
	if record, ok := e.records[request.Actor.Handle]; ok && record.state.Actor == request.Actor {
		if record.state.Generation == request.Generation && record.state.AbilityID == request.AbilityID {
			return record.state
		}
		if record.state.Running() {
			rejected.Status = contracts.NPCExecutionBusy
			return rejected
		}
		if request.Generation <= record.state.Generation {
			return rejected
		}
	}
	definition, ok := e.actions.definitions.Get(request.AbilityID)
	if !ok || definition.Audience != actiondefs.AudienceNPC || definition.Combat == nil || definition.Combat.RawDamage == nil || definition.Sector == nil || e.actions.handlers[definition.ID] == nil || request.Target.Kind != contracts.NPCTargetEntity || !e.actorAllowed(request.Actor, request.AbilityID) || !e.confirmedTarget(request.Actor, request.Target.Entity) {
		return rejected
	}
	if e.actions.isOnCooldown(e.world, request.Actor.Handle, request.AbilityID) {
		rejected.Status = contracts.NPCExecutionBusy
		return rejected
	}
	if ecs.HasComponent[components.ActiveGameAction](e.world, request.Actor.Handle) || ecs.HasComponent[components.ActiveCyclicAction](e.world, request.Actor.Handle) {
		rejected.Status = contracts.NPCExecutionBusy
		return rejected
	}
	actor, aok := ecs.GetComponent[components.Transform](e.world, request.Actor.Handle)
	target, tok := ecs.GetComponent[components.Transform](e.world, request.Target.Entity.Handle)
	collider, cok := ecs.GetComponent[components.Collider](e.world, request.Target.Entity.Handle)
	aim, valid := normalizeActionHeading(math.Atan2(target.Y-actor.Y, target.X-actor.X))
	if !aok || !tok || !cok || !valid || !finiteNPCPoint(actor.X, actor.Y) || !finiteNPCPoint(target.X, target.Y) || !positiveCombatParameter(collider.HalfWidth) || !positiveCombatParameter(collider.HalfHeight) {
		return rejected
	}
	if makeSectorGeometry(aim, definition.Sector.Angle).distanceSquared(target.X-actor.X, target.Y-actor.Y, collider.HalfWidth, collider.HalfHeight) > definition.Sector.Range*definition.Sector.Range || !e.lineClear(request.Actor.Handle, request.Target.Entity.Handle) {
		rejected.Status = contracts.NPCExecutionBlocked
		return rejected
	}
	info, _ := ecs.GetComponent[components.EntityInfo](e.world, request.Actor.Handle)
	state := contracts.NPCAbilityState{Actor: request.Actor, AbilityID: request.AbilityID, Generation: request.Generation, Phase: contracts.NPCAbilityWindup, Status: contracts.NPCExecutionAccepted}
	e.records[request.Actor.Handle] = npcAbilityRecord{state: state, typeID: info.TypeID}
	playerstate.StopMovement(e.world, request.Actor.Handle)
	if actor.Direction != aim {
		ecs.WithComponent(e.world, request.Actor.Handle, func(transform *components.Transform) { transform.Direction = aim })
		ecs.WithComponent(e.world, request.Actor.Handle, func(movement *components.Movement) { movement.Direction.UpdatePending = true })
	}
	active := components.ActiveGameAction{ActionID: request.AbilityID, Generation: request.Generation, DirectAttempt: true, MovementOwned: true}
	e.actions.startTarget(e.world, request.Actor.EntityID, request.Actor.Handle, definition, active, ActionTarget{ObjectID: request.Target.Entity.EntityID, ObjectHandle: request.Target.Entity.Handle, AimAngle: aim})
	if cycle, ok := ecs.GetComponent[components.ActiveCyclicAction](e.world, request.Actor.Handle); ok && cycle.ActionGeneration == request.Generation {
		combatpresentation.Start(e.world, request.Actor.Handle, request.Generation, aim, *definition.Sector, cycle.CycleDurationTicks)
	}
	return e.State(request.Actor, request.Generation)
}

func (e *NPCAbilityExecutor) State(actor contracts.EntityRef, generation uint64) contracts.NPCAbilityState {
	if record, ok := e.records[actor.Handle]; ok && record.state.Actor == actor && record.state.Generation == generation {
		return record.state
	}
	return contracts.NPCAbilityState{Actor: actor, Generation: generation}
}

func (e *NPCAbilityExecutor) Cancel(actor contracts.EntityRef, generation uint64, reason contracts.NPCRetireReason) contracts.NPCAbilityState {
	state := e.State(actor, generation)
	record, found := e.records[actor.Handle]
	if !found || record.state.Actor != actor || record.state.Generation != generation {
		return state
	}
	if state.Running() {
		if active, ok := ecs.GetComponent[components.ActiveGameAction](e.world, actor.Handle); ok && active.Generation == generation && active.ActionID == state.AbilityID {
			e.actions.Cancel(e.world, actor.EntityID, actor.Handle)
		}
		state = e.State(actor, generation)
	}
	if reason == contracts.NPCRetireDespawn || reason == contracts.NPCRetireTransform || reason == contracts.NPCRetireQuarantine {
		delete(e.records, actor.Handle)
	}
	return state
}

func (e *NPCAbilityExecutor) validActive(actor types.Handle, active components.ActiveGameAction) bool {
	record, ok := e.records[actor]
	if !ok || !record.state.Running() || record.state.Generation != active.Generation || record.state.AbilityID != active.ActionID || !e.actorAllowed(record.state.Actor, active.ActionID) || !e.targetAllowed(record.state.Actor, contracts.EntityRef{Handle: active.TargetHandle, EntityID: active.TargetID}) {
		return false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](e.world, actor)
	return ok && info.TypeID == record.typeID
}

func (e *NPCAbilityExecutor) finished(actor types.Handle, active components.ActiveGameAction, success bool) {
	if !e.isNPCAction(active.ActionID) {
		return
	}
	combatpresentation.Stop(e.world, actor, active.Generation)
	record, ok := e.records[actor]
	if !ok || record.state.Generation != active.Generation || record.state.AbilityID != active.ActionID {
		return
	}
	record.state.Phase, record.state.Status = contracts.NPCAbilityFinished, contracts.NPCExecutionCanceled
	if success {
		record.state.Status = contracts.NPCExecutionCompleted
	}
	e.records[actor] = record
	// Wake only schedules a future policy evaluation; it never starts nested combat.
	ecs.WakeBehaviorTick(e.world, record.state.Actor.EntityID, "npc_ai", ecs.GetResource[ecs.TimeState](e.world).Tick)
}

func (e *NPCAbilityExecutor) retire(actor types.Handle) {
	if record, ok := e.records[actor]; ok {
		e.Cancel(record.state.Actor, record.state.Generation, contracts.NPCRetireDespawn)
	}
	delete(e.records, actor)
}

func finiteNPCPoint(x, y float64) bool {
	return !math.IsNaN(x+y) && !math.IsInf(x+y, 0) && math.Abs(x) < 1<<30 && math.Abs(y) < 1<<30
}

// lineClear is deliberately bounded and fails closed on missing terrain or
// query overflow. It tests the exact segment, not an entire enclosing tile box.
func (e *NPCAbilityExecutor) lineClear(actor, target types.Handle) bool {
	from, ok := ecs.GetComponent[components.Transform](e.world, actor)
	if !ok {
		return false
	}
	to, ok := ecs.GetComponent[components.Transform](e.world, target)
	if !ok || !finiteNPCPoint(from.X, from.Y) || !finiteNPCPoint(to.X, to.Y) {
		return false
	}
	bounds := core.ColliderBounds{MinX: math.Min(from.X, to.X), MinY: math.Min(from.Y, to.Y), MaxX: math.Max(from.X, to.X), MaxY: math.Max(from.Y, to.Y)}
	// Include zero-width segments in the spatial query.
	bounds.MinX -= sectorContactEpsilon
	bounds.MinY -= sectorContactEpsilon
	bounds.MaxX += sectorContactEpsilon
	bounds.MaxY += sectorContactEpsilon
	candidates, err := e.spatial.QueryBoundedInto(bounds, e.obstacles[:0], core.ColliderQueryBudget{MaxCells: 64, MaxVisits: 256})
	if err != nil {
		return false
	}
	actorCollider, ok := ecs.GetComponent[components.Collider](e.world, actor)
	if !ok {
		return false
	}
	for _, h := range candidates {
		if h == actor || h == target || !e.world.Alive(h) {
			continue
		}
		collider, ok := ecs.GetComponent[components.Collider](e.world, h)
		if !ok {
			return false
		}
		if actorCollider.Layer&collider.Mask == 0 && collider.Layer&actorCollider.Mask == 0 {
			continue
		}
		tr, ok := ecs.GetComponent[components.Transform](e.world, h)
		if !ok || !finiteNPCPoint(tr.X, tr.Y) || !positiveCombatParameter(collider.HalfWidth) || !positiveCombatParameter(collider.HalfHeight) {
			return false
		}
		if npcSegmentIntersectsBox(from.X, from.Y, to.X, to.Y, tr.X-collider.HalfWidth, tr.Y-collider.HalfHeight, tr.X+collider.HalfWidth, tr.Y+collider.HalfHeight) {
			return false
		}
	}
	minX, maxX := int(math.Floor(bounds.MinX/constt.CoordPerTile)), int(math.Floor(bounds.MaxX/constt.CoordPerTile))
	minY, maxY := int(math.Floor(bounds.MinY/constt.CoordPerTile)), int(math.Floor(bounds.MaxY/constt.CoordPerTile))
	if (maxX-minX+1)*(maxY-minY+1) > 64 {
		return false
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			tx, ty := float64(x*constt.CoordPerTile), float64(y*constt.CoordPerTile)
			if !npcSegmentIntersectsBox(from.X, from.Y, to.X, to.Y, tx, ty, tx+constt.CoordPerTile, ty+constt.CoordPerTile) {
				continue
			}
			cx, cy := int(math.Floor(float64(x)/constt.ChunkSize)), int(math.Floor(float64(y)/constt.ChunkSize))
			chunk := e.chunks.GetChunkFast(types.ChunkCoord{X: cx, Y: cy})
			if chunk == nil || chunk.GetState() != types.ChunkStateActive || !chunk.IsTilePassable(x-cx*constt.ChunkSize, y-cy*constt.ChunkSize, constt.ChunkSize) || chunk.IsTileSwimmable(x-cx*constt.ChunkSize, y-cy*constt.ChunkSize, constt.ChunkSize) {
				return false
			}
		}
	}
	return true
}

func npcSegmentIntersectsBox(x0, y0, x1, y1, minX, minY, maxX, maxY float64) bool {
	lo, hi := 0.0, 1.0
	for _, axis := range [2][4]float64{{x0, x1 - x0, minX, maxX}, {y0, y1 - y0, minY, maxY}} {
		if axis[1] == 0 {
			if axis[0] < axis[2] || axis[0] > axis[3] {
				return false
			}
			continue
		}
		a, b := (axis[2]-axis[0])/axis[1], (axis[3]-axis[0])/axis[1]
		if a > b {
			a, b = b, a
		}
		lo = math.Max(lo, a)
		hi = math.Min(hi, b)
		if lo > hi {
			return false
		}
	}
	return true
}
