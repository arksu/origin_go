package behaviors

import (
	"encoding/json"
	"fmt"
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
)

// These are encounter policy parameters. Ability reach, damage, phases and
// cooldown remain exclusively in the common action definition/executor.
type cautiousDefenderParameters struct {
	NoticeRadiusTiles      float64 `json:"noticeRadiusTiles"`
	WatchMinSeconds        float64 `json:"watchMinSeconds"`
	WatchMaxSeconds        float64 `json:"watchMaxSeconds"`
	RetreatProbability     float64 `json:"retreatProbability"`
	RetreatTriggerTiles    float64 `json:"retreatTriggerTiles"`
	RetreatDistanceTiles   float64 `json:"retreatDistanceTiles"`
	RearmRadiusTiles       float64 `json:"rearmRadiusTiles"`
	RearmSeconds           float64 `json:"rearmSeconds"`
	DefenseRadiusTiles     float64 `json:"defenseRadiusTiles"`
	DefenseHomeRadiusTiles float64 `json:"defenseHomeRadiusTiles"`
	LowHPFraction          float64 `json:"lowHPFraction"`
	ChaseFailureSeconds    float64 `json:"chaseFailureSeconds"`
}

func (cautiousDefenderParameters) NPCProfileParameters()                  {}
func (p cautiousDefenderParameters) MinimumCombatHealthFraction() float64 { return p.LowHPFraction }

func defaultCautiousParameters() cautiousDefenderParameters {
	return cautiousDefenderParameters{NoticeRadiusTiles: 4, WatchMinSeconds: 2, WatchMaxSeconds: 4, RetreatProbability: .15, RetreatTriggerTiles: 1.5, RetreatDistanceTiles: 3, RearmRadiusTiles: 6, RearmSeconds: 5, DefenseRadiusTiles: 6, DefenseHomeRadiusTiles: 12, LowHPFraction: .25, ChaseFailureSeconds: 3}
}

func compileCautiousParameters(raw json.RawMessage) (contracts.NPCProfileParameters, error) {
	p := defaultCautiousParameters()
	if len(raw) > 0 && string(raw) != "null" {
		if err := decodeStrictJSON(raw, &p); err != nil {
			return nil, err
		}
	}
	for _, value := range []float64{p.NoticeRadiusTiles, p.WatchMinSeconds, p.WatchMaxSeconds, p.RetreatTriggerTiles, p.RetreatDistanceTiles, p.RearmRadiusTiles, p.RearmSeconds, p.DefenseRadiusTiles, p.DefenseHomeRadiusTiles, p.LowHPFraction, p.ChaseFailureSeconds} {
		if !npcFinitePositive(value) || value > 60 {
			return nil, fmt.Errorf("cautious defender parameters must be finite and within bounds")
		}
	}
	if p.NoticeRadiusTiles > p.RearmRadiusTiles || p.RearmRadiusTiles > 6 || p.WatchMinSeconds > p.WatchMaxSeconds || p.LowHPFraction >= 1 || math.IsNaN(p.RetreatProbability) || p.RetreatProbability < 0 || p.RetreatProbability > 1 {
		return nil, fmt.Errorf("invalid cautious defender parameter range")
	}
	return p, nil
}

func cautiousParameters(cfg contracts.NPCBehaviorConfig) cautiousDefenderParameters {
	if p, ok := cfg.Parameters.(cautiousDefenderParameters); ok {
		return p
	}
	return defaultCautiousParameters()
}

func cautiousDefenderProfile() NPCProfile {
	return NPCProfile{ID: "cautious_defender", Capabilities: []string{"locomotion", "abilities"}, PerceptionRadius: 6 * constt.CoordPerTile, RequiresAbilityAvailability: true,
		CompileParams: compileCautiousParameters,
		Validate: func(cfg contracts.NPCBehaviorConfig) error {
			if err := validatePassiveWander(cfg); err != nil {
				return err
			}
			if len(cfg.Abilities) != 1 {
				return fmt.Errorf("cautious_defender requires exactly one defensive ability")
			}
			return nil
		}, Init: cautiousRest, Decide: decideCautiousDefender, OnResult: cautiousDefenderResult}
}

// Polls align to a stable actor-specific phase to avoid synchronizing animals
// spawned together. Activity timers remain independent of decision polling.
func cautiousNextPoll(obs NPCObservation, alert bool) uint64 {
	seconds := 1.0
	if alert {
		seconds = .5
	}
	period := npcDurationTicks(seconds, obs.TicksPerSecond)
	next := obs.Tick + 1
	return next + (uint64(obs.Actor.EntityID)%period+period-next%period)%period
}

