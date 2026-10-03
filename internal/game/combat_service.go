package game

import (
	"go.uber.org/zap"
	"math"
	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/entitystats"
	"origin/internal/types"
	"sort"
	"time"
)

type CombatResult struct {
	ActorID       types.EntityID
	ActorHandle   types.Handle
	ExecutionID   uint64
	EventSequence uint64
	TimeMs        int64
	Hits          []CombatHit
}

type CombatService struct {
	ecs.BaseSystem
	world         *ecs.World
	receivers     *CombatReceivers
	enabled       bool
	logger        *zap.Logger
	active        map[types.Handle]struct{}
	eventSequence uint64
	Strength      CombatStrengthProvider
	OnState       func(types.Handle)
	OnResult      func(CombatResult)
	actions       *ActionService
}

func NewCombatService(world *ecs.World, receivers *CombatReceivers, enabled bool, logger *zap.Logger) *CombatService {
	return &CombatService{BaseSystem: ecs.NewBaseSystem("CombatSystem", 316), world: world, receivers: receivers, enabled: enabled, logger: logger, active: make(map[types.Handle]struct{}), Strength: baseCombatStrength}
}

func combatActorRestricted(world *ecs.World, actor types.Handle) bool {
	if !world.Alive(actor) {
		return true
	}
	if movement, ok := ecs.GetComponent[components.Movement](world, actor); ok && movement.State == constt.StateStunned {
		return true
	}
	if health, ok := ecs.GetComponent[components.EntityHealth](world, actor); ok && (health.HHP <= 0 || health.SHP <= 0 || health.KOUntilTick != 0) {
		return true
	}
	return false
}

func combatConflictingWork(world *ecs.World, actor types.Handle) bool {
	if active, ok := ecs.GetComponent[components.ActiveGameAction](world, actor); ok && active.Phase != components.GameActionSelecting {
		return true
	}
	return ecs.HasComponent[components.ActiveCyclicAction](world, actor) ||
		ecs.HasComponent[components.ActiveCraft](world, actor) ||
		ecs.HasComponent[components.PendingBuildPlacement](world, actor) ||
		ecs.HasComponent[components.PendingContextAction](world, actor) ||
		ecs.HasComponent[components.LiftCarryState](world, actor) ||
		ecs.HasComponent[components.PendingLiftTransition](world, actor)
}

func (service *CombatService) Busy(actor types.Handle) bool {
	state, ok := ecs.GetComponent[components.CombatState](service.world, actor)
	if ok && state.Execution != nil && state.Execution.StrikeResolved && !ecs.GetResource[ecs.TimeState](service.world).Now.Before(state.Execution.RecoveryEnd) {
		service.finish(actor)
	}
	return components.CombatCommitted(service.world, actor)
}

