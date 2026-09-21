package inventory

import (
	"fmt"

	constt "origin/internal/const"
	"origin/internal/craftdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

type CraftConsumeInputsResult struct {
	Success           bool
	Overflow          bool
	UpdatedContainers []*ContainerInfo
	QualityWeighted   uint64
	QualityWeightSum  uint64
	SourceItemKey     string
	SourceItemID      types.EntityID
	prepared          []craftPreparedContainer
}

type craftPreparedContainer struct {
	handle    types.Handle
	container components.InventoryContainer
}

// ResolveCraftOutputs keeps preview metadata out of mapped runtime placement and creation.
func ResolveCraftOutputs(craft *craftdefs.CraftDef, inputs CraftConsumeInputsResult) ([]craftdefs.CraftOutput, error) {
	if craft == nil {
		return nil, fmt.Errorf("craft definition missing")
	}
	if craft.OutputByInputKey == nil {
		return craft.Outputs, nil
	}
	if !inputs.Success || inputs.Overflow || inputs.SourceItemKey == "" {
		return nil, fmt.Errorf("craft source input unavailable")
	}
	targetKey, ok := craft.OutputByInputKey[inputs.SourceItemKey]
	if !ok {
		return nil, &CraftMapEntryMissingError{SourceItemKey: inputs.SourceItemKey}
	}
	if _, ok := itemdefs.Global().GetByKey(targetKey); !ok {
		return nil, fmt.Errorf("craft mapped target item unknown: %s", targetKey)
	}
	return []craftdefs.CraftOutput{{ItemKey: targetKey, Count: 1}}, nil
}

type CraftMapEntryMissingError struct{ SourceItemKey string }

func (e *CraftMapEntryMissingError) Error() string {
	return fmt.Sprintf("no mapped output entry for source item %s", e.SourceItemKey)
}

type CraftGiveOrDropResult struct {
	Success           bool
	UpdatedContainers []*ContainerInfo
	DiscoveryLPGained int64
	AnyDropped        bool
}

// CanFitResolvedCraftOutputs simulates give placement (grid+nested+hand) for one cycle's resolved outputs.
// It intentionally does NOT model world-drop fallback: start-craft precheck requires one full cycle to fit
// into inventory tree + hand before crafting can begin.
func (e *InventoryExecutor) CanFitResolvedCraftOutputs(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, outputs []craftdefs.CraftOutput, quality uint32) bool {
	if e == nil || e.service == nil || w == nil {
		return false
	}
	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return false
	}
	clones := make(map[types.Handle]components.InventoryContainer, len(owner.Inventories))
	for _, link := range owner.Inventories {
		if !w.Alive(link.Handle) {
			continue
		}
		container, ok := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !ok {
			continue
		}
		clones[link.Handle] = containerClone(container)
	}

	gridLinks := orderedGridLinks(owner.Inventories, playerID, defaultGivePlacementPolicy)
	handLink, hasHand := playerHandLink(owner, playerID)

	for _, out := range outputs {
		itemDef, ok := itemdefs.Global().GetByKey(out.ItemKey)
		if !ok {
			return false
		}
		for i := uint32(0); i < out.Count; i++ {
			tmpItem := components.InvItem{
				TypeID:   uint32(itemDef.DefID),
				Resource: itemDef.ResolveResource(false),
				Quality:  quality,
				Quantity: 1,
				W:        uint8(itemDef.Size.W),
				H:        uint8(itemDef.Size.H),
			}
			placed := false
			for _, link := range gridLinks {
				container, exists := clones[link.Handle]
				if !exists {
					continue
				}
				if !e.service.canPlaceInContainer(w, &tmpItem, &owner, link.Handle, &container) {
					continue
				}
				found, x, y := e.service.placementService.FindFreeSpace(&container, tmpItem.W, tmpItem.H)
				if !found {
					continue
				}
				tmpItem.X, tmpItem.Y = x, y
				container.Items = append(container.Items, tmpItem)
				clones[link.Handle] = container
				placed = true
				break
			}
			if placed {
				continue
			}
			if !hasHand {
				return false
			}
			hand, exists := clones[handLink.Handle]
			if !exists {
				return false
			}
			// Intentionally strict: if hand is already occupied at precheck time, the cycle is considered
			// "no space" even though runtime output spawning could fall back to dropping the item to world.
			if len(hand.Items) > 0 {
				return false
			}
			if !e.service.canPlaceInContainer(w, &tmpItem, &owner, handLink.Handle, &hand) {
				return false
			}
			hand.Items = append(hand.Items, tmpItem)
			clones[handLink.Handle] = hand
		}
	}
	return true
}