func cautiousRest(state components.NPCPolicyState, obs NPCObservation, cfg contracts.NPCBehaviorConfig) NPCDecision {
	d := restPassiveWander(state, obs, cfg)
	d.State.Cautious.IdleUntilTick = d.NextDecisionTick
	d.NextDecisionTick = cautiousNextPoll(obs, false)
	return d
}

func cautiousDecision(state components.NPCPolicyState, obs NPCObservation, intent contracts.NPCIntent) NPCDecision {
	return NPCDecision{State: state, Intent: intent, NextDecisionTick: cautiousNextPoll(obs, state.Activity != components.NPCIdle && state.Activity != components.NPCWander)}
}

func decideCautiousDefender(state components.NPCPolicyState, obs NPCObservation, cfg contracts.NPCBehaviorConfig) NPCDecision {
	p := cautiousParameters(cfg)
	m := &state.Cautious
	newDamage := obs.DamageRevision != 0 && obs.DamageRevision != m.DamageRevision
	lowHP := obs.HasHP && obs.MaxHP > 0 && obs.HP <= obs.MaxHP*p.LowHPFraction
	if newDamage {
		m.DamageRevision = obs.DamageRevision
		m.Habituated, m.AbsenceTracking, m.ChaseTracking = true, false, false
		state.Activity = components.NPCDefend
		if lowHP {
			state.Activity, m.RetreatPending = components.NPCRetreat, true
		}
	}
	if state.Activity == components.NPCDefend && lowHP {
		state.Activity, m.RetreatPending = components.NPCRetreat, true
	}

	// An executing ability always keeps movement ownership until cancellation
	// has completed. A new attacker affects only the next bite; retreat or a
	// defense stop can interrupt the accepted execution.
	defenseTarget := obs.Target
	if obs.Ability.Running() {
		defenseTarget = obs.AbilityTarget
	}
	if state.Activity == components.NPCDefend && !cautiousDefenseTargetEligible(defenseTarget, obs, p) {
		if obs.Ability.Running() && cautiousDefenseTargetEligible(obs.Target, obs, p) {
			// Retire the old bite without discarding a newer eligible aggressor.
			return cautiousDecision(state, obs, contracts.NPCIntent{Kind: contracts.NPCIntentCancelAbility, CancelGeneration: obs.Ability.Generation})
		}
		state.Activity, m.ChaseTracking = components.NPCReturnHome, false
		obs.Moving = false // The previous chase route cannot become a return route.
	}
	if obs.Ability.Running() {
		if state.Activity != components.NPCDefend {
			return cautiousDecision(state, obs, contracts.NPCIntent{Kind: contracts.NPCIntentCancelAbility, CancelGeneration: obs.Ability.Generation})
		}
		m.ChaseTracking = false
		return cautiousDecision(state, obs, contracts.NPCIntent{})
	}
	if obs.Ability.Status == contracts.NPCExecutionCompleted {
		m.ChaseTracking = false
	}

	switch state.Activity {
	case components.NPCDefend:
		return cautiousDefend(state, obs, cfg, p)
	case components.NPCRetreat:
		if m.RetreatPending {
			m.RetreatPending = false
			threat := obs.Target
			if !newDamage && obs.DamageRevision == 0 {
				threat = obs.Encounter
			}
			// A zero-distance threat still needs a deterministic escape direction.
			if threat.Valid && npcDistanceSquared(obs.Position, threat.Position) == 0 {
				threat.Position.X--
			}
			return cautiousDecision(state, obs, NPCFlee(obs.Position, threat, p.RetreatDistanceTiles*constt.CoordPerTile, obs.Tick+npcDurationTicks(cfg.MaxWalkSeconds, obs.TicksPerSecond)))
		}
		if obs.Moving && obs.Tick < obs.ActivityDeadlineTick {
			return cautiousDecision(state, obs, contracts.NPCIntent{})
		}
		state.Activity = components.NPCReturnHome
		return cautiousReturnHome(state, obs, cfg)
	case components.NPCReturnHome:
		return cautiousReturnHome(state, obs, cfg)
	}

	// Unknown perception cannot establish absence, rearm an encounter, or select
	// a replacement threat. Known presence within six tiles retains habituation.
	if !obs.PerceptionKnown || (obs.NearbyPlayer.Valid && npcDistanceSquared(obs.Position, obs.NearbyPlayer.Position) <= npcSquare(p.RearmRadiusTiles*constt.CoordPerTile)) {
		m.AbsenceTracking = false
	} else if m.Habituated {
		if !m.AbsenceTracking {
			m.AbsenceTracking, m.AbsentSinceTick = true, obs.Tick
		}
		if obs.Tick-m.AbsentSinceTick >= npcDurationTicks(p.RearmSeconds, obs.TicksPerSecond) {
			m.Habituated, m.AbsenceTracking, m.RetreatChosen = false, false, false
			m.Encounter = contracts.EntityRef{}
		}
	}
	if !m.Habituated && obs.PerceptionKnown && obs.NearbyPlayer.Valid && npcDistanceSquared(obs.Position, obs.NearbyPlayer.Position) <= npcSquare(p.NoticeRadiusTiles*constt.CoordPerTile) {
		m.Encounter, m.Habituated = obs.NearbyPlayer.Ref, true
		m.RetreatChosen = npcRandom(&state.RandomState) < p.RetreatProbability
		m.WatchUntilTick = obs.Tick + npcDurationTicks(p.WatchMinSeconds+npcRandom(&state.RandomState)*(p.WatchMaxSeconds-p.WatchMinSeconds), obs.TicksPerSecond)
		state.Activity = components.NPCWatch
		obs.Encounter = obs.NearbyPlayer
		m.LastEncounterDistanceSquared = npcDistanceSquared(obs.Position, obs.Encounter.Position)
	}
	if state.Activity == components.NPCWatch {
		if !obs.Encounter.Valid || !obs.Encounter.HasHP || obs.Encounter.HP <= 0 || obs.Encounter.Incapacitated || obs.Tick >= m.WatchUntilTick {
			return cautiousRest(state, obs, cfg)
		}
		distance := npcDistanceSquared(obs.Position, obs.Encounter.Position)
		approaching := obs.Encounter.Moving && distance < m.LastEncounterDistanceSquared-0.000001
		m.LastEncounterDistanceSquared = distance
		if m.RetreatChosen && approaching && distance <= npcSquare(p.RetreatTriggerTiles*constt.CoordPerTile) {
			m.RetreatChosen = false
			state.Activity = components.NPCRetreat
			threat := obs.Encounter
			if npcDistanceSquared(obs.Position, threat.Position) == 0 {
				threat.Position.X--
			}
			return cautiousDecision(state, obs, NPCFlee(obs.Position, threat, p.RetreatDistanceTiles*constt.CoordPerTile, obs.Tick+npcDurationTicks(cfg.MaxWalkSeconds, obs.TicksPerSecond)))
		}
		return cautiousDecision(state, obs, contracts.NPCIntent{Kind: contracts.NPCIntentFace, Face: obs.Encounter.Position})
	}
	// Reuse passive movement selection with its own idle deadline. Frequent
	// perception polls must neither reroll idle times nor restart wander routes.
	obs.NextDecisionTick = m.IdleUntilTick
	if state.Activity == components.NPCIdle && obs.Tick >= m.IdleUntilTick && npcDistanceSquared(obs.Position, obs.Home) > npcSquare(cfg.HomeRadiusTiles*constt.CoordPerTile) {
		state.Activity = components.NPCReturnHome
		return cautiousReturnHome(state, obs, cfg)
	}
	d := decidePassiveWander(state, obs, cfg)
	if d.State.Activity == components.NPCIdle {
		d.State.Cautious.IdleUntilTick = d.NextDecisionTick
	}
	d.NextDecisionTick = cautiousNextPoll(obs, false)
	return d
}

