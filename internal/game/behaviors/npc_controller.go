package behaviors

import (
	"fmt"
	"math"
	"reflect"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/playerstate"
	"origin/internal/types"
)

type NPCControllerConfig struct {
	Chunks    contracts.TreeChunkProvider
	Catalog   *NPCCatalog
	Abilities contracts.NPCAbilityExecutor
}

// NPCController belongs to exactly one World and is called only on its shard's
// execution path. It owns intent execution; policies cannot write movement.
type NPCController struct {
	world      *ecs.World
	chunks     contracts.TreeChunkProvider
	catalog    *NPCCatalog
	abilities  contracts.NPCAbilityExecutor
	generation uint64
	candidates [256]types.Handle
}

func npcNilInterface(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}
func AttachNPCController(w *ecs.World, cfg NPCControllerConfig) (*NPCController, error) {
	if w == nil || npcNilInterface(cfg.Chunks) {
		return nil, fmt.Errorf("NPCController requires world and chunks")
	}
	if _, exists := GetNPCController(w); exists {
		return nil, fmt.Errorf("NPCController already attached")
	}
	if cfg.Catalog == nil {
		cfg.Catalog = DefaultNPCCatalog()
	}
	if cfg.Catalog.profiles == nil || cfg.Catalog.capabilities == nil || cfg.Catalog.abilities == nil {
		return nil, fmt.Errorf("NPCController requires a constructed catalog")
	}
	if len(cfg.Catalog.abilities) > 0 && npcNilInterface(cfg.Abilities) {
		return nil, fmt.Errorf("NPC ability catalog requires an executor")
	}
	if npcNilInterface(cfg.Abilities) {
		cfg.Abilities = nil
	}
	ecs.SetResource(w, NPCController{world: w, chunks: cfg.Chunks, catalog: cfg.Catalog, abilities: cfg.Abilities})
	c := ecs.GetResource[NPCController](w)
	ecs.SetResource(w, contracts.NPCLifecycle{Controller: c})
	w.AddDespawnObserver(func(h types.Handle) { c.Retire(h, contracts.NPCRetireDespawn) })
	return c, nil
}
func GetNPCController(w *ecs.World) (*NPCController, bool) {
	if w == nil {
		return nil, false
	}
	return ecs.TryGetResource[NPCController](w)
}
func (c *NPCController) nextGeneration() uint64 {
	if c.generation == math.MaxUint64 {
		return 0
	}
	c.generation++
	return c.generation
}