// PreviewCraftInputs prepares inputs without mutation; commit the result only within the same synchronous cycle.
func (e *InventoryExecutor) PreviewCraftInputs(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	craft *craftdefs.CraftDef,
) CraftConsumeInputsResult {
	return e.prepareCraftInputs(w, playerID, playerHandle, craft)
}

func (e *InventoryExecutor) prepareCraftInputs(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	craft *craftdefs.CraftDef,
) CraftConsumeInputsResult {
	result := CraftConsumeInputsResult{}
	if e == nil || e.service == nil || w == nil || craft == nil {
		return result
	}
	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return result
	}

	orderedLinks := craftOrderedInventoryLinks(owner, playerID)
	clones := make(map[types.Handle]components.InventoryContainer, len(orderedLinks))
	for _, link := range orderedLinks {
		if !w.Alive(link.Handle) {
			continue
		}
		container, ok := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !ok {
			continue
		}
		clones[link.Handle] = containerClone(container)
	}

	changed := make(map[types.Handle]struct{}, len(orderedLinks))
	var weightedSum uint64
	var weightSum uint64

	processInput := func(input craftdefs.CraftInput) bool {
		remaining := input.Count
		for _, link := range orderedLinks {
			if remaining == 0 {
				break
			}
			container, ok := clones[link.Handle]
			if !ok {
				continue
			}
			modified := false
			for idx := 0; idx < len(container.Items) && remaining > 0; {
				item := &container.Items[idx]
				def, ok := itemdefs.Global().GetByID(int(item.TypeID))
				if !ok || !craftInputMatchesItemDef(input, def) {
					idx++
					continue
				}
				consumeQty := item.Quantity
				if consumeQty > remaining {
					consumeQty = remaining
				}

				if consumeQty == 0 {
					idx++
					continue
				}
				if input.ItemTag != "" && result.SourceItemKey == "" {
					result.SourceItemKey = def.Key
					result.SourceItemID = item.ItemID
				}
				weightedTerm, ok := mulUint64Checked(uint64(item.Quality), uint64(input.QualityWeight))
				if !ok {
					result.Overflow = true
					return false
				}
				weightedTerm, ok = mulUint64Checked(weightedTerm, uint64(consumeQty))
				if !ok {
					result.Overflow = true
					return false
				}
				nextWeighted, ok := addUint64Checked(weightedSum, weightedTerm)
				if !ok {
					result.Overflow = true
					return false
				}

				weightTerm, ok := mulUint64Checked(uint64(input.QualityWeight), uint64(consumeQty))
				if !ok {
					result.Overflow = true
					return false
				}
				nextWeightSum, ok := addUint64Checked(weightSum, weightTerm)
				if !ok {
					result.Overflow = true
					return false
				}
				weightedSum = nextWeighted
				weightSum = nextWeightSum

				if item.Quantity == consumeQty {
					container.Items = append(container.Items[:idx], container.Items[idx+1:]...)
				} else {
					item.Quantity -= consumeQty
					idx++
				}
				remaining -= consumeQty
				modified = true
			}
			if modified {
				if container.Kind == constt.InventoryHand && len(container.Items) == 0 {
					container.HandMouseOffsetX = 0
					container.HandMouseOffsetY = 0
				}
				clones[link.Handle] = container
				changed[link.Handle] = struct{}{}
			}
		}
		if remaining > 0 {
			return false
		}
		return true
	}

	// Exact-item requirements reserve matches first so generic tag requirements
	// cannot consume items intended for exact inputs.
	for _, input := range craft.Inputs {
		if input.ItemKey == "" {
			continue
		}
		if !processInput(input) {
			return result
		}
	}
	for _, input := range craft.Inputs {
		if input.ItemKey != "" {
			continue
		}
		if !processInput(input) {
			return result
		}
	}

	result.Success = true
	result.QualityWeighted = weightedSum
	result.QualityWeightSum = weightSum

	for _, link := range orderedLinks {
		if _, modified := changed[link.Handle]; modified {
			result.prepared = append(result.prepared, craftPreparedContainer{handle: link.Handle, container: clones[link.Handle]})
		}
	}
	return result
}

