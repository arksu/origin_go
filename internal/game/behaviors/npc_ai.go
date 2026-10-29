package behaviors

import (
	"fmt"
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/playerstate"
	"origin/internal/types"
)

const npcAIKey = "npc_ai"
const npcCandidateAttempts = 4

type npcAIBehavior struct{}

func (npcAIBehavior) Key() string { return npcAIKey }

func (npcAIBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil || ctx.Def == nil {
		return 0, fmt.Errorf("npc_ai requires an object definition")
	}
	def, ok := ctx.Def.(*objectdefs.ObjectDef)
	if !ok {
		return 0, fmt.Errorf("npc_ai requires an object definition")
	}
	var cfg contracts.NPCBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid npc_ai config: %w", err)
	}
	if cfg.Profile != "passive_wander" {
		return 0, fmt.Errorf("unknown npc_ai profile %q", cfg.Profile)
	}
	// Bounds keep candidate generation, deadline arithmetic and swept terrain work small.
	for _, value := range []float64{cfg.Speed, cfg.IdleMinSeconds, cfg.IdleMaxSeconds, cfg.WanderMinTiles, cfg.WanderMaxTiles, cfg.HomeRadiusTiles, cfg.MaxWalkSeconds} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return 0, fmt.Errorf("npc_ai parameters must be finite and positive")
		}
	}
	if cfg.Speed > 256 || cfg.IdleMinSeconds > cfg.IdleMaxSeconds || cfg.IdleMaxSeconds > 3600 || cfg.WanderMinTiles > cfg.WanderMaxTiles || cfg.WanderMaxTiles > cfg.HomeRadiusTiles || cfg.HomeRadiusTiles > 64 || cfg.MaxWalkSeconds > 60 {
		return 0, fmt.Errorf("npc_ai parameters exceed supported bounds")
	}
	if def.Static == nil || *def.Static || def.Components == nil || def.Components.Collider == nil || def.Components.Collider.W <= 0 || def.Components.Collider.H <= 0 || def.Components.Collider.W > constt.CoordPerTile*4 || def.Components.Collider.H > constt.CoordPerTile*4 {
		return 0, fmt.Errorf("npc_ai requires a dynamic object with a bounded collider")
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	def.SetNPCBehaviorConfig(cfg)
	return cfg.Priority, nil
}

func (npcAIBehavior) ApplyRuntime(*contracts.BehaviorRuntimeContext) contracts.BehaviorRuntimeResult {
	return contracts.BehaviorRuntimeResult{}
}

func (npcAIBehavior) InitObject(ctx *contracts.BehaviorObjectInitContext) error {
	if ctx == nil {
		return nil
	}
	def, ok := npcDefinition(ctx.World, ctx.Handle, ctx.EntityID, ctx.EntityType)
	if !ok {
		return nil
	}
	transform, ok := ecs.GetComponent[components.Transform](ctx.World, ctx.Handle)
	if !ok {
		return fmt.Errorf("npc_ai object has no transform")
	}
	if _, exists := ecs.GetComponent[components.NPCBrain](ctx.World, ctx.Handle); exists && (ctx.Reason != contracts.ObjectBehaviorInitReasonTransform || ctx.PreviousType == ctx.EntityType) {
		brain, _ := ecs.GetComponent[components.NPCBrain](ctx.World, ctx.Handle)
		ecs.ScheduleBehaviorTick(ctx.World, ctx.EntityID, npcAIKey, brain.DeadlineTick)
		return nil
	}
	ts := ecs.GetResource[ecs.TimeState](ctx.World)
	brain := components.NPCBrain{HomeX: transform.X, HomeY: transform.Y, RandomState: uint64(ctx.EntityID) ^ 0x9e3779b97f4a7c15}
	if brain.RandomState == 0 {
		brain.RandomState = 1
	}
	brain = npcRest(brain, ts.Tick, def.NPCConfig, npcTicksPerSecond(ts))
	ecs.AddComponent(ctx.World, ctx.Handle, brain)
	ecs.AddComponent(ctx.World, ctx.Handle, components.Movement{Mode: constt.Walk, State: constt.StateIdle, Speed: def.NPCConfig.Speed})
	ecs.AddComponent(ctx.World, ctx.Handle, components.CollisionResult{FinalX: transform.X, FinalY: transform.Y})
	ecs.ScheduleBehaviorTick(ctx.World, ctx.EntityID, npcAIKey, brain.DeadlineTick)
	return nil
}

// npcObservation is a copied, read-only input to the decision policy. Future
// profiles can receive stimuli here without queues or components per stimulus.
type npcObservation struct {
	X, Y   float64
	Tick   uint64
	Moving bool
}
type npcPoint struct{ X, Y float64 }
type npcDecision struct {
	Brain           components.NPCBrain
	Candidates      [npcCandidateAttempts]npcPoint
	StartWalk, Stop bool
}

