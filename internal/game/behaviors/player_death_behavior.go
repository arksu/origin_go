package behaviors

import (
	"fmt"
	"math/rand"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

const (
	playerDeathBehaviorKey = "player_death"
	actionUnequip          = "unequip"

	reasonUnequipUnavailable  = "UNEQUIP_UNAVAILABLE"
	reasonUnequipGiveFailed   = "UNEQUIP_GIVE_FAILED"
	reasonUnequipStateChanged = "UNEQUIP_STATE_CHANGED"
)

type playerDeathBehavior struct{}

type unequipCandidate struct {
	ItemIndex int
	ItemKey   string
	Quality   uint32
}

func (playerDeathBehavior) Key() string { return playerDeathBehaviorKey }

func (playerDeathBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("player_death def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, playerDeathBehaviorKey)
}

func (playerDeathBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	_, candidates := collectUnequipCandidates(ctx.World, ctx.TargetID)
	if len(candidates) == 0 {
		return nil
	}
	return []contracts.ContextAction{
		{
			ActionID: actionUnequip,
			Title:    "Unequip",
		},
	}
}

func (playerDeathBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.World == nil || ctx.ActionID != actionUnequip {
		return contracts.BehaviorResult{OK: false}
	}
	_, candidates := collectUnequipCandidates(ctx.World, ctx.TargetID)
	if len(candidates) == 0 {
		return contracts.BehaviorResult{OK: false}
	}
	return contracts.BehaviorResult{OK: true}
}

func (playerDeathBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil || ctx.World == nil || ctx.ActionID != actionUnequip {
		return contracts.BehaviorResult{OK: false}
	}

	equipmentHandle, candidates := collectUnequipCandidates(ctx.World, ctx.TargetID)
	if len(candidates) == 0 {
		return contracts.BehaviorResult{OK: false}
	}

	deps := resolveExecutionDeps(ctx.Deps)
	if deps.GiveItem == nil {
		return contracts.BehaviorResult{
			OK:          false,
			UserVisible: true,
			ReasonCode:  reasonUnequipUnavailable,
			Severity:    contracts.BehaviorAlertSeverityWarning,
		}
	}

	candidate := candidates[rand.Intn(len(candidates))]
	outcome := deps.GiveItem(ctx.World, ctx.PlayerID, ctx.PlayerHandle, candidate.ItemKey, 1, candidate.Quality)
	if !outcome.Success {
		return contracts.BehaviorResult{
			OK:          false,
			UserVisible: true,
			ReasonCode:  reasonUnequipGiveFailed,
			Severity:    contracts.BehaviorAlertSeverityWarning,
		}
	}

	removed := false
	ecs.WithComponent(ctx.World, equipmentHandle, func(container *components.InventoryContainer) {
		if candidate.ItemIndex < 0 || candidate.ItemIndex >= len(container.Items) {
			return
		}
		container.Items = append(container.Items[:candidate.ItemIndex], container.Items[candidate.ItemIndex+1:]...)
		container.Version++
		removed = true
	})
	if !removed {
		return contracts.BehaviorResult{
			OK:          false,
			UserVisible: true,
			ReasonCode:  reasonUnequipStateChanged,
			Severity:    contracts.BehaviorAlertSeverityWarning,
		}
	}

	markCorpseDirtyAfterUnequip(ctx.World, ctx.TargetID, ctx.TargetHandle)
	return contracts.BehaviorResult{OK: true}
}

func collectUnequipCandidates(w *ecs.World, corpseID types.EntityID) (types.Handle, []unequipCandidate) {
	if w == nil || corpseID == 0 {
		return types.InvalidHandle, nil
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	equipmentHandle, found := refIndex.Lookup(constt.InventoryEquipment, corpseID, 0)
	if !found || !w.Alive(equipmentHandle) {
		return types.InvalidHandle, nil
	}

	equipment, hasEquipment := ecs.GetComponent[components.InventoryContainer](w, equipmentHandle)
	if !hasEquipment || equipment.Kind != constt.InventoryEquipment {
		return types.InvalidHandle, nil
	}

	itemRegistry := itemdefs.Global()
	if itemRegistry == nil {
		return equipmentHandle, nil
	}

	candidates := make([]unequipCandidate, 0, len(equipment.Items))
	for index, item := range equipment.Items {
		if item.EquipSlot == netproto.EquipSlot_EQUIP_SLOT_NONE {
			continue
		}

		itemDef, found := itemRegistry.GetByID(int(item.TypeID))
		if !found || itemDef == nil || itemDef.Key == "" {
			continue
		}
		if itemDef.Container != nil {
			continue
		}

		candidates = append(candidates, unequipCandidate{
			ItemIndex: index,
			ItemKey:   itemDef.Key,
			Quality:   item.Quality,
		})
	}

	return equipmentHandle, candidates
}

func markCorpseDirtyAfterUnequip(w *ecs.World, corpseID types.EntityID, corpseHandle types.Handle) {
	if w == nil || corpseID == 0 {
		return
	}
	if corpseHandle == types.InvalidHandle || !w.Alive(corpseHandle) {
		corpseHandle = w.GetHandleByEntityID(corpseID)
	}
	if corpseHandle == types.InvalidHandle || !w.Alive(corpseHandle) {
		return
	}

	if _, hasState := ecs.GetComponent[components.ObjectInternalState](w, corpseHandle); hasState {
		ecs.WithComponent(w, corpseHandle, func(state *components.ObjectInternalState) {
			state.IsDirty = true
		})
	} else {
		ecs.AddComponent(w, corpseHandle, components.ObjectInternalState{IsDirty: true})
	}
	ecs.MarkObjectBehaviorDirty(w, corpseHandle)
}