func (c *NPCController) valid(ref contracts.EntityRef) bool {
	if !c.world.Alive(ref.Handle) || ecs.ObjectDestructionPending(c.world, ref.Handle) {
		return false
	}
	id, ok := c.world.GetExternalID(ref.Handle)
	return ok && id == ref.EntityID
}
func (c *NPCController) init(ctx *contracts.BehaviorObjectInitContext) error {
	def, ok := npcDefinition(c.world, ctx.Handle, ctx.EntityID, ctx.EntityType)
	if !ok {
		return nil
	}
	profile, ok := c.catalog.profiles[def.NPCConfig.Profile]
	if !ok {
		return fmt.Errorf("NPC profile %q is not registered", def.NPCConfig.Profile)
	}
	if profile.PerceptionRadius > 0 {
		if spatial, exists := ecs.TryGetResource[*core.WorldColliderSpatial](c.world); !exists || *spatial == nil || (*spatial).Index == nil {
			return fmt.Errorf("NPC perception requires collider spatial index")
		}
	}
	if profile.RequiresAbilityAvailability {
		if _, exists := c.abilities.(contracts.NPCAbilityAvailability); !exists {
			return fmt.Errorf("NPC profile requires ability availability")
		}
	}
	transform, ok := ecs.GetComponent[components.Transform](c.world, ctx.Handle)
	if !ok {
		return fmt.Errorf("npc_ai object has no transform")
	}
	if brain, exists := ecs.GetComponent[components.NPCBrain](c.world, ctx.Handle); exists && (ctx.Reason != contracts.ObjectBehaviorInitReasonTransform || ctx.PreviousType == ctx.EntityType) {
		ecs.ScheduleBehaviorTick(c.world, ctx.EntityID, npcAIKey, brain.NextDecisionTick)
		return nil
	}
	brain := components.NPCBrain{HomeX: transform.X, HomeY: transform.Y, NPCPolicyState: components.NPCPolicyState{RandomState: uint64(ctx.EntityID) ^ 0x9e3779b97f4a7c15}}
	if brain.RandomState == 0 {
		brain.RandomState = 1
	}
	ecs.AddComponent(c.world, ctx.Handle, components.Movement{Mode: constt.Walk, State: constt.StateIdle, Speed: def.NPCConfig.Speed})
	ecs.AddComponent(c.world, ctx.Handle, components.CollisionResult{FinalX: transform.X, FinalY: transform.Y})
	obs := c.observe(ctx.Handle, ctx.EntityID, brain, ecs.GetResource[ecs.TimeState](c.world).Tick)
	c.observeProfile(profile, brain, *def.NPCConfig, float64(def.HP), &obs)
	d := NPCDecision{State: brain.NPCPolicyState, NextDecisionTick: obs.Tick + 1}
	if profile.Init != nil {
		d = profile.Init(brain.NPCPolicyState, obs, *def.NPCConfig)
	}
	brain.NPCPolicyState = d.State
	ecs.AddComponent(c.world, ctx.Handle, brain)
	result := c.execute(ctx.Handle, &brain, obs, *def.NPCConfig, d.Intent)
	brain.LastResult = result.Status
	c.schedule(ctx.EntityID, &brain, d.NextDecisionTick, obs.Tick)
	ecs.AddComponent(c.world, ctx.Handle, brain)
	return nil
}
func (c *NPCController) observe(h types.Handle, id types.EntityID, brain components.NPCBrain, tick uint64) NPCObservation {
	transform, _ := ecs.GetComponent[components.Transform](c.world, h)
	movement, _ := ecs.GetComponent[components.Movement](c.world, h)
	obs := NPCObservation{Actor: contracts.EntityRef{Handle: h, EntityID: id}, Position: contracts.NPCPoint{X: transform.X, Y: transform.Y}, Home: contracts.NPCPoint{X: brain.HomeX, Y: brain.HomeY}, Tick: tick, TicksPerSecond: npcTicksPerSecond(ecs.GetResource[ecs.TimeState](c.world)), Moving: movement.State == constt.StateMoving, LastResult: brain.LastResult, ActivityDeadlineTick: brain.ActivityDeadlineTick, NextDecisionTick: brain.NextDecisionTick}
	obs.Target = c.observeEntity(brain.Target)
	obs.DamageRevision = brain.DamageRevision
	if hp, exists := ecs.GetComponent[components.ObjectInternalState](c.world, h); exists && hp.HasHP {
		obs.HP, obs.HasHP = float64(hp.HP), true
	}
	if brain.Ability.Generation != 0 && c.abilities != nil {
		obs.Ability = c.abilities.State(obs.Actor, brain.Ability.Generation)
		if brain.Ability.Target.Kind == contracts.NPCTargetEntity {
			obs.AbilityTarget = c.observeEntity(brain.Ability.Target.Entity)
		}
	}
	return obs
}
func (c *NPCController) observeEntity(ref contracts.EntityRef) NPCObservedEntity {
	if !c.valid(ref) {
		return NPCObservedEntity{}
	}
	tr, ok := ecs.GetComponent[components.Transform](c.world, ref.Handle)
	if !ok {
		return NPCObservedEntity{}
	}
	entity := NPCObservedEntity{Ref: ref, Position: contracts.NPCPoint{X: tr.X, Y: tr.Y}, Valid: true}
	if move, exists := ecs.GetComponent[components.Movement](c.world, ref.Handle); exists {
		entity.Moving = move.State == constt.StateMoving
	}
	if hp, exists := ecs.GetComponent[components.EntityHealth](c.world, ref.Handle); exists {
		entity.HP, entity.HasHP = hp.SHP, true
		entity.Incapacitated = playerstate.IsIncapacitated(c.world, ref.Handle)
	}
	return entity
}

