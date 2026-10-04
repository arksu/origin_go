package game

import (
	"origin/internal/actionanimationdefs"
	"origin/internal/actiondefs"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func (service *ActionService) isOnCooldown(world *ecs.World, player types.Handle, id string) bool {
	cooldowns, _ := ecs.GetComponent[components.ActionCooldowns](world, player)
	return cooldowns.ByAction[id].ExpiresAtMs > ecs.GetResource[ecs.TimeState](world).UnixMs
}

func (service *ActionService) rejectCooldown(world *ecs.World, playerID types.EntityID, player types.Handle, id string) bool {
	if !service.isOnCooldown(world, player, id) {
		return false
	}
	service.alert(playerID, "ACTION_ON_COOLDOWN")
	service.SendState(world, playerID, player)
	return true
}

func (service *ActionService) startCooldown(world *ecs.World, player types.Handle, definition *actiondefs.Definition) {
	if definition.Cooldown == 0 {
		return
	}
	now := ecs.GetResource[ecs.TimeState](world).UnixMs
	cooldowns, _ := ecs.GetComponent[components.ActionCooldowns](world, player)
	if cooldowns.ByAction == nil {
		cooldowns.ByAction = make(map[string]components.ActionCooldown)
	}
	for id, cooldown := range cooldowns.ByAction {
		if cooldown.ExpiresAtMs <= now {
			delete(cooldowns.ByAction, id)
		}
	}
	cooldowns.ByAction[definition.ID] = components.ActionCooldown{StartedAtMs: now, ExpiresAtMs: now + int64(definition.Cooldown)}
	ecs.AddComponent(world, player, cooldowns)
}

func (service *ActionService) cooldownSnapshot(world *ecs.World, player types.Handle) []*netproto.ActionCooldown {
	cooldowns, _ := ecs.GetComponent[components.ActionCooldowns](world, player)
	now := ecs.GetResource[ecs.TimeState](world).UnixMs
	var snapshot []*netproto.ActionCooldown
	for _, definition := range service.definitions.All() {
		if cooldown := cooldowns.ByAction[definition.ID]; cooldown.ExpiresAtMs > now {
			snapshot = append(snapshot, &netproto.ActionCooldown{ActionId: definition.ID, StartedAtMs: cooldown.StartedAtMs, ExpiresAtMs: cooldown.ExpiresAtMs})
		}
	}
	return snapshot
}

func (service *ActionService) resumeAfterCooldown(world *ecs.World, playerID types.EntityID, player types.Handle, definition *actiondefs.Definition, active components.ActiveGameAction) {
	target := actionTarget(active)
	if reason := service.validateCycleTarget(world, playerID, player, definition, target); reason != "" {
		service.Complete(world, playerID, player, active.Generation, false, reason)
		return
	}
	if service.isOnCooldown(world, player, definition.ID) {
		return
	}
	cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	if !exists || cycle.ActionGeneration != active.Generation || !cycle.ActionCompletionStarted {
		service.Cancel(world, playerID, player)
		return
	}
	service.startNextCycle(world, playerID, player, definition, active, cycle)
}

func (service *ActionService) startNextCycle(world *ecs.World, playerID types.EntityID, player types.Handle, definition *actiondefs.Definition, active components.ActiveGameAction, cycle components.ActiveCyclicAction) {
	active.Phase = components.GameActionExecuting
	ecs.AddComponent(world, player, active)
	cycle.CycleElapsedTicks = 0
	cycle.CycleIndex++
	cycle.StartedTick = ecs.GetResource[ecs.TimeState](world).Tick
	cycle.ActionCompletionStarted = false
	cyclicaction.Start(world, player, cycle, actionanimationdefs.Source{Kind: "menu", ID: definition.ID})
	service.SendState(world, playerID, player)
}

func loadCharacterActionCooldowns(raw []byte, nowMs int64) (components.ActionCooldowns, error) {
	cooldowns, err := components.UnmarshalActionCooldowns(raw)
	if err != nil {
		return components.ActionCooldowns{}, err
	}
	registry := actiondefs.Global()
	for id, cooldown := range cooldowns.ByAction {
		known := false
		if registry != nil {
			_, known = registry.Get(id)
		}
		if !known || cooldown.ExpiresAtMs <= nowMs {
			delete(cooldowns.ByAction, id)
		}
	}
	return cooldowns, nil
}
