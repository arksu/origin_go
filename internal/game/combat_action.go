package game

import (
	"fmt"
	"origin/internal/actiondefs"
	"origin/internal/combat"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func (service *ActionService) SetCombatService(combat *CombatService) {
	service.combat = combat
	combat.actions = service
}
func (service *ActionService) IsCombatAction(id string) bool {
	definition, ok := service.definitions.Get(id)
	return ok && definition.Execution.Combat != nil
}
func (service *ActionService) CombatSupported() bool {
	return service.combat != nil && service.combat.enabled
}
func (service *ActionService) ActivateCombat(world *ecs.World, id types.EntityID, actor types.Handle, actionID string) {
	if !service.IsCombatAction(actionID) {
		service.alert(id, "ACTION_UNKNOWN")
		return
	}
	service.Activate(world, id, actor, actionID)
}
func (service *ActionService) CommitCombat(world *ecs.World, id types.EntityID, actor types.Handle, click *netproto.MapClick) {
	if world != service.world || !service.CombatSupported() || click == nil || click.CombatAttempt == nil {
		return
	}
	attempt := click.CombatAttempt
	active, ok := ecs.GetComponent[components.ActiveGameAction](world, actor)
	if !ok || active.ActionID != attempt.ActionId || active.Generation != attempt.SelectionGeneration || active.Phase != components.GameActionSelecting {
		service.alert(id, "COMBAT_STALE_SELECTION")
		return
	}
	definition, ok := service.definitions.Get(active.ActionID)
	if !ok || definition.Execution.Combat == nil {
		service.alert(id, "ACTION_UNKNOWN")
		return
	}
	if reason := service.nonStaminaReason(world, id, actor, definition); reason != "" {
		service.alert(id, reason)
		return
	}
	transform, ok := ecs.GetComponent[components.Transform](world, actor)
	if !ok {
		service.alert(id, "COMBAT_UNAVAILABLE")
		return
	}
	direction, err := combat.Direction(combat.Point{X: transform.X, Y: transform.Y}, combat.Point{X: float64(click.X), Y: float64(click.Y)})
	if err != nil {
		service.alert(id, "COMBAT_INVALID_DIRECTION")
		return
	}
	weapon, err := resolveCombatWeapon(world, id, definition)
	if err != nil {
		service.alert(id, "COMBAT_INVALID_WEAPON")
		return
	}
	if reason := service.combat.Begin(actor, definition, weapon, direction, active.Generation); reason != "" {
		service.alert(id, reason)
	}
}

type combatActionHandler struct {
	combat     *CombatService
	definition *actiondefs.Definition
}

func (handler *combatActionHandler) UnavailableReason(world *ecs.World, id types.EntityID, actor types.Handle) string {
	if handler.combat == nil || !handler.combat.enabled {
		return "COMBAT_DISABLED"
	}
	if handler.combat.Busy(actor) || combatConflictingWork(world, actor) {
		return "ACTION_BUSY"
	}
	if combatActorRestricted(world, actor) {
		return "COMBAT_INTERRUPTED"
	}
	if _, err := resolveCombatWeapon(world, id, handler.definition); err != nil {
		return "COMBAT_INVALID_WEAPON"
	}
	return ""
}
func (*combatActionHandler) ValidateTarget(*ecs.World, types.EntityID, types.Handle, ActionTarget) string {
	return "COMBAT_REQUEST_REQUIRED"
}
func (*combatActionHandler) Start(*ecs.World, types.EntityID, types.Handle, ActionTarget, uint64) ActionResult {
	return ActionResult{Outcome: ActionRejected, Reason: "COMBAT_REQUEST_REQUIRED"}
}
func (*combatActionHandler) Cancel(*ecs.World, types.EntityID, types.Handle, components.ActiveGameAction) {
}

func (service *ActionService) combatActionState(world *ecs.World, actor types.Handle, state *netproto.S2C_ActionStateChanged) *netproto.S2C_ActionStateChanged {
	state.Generation = fmt.Sprintf("%d:%d", world.Layer, actor)
	if combat, ok := ecs.GetComponent[components.CombatState](world, actor); ok {
		state.StreamEpoch = combat.StreamEpoch
	}
	if active, ok := ecs.GetComponent[components.ActiveGameAction](world, actor); ok {
		state.SelectionGeneration = active.Generation
		if definition, exists := service.definitions.Get(active.ActionID); exists && definition.Execution.Combat != nil {
			id, _ := world.GetExternalID(actor)
			if weapon, err := resolveCombatWeapon(world, id, definition); err == nil {
				state.CombatRange = weapon.Range
			}
		}
	}
	return state
}

func combatProfileMessage(profile *actiondefs.CombatProfile) *netproto.CombatProfile {
	if profile == nil {
		return nil
	}
	return &netproto.CombatProfile{Selection: profile.Selection, SectorAngleDegrees: profile.SectorAngleDegrees, WindupMs: uint64(profile.WindupMs), RecoveryMs: uint64(profile.RecoveryMs), CooldownMs: uint64(profile.CooldownMs), DamageMultiplier: profile.DamageMultiplier}
}