// decidePassiveWander has no ECS, movement or terrain side effects.
func decidePassiveWander(brain components.NPCBrain, observation npcObservation, cfg *contracts.NPCBehaviorConfig, ticksPerSecond float64) npcDecision {
	d := npcDecision{Brain: brain}
	if brain.Activity == components.NPCWander {
		if observation.Moving && observation.Tick < brain.DeadlineTick {
			return d
		}
		d.Brain = npcRest(brain, observation.Tick, cfg, ticksPerSecond)
		d.Stop = true
		return d
	}
	if observation.Tick < brain.DeadlineTick {
		return d
	}
	d.StartWalk = true
	d.Brain.Activity = components.NPCWander
	d.Brain.DeadlineTick = observation.Tick + npcDurationTicks(cfg.MaxWalkSeconds, ticksPerSecond)
	for i := range d.Candidates {
		angle := npcRandom(&d.Brain.RandomState) * 2 * math.Pi
		distance := (cfg.WanderMinTiles + npcRandom(&d.Brain.RandomState)*(cfg.WanderMaxTiles-cfg.WanderMinTiles)) * constt.CoordPerTile
		d.Candidates[i] = npcPoint{math.Round(observation.X + math.Cos(angle)*distance), math.Round(observation.Y + math.Sin(angle)*distance)}
	}
	return d
}

func npcRest(brain components.NPCBrain, tick uint64, cfg *contracts.NPCBehaviorConfig, ticksPerSecond float64) components.NPCBrain {
	brain.Activity = components.NPCIdle
	seconds := cfg.IdleMinSeconds + npcRandom(&brain.RandomState)*(cfg.IdleMaxSeconds-cfg.IdleMinSeconds)
	brain.DeadlineTick = tick + npcDurationTicks(seconds, ticksPerSecond)
	return brain
}

func npcRandom(state *uint64) float64 {
	x := *state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	*state = x
	return float64((x*2685821657736338717)>>11) * (1.0 / (1 << 53))
}

func npcTicksPerSecond(ts *ecs.TimeState) float64 {
	if ts.TickPeriod > 0 {
		return 1 / ts.TickPeriod.Seconds()
	}
	if ts.TickRate > 0 {
		return float64(ts.TickRate)
	}
	return 10
}

func npcDurationTicks(seconds, rate float64) uint64 {
	return uint64(math.Max(1, math.Ceil(seconds*rate)))
}

func npcDefinition(w *ecs.World, h types.Handle, id types.EntityID, entityType uint32) (*objectdefs.ObjectDef, bool) {
	if w == nil || !w.Alive(h) || ecs.ObjectDestructionPending(w, h) {
		return nil, false
	}
	actualID, ok := w.GetExternalID(h)
	if !ok || actualID != id {
		return nil, false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](w, h)
	registry := objectdefs.Global()
	if !ok || info.TypeID != entityType || registry == nil {
		return nil, false
	}
	def, ok := registry.GetByID(int(entityType))
	return def, ok && def.NPCConfig != nil && def.HasBehavior(npcAIKey)
}

func (npcAIBehavior) OnScheduledTick(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
	if ctx == nil {
		return contracts.BehaviorTickResult{}, nil
	}
	def, ok := npcDefinition(ctx.World, ctx.Handle, ctx.EntityID, ctx.EntityType)
	if !ok {
		return contracts.BehaviorTickResult{}, nil
	}
	brain, hasBrain := ecs.GetComponent[components.NPCBrain](ctx.World, ctx.Handle)
	movement, hasMovement := ecs.GetComponent[components.Movement](ctx.World, ctx.Handle)
	transform, hasTransform := ecs.GetComponent[components.Transform](ctx.World, ctx.Handle)
	collider, hasCollider := ecs.GetComponent[components.Collider](ctx.World, ctx.Handle)
	if !hasBrain || !hasMovement || !hasTransform || !hasCollider {
		return contracts.BehaviorTickResult{}, fmt.Errorf("npc_ai object has incomplete runtime components")
	}
	rate := npcTicksPerSecond(ecs.GetResource[ecs.TimeState](ctx.World))
	d := decidePassiveWander(brain, npcObservation{transform.X, transform.Y, ctx.CurrentTick, movement.State == constt.StateMoving}, def.NPCConfig, rate)
	if d.Stop {
		playerstate.StopMovement(ctx.World, ctx.Handle)
	}
	if d.StartWalk {
		accepted := false
		if ctx.Deps != nil && ctx.Deps.Chunks != nil {
			for _, point := range d.Candidates {
				dx, dy := point.X-brain.HomeX, point.Y-brain.HomeY
				radius := def.NPCConfig.HomeRadiusTiles * constt.CoordPerTile
				if dx*dx+dy*dy <= radius*radius && npcTerrainAllowed(ctx.Deps.Chunks, point.X, point.Y, point.X, point.Y, collider) {
					movement.SetTargetPoint(int(point.X), int(point.Y))
					ecs.AddComponent(ctx.World, ctx.Handle, movement)
					accepted = true
					break
				}
			}
		}
		if !accepted {
			d.Brain = npcRest(d.Brain, ctx.CurrentTick, def.NPCConfig, rate)
		}
	}
	ecs.AddComponent(ctx.World, ctx.Handle, d.Brain)
	ecs.ScheduleBehaviorTick(ctx.World, ctx.EntityID, npcAIKey, d.Brain.DeadlineTick)
	// AI state is transient and must not dirty ObjectInternalState or appearance.
	return contracts.BehaviorTickResult{}, nil
}