func (c *NPCController) observeProfile(profile NPCProfile, brain components.NPCBrain, cfg contracts.NPCBehaviorConfig, maxHP float64, obs *NPCObservation) {
	obs.MaxHP = maxHP
	if profile.PerceptionRadius > 0 {
		obs.Encounter = c.observeEntity(brain.Cautious.Encounter)
		spatial, exists := ecs.TryGetResource[*core.WorldColliderSpatial](c.world)
		if exists && *spatial != nil && (*spatial).Index != nil {
			radius := profile.PerceptionRadius
			candidates, err := (*spatial).Index.QueryBoundedInto(core.ColliderBounds{MinX: obs.Position.X - radius, MinY: obs.Position.Y - radius, MaxX: obs.Position.X + radius, MaxY: obs.Position.Y + radius}, c.candidates[:0], core.ColliderQueryBudget{MaxCells: 256, MaxVisits: 1024})
			if err == nil {
				obs.PerceptionKnown = true
				nearestSq := radius * radius
				for _, h := range candidates {
					if !ecs.HasComponent[components.CharacterProfile](c.world, h) {
						continue
					}
					id, found := c.world.GetExternalID(h)
					if !found {
						continue
					}
					entity := c.observeEntity(contracts.EntityRef{Handle: h, EntityID: id})
					if !entity.Valid || !entity.HasHP || entity.HP <= 0 || entity.Incapacitated {
						continue
					}
					distance := npcDistanceSquared(obs.Position, entity.Position)
					if distance < nearestSq || (distance == nearestSq && (!obs.NearbyPlayer.Valid || id < obs.NearbyPlayer.Ref.EntityID)) {
						obs.NearbyPlayer, nearestSq = entity, distance
					}
				}
			}
		}
	}
	if profile.RequiresAbilityAvailability && len(cfg.Abilities) > 0 {
		if availability, exists := c.abilities.(contracts.NPCAbilityAvailability); exists {
			obs.AvailableAbility = availability.Availability(obs.Actor, cfg.Abilities[0], contracts.NPCTarget{Kind: contracts.NPCTargetEntity, Entity: obs.Target.Ref})
		}
	}
}
func (c *NPCController) tick(ctx *contracts.BehaviorTickContext) error {
	def, ok := npcDefinition(c.world, ctx.Handle, ctx.EntityID, ctx.EntityType)
	if !ok {
		return nil
	}
	brain, ok := ecs.GetComponent[components.NPCBrain](c.world, ctx.Handle)
	_, hasMove := ecs.GetComponent[components.Movement](c.world, ctx.Handle)
	_, hasTransform := ecs.GetComponent[components.Transform](c.world, ctx.Handle)
	_, hasCollider := ecs.GetComponent[components.Collider](c.world, ctx.Handle)
	if !ok || !hasMove || !hasTransform || !hasCollider {
		return fmt.Errorf("npc_ai object has incomplete runtime components")
	}
	profile, ok := c.catalog.profiles[def.NPCConfig.Profile]
	if !ok {
		return fmt.Errorf("unknown NPC profile %q", def.NPCConfig.Profile)
	}
	obs := c.observe(ctx.Handle, ctx.EntityID, brain, ctx.CurrentTick)
	c.observeProfile(profile, brain, *def.NPCConfig, float64(def.HP), &obs)
	c.reconcileAbility(ctx.Handle, &brain, &obs)
	d := profile.Decide(brain.NPCPolicyState, obs, *def.NPCConfig)
	brain.NPCPolicyState = d.State
	result := c.execute(ctx.Handle, &brain, obs, *def.NPCConfig, d.Intent)
	if d.Intent.Kind != contracts.NPCIntentContinue && d.Intent.Kind != contracts.NPCIntentWait {
		brain.LastResult = result.Status
	}
	if profile.OnResult != nil && d.Intent.Kind != contracts.NPCIntentContinue && d.Intent.Kind != contracts.NPCIntentWait {
		obs.ActivityDeadlineTick = brain.ActivityDeadlineTick
		feedback := profile.OnResult(brain.NPCPolicyState, obs, *def.NPCConfig, result)
		brain.NPCPolicyState = feedback.State
		// Feedback may issue one follow-up intent (for example, stop after a blocked
		// route). It is executed once, without recursively calling the policy.
		if feedback.Intent.Kind != contracts.NPCIntentContinue {
			c.execute(ctx.Handle, &brain, obs, *def.NPCConfig, feedback.Intent)
		}
		if feedback.NextDecisionTick != 0 {
			d.NextDecisionTick = feedback.NextDecisionTick
		}
	}
	c.schedule(ctx.EntityID, &brain, d.NextDecisionTick, ctx.CurrentTick)
	ecs.AddComponent(c.world, ctx.Handle, brain)
	return nil
}
func (c *NPCController) schedule(id types.EntityID, brain *components.NPCBrain, next, tick uint64) {
	if next <= tick {
		next = tick + 1
	}
	brain.NextDecisionTick = next
	ecs.ScheduleBehaviorTick(c.world, id, npcAIKey, next)
}
func (c *NPCController) execute(h types.Handle, brain *components.NPCBrain, obs NPCObservation, cfg contracts.NPCBehaviorConfig, intent contracts.NPCIntent) contracts.NPCExecutionResult {
	result := contracts.NPCExecutionResult{Status: contracts.NPCExecutionAccepted}
	if !c.valid(obs.Actor) {
		result.Status = contracts.NPCExecutionRejected
		return result
	}
	switch intent.Kind {
	case contracts.NPCIntentContinue, contracts.NPCIntentWait:
		return result
	case contracts.NPCIntentMove, contracts.NPCIntentStop, contracts.NPCIntentFace:
		if brain.MovementLease.Owner == components.NPCMovementAbility && brain.Ability.Generation != 0 {
			result.Status = contracts.NPCExecutionBusy
			return result
		}
		if !c.catalog.canMove(cfg) {
			result.Status = contracts.NPCExecutionRejected
			return result
		}
		if intent.Kind == contracts.NPCIntentStop {
			c.stop(h, brain)
			result.Status = contracts.NPCExecutionCompleted
			return result
		}
		if intent.Kind == contracts.NPCIntentFace {
			if !npcValidPoint(intent.Face) {
				return contracts.NPCExecutionResult{Status: contracts.NPCExecutionRejected}
			}
			c.stop(h, brain)
			tr, ok := ecs.GetComponent[components.Transform](c.world, h)
			if !ok {
				return contracts.NPCExecutionResult{Status: contracts.NPCExecutionRejected}
			}
			dx, dy := intent.Face.X-tr.X, intent.Face.Y-tr.Y
			if dx != 0 || dy != 0 {
				direction := math.Atan2(dy, dx)
				if tr.Direction != direction {
					tr.Direction = direction
					ecs.AddComponent(c.world, h, tr)
					ecs.WithComponent(c.world, h, func(movement *components.Movement) { movement.Direction.UpdatePending = true })
				}
			}
			return result
		}
		return c.move(h, brain, obs, intent.Move)
	case contracts.NPCIntentUseAbility:
		return c.useAbility(h, brain, obs, cfg, intent.Ability)
	case contracts.NPCIntentCancelAbility:
		if !c.catalog.canUseAbility(cfg) {
			return contracts.NPCExecutionResult{Status: contracts.NPCExecutionRejected}
		}
		return c.requestAbilityCancel(h, brain, obs.Actor, intent.CancelGeneration, contracts.NPCRetireRequested)
	default:
		result.Status = contracts.NPCExecutionRejected
		return result
	}
}
func (c *NPCController) stop(h types.Handle, brain *components.NPCBrain) {
	playerstate.StopMovement(c.world, h)
	brain.MovementLease = components.NPCMovementLease{}
	brain.ActivityDeadlineTick = 0
}
func (c *NPCController) move(h types.Handle, brain *components.NPCBrain, obs NPCObservation, intent contracts.NPCMoveIntent) contracts.NPCExecutionResult {
	rejected := contracts.NPCExecutionResult{Status: contracts.NPCExecutionRejected}
	if intent.Count == 0 || intent.Count > npcCandidateAttempts || intent.DeadlineTick <= obs.Tick || intent.DeadlineTick-obs.Tick > npcDurationTicks(60, obs.TicksPerSecond) || math.IsNaN(intent.HomeRadius) || math.IsInf(intent.HomeRadius, 0) || intent.HomeRadius < 0 || intent.HomeRadius > 64*constt.CoordPerTile {
		return rejected
	}
	if intent.Target != (contracts.EntityRef{}) && !c.valid(intent.Target) {
		return rejected
	}
	box, ok := ecs.GetComponent[components.Collider](c.world, h)
	if !ok {
		return rejected
	}
	movement, ok := ecs.GetComponent[components.Movement](c.world, h)
	if !ok {
		return rejected
	}
	for i := 0; i < int(intent.Count); i++ {
		p := intent.Candidates[i]
		if !npcValidPoint(p) {
			continue
		}
		// Movement.SetTargetPoint stores integer coordinates: validate that exact point.
		p.X, p.Y = math.Trunc(p.X), math.Trunc(p.Y)
		if !npcWithinHome(p.X, p.Y, brain.HomeX, brain.HomeY, intent.HomeRadius) || !npcTerrainAllowed(c.chunks, p.X, p.Y, p.X, p.Y, box) {
			continue
		}
		generation := c.nextGeneration()
		if generation == 0 {
			return rejected
		}
		movement.SetTargetPoint(int(p.X), int(p.Y))
		ecs.AddComponent(c.world, h, movement)
		brain.MovementLease = components.NPCMovementLease{Owner: components.NPCMovementAI, Generation: generation, DeadlineTick: intent.DeadlineTick, HomeRadius: intent.HomeRadius, Target: intent.Target}
		brain.ActivityDeadlineTick = intent.DeadlineTick
		return contracts.NPCExecutionResult{Status: contracts.NPCExecutionAccepted, Generation: generation}
	}
	return contracts.NPCExecutionResult{Status: contracts.NPCExecutionBlocked}
}
func npcWithinHome(x, y, homeX, homeY, radius float64) bool {
	if radius == 0 {
		return true
	}
	dx, dy := x-homeX, y-homeY
	return dx*dx+dy*dy <= radius*radius
}
func (c *NPCController) targetValid(target contracts.NPCTarget) bool {
	switch target.Kind {
	case contracts.NPCTargetNone:
		return true
	case contracts.NPCTargetEntity:
		return c.valid(target.Entity)
	case contracts.NPCTargetPoint:
		return npcValidPoint(target.Point)
	case contracts.NPCTargetDirection:
		return npcValidPoint(target.Direction) && math.Hypot(target.Direction.X, target.Direction.Y) > 0
	}
	return false
}
func npcMatchingAbility(state contracts.NPCAbilityState, actor contracts.EntityRef, ref components.NPCAbilityRef) bool {
	return state.Actor == actor && state.Generation == ref.Generation && state.AbilityID == ref.ID
}
func (c *NPCController) reconcileAbility(h types.Handle, brain *components.NPCBrain, obs *NPCObservation) {
	if brain.Ability.Generation == 0 {
		return
	}
	matching := npcMatchingAbility(obs.Ability, obs.Actor, brain.Ability)
	// Completion may itself remove the target. Consume a matching terminal result
	// before checking eligibility that only applies to a still-running execution.
	if matching && !obs.Ability.Running() {
		brain.LastResult = npcTerminalAbilityStatus(obs.Ability)
		if brain.MovementLease.Owner == components.NPCMovementAbility {
			c.stop(h, brain)
			obs.Moving = false
		}
		brain.Ability = components.NPCAbilityRef{}
		obs.LastResult = brain.LastResult
		return
	}
	if !matching || !c.targetValid(brain.Ability.Target) {
		c.cancelAbility(h, brain, obs.Actor, contracts.NPCRetireInvalidTarget)
		brain.LastResult = contracts.NPCExecutionRejected
		obs.Ability = contracts.NPCAbilityState{}
		obs.LastResult = brain.LastResult
		obs.Moving = false
	}
}

