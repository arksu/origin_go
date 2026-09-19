package behaviors

import (
	"fmt"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

type burnerBehavior struct{}

func (burnerBehavior) Key() string { return "burner" }

func (burnerBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if burnerFuelValue(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetHandle) == 0 {
		return nil
	}
	return []contracts.ContextAction{{ActionID: "add_fuel", Title: "Add fuel"}}
}

func (burnerBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != "add_fuel" || burnerFuelValue(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetHandle) == 0 {
		return contracts.BehaviorResult{OK: false}
	}
	return contracts.BehaviorResult{OK: true}
}

func (burnerBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != "add_fuel" {
		return contracts.BehaviorResult{OK: false}
	}
	value := burnerFuelValue(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetHandle)
	if value == 0 {
		return contracts.BehaviorResult{OK: false}
	}
	info, ok := ecs.GetComponent[components.EntityInfo](ctx.World, ctx.TargetHandle)
	if !ok {
		return contracts.BehaviorResult{OK: false}
	}
	def, ok := objectdefs.Global().GetByID(int(info.TypeID))
	if !ok || def.BurnerConfig == nil {
		return contracts.BehaviorResult{OK: false}
	}
	owner, ok := ecs.GetComponent[components.InventoryOwner](ctx.World, ctx.PlayerHandle)
	if !ok {
		return contracts.BehaviorResult{OK: false}
	}
	for _, link := range owner.Inventories {
		if link.Kind == constt.InventoryHand && link.OwnerID == ctx.PlayerID {
			ecs.MutateComponent[components.InventoryContainer](ctx.World, link.Handle, func(hand *components.InventoryContainer) bool {
				if len(hand.Items) != 1 {
					return false
				}
				hand.Items = nil
				hand.Version++
				return true
			})
			break
		}
	}
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		b, ok := components.GetBehaviorState[components.BurnerBehaviorState](*state, "burner")
		if !ok || b == nil {
			return
		}
		if value > def.BurnerConfig.FuelCapacity-b.Fuel {
			b.Fuel = def.BurnerConfig.FuelCapacity
		} else {
			b.Fuel += value
		}
		state.IsDirty = true
	})
	ecs.MutateComponent[components.StationState](ctx.World, ctx.TargetHandle, func(station *components.StationState) bool {
		if station.CurrentState == "unlit" {
			station.CurrentState = "burning"
			return true
		}
		return false
	})
	if ctx.Deps != nil && ctx.Deps.InventoryUpdate != nil {
		ctx.Deps.InventoryUpdate(ctx.World, ctx.PlayerID, ctx.PlayerHandle)
	}
	return contracts.BehaviorResult{OK: true}
}

func burnerFuelValue(w *ecs.World, playerID types.EntityID, playerHandle, targetHandle types.Handle) uint32 {
	if w == nil {
		return 0
	}
	info, ok := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !ok {
		return 0
	}
	def, ok := objectdefs.Global().GetByID(int(info.TypeID))
	if !ok || def.BurnerConfig == nil {
		return 0
	}
	owner, ok := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !ok {
		return 0
	}
	var item components.InvItem
	found := false
	for _, link := range owner.Inventories {
		if link.Kind == constt.InventoryHand && link.OwnerID == playerID {
			h, ok := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
			if ok && len(h.Items) == 1 {
				item = h.Items[0]
				found = true
			}
			break
		}
	}
	if !found {
		return 0
	}
	itemDef, ok := itemdefs.Global().GetByID(int(item.TypeID))
	if !ok {
		return 0
	}
	accepted := map[string]bool{}
	for _, key := range def.BurnerConfig.FuelAbilities {
		accepted[key] = true
	}
	var total uint32
	for key, value := range itemDef.Abilities {
		if accepted[key] {
			total += value
		}
	}
	return total
}

func (burnerBehavior) InitObject(ctx *contracts.BehaviorObjectInitContext) error {
	if ctx == nil || ctx.World == nil || !ctx.World.Alive(ctx.Handle) || (ctx.Reason != contracts.ObjectBehaviorInitReasonSpawn && ctx.Reason != contracts.ObjectBehaviorInitReasonRestore) {
		return nil
	}
	def, found := objectdefs.Global().GetByID(int(ctx.EntityType))
	if !found || def.BurnerConfig == nil {
		return nil
	}
	now := ecs.GetResource[ecs.TimeState](ctx.World).RuntimeSecondsTotal
	ecs.WithComponent(ctx.World, ctx.Handle, func(state *components.ObjectInternalState) {
		if existing, ok := components.GetBehaviorState[components.BurnerBehaviorState](*state, "burner"); ok && existing != nil {
			if ctx.Reason == contracts.ObjectBehaviorInitReasonRestore {
				for existing.Fuel > 0 && now >= existing.NextFuelBurnAtRuntimeSecond {
					existing.Fuel--
					existing.NextFuelBurnAtRuntimeSecond += int64(def.BurnerConfig.SecondsPerFuel)
					state.IsDirty = true
				}
			}
			return
		}
		components.SetBehaviorState(state, "burner", &components.BurnerBehaviorState{Fuel: def.BurnerConfig.InitialFuel, NextFuelBurnAtRuntimeSecond: now + int64(def.BurnerConfig.SecondsPerFuel)})
		state.IsDirty = true
	})
	return nil
}
func (burnerBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil || ctx.Def == nil {
		return 0, fmt.Errorf("burner config target def is nil")
	}
	var cfg contracts.BurnerBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid burner config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	if len(cfg.FuelAbilities) == 0 {
		return 0, fmt.Errorf("burner.fuelAbilities is required")
	}
	seen := map[string]struct{}{}
	for _, ability := range cfg.FuelAbilities {
		ability = strings.TrimSpace(ability)
		if ability == "" {
			return 0, fmt.Errorf("burner.fuelAbilities contains an empty ability")
		}
		if _, ok := seen[ability]; ok {
			return 0, fmt.Errorf("burner.fuelAbilities duplicate %q", ability)
		}
		seen[ability] = struct{}{}
	}
	if cfg.FuelCapacity == 0 {
		return 0, fmt.Errorf("burner.fuelCapacity must be > 0")
	}
	if cfg.SecondsPerFuel == 0 {
		return 0, fmt.Errorf("burner.secondsPerFuel must be > 0")
	}
	if cfg.InitialFuel > cfg.FuelCapacity {
		return 0, fmt.Errorf("burner.initialFuel must not exceed fuelCapacity")
	}
	if strings.TrimSpace(cfg.OnExhausted.DropItem) == "" || !cfg.OnExhausted.Despawn {
		return 0, fmt.Errorf("burner.onExhausted must drop an item and despawn")
	}
	ctx.Def.SetBurnerBehaviorConfig(cfg)
	return cfg.Priority, nil
}
