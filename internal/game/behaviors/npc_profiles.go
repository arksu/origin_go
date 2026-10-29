package behaviors

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
)

const npcCandidateAttempts = contracts.NPCCandidateLimit

type NPCObservedEntity struct {
	Ref           contracts.EntityRef
	Position      contracts.NPCPoint
	Valid         bool
	Moving        bool
	HP            float64
	HasHP         bool
	Incapacitated bool
}
type NPCObservation struct {
	Actor                contracts.EntityRef
	Position, Home       contracts.NPCPoint
	Tick                 uint64
	TicksPerSecond       float64
	Moving               bool
	Target               NPCObservedEntity
	Ability              contracts.NPCAbilityState
	AbilityTarget        NPCObservedEntity
	LastResult           contracts.NPCExecutionStatus
	ActivityDeadlineTick uint64
	NextDecisionTick     uint64
	HP, MaxHP            float64
	HasHP                bool
	DamageRevision       uint64
	NearbyPlayer         NPCObservedEntity
	Encounter            NPCObservedEntity
	PerceptionKnown      bool
	AvailableAbility     contracts.NPCAbilityAvailabilityState
}
type NPCDecision struct {
	State            components.NPCPolicyState
	Intent           contracts.NPCIntent
	NextDecisionTick uint64
}

// Policies receive only copied state and observations, never a World or executor.
type NPCPolicy func(components.NPCPolicyState, NPCObservation, contracts.NPCBehaviorConfig) NPCDecision

// NPCResultPolicy may return at most one follow-up intent, executed without another callback.
type NPCResultPolicy func(components.NPCPolicyState, NPCObservation, contracts.NPCBehaviorConfig, contracts.NPCExecutionResult) NPCDecision
type NPCProfile struct {
	ID            string
	Capabilities  []string
	Validate      func(contracts.NPCBehaviorConfig) error
	CompileParams func(json.RawMessage) (contracts.NPCProfileParameters, error)
	Init          NPCPolicy
	Decide        NPCPolicy
	OnResult      NPCResultPolicy
	// PerceptionRadius is a bounded center-distance query in world coordinates.
	PerceptionRadius            float64
	RequiresAbilityAvailability bool
}
type NPCCapability struct {
	ID               string
	Move, UseAbility bool
}

// NPCCatalog owns copied registration metadata and is immutable after construction.
type NPCCatalog struct {
	profiles     map[string]NPCProfile
	capabilities map[string]NPCCapability
	abilities    map[string]contracts.NPCAbilityDefinition
}

func NewNPCCatalog(profiles []NPCProfile, capabilities []NPCCapability, abilities []contracts.NPCAbilityDefinition) (*NPCCatalog, error) {
	c := &NPCCatalog{profiles: make(map[string]NPCProfile), capabilities: make(map[string]NPCCapability), abilities: make(map[string]contracts.NPCAbilityDefinition)}
	for _, cap := range append([]NPCCapability{{ID: "locomotion", Move: true}, {ID: "abilities", UseAbility: true}}, capabilities...) {
		if strings.TrimSpace(cap.ID) != cap.ID || cap.ID == "" || (!cap.Move && !cap.UseAbility) {
			return nil, fmt.Errorf("invalid NPC capability %q", cap.ID)
		}
		if _, exists := c.capabilities[cap.ID]; exists {
			return nil, fmt.Errorf("duplicate NPC capability %q", cap.ID)
		}
		c.capabilities[cap.ID] = cap
	}
	passive := NPCProfile{ID: "passive_wander", Capabilities: []string{"locomotion"}, Validate: validatePassiveWander, Init: restPassiveWander, Decide: decidePassiveWander, OnResult: passiveWanderResult}
	for _, p := range append([]NPCProfile{passive}, profiles...) {
		if p.ID == "" || strings.TrimSpace(p.ID) != p.ID || p.Decide == nil || len(p.Capabilities) > 8 {
			return nil, fmt.Errorf("invalid NPC profile %q", p.ID)
		}
		if _, exists := c.profiles[p.ID]; exists {
			return nil, fmt.Errorf("duplicate NPC profile %q", p.ID)
		}
		p.Capabilities = append([]string(nil), p.Capabilities...)
		for _, id := range p.Capabilities {
			if _, ok := c.capabilities[id]; !ok {
				return nil, fmt.Errorf("profile %q requires unknown capability %q", p.ID, id)
			}
		}
		c.profiles[p.ID] = p
	}
	for _, a := range abilities {
		if a.ID == "" || strings.TrimSpace(a.ID) != a.ID || a.TargetKind > contracts.NPCTargetDirection {
			return nil, fmt.Errorf("invalid NPC ability %q", a.ID)
		}
		if _, exists := c.abilities[a.ID]; exists {
			return nil, fmt.Errorf("duplicate NPC ability %q", a.ID)
		}
		c.abilities[a.ID] = a
	}
	return c, nil
}