func (c *NPCController) useAbility(h types.Handle, brain *components.NPCBrain, obs NPCObservation, cfg contracts.NPCBehaviorConfig, request contracts.NPCAbilityRequest) contracts.NPCExecutionResult {
	result := contracts.NPCExecutionResult{Status: contracts.NPCExecutionRejected}
	if brain.Ability.Generation != 0 {
		result.Status = contracts.NPCExecutionBusy
		result.Generation = brain.Ability.Generation
		return result
	}
	def, ok := c.catalog.abilities[request.AbilityID]
	if !ok || c.abilities == nil || !c.catalog.canUseAbility(cfg) || !npcContains(cfg.Abilities, request.AbilityID) || request.Target.Kind != def.TargetKind || !c.targetValid(request.Target) {
		return result
	}
	generation := c.nextGeneration()
	if generation == 0 {
		return result
	}
	// Transfer movement ownership before the executor can establish its route.
	// An owning ability interrupts any prior AI route even if its start is rejected.
	if def.OwnsMovement {
		c.stop(h, brain)
	}
	request.Actor = obs.Actor
	request.Generation = generation
	ref := components.NPCAbilityRef{ID: request.AbilityID, Generation: request.Generation, Target: request.Target}
	state := c.abilities.TryStart(request)
	if !npcMatchingAbility(state, obs.Actor, ref) || (state.Running() && state.Status != contracts.NPCExecutionAccepted) {
		c.abilities.Cancel(obs.Actor, request.Generation, contracts.NPCRetireInvalidTarget)
		return result
	}
	result.Status, result.Generation = state.Status, state.Generation
	if state.Running() && state.Status == contracts.NPCExecutionAccepted {
		brain.Ability = ref
		if def.OwnsMovement {
			// An executor may establish its movement during TryStart. Retire only the
			// previous AI lease; never overwrite movement accepted by the executor.
			brain.MovementLease = components.NPCMovementLease{Owner: components.NPCMovementAbility, Generation: request.Generation}
			brain.ActivityDeadlineTick = 0
		}
	}
	return result
}
func (c *NPCController) cancelAbility(h types.Handle, brain *components.NPCBrain, actor contracts.EntityRef, reason contracts.NPCRetireReason) {
	if brain.Ability.Generation != 0 && c.abilities != nil {
		c.abilities.Cancel(actor, brain.Ability.Generation, reason)
	}
	if brain.MovementLease.Owner == components.NPCMovementAbility {
		c.stop(h, brain)
	}
	brain.Ability = components.NPCAbilityRef{}
}

