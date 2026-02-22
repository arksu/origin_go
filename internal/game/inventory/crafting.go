package inventory

import (
	"time"

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
}

type CraftGiveOrDropResult struct {
	Success           bool
	UpdatedContainers []*ContainerInfo
	DiscoveryLPGained int64
	AnyDropped        bool
}

// HasCraftInputs checks whether the player inventory tree (root grids + nested + hand) has all inputs for one cycle.
func (e *InventoryExecutor) HasCraftInputs(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	craft *craftdefs.CraftDef,
) bool {
	preview := e.PreviewCraftInputs(w, playerID, playerHandle, craft)
	return preview.Success && !preview.Overflow
}

// CanFitCraftOutputsOneCycle simulates give placement (grid+nested+hand) for all outputs of one craft cycle.
// It intentionally does NOT model world-drop fallback: start-craft precheck requires one full cycle to fit
// into inventory tree + hand before crafting can begin.
func (e *InventoryExecutor) CanFitCraftOutputsOneCycle(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	craft *craftdefs.CraftDef,
	quality uint32,
) bool {
	if e == nil || e.service == nil || w == nil || craft == nil {
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

	for _, out := range craft.Outputs {
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

// ConsumeCraftInputs consumes one cycle inputs from player inventories and returns quality aggregation data.
func (e *InventoryExecutor) PreviewCraftInputs(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	craft *craftdefs.CraftDef,
) CraftConsumeInputsResult {
	return e.consumeCraftInputsInternal(w, playerID, playerHandle, craft, false)
}

// ConsumeCraftInputs consumes one cycle inputs from player inventories and returns quality aggregation data.
func (e *InventoryExecutor) ConsumeCraftInputs(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	craft *craftdefs.CraftDef,
) CraftConsumeInputsResult {
	return e.consumeCraftInputsInternal(w, playerID, playerHandle, craft, true)
}

func (e *InventoryExecutor) consumeCraftInputsInternal(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	craft *craftdefs.CraftDef,
	commit bool,
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

	if !commit {
		return result
	}

	updatedOwner, _ := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	updated := make([]*ContainerInfo, 0, len(changed))
	for handle := range changed {
		clone := clones[handle]
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
	if !hasInfo || !hasTransform || !hasChunk {
		return false
	}

	params := SpawnDroppedEntityParams{
		DroppedEntityID: e.service.idAllocator.GetFreeID(),
		ItemID:          e.service.idAllocator.GetFreeID(),
		TypeID:          uint32(itemDef.DefID),
		Resource:        itemDef.ResolveResource(false),
		Quality:         quality,
		Quantity:        1,
		W:               uint8(itemDef.Size.W),
		H:               uint8(itemDef.Size.H),
		DropX:           int(playerTransform.X),
		DropY:           int(playerTransform.Y),
		Region:          playerEntityInfo.Region,
		Layer:           playerEntityInfo.Layer,
		ChunkX:          playerChunk.CurrentChunkX,
		ChunkY:          playerChunk.CurrentChunkY,
		DropperID:       playerID,
		NowUnix:         time.Now().Unix(),
	}

	if _, ok := SpawnDroppedEntity(w, params); !ok {
		return false
	}
	e.registerDroppedSpatial(w, params.DroppedEntityID)
	if e.visionForcer != nil {
		e.visionForcer.ForceUpdateForObserver(w, playerHandle)
	}
	if e.service.persister != nil {
		if err := PersistDroppedEntity(e.service.persister, params, nil); err != nil && e.logger != nil {
			e.logger.Warn("Failed to persist crafted dropped item",
				zap.Uint64("player_id", uint64(playerID)),
				zap.Error(err),
			)
		}
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