func (service *CombatService) Begin(actor types.Handle, definition *actiondefs.Definition, weapon combatWeapon, direction combat.Point, selection uint64) string {
	world := service.world
	if !service.enabled {
		return "COMBAT_DISABLED"
	}
	if definition == nil || definition.Execution.Combat == nil || service.receivers == nil || service.Strength == nil {
		return "COMBAT_UNAVAILABLE"
	}
	if service.Busy(actor) || combatConflictingWork(world, actor) {
		return "ACTION_BUSY"
	}
	if combatActorRestricted(world, actor) {
		return "COMBAT_INTERRUPTED"
	}
	profile := definition.Execution.Combat
	if err := (combat.Sector{Direction: direction, Range: weapon.Range, AngleDegrees: profile.SectorAngleDegrees}).Validate(); err != nil {
		return "COMBAT_INVALID_DIRECTION"
	}
	if err := weapon.Validate(); err != nil {
		return "COMBAT_INVALID_WEAPON"
	}
	if _, ok := ecs.GetComponent[components.Transform](world, actor); !ok {
		return "COMBAT_UNAVAILABLE"
	}
	if profile.WindupMs <= 0 || profile.RecoveryMs <= 0 || profile.CooldownMs <= 0 || definition.Execution.Stamina < 0 || math.IsNaN(definition.Execution.Stamina) || math.IsInf(definition.Execution.Stamina, 0) {
		return "COMBAT_INVALID_DEFINITION"
	}
	state, _ := ecs.GetComponent[components.CombatState](world, actor)
	timing := ecs.GetResource[ecs.TimeState](world)
	if timing.Now.Before(state.Cooldowns[definition.ID]) {
		return "COMBAT_COOLDOWN"
	}
	if state.Sequence == math.MaxUint64 || service.eventSequence == math.MaxUint64 {
		return "COMBAT_UNAVAILABLE"
	}
	stats, hasStats := ecs.GetComponent[components.EntityStats](world, actor)
	if !hasStats || math.IsNaN(stats.Stamina) || math.IsInf(stats.Stamina, 0) || stats.Stamina < definition.Execution.Stamina {
		return "LOW_STAMINA"
	}
	actorID, _ := world.GetExternalID(actor)
	// Clear intents before payment; no deferred pickup/link may finish during commitment.
	if _, _, err := ecs.BreakLinkForPlayer(world, actorID, ecs.LinkBreakMoved); err != nil {
		service.logger.Error("combat link cleanup failed", zap.Error(err))
		return "COMBAT_UNAVAILABLE"
	}
	systems.ClearPlayerInteractionIntents(world, actor, actorID)
	if state.Cooldowns == nil {
		state.Cooldowns = make(map[string]time.Time)
	}
	state.Sequence++
	state.Revision++
	state.Cooldowns[definition.ID] = timing.Now.Add(time.Duration(profile.CooldownMs) * time.Millisecond)
	strikeAt := timing.Now.Add(time.Duration(profile.WindupMs) * time.Millisecond)
	state.Execution = &components.CombatExecution{
		ID: state.Sequence, ActionID: definition.ID, SelectionGeneration: selection, Direction: direction, Weapon: weapon.Weapon,
		WeaponItemID: weapon.ItemID, WeaponSlot: weapon.Slot, AngleDegrees: profile.SectorAngleDegrees, Multiplier: profile.DamageMultiplier, Nearest: profile.Selection == actiondefs.SelectionNearest,
		StartedAt: timing.Now, StrikeAt: strikeAt, RecoveryEnd: strikeAt.Add(time.Duration(profile.RecoveryMs) * time.Millisecond), StartedAtMs: timing.UnixMs,
	}
	state.LastCombatEventAt = max(state.LastCombatEventAt, timing.UnixMs)
	stats.Stamina -= definition.Execution.Stamina
	ecs.AddComponent(world, actor, stats)
	ecs.AddComponent(world, actor, state)
	ecs.WithComponent(world, actor, func(active *components.ActiveGameAction) { active.Phase = components.GameActionWindup })
	con := characterattrs.DefaultValue
	if profile, ok := ecs.GetComponent[components.CharacterProfile](world, actor); ok {
		con = characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	ecs.MarkPlayerStatsDirtyByHandle(world, actor, ecs.ResolvePlayerStatsTTLms(world))
	ecs.UpdateEntityStatsRegenSchedule(world, actor, stats.Stamina, stats.Energy, entitystats.MaxStaminaFromCon(con))
	ecs.MarkMovementModeDirtyByHandle(world, actor)
	service.active[actor] = struct{}{}
	service.eventSequence++
	state.Execution.StartEventSequence = service.eventSequence
	service.changed(actor)
	return ""
}

func (service *CombatService) changed(actor types.Handle) {
	cyclicaction.SyncCombat(service.world, actor)
	if service.actions != nil {
		if id, ok := service.world.GetExternalID(actor); ok {
			service.actions.SendState(service.world, id, actor)
		}
	}
	if service.OnState != nil {
		service.OnState(actor)
	}
}

func (service *CombatService) finish(actor types.Handle) {
	ecs.WithComponent(service.world, actor, func(state *components.CombatState) { state.Execution = nil; state.Revision++ })
	ecs.RemoveComponent[components.ActiveGameAction](service.world, actor)
	delete(service.active, actor)
	ecs.MarkMovementModeDirtyByHandle(service.world, actor)
	service.changed(actor)
}

// Interrupt is for external stun/KO/death/invalidation/teardown, never ordinary damage.
func (service *CombatService) Interrupt(actor types.Handle) {
	if components.CombatCommitted(service.world, actor) {
		service.finish(actor)
	}
	delete(service.active, actor)
}

func (service *CombatService) Update(world *ecs.World, _ float64) {
	if world != service.world {
		return
	}
	handles := make([]types.Handle, 0, len(service.active))
	for actor := range service.active {
		if !world.Alive(actor) {
			delete(service.active, actor)
			continue
		}
		if combatActorRestricted(world, actor) {
			service.Interrupt(actor)
			continue
		}
		handles = append(handles, actor)
	}
	sort.Slice(handles, func(left, right int) bool {
		first, _ := ecs.GetComponent[components.CombatState](world, handles[left])
		second, _ := ecs.GetComponent[components.CombatState](world, handles[right])
		if first.Execution == nil || second.Execution == nil {
			return handles[left] < handles[right]
		}
		if !first.Execution.StrikeAt.Equal(second.Execution.StrikeAt) {
			return first.Execution.StrikeAt.Before(second.Execution.StrikeAt)
		}
		if first.Execution.ID != second.Execution.ID {
			return first.Execution.ID < second.Execution.ID
		}
		firstID, _ := world.GetExternalID(handles[left])
		secondID, _ := world.GetExternalID(handles[right])
		return firstID < secondID
	})
	timing := ecs.GetResource[ecs.TimeState](world)
	for _, actor := range handles {
		state, ok := ecs.GetComponent[components.CombatState](world, actor)
		if !ok || state.Execution == nil {
			delete(service.active, actor)
			continue
		}
		execution := state.Execution
		if execution.WeaponItemID != 0 && !service.weaponStillValid(actor, execution) {
			service.Interrupt(actor)
			continue
		}
		if !execution.StrikeResolved && !timing.Now.Before(execution.StrikeAt) {
			// Retire the strike before callbacks; errors cannot cause a retry or refund.
			execution.StrikeResolved = true
			ecs.WithComponent(world, actor, func(current *components.CombatState) {
				current.Revision++
				current.LastCombatEventAt = max(current.LastCombatEventAt, timing.UnixMs)
			})
			ecs.WithComponent(world, actor, func(active *components.ActiveGameAction) { active.Phase = components.GameActionRecovery })
			ecs.MarkMovementModeDirtyByHandle(world, actor)
			service.strike(actor, execution)
			service.changed(actor)
		}
		if !timing.Now.Before(execution.RecoveryEnd) && components.CombatCommitted(world, actor) {
			service.finish(actor)
		}
	}
}

func (service *CombatService) weaponStillValid(actor types.Handle, execution *components.CombatExecution) bool {
	if service.actions == nil {
		return false
	}
	definition, ok := service.actions.definitions.Get(execution.ActionID)
	if !ok {
		return false
	}
	actorID, _ := service.world.GetExternalID(actor)
	weapon, err := resolveCombatWeapon(service.world, actorID, definition)
	return err == nil && weapon.ItemID == execution.WeaponItemID && weapon.Weapon == execution.Weapon && weapon.Slot == execution.WeaponSlot
}

func (service *CombatService) strike(actor types.Handle, execution *components.CombatExecution) {
	world := service.world
	actorID, _ := world.GetExternalID(actor)
	timing := ecs.GetResource[ecs.TimeState](world)
	service.eventSequence++
	result := CombatResult{ActorID: actorID, ActorHandle: actor, ExecutionID: execution.ID, EventSequence: service.eventSequence, TimeMs: timing.UnixMs}
	defer func() {
		if service.OnResult != nil {
			service.OnResult(result)
		}
	}()
	transform, ok := ecs.GetComponent[components.Transform](world, actor)
	if !ok {
		service.logger.Error("combat strike missing transform")
		return
	}
	strength, err := service.Strength(world, actor)
	if err != nil {
		service.logger.Error("combat strength unavailable", zap.Error(err))
		return
	}
	raw, err := combat.RawDamage(execution.Weapon, strength, execution.Multiplier)
	if err != nil {
		service.logger.Error("combat damage invalid", zap.Error(err))
		return
	}
	sector := combat.Sector{Origin: combat.Point{X: transform.X, Y: transform.Y}, Direction: execution.Direction, Range: execution.Weapon.Range, AngleDegrees: execution.AngleDegrees}
	contacts, err := service.receivers.Contacts(actorID, sector, execution.Nearest)
	if err != nil {
		service.logger.Error("combat contacts failed", zap.Error(err))
		return
	}
	for _, contact := range contacts {
		service.eventSequence++
		hit, applied, err := service.receivers.Apply(CombatHit{EventSequence: service.eventSequence, ExecutionID: execution.ID, AttackerID: actorID, AttackerIncarnation: actor, TargetID: contact.ID, TargetIncarnation: contact.Incarnation, RawDamage: raw, TimeMs: timing.UnixMs})
		if err != nil {
			service.logger.Error("combat hit failed", zap.Uint64("target_id", uint64(contact.ID)), zap.Error(err))
			continue
		}
		if applied {
			result.Hits = append(result.Hits, hit)
			ecs.WithComponent(world, actor, func(state *components.CombatState) {
				state.LastCombatEventAt = max(state.LastCombatEventAt, timing.UnixMs)
			})
		}
	}
}
