package behaviors

import (
	"fmt"
	"math"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

type dryingBehavior struct{}

func (dryingBehavior) Key() string { return "drying" }

func (dryingBehavior) ApplyRuntime(*contracts.BehaviorRuntimeContext) contracts.BehaviorRuntimeResult {
	return contracts.BehaviorRuntimeResult{}
}

func (dryingBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil || ctx.Def == nil {
		return 0, fmt.Errorf("drying config target def is nil")
	}
	def, ok := ctx.Def.(*objectdefs.ObjectDef)
	if !ok || def == nil {
		return 0, fmt.Errorf("drying requires an object definition")
	}
	if !def.HasBehavior("container") || def.Components == nil || len(def.Components.Inventory) != 1 {
		return 0, fmt.Errorf("drying requires container behavior and one 2x2 root grid")
	}
	grid := def.Components.Inventory[0]
	if grid.Key != 0 || (grid.Kind != "" && grid.Kind != "grid") || grid.W != 2 || grid.H != 2 {
		return 0, fmt.Errorf("drying requires one 2x2 root grid with key 0")
	}
	var cfg contracts.DryingBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid drying config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	seen := make(map[string]struct{}, len(cfg.Processes))
	for i := range cfg.Processes {
		process := &cfg.Processes[i]
		process.InputItemKey = strings.TrimSpace(process.InputItemKey)
		process.OutputItemKey = strings.TrimSpace(process.OutputItemKey)
		if process.InputItemKey == "" || process.OutputItemKey == "" {
			return 0, fmt.Errorf("drying process %d requires inputItemKey and outputItemKey", i)
		}
		if process.DurationSeconds <= 0 {
			return 0, fmt.Errorf("drying process %d durationSeconds must be positive", i)
		}
		if _, duplicate := seen[process.InputItemKey]; duplicate {
			return 0, fmt.Errorf("drying repeats input item %q", process.InputItemKey)
		}
		seen[process.InputItemKey] = struct{}{}
		items := itemdefs.Global()
		if items == nil {
			return 0, fmt.Errorf("drying requires loaded item definitions")
		}
		input, found := items.GetByKey(process.InputItemKey)
		if !found {
			return 0, fmt.Errorf("drying input item %q does not exist", process.InputItemKey)
		}
		output, found := items.GetByKey(process.OutputItemKey)
		if !found {
			return 0, fmt.Errorf("drying output item %q does not exist", process.OutputItemKey)
		}
		if !dryingItemDefinitionAllowed(input) || !dryingItemDefinitionAllowed(output) {
			return 0, fmt.Errorf("drying process %d requires nonstackable grid items without nested inventories", i)
		}
		if input.Size.W > grid.W || input.Size.H > grid.H || output.Size.W > input.Size.W || output.Size.H > input.Size.H {
			return 0, fmt.Errorf("drying process %d output must fit the input area in the 2x2 grid", i)
		}
		process.InputTypeID, process.OutputTypeID = uint32(input.DefID), uint32(output.DefID)
		process.InputWidth, process.InputHeight = uint8(input.Size.W), uint8(input.Size.H)
		process.OutputWidth, process.OutputHeight = uint8(output.Size.W), uint8(output.Size.H)
	}
	def.SetDryingBehaviorConfig(cfg)
	return cfg.Priority, nil
}

func dryingItemDefinitionAllowed(def *itemdefs.ItemDef) bool {
	return def != nil && def.DefID > 0 && def.Size.W > 0 && def.Size.H > 0 &&
		def.Container == nil && (def.Stack == nil || def.Stack.Mode == itemdefs.StackModeNone) &&
		(def.Allowed.Grid == nil || *def.Allowed.Grid)
}

func (dryingBehavior) InitObject(ctx *contracts.BehaviorObjectInitContext) error {
	if ctx == nil || (ctx.Reason != contracts.ObjectBehaviorInitReasonSpawn && ctx.Reason != contracts.ObjectBehaviorInitReasonRestore && ctx.Reason != contracts.ObjectBehaviorInitReasonTransform) {
		return nil
	}
	def, valid := dryingDefinition(ctx.World, ctx.Handle, ctx.EntityID, ctx.EntityType)
	if !valid {
		return nil
	}
	state := ensureDryingState(ctx.World, ctx.Handle)
	if state == nil {
		return fmt.Errorf("drying object has no internal state")
	}
	_, inventory, hasInventory := dryingRootInventory(ctx.World, ctx.EntityID)
	if hasInventory {
		if _, err := reconcileDrying(ctx.World, ctx.Handle, def.DryingConfig, state, inventory, ecs.GetResource[ecs.TimeState](ctx.World).RuntimeSecondsTotal); err != nil {
			return err
		}
	}
	// Restored deadlines are dispatched after chunk activation, never here.
	scheduleDrying(ctx.World, ctx.Handle, ctx.EntityID, state)
	return nil
}

func (dryingBehavior) OnRootInventoryMutation(ctx *contracts.RootInventoryMutationContext) (contracts.BehaviorTickResult, error) {
	if ctx == nil {
		return contracts.BehaviorTickResult{}, nil
	}
	def, valid := dryingDefinition(ctx.World, ctx.Handle, ctx.EntityID, ctx.EntityType)
	if !valid {
		return contracts.BehaviorTickResult{}, nil
	}
	_, inventory, found := dryingRootInventory(ctx.World, ctx.EntityID)
	if !found {
		return contracts.BehaviorTickResult{}, fmt.Errorf("drying root inventory is unavailable")
	}
	state := ensureDryingState(ctx.World, ctx.Handle)
	if state == nil {
		return contracts.BehaviorTickResult{}, fmt.Errorf("drying object has no internal state")
	}
	changed, err := reconcileDrying(ctx.World, ctx.Handle, def.DryingConfig, state, inventory, ecs.GetResource[ecs.TimeState](ctx.World).RuntimeSecondsTotal)
	scheduleDrying(ctx.World, ctx.Handle, ctx.EntityID, state)
	return contracts.BehaviorTickResult{StateChanged: changed}, err
}

func (dryingBehavior) OnScheduledRuntimeTick(ctx *contracts.BehaviorRuntimeTickContext) (contracts.BehaviorTickResult, error) {
	if ctx == nil {
		return contracts.BehaviorTickResult{}, nil
	}
	def, valid := dryingDefinition(ctx.World, ctx.Handle, ctx.EntityID, ctx.EntityType)
	if !valid {
		return contracts.BehaviorTickResult{}, nil
	}
	inventoryHandle, inventory, found := dryingRootInventory(ctx.World, ctx.EntityID)
	if !found {
		return contracts.BehaviorTickResult{}, fmt.Errorf("drying root inventory is unavailable")
	}
	state := ensureDryingState(ctx.World, ctx.Handle)
	if state == nil {
		return contracts.BehaviorTickResult{}, fmt.Errorf("drying object has no internal state")
	}
	changed, err := reconcileDrying(ctx.World, ctx.Handle, def.DryingConfig, state, inventory, ctx.CurrentRuntimeSeconds)
	if err != nil {
		return contracts.BehaviorTickResult{StateChanged: changed}, err
	}
	// Prepare all due results before changing a slot. Failed allocation must not
	// leave a partial conversion without a revision or viewer notification.
	var outputIDs [components.MaxDryingItems]types.EntityID
	var outputDefs [components.MaxDryingItems]*itemdefs.ItemDef
	for i, entry := range state.Entries {
		if ctx.CurrentRuntimeSeconds < entry.CompletionRuntimeSeconds {
			continue
		}
		process := dryingProcess(def.DryingConfig, entry.InputTypeID)
		if dryingInputIndex(inventory.Items, entry, process) < 0 {
			continue
		}
		var prepareErr error
		if ctx.Deps == nil || ctx.Deps.IDAllocator == nil {
			prepareErr = fmt.Errorf("drying item ID allocator is unavailable")
		} else if itemdefs.Global() == nil {
			prepareErr = fmt.Errorf("drying item definitions are unavailable")
		} else {
			outputDef, exists := itemdefs.Global().GetByID(int(process.OutputTypeID))
			if !exists || !dryingItemDefinitionAllowed(outputDef) || uint8(outputDef.Size.W) != process.OutputWidth || uint8(outputDef.Size.H) != process.OutputHeight {
				prepareErr = fmt.Errorf("drying output definition changed")
			} else {
				itemID := ctx.Deps.IDAllocator.GetFreeID()
				if itemID == 0 || itemID == entry.InputItemID {
					prepareErr = fmt.Errorf("drying item ID allocator returned an invalid ID")
				}
				for _, priorID := range outputIDs[:i] {
					if priorID != 0 && priorID == itemID {
						prepareErr = fmt.Errorf("drying item ID allocator repeated an ID")
					}
				}
				outputIDs[i], outputDefs[i] = itemID, outputDef
			}
		}
		if prepareErr != nil {
			rescheduleDryingRetry(ctx.World, ctx.Handle, ctx.EntityID, ctx.CurrentRuntimeSeconds)
			return contracts.BehaviorTickResult{StateChanged: changed}, prepareErr
		}
	}
	converted := false
	entryIndex := 0
	for i := 0; i < len(state.Entries); entryIndex++ {
		entry := state.Entries[i]
		if ctx.CurrentRuntimeSeconds < entry.CompletionRuntimeSeconds {
			i++
			continue
		}
		process := dryingProcess(def.DryingConfig, entry.InputTypeID)
		itemIndex := dryingInputIndex(inventory.Items, entry, process)
		if itemIndex < 0 {
			removeDryingEntry(state, i)
			changed = true
			continue
		}
		outputDef := outputDefs[entryIndex]
		itemID := outputIDs[entryIndex]
		input := inventory.Items[itemIndex]
		inventory.Items[itemIndex] = components.InvItem{
			ItemID: itemID, TypeID: process.OutputTypeID, Resource: outputDef.ResolveResource(false),
			Quality: input.Quality, Quantity: 1,
			W: process.OutputWidth, H: process.OutputHeight, X: input.X, Y: input.Y,
		}
		removeDryingEntry(state, i)
		changed, converted = true, true
	}
	if converted {
		ecs.WithComponent(ctx.World, inventoryHandle, func(container *components.InventoryContainer) {
			container.Version++
		})
		// A declared result may itself be another process input.
		_, err = reconcileDrying(ctx.World, ctx.Handle, def.DryingConfig, state, inventory, ctx.CurrentRuntimeSeconds)
	}
	markDryingDirty(ctx.World, ctx.Handle, changed)
	scheduleDrying(ctx.World, ctx.Handle, ctx.EntityID, state)
	if converted && ctx.Deps != nil && ctx.Deps.RootInventoryUpdate != nil {
		ctx.Deps.RootInventoryUpdate(ctx.World, ctx.EntityID)
	}
	return contracts.BehaviorTickResult{StateChanged: changed}, err
}

func dryingDefinition(w *ecs.World, handle types.Handle, entityID types.EntityID, entityType uint32) (*objectdefs.ObjectDef, bool) {
	if w == nil || !w.Alive(handle) || ecs.ObjectDestructionPending(w, handle) {
		return nil, false
	}
	actualID, hasID := w.GetExternalID(handle)
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	registry := objectdefs.Global()
	if !hasID || actualID != entityID || !hasInfo || info.TypeID != entityType || registry == nil {
		return nil, false
	}
	def, found := registry.GetByID(int(entityType))
	return def, found && def.DryingConfig != nil
}

func dryingRootInventory(w *ecs.World, entityID types.EntityID) (types.Handle, components.InventoryContainer, bool) {
	index := ecs.GetResource[ecs.InventoryRefIndex](w)
	handle, found := index.Lookup(constt.InventoryGrid, entityID, 0)
	if !found || !w.Alive(handle) {
		return types.InvalidHandle, components.InventoryContainer{}, false
	}
	container, found := ecs.GetComponent[components.InventoryContainer](w, handle)
	return handle, container, found && container.OwnerID == entityID && container.Kind == constt.InventoryGrid && container.Key == 0 && container.Width == 2 && container.Height == 2
}

func ensureDryingState(w *ecs.World, handle types.Handle) *components.DryingBehaviorState {
	var drying *components.DryingBehaviorState
	ecs.WithComponent(w, handle, func(internal *components.ObjectInternalState) {
		if existing, exists := components.GetBehaviorState[components.DryingBehaviorState](*internal, "drying"); exists && existing != nil {
			drying = existing
			return
		}
		drying = &components.DryingBehaviorState{Entries: make([]components.DryingItemState, 0, components.MaxDryingItems)}
		components.SetBehaviorState(internal, "drying", drying)
	})
	return drying
}

func dryingProcess(config *objectdefs.DryingBehaviorConfig, typeID uint32) *objectdefs.DryingProcessConfig {
	for i := range config.Processes {
		if config.Processes[i].InputTypeID == typeID {
			return &config.Processes[i]
		}
	}
	return nil
}

func dryingInputIndex(items []components.InvItem, entry components.DryingItemState, process *objectdefs.DryingProcessConfig) int {
	if process == nil {
		return -1
	}
	for i, item := range items {
		if item.ItemID == entry.InputItemID && item.TypeID == entry.InputTypeID && dryingInputEligible(item, process) {
			return i
		}
	}
	return -1
}

func dryingInputEligible(item components.InvItem, process *objectdefs.DryingProcessConfig) bool {
	return process != nil && item.ItemID != 0 && item.TypeID == process.InputTypeID && item.Quantity == 1 &&
		item.W == process.InputWidth && item.H == process.InputHeight &&
		int(item.X)+int(item.W) <= 2 && int(item.Y)+int(item.H) <= 2 &&
		process.OutputWidth <= item.W && process.OutputHeight <= item.H
}

func reconcileDrying(w *ecs.World, handle types.Handle, config *objectdefs.DryingBehaviorConfig, state *components.DryingBehaviorState, inventory components.InventoryContainer, now int64) (bool, error) {
	if len(inventory.Items) > components.MaxDryingItems || len(state.Entries) > components.MaxDryingItems || now < 0 {
		return false, fmt.Errorf("drying state exceeds its bounded grid or has invalid runtime")
	}
	changed := false
	for i := 0; i < len(state.Entries); {
		entry := state.Entries[i]
		if dryingInputIndex(inventory.Items, entry, dryingProcess(config, entry.InputTypeID)) < 0 {
			removeDryingEntry(state, i)
			changed = true
			continue
		}
		i++
	}
	for _, item := range inventory.Items {
		process := dryingProcess(config, item.TypeID)
		if !dryingInputEligible(item, process) {
			continue
		}
		found := false
		for _, entry := range state.Entries {
			if entry.InputItemID == item.ItemID {
				found = true
				break
			}
		}
		if found {
			continue
		}
		if process.DurationSeconds <= 0 || now > math.MaxInt64-process.DurationSeconds {
			markDryingDirty(w, handle, changed)
			return changed, fmt.Errorf("drying completion runtime overflows")
		}
		state.Entries = append(state.Entries, components.DryingItemState{
			InputItemID: item.ItemID, InputTypeID: item.TypeID,
			CompletionRuntimeSeconds: now + process.DurationSeconds,
		})
		changed = true
	}
	markDryingDirty(w, handle, changed)
	return changed, nil
}

func removeDryingEntry(state *components.DryingBehaviorState, index int) {
	copy(state.Entries[index:], state.Entries[index+1:])
	state.Entries[len(state.Entries)-1] = components.DryingItemState{}
	state.Entries = state.Entries[:len(state.Entries)-1]
}

func markDryingDirty(w *ecs.World, handle types.Handle, changed bool) {
	if changed {
		ecs.WithComponent(w, handle, func(state *components.ObjectInternalState) { state.IsDirty = true })
		ecs.MarkObjectBehaviorDirty(w, handle)
	}
}

func scheduleDrying(w *ecs.World, handle types.Handle, entityID types.EntityID, state *components.DryingBehaviorState) {
	if len(state.Entries) == 0 {
		ecs.CancelBehaviorRuntime(w, handle, "drying")
		return
	}
	due := state.Entries[0].CompletionRuntimeSeconds
	for _, entry := range state.Entries[1:] {
		if entry.CompletionRuntimeSeconds < due {
			due = entry.CompletionRuntimeSeconds
		}
	}
	ecs.ScheduleBehaviorRuntime(w, handle, entityID, "drying", due)
}

func rescheduleDryingRetry(w *ecs.World, handle types.Handle, entityID types.EntityID, now int64) {
	if now < math.MaxInt64 {
		ecs.ScheduleBehaviorRuntime(w, handle, entityID, "drying", now+1)
	}
}