func cautiousDefend(state components.NPCPolicyState, obs NPCObservation, cfg contracts.NPCBehaviorConfig, p cautiousDefenderParameters) NPCDecision {
	m := &state.Cautious
	inRange := obs.AvailableAbility.Range > 0 && npcDistanceSquared(obs.Position, obs.Target.Position) <= npcSquare(obs.AvailableAbility.Range)
	if inRange && !obs.AvailableAbility.Ready {
		// A legitimate cooldown wait is not an unsuccessful attack attempt.
		m.ChaseTracking = false
		return cautiousDecision(state, obs, contracts.NPCIntent{Kind: contracts.NPCIntentFace, Face: obs.Target.Position})
	}
	if !m.ChaseTracking || npcDistanceSquared(obs.Position, m.LastChasePosition) >= 1 {
		m.ChaseTracking, m.ChaseFailedSinceTick, m.LastChasePosition = true, obs.Tick, obs.Position
	}
	if obs.Tick-m.ChaseFailedSinceTick >= npcDurationTicks(p.ChaseFailureSeconds, obs.TicksPerSecond) {
		state.Activity, m.ChaseTracking = components.NPCReturnHome, false
		obs.Moving = false
		return cautiousReturnHome(state, obs, cfg)
	}
	if inRange {
		return cautiousDecision(state, obs, contracts.NPCIntent{Kind: contracts.NPCIntentUseAbility, Ability: contracts.NPCAbilityRequest{AbilityID: cfg.Abilities[0], Target: contracts.NPCTarget{Kind: contracts.NPCTargetEntity, Entity: obs.Target.Ref}}})
	}
	return cautiousDecision(state, obs, NPCChase(obs.Target, obs.Tick+npcDurationTicks(p.ChaseFailureSeconds, obs.TicksPerSecond)))
}