// requestAbilityCancel respects executor interruption rules without scheduling
// or publishing component state; the caller commits its local brain once.
func (c *NPCController) requestAbilityCancel(h types.Handle, brain *components.NPCBrain, actor contracts.EntityRef, generation uint64, reason contracts.NPCRetireReason) contracts.NPCExecutionResult {
	result := contracts.NPCExecutionResult{Status: contracts.NPCExecutionRejected, Generation: generation}
	if c.abilities == nil || generation == 0 || brain.Ability.Generation != generation {
		return result
	}
	state := c.abilities.State(actor, generation)
	if !npcMatchingAbility(state, actor, brain.Ability) {
		return result
	}
	if state.Running() {
		state = c.abilities.Cancel(actor, generation, reason)
		if !npcMatchingAbility(state, actor, brain.Ability) {
			return result
		}
		if state.Running() {
			result.Status = contracts.NPCExecutionBusy
			return result
		}
	}
	result.Status = npcTerminalAbilityStatus(state)
	if brain.MovementLease.Owner == components.NPCMovementAbility {
		c.stop(h, brain)
	}
	brain.Ability = components.NPCAbilityRef{}
	return result
}
func npcTerminalAbilityStatus(state contracts.NPCAbilityState) contracts.NPCExecutionStatus {
	switch state.Status {
	case contracts.NPCExecutionCompleted, contracts.NPCExecutionCanceled, contracts.NPCExecutionTimeout, contracts.NPCExecutionBlocked, contracts.NPCExecutionRejected:
		return state.Status
	default:
		return contracts.NPCExecutionRejected
	}
}
func (c *NPCController) CancelAbility(actor contracts.EntityRef, generation uint64, reason contracts.NPCRetireReason) contracts.NPCExecutionResult {
	result := contracts.NPCExecutionResult{Status: contracts.NPCExecutionRejected}
	if !c.valid(actor) {
		return result
	}
	brain, ok := ecs.GetComponent[components.NPCBrain](c.world, actor.Handle)
	if !ok || generation == 0 || brain.Ability.Generation != generation {
		return result
	}
	result = c.requestAbilityCancel(actor.Handle, &brain, actor, generation, reason)
	brain.LastResult = result.Status
	ecs.AddComponent(c.world, actor.Handle, brain)
	ecs.WakeBehaviorTick(c.world, actor.EntityID, npcAIKey, ecs.GetResource[ecs.TimeState](c.world).Tick)
	return result
}