var defaultNPCCatalog = func() *NPCCatalog {
	c, err := NewNPCCatalog([]NPCProfile{cautiousDefenderProfile()}, nil, []contracts.NPCAbilityDefinition{{ID: "fox_bite", TargetKind: contracts.NPCTargetEntity, OwnsMovement: true}})
	if err != nil {
		panic(err)
	}
	return c
}()

func DefaultNPCCatalog() *NPCCatalog { return defaultNPCCatalog }

func (c *NPCCatalog) validate(cfg *contracts.NPCBehaviorConfig) error {
	if len(cfg.Capabilities) > 8 || len(cfg.Abilities) > 8 {
		return fmt.Errorf("NPC supports at most eight capabilities and abilities")
	}
	p, ok := c.profiles[cfg.Profile]
	if !ok {
		return fmt.Errorf("unknown npc_ai profile %q", cfg.Profile)
	}
	if !npcFinitePositive(cfg.Speed) || cfg.Speed > 256 {
		return fmt.Errorf("npc_ai speed exceeds supported bounds")
	}
	if len(cfg.Capabilities) == 0 {
		cfg.Capabilities = append([]string(nil), p.Capabilities...)
	}
	for _, id := range cfg.Capabilities {
		if _, ok := c.capabilities[id]; !ok {
			return fmt.Errorf("unknown NPC capability %q", id)
		}
	}
	for _, id := range p.Capabilities {
		if !npcContains(cfg.Capabilities, id) {
			return fmt.Errorf("profile %q requires capability %q", p.ID, id)
		}
	}
	for _, id := range cfg.Abilities {
		if _, ok := c.abilities[id]; !ok {
			return fmt.Errorf("unknown NPC ability %q", id)
		}
		if !c.canUseAbility(*cfg) {
			return fmt.Errorf("NPC abilities require an ability capability")
		}
	}
	if p.CompileParams != nil {
		value, err := p.CompileParams(cfg.Params)
		if err != nil {
			return fmt.Errorf("profile %q params: %w", p.ID, err)
		}
		cfg.Parameters = value
	} else if len(cfg.Params) > 0 && string(cfg.Params) != "null" && string(cfg.Params) != "{}" {
		return fmt.Errorf("profile %q does not accept params", p.ID)
	}
	if p.Validate != nil {
		return p.Validate(*cfg)
	}
	return nil
}
func (c *NPCCatalog) canMove(cfg contracts.NPCBehaviorConfig) bool {
	for _, id := range c.configCapabilities(cfg) {
		if c.capabilities[id].Move {
			return true
		}
	}
	return false
}
func (c *NPCCatalog) canUseAbility(cfg contracts.NPCBehaviorConfig) bool {
	for _, id := range c.configCapabilities(cfg) {
		if c.capabilities[id].UseAbility {
			return true
		}
	}
	return false
}
func npcContains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func npcFinitePositive(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
func validatePassiveWander(cfg contracts.NPCBehaviorConfig) error {
	for _, v := range []float64{cfg.IdleMinSeconds, cfg.IdleMaxSeconds, cfg.WanderMinTiles, cfg.WanderMaxTiles, cfg.HomeRadiusTiles, cfg.MaxWalkSeconds} {
		if !npcFinitePositive(v) {
			return fmt.Errorf("npc_ai parameters must be finite and positive")
		}
	}
	if cfg.IdleMinSeconds > cfg.IdleMaxSeconds || cfg.IdleMaxSeconds > 3600 || cfg.WanderMinTiles > cfg.WanderMaxTiles || cfg.WanderMaxTiles > cfg.HomeRadiusTiles || cfg.HomeRadiusTiles > 64 || cfg.MaxWalkSeconds > 60 {
		return fmt.Errorf("npc_ai parameters exceed supported bounds")
	}
	return nil
}

func restPassiveWander(state components.NPCPolicyState, obs NPCObservation, cfg contracts.NPCBehaviorConfig) NPCDecision {
	state.Activity = components.NPCIdle
	seconds := cfg.IdleMinSeconds + npcRandom(&state.RandomState)*(cfg.IdleMaxSeconds-cfg.IdleMinSeconds)
	return NPCDecision{State: state, Intent: contracts.NPCIntent{Kind: contracts.NPCIntentStop}, NextDecisionTick: obs.Tick + npcDurationTicks(seconds, obs.TicksPerSecond)}
}
func decidePassiveWander(state components.NPCPolicyState, obs NPCObservation, cfg contracts.NPCBehaviorConfig) NPCDecision {
	if state.Activity == components.NPCWander {
		if obs.Moving && obs.Tick < obs.ActivityDeadlineTick {
			return NPCDecision{State: state, NextDecisionTick: obs.ActivityDeadlineTick}
		}
		return restPassiveWander(state, obs, cfg)
	}
	if obs.Tick < obs.NextDecisionTick {
		return NPCDecision{State: state, NextDecisionTick: obs.NextDecisionTick}
	}
	state.Activity = components.NPCWander
	deadline := obs.Tick + npcDurationTicks(cfg.MaxWalkSeconds, obs.TicksPerSecond)
	move := contracts.NPCMoveIntent{Count: npcCandidateAttempts, DeadlineTick: deadline, HomeRadius: cfg.HomeRadiusTiles * constt.CoordPerTile}
	for i := range move.Candidates {
		angle := npcRandom(&state.RandomState) * 2 * math.Pi
		distance := (cfg.WanderMinTiles + npcRandom(&state.RandomState)*(cfg.WanderMaxTiles-cfg.WanderMinTiles)) * constt.CoordPerTile
		move.Candidates[i] = contracts.NPCPoint{X: math.Round(obs.Position.X + math.Cos(angle)*distance), Y: math.Round(obs.Position.Y + math.Sin(angle)*distance)}
	}
	return NPCDecision{State: state, Intent: contracts.NPCIntent{Kind: contracts.NPCIntentMove, Move: move}, NextDecisionTick: deadline}
}
func passiveWanderResult(state components.NPCPolicyState, obs NPCObservation, cfg contracts.NPCBehaviorConfig, result contracts.NPCExecutionResult) NPCDecision {
	if result.Status == contracts.NPCExecutionBlocked || result.Status == contracts.NPCExecutionRejected {
		return restPassiveWander(state, obs, cfg)
	}
	return NPCDecision{State: state, NextDecisionTick: obs.ActivityDeadlineTick}
}

// NPCMoveTo is a reusable point-movement intent; the controller owns validation.
func NPCMoveTo(point contracts.NPCPoint, deadline uint64, homeRadius float64) contracts.NPCIntent {
	return contracts.NPCIntent{Kind: contracts.NPCIntentMove, Move: contracts.NPCMoveIntent{Candidates: [npcCandidateAttempts]contracts.NPCPoint{point}, Count: 1, DeadlineTick: deadline, HomeRadius: homeRadius}}
}

// NPCChase follows an observed entity without imposing a passive-wander home rule.
func NPCChase(target NPCObservedEntity, deadline uint64) contracts.NPCIntent {
	intent := NPCMoveTo(target.Position, deadline, 0)
	intent.Move.Target = target.Ref
	if !target.Valid {
		intent.Kind = contracts.NPCIntentStop
	}
	return intent
}
func NPCFlee(position contracts.NPCPoint, threat NPCObservedEntity, distance float64, deadline uint64) contracts.NPCIntent {
	dx, dy := position.X-threat.Position.X, position.Y-threat.Position.Y
	length := math.Hypot(dx, dy)
	if !threat.Valid || length == 0 || !npcFinitePositive(distance) {
		return contracts.NPCIntent{Kind: contracts.NPCIntentStop}
	}
	return NPCMoveTo(contracts.NPCPoint{X: math.Round(position.X + dx/length*distance), Y: math.Round(position.Y + dy/length*distance)}, deadline, 0)
}
func npcRandom(state *uint64) float64 {
	x := *state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	*state = x
	return float64((x*2685821657736338717)>>11) * (1.0 / (1 << 53))
}
func npcDurationTicks(seconds, rate float64) uint64 {
	return uint64(math.Max(1, math.Ceil(seconds*rate)))
}

func (c *NPCCatalog) configCapabilities(cfg contracts.NPCBehaviorConfig) []string {
	if len(cfg.Capabilities) > 0 {
		return cfg.Capabilities
	}
	return c.profiles[cfg.Profile].Capabilities
}