func cautiousDefenseTargetEligible(target NPCObservedEntity, obs NPCObservation, p cautiousDefenderParameters) bool {
	return target.Valid && target.HasHP && target.HP > 0 && !target.Incapacitated && npcDistanceSquared(obs.Position, target.Position) <= npcSquare(p.DefenseRadiusTiles*constt.CoordPerTile) && npcDistanceSquared(obs.Position, obs.Home) <= npcSquare(p.DefenseHomeRadiusTiles*constt.CoordPerTile)
}

func cautiousReturnHome(state components.NPCPolicyState, obs NPCObservation, cfg contracts.NPCBehaviorConfig) NPCDecision {
	distance := math.Sqrt(npcDistanceSquared(obs.Position, obs.Home))
	if distance <= constt.CoordPerTile {
		return cautiousRest(state, obs, cfg)
	}
	if state.Cautious.ReturnRetryTick != 0 {
		if obs.Tick < state.Cautious.ReturnRetryTick {
			return NPCDecision{State: state, Intent: contracts.NPCIntent{Kind: contracts.NPCIntentStop}, NextDecisionTick: state.Cautious.ReturnRetryTick}
		}
		// The previous failure was consumed when the delay began. Retry once
		// even if an early wake retained that failure observation.
		state.Cautious.ReturnRetryTick = 0
	} else if obs.LastResult == contracts.NPCExecutionBlocked || obs.LastResult == contracts.NPCExecutionRejected || obs.LastResult == contracts.NPCExecutionTimeout {
		state.Cautious.ReturnRetryTick = obs.Tick + npcDurationTicks(1, obs.TicksPerSecond)
		return NPCDecision{State: state, Intent: contracts.NPCIntent{Kind: contracts.NPCIntentStop}, NextDecisionTick: state.Cautious.ReturnRetryTick}
	}
	if obs.Moving && obs.Tick < obs.ActivityDeadlineTick {
		return cautiousDecision(state, obs, contracts.NPCIntent{})
	}
	step := math.Min(distance, 2*constt.CoordPerTile)
	point := contracts.NPCPoint{X: obs.Position.X + (obs.Home.X-obs.Position.X)*step/distance, Y: obs.Position.Y + (obs.Home.Y-obs.Position.Y)*step/distance}
	return cautiousDecision(state, obs, NPCMoveTo(point, obs.Tick+npcDurationTicks(cfg.MaxWalkSeconds, obs.TicksPerSecond), 0))
}

func cautiousDefenderResult(state components.NPCPolicyState, obs NPCObservation, cfg contracts.NPCBehaviorConfig, result contracts.NPCExecutionResult) NPCDecision {
	if obs.Ability.Running() && (result.Status == contracts.NPCExecutionCanceled || result.Status == contracts.NPCExecutionCompleted) {
		obs.Ability, obs.Moving = contracts.NPCAbilityState{}, false
		return decideCautiousDefender(state, obs, cfg)
	}
	if state.Activity == components.NPCDefend && result.Status == contracts.NPCExecutionAccepted && obs.AvailableAbility.Range > 0 && npcDistanceSquared(obs.Position, obs.Target.Position) <= npcSquare(obs.AvailableAbility.Range) {
		state.Cautious.ChaseTracking = false
	}
	if result.Status == contracts.NPCExecutionBlocked || result.Status == contracts.NPCExecutionRejected {
		switch state.Activity {
		case components.NPCWander:
			return cautiousRest(state, obs, cfg)
		case components.NPCReturnHome:
			state.Cautious.ReturnRetryTick = obs.Tick + npcDurationTicks(1, obs.TicksPerSecond)
			return NPCDecision{State: state, Intent: contracts.NPCIntent{Kind: contracts.NPCIntentStop}, NextDecisionTick: state.Cautious.ReturnRetryTick}
		case components.NPCRetreat:
			state.Activity = components.NPCReturnHome
			return cautiousDecision(state, obs, contracts.NPCIntent{Kind: contracts.NPCIntentStop})
		case components.NPCDefend:
			return cautiousDecision(state, obs, contracts.NPCIntent{Kind: contracts.NPCIntentFace, Face: obs.Target.Position})
		}
	}
	return NPCDecision{State: state}
}

func npcSquare(value float64) float64 { return value * value }
func npcDistanceSquared(a, b contracts.NPCPoint) float64 {
	return npcSquare(a.X-b.X) + npcSquare(a.Y-b.Y)
}