// CommitPreparedCraftInputs publishes the exact input selection that passed final output validation.
// Prepared results are single-use and must not be retained across ticks.
func (e *InventoryExecutor) CommitPreparedCraftInputs(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, prepared *CraftConsumeInputsResult) CraftConsumeInputsResult {
	if e == nil || w == nil || prepared == nil || !prepared.Success || prepared.Overflow || len(prepared.prepared) == 0 {
		return CraftConsumeInputsResult{}
	}
	for _, entry := range prepared.prepared {
		current, ok := ecs.GetComponent[components.InventoryContainer](w, entry.handle)
		if !w.Alive(entry.handle) || !ok || current.Version != entry.container.Version {
			return CraftConsumeInputsResult{}
		}
	}
	result := *prepared
	preparedContainers := prepared.prepared
	prepared.prepared = nil
	result.prepared = nil
	updatedOwner, _ := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	updated := make([]*ContainerInfo, 0, len(preparedContainers))
	for _, entry := range preparedContainers {
		handle, clone := entry.handle, entry.container
		ecs.MutateComponent[components.InventoryContainer](w, handle, func(c *components.InventoryContainer) bool {
			c.Items = clone.Items
			c.HandMouseOffsetX = clone.HandMouseOffsetX
			c.HandMouseOffsetY = clone.HandMouseOffsetY
			c.Version++
			return true
		})
		current, _ := ecs.GetComponent[components.InventoryContainer](w, handle)
		updated = append(updated, &ContainerInfo{
			Handle:    handle,
			Container: &current,
			Owner:     &updatedOwner,
		})
	}
	result.UpdatedContainers = e.applyNestedCascade(w, playerID, updated)
	return result
}

// GiveCraftOutputOrDrop attempts standard give placement first and falls back to dropping each failed item unit.
func (e *InventoryExecutor) GiveCraftOutputOrDrop(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	itemKey string,
	count uint32,
	quality uint32,
) CraftGiveOrDropResult {
	result := CraftGiveOrDropResult{Success: true}
	if e == nil || e.service == nil || w == nil || count == 0 {
		return result
	}

	for i := uint32(0); i < count; i++ {
		give := e.service.GiveItem(w, playerID, playerHandle, itemKey, 1, quality)
		if give != nil && give.Success && give.GrantedCount == 1 {
			result.UpdatedContainers = mergeUpdatedContainerInfos(result.UpdatedContainers, give.UpdatedContainers)
			result.DiscoveryLPGained += give.DiscoveryLPGained
			continue
		}

		if !e.dropCraftOutputAtPlayer(w, playerID, playerHandle, itemKey, quality) {
			result.Success = false
			return result
		}
		result.AnyDropped = true
	}

	if len(result.UpdatedContainers) > 0 {
		result.UpdatedContainers = e.applyNestedCascade(w, playerID, result.UpdatedContainers)
	}
	return result
}

