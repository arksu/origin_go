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
	"origin/internal/playerstate"
	"origin/internal/types"
)

const (
	// CorpseDecaySeconds counts server runtime, including time in unloaded chunks.
	CorpseDecaySeconds int64 = 120

	playerDeadBehaviorKey = "player_dead"
	actionUnequip         = "unequip"

	reasonUnequipUnavailable  = "UNEQUIP_UNAVAILABLE"
	reasonUnequipGiveFailed   = "UNEQUIP_GIVE_FAILED"
	reasonUnequipStateChanged = "UNEQUIP_STATE_CHANGED"
)

type playerDeadBehavior struct{}

type unequipCandidate struct {
	ItemIndex int
	ItemKey   string
	Quality   uint32
}

func (playerDeadBehavior) RequiresItemMutation(string) bool { return true }

func (playerDeadBehavior) Key() string { return playerDeadBehaviorKey }

func (playerDeadBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("player_dead def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, playerDeadBehaviorKey)
}

func (playerDeadBehavior) InitObject(ctx *contracts.BehaviorObjectInitContext) error {
	if ctx == nil || ctx.World == nil || ctx.Handle == types.InvalidHandle || !ctx.World.Alive(ctx.Handle) || ctx.EntityID == 0 {
		return nil
	}
	if ctx.Reason != contracts.ObjectBehaviorInitReasonSpawn && ctx.Reason != contracts.ObjectBehaviorInitReasonRestore && ctx.Reason != contracts.ObjectBehaviorInitReasonTransform {
		return nil
	}
	now := ecs.GetResource[ecs.TimeState](ctx.World).RuntimeSecondsTotal
	var deadline int64
	ecs.WithComponent(ctx.World, ctx.Handle, func(state *components.ObjectInternalState) {
		if existing, ok := components.GetBehaviorState[components.CorpseDecayBehaviorState](*state, playerDeadBehaviorKey); ok && existing != nil && existing.DecayAtRuntimeSeconds > 0 {
			deadline = existing.DecayAtRuntimeSeconds
			return
		}
		deadline = now + CorpseDecaySeconds
		components.SetBehaviorState(state, playerDeadBehaviorKey, &components.CorpseDecayBehaviorState{DecayAtRuntimeSeconds: deadline})
		state.IsDirty = true
	})
	if deadline > 0 {
		ecs.EnsureCorpseDecaySchedule(ctx.World).Schedule(ctx.Handle, ctx.EntityID, deadline)
	}
	return nil
}

func (playerDeadBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
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

func (playerDeadBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.World == nil || ctx.ActionID != actionUnequip {
		return contracts.BehaviorResult{OK: false}
	}
	_, candidates := collectUnequipCandidates(ctx.World, ctx.TargetID)
	if len(candidates) == 0 {
		return contracts.BehaviorResult{OK: false}
	}
	return contracts.BehaviorResult{OK: true}
}

func (playerDeadBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil || ctx.World == nil || ctx.ActionID != actionUnequip {
		return contracts.BehaviorResult{OK: false}
	}

	if playerstate.ItemsLocked(ctx.World, ctx.PlayerHandle) {
		return contracts.BehaviorResult{OK: false, UserVisible: true, ReasonCode: playerstate.ItemsLockedReason, Severity: contracts.BehaviorAlertSeverityWarning}
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

	markCorpseDirtyAfterUnequip(ctx.World, ctx.TargetHandle)
	ecs.MarkCharacterVisualDirty(ctx.World, ctx.TargetID)
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

func markCorpseDirtyAfterUnequip(w *ecs.World, corpseHandle types.Handle) {
	if w == nil || corpseHandle == types.InvalidHandle || !w.Alive(corpseHandle) {
		return
	}

	ecs.WithComponent(w, corpseHandle, func(state *components.ObjectInternalState) {
		state.IsDirty = true
	})
	ecs.MarkObjectBehaviorDirty(w, corpseHandle)
}