// SetTarget is a bounded stimulus ingress, called only under the shard lock.
func (c *NPCController) SetTarget(actor, target contracts.EntityRef) bool {
	if !c.valid(actor) || (target != (contracts.EntityRef{}) && !c.valid(target)) {
		return false
	}
	brain, ok := ecs.GetComponent[components.NPCBrain](c.world, actor.Handle)
	if !ok {
		return false
	}
	brain.Target = target
	ecs.AddComponent(c.world, actor.Handle, brain)
	ecs.WakeBehaviorTick(c.world, actor.EntityID, npcAIKey, ecs.GetResource[ecs.TimeState](c.world).Tick)
	return true
}

// NotifyDamage records only a confirmed player attack after the complete damage
// batch committed a positive HP loss. Callers hold the owning shard lock. Mere
// targeting, misses, and zero damage must not call this ingress.
func (c *NPCController) NotifyDamage(target, source contracts.EntityRef) bool {
	if !c.valid(target) || !c.valid(source) || !ecs.HasComponent[components.CharacterProfile](c.world, source.Handle) {
		return false
	}
	brain, exists := ecs.GetComponent[components.NPCBrain](c.world, target.Handle)
	if !exists {
		return false
	}
	brain.Target, brain.DamageSource = source, source
	brain.DamageRevision++
	if brain.DamageRevision == 0 {
		brain.DamageRevision = 1
	}
	ecs.AddComponent(c.world, target.Handle, brain)
	ecs.WakeBehaviorTick(c.world, target.EntityID, npcAIKey, ecs.GetResource[ecs.TimeState](c.world).Tick)
	return true
}
func (c *NPCController) Retire(h types.Handle, reason contracts.NPCRetireReason) {
	// Destruction-pending actors still need cancellation before components vanish.
	if !c.world.Alive(h) {
		return
	}
	brain, ok := ecs.GetComponent[components.NPCBrain](c.world, h)
	if !ok {
		return
	}
	id, ok := c.world.GetExternalID(h)
	if !ok {
		return
	}
	c.cancelAbility(h, &brain, contracts.EntityRef{Handle: h, EntityID: id}, reason)
	c.stop(h, &brain)
	brain.Target = contracts.EntityRef{}
	brain.DamageSource = contracts.EntityRef{}
	brain.NextDecisionTick = 0
	brain.LastResult = contracts.NPCExecutionCanceled
	ecs.AddComponent(c.world, h, brain)
	ecs.CancelBehaviorTick(c.world, id, npcAIKey)
}