func (e *InventoryExecutor) dropCraftOutputAtPlayer(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	itemKey string,
	quality uint32,
) bool {
	itemDef, ok := itemdefs.Global().GetByKey(itemKey)
	if !ok || e.service == nil || e.service.idAllocator == nil {
		return false
	}
	playerEntityInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, playerHandle)
	playerTransform, hasTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	playerChunk, hasChunk := ecs.GetComponent[components.ChunkRef](w, playerHandle)
	if !hasInfo || !hasTransform || !hasChunk || e.service.persister == nil {
		return false
	}

	droppedItemID := e.service.idAllocator.GetFreeID()
	params := SpawnDroppedEntityParams{
		DroppedEntityID:   droppedItemID,
		ItemID:            droppedItemID,
		TypeID:            uint32(itemDef.DefID),
		Resource:          itemDef.ResolveResource(false),
		Quality:           quality,
		Quantity:          1,
		W:                 uint8(itemDef.Size.W),
		H:                 uint8(itemDef.Size.H),
		DropX:             int(playerTransform.X),
		DropY:             int(playerTransform.Y),
		Region:            playerEntityInfo.Region,
		Layer:             playerEntityInfo.Layer,
		ChunkX:            playerChunk.CurrentChunkX,
		ChunkY:            playerChunk.CurrentChunkY,
		DropperID:         playerID,
		NowRuntimeSeconds: ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal,
	}

	if err := validateSpawnDroppedEntityParams(w, params); err != nil {
		if e.logger != nil {
			e.logger.Warn("Invalid crafted dropped item parameters", zap.Error(err))
		}
		return false
	}

	if err := PersistDroppedEntity(e.service.persister, params, nil); err != nil {
		if e.logger != nil {
			e.logger.Warn("Failed to persist crafted dropped item", zap.Error(err))
		}
		return false
	}

	if _, err := SpawnDroppedEntity(w, params); err != nil {
		if e.logger != nil {
			e.logger.Warn("Failed to spawn crafted dropped item", zap.Error(err))
		}
		if deleteErr := e.service.persister.DeleteObject(playerEntityInfo.Region, droppedItemID); deleteErr != nil && e.logger != nil {
			e.logger.Error("Failed to clean up unspawned crafted dropped item", zap.Error(deleteErr))
		}
		return false
	}
	e.registerDroppedSpatial(w, params.DroppedEntityID)
	if e.visionForcer != nil {
		e.visionForcer.ForceUpdateForObserver(w, playerHandle)
	}
	return true
}

func craftOrderedInventoryLinks(owner components.InventoryOwner, playerID types.EntityID) []components.InventoryLink {
	out := make([]components.InventoryLink, 0, len(owner.Inventories))
	out = append(out, orderedGridLinks(owner.Inventories, playerID, defaultGivePlacementPolicy)...)
	if hand, ok := playerHandLink(owner, playerID); ok {
		out = append(out, hand)
	}
	return out
}

func playerHandLink(owner components.InventoryOwner, playerID types.EntityID) (components.InventoryLink, bool) {
	for _, link := range owner.Inventories {
		if link.Kind == constt.InventoryHand && link.OwnerID == playerID && link.Key == 0 {
			return link, true
		}
	}
	return components.InventoryLink{}, false
}

func containerClone(src components.InventoryContainer) components.InventoryContainer {
	dst := src
	if len(src.Items) > 0 {
		dst.Items = append([]components.InvItem(nil), src.Items...)
	}
	return dst
}

func mulUint64Checked(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if a > ^uint64(0)/b {
		return 0, false
	}
	return a * b, true
}

func addUint64Checked(a, b uint64) (uint64, bool) {
	if a > ^uint64(0)-b {
		return 0, false
	}
	return a + b, true
}

func craftInputMatchesItemDef(input craftdefs.CraftInput, def *itemdefs.ItemDef) bool {
	if def == nil {
		return false
	}
	if input.ItemKey != "" {
		return def.Key == input.ItemKey
	}
	if input.ItemTag != "" {
		return itemDefHasTag(def, input.ItemTag)
	}
	return false
}

func itemDefHasTag(def *itemdefs.ItemDef, want string) bool {
	if def == nil || want == "" {
		return false
	}
	for _, tag := range def.Tags {
		if tag == want {
			return true
		}
	}
	return false
}
