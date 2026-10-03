package game

import (
	"fmt"
	"math"
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/types"
	"strconv"
)

// Range controls follow the public-test command policy. Each player controls
// only their own range and actor; no target ID is accepted from chat.
func (handler *ChatAdminCommandHandler) handleCombatRange(w *ecs.World, playerID types.EntityID, player types.Handle, args []string) {
	service := handler.combatRange
	if err := service.available(player); err != nil {
		handler.sendSystemMessage(playerID, err.Error())
		return
	}
	if len(args) == 0 {
		handler.sendSystemMessage(playerID, "usage: /combat-range create [small|large|tied|moving|blocker], reset, remove, axe, stamina <0..max>, strength <0.01..1000>, interrupt")
		return
	}
	var err error
	switch args[0] {
	case "create":
		if len(args) > 2 {
			err = fmt.Errorf("usage: /combat-range create [preset]")
			break
		}
		preset := "small"
		if len(args) == 2 {
			preset = args[1]
		}
		err = service.Create(player, preset)
		if err == nil {
			if !handler.giveCombatAxe(w, playerID, player) {
				return
			}
			if handler.visionForcer != nil {
				handler.visionForcer.ForceUpdateForObserver(w, player)
			}
		}
	case "axe":
		if len(args) != 1 {
			err = fmt.Errorf("usage: /combat-range axe")
			break
		}
		if !handler.giveCombatAxe(w, playerID, player) {
			return
		}
	case "reset":
		if len(args) != 1 {
			err = fmt.Errorf("usage: /combat-range reset")
			break
		}
		err = service.Reset(player)
	case "remove":
		if len(args) != 1 {
			err = fmt.Errorf("usage: /combat-range remove")
			break
		}
		service.Remove(player)
	case "interrupt":
		if len(args) != 1 {
			err = fmt.Errorf("usage: /combat-range interrupt")
			break
		}
		service.combat.Interrupt(player)
	case "stamina", "strength":
		if len(args) != 2 {
			err = fmt.Errorf("usage: /combat-range %s <value>", args[0])
			break
		}
		layout := service.ranges[player]
		if layout == nil {
			err = fmt.Errorf("create your range first")
			break
		}
		value, parseErr := strconv.ParseFloat(args[1], 64)
		if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			err = fmt.Errorf("value must be a finite number")
			break
		}
		if args[0] == "strength" {
			if value < .01 || value > 1000 {
				err = fmt.Errorf("strength must be 0.01..1000")
				break
			}
			layout.strength = &value
		} else {
			profile, ok := ecs.GetComponent[components.CharacterProfile](w, player)
			if !ok {
				err = fmt.Errorf("character profile unavailable")
				break
			}
			maximum := entitystats.MaxStaminaFromCon(characterattrs.Get(profile.Attributes, characterattrs.CON))
			if value < 0 || value > maximum {
				err = fmt.Errorf("stamina must be 0..%g", maximum)
				break
			}
			if !ecs.HasComponent[components.EntityStats](w, player) {
				err = fmt.Errorf("character stats unavailable")
				break
			}
			ecs.WithComponent(w, player, func(stats *components.EntityStats) {
				stats.Stamina = value
				ecs.UpdateEntityStatsRegenSchedule(w, player, value, stats.Energy, maximum)
			})
			ecs.MarkPlayerStatsDirty(w, playerID, 0)
		}
	default:
		err = fmt.Errorf("unknown range command")
	}
	if err != nil {
		handler.sendSystemMessage(playerID, err.Error())
		return
	}
	handler.sendSystemMessage(playerID, "Combat range: "+args[0]+" complete")
}

func (handler *ChatAdminCommandHandler) giveCombatAxe(w *ecs.World, playerID types.EntityID, player types.Handle) bool {
	owner, _ := ecs.GetComponent[components.InventoryOwner](w, player)
	for _, link := range owner.Inventories {
		container, ok := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !ok {
			continue
		}
		for _, item := range container.Items {
			if item.TypeID == 1002 && item.Quality == 10 {
				return true
			}
		}
	}
	if handler.inventoryExecutor == nil {
		handler.sendSystemMessage(playerID, "Axe grant unavailable")
		return false
	}
	return handler.handleGive(w, playerID, player, []string{"stone_axe", "1", "10"})
}
