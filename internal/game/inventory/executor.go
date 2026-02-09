package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

// DroppedItemSpatialRegistrar adds/removes dropped item entities to/from chunk spatial hash.
type DroppedItemSpatialRegistrar interface {
	AddStaticToChunkSpatial(handle types.Handle, chunkX, chunkY, x, y int)
	RemoveStaticFromChunkSpatial(handle types.Handle, chunkX, chunkY, x, y int)
}

// InventoryExecutor implements systems.InventoryOperationExecutor interface
type InventoryExecutor struct {
	service          *InventoryOperationService
	spatialRegistrar DroppedItemSpatialRegistrar
	logger           *zap.Logger
}

func NewInventoryExecutor(
	logger *zap.Logger,
	idAlloc EntityIDAllocator,
	persister DroppedItemPersister,
	spatialRegistrar DroppedItemSpatialRegistrar,
) *InventoryExecutor {
	return &InventoryExecutor{
		service:          NewInventoryOperationService(logger, idAlloc, persister),
		spatialRegistrar: spatialRegistrar,
		logger:           logger,
	}
}

func (e *InventoryExecutor) ExecuteOperation(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	op *netproto.InventoryOp,
) systems.InventoryOpResult {
	result := e.service.ExecuteOperation(w, playerID, playerHandle, op)

	// Convert internal result to systems.InventoryOpResult
	opResult := systems.InventoryOpResult{
		Success:           result.Success,
		ErrorCode:         result.ErrorCode,
		Message:           result.Message,
		UpdatedContainers: make([]systems.InventoryContainerState, 0, len(result.UpdatedContainers)),
	}

	// Register/unregister dropped entity in chunk spatial
	e.handleDroppedItemSpatial(w, result)

	// Check cascade: if operation changed a nested container, update parent item resource
	e.checkNestedCascade(w, playerID, result)

	// Collect nested container refs that should be closed on the client
	e.collectClosedContainerRefs(w, result)
	opResult.ClosedContainerRefs = result.ClosedContainerRefs

	// Convert updated containers
	for _, container := range result.UpdatedContainers {
		containerState := e.convertContainerToState(w, container)
		opResult.UpdatedContainers = append(opResult.UpdatedContainers, containerState)
	}

	return opResult
}

// GiveItem creates a new item and adds it to the player's inventory (or drops it).
// Returns updated container states for client sync.
func (e *InventoryExecutor) GiveItem(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	itemKey string,
	count uint32,
	quality uint32,
) *GiveItemResult {
	result := e.service.GiveItem(w, playerID, playerHandle, itemKey, count, quality)

	// Register dropped entity in chunk spatial if item was dropped
	if result.Success && result.SpawnedDroppedEntityID != nil {
		e.registerDroppedSpatial(w, *result.SpawnedDroppedEntityID)
	}

	return result
}

// registerDroppedSpatial adds a newly spawned dropped entity to chunk spatial.
func (e *InventoryExecutor) registerDroppedSpatial(w *ecs.World, entityID types.EntityID) {
	if e.spatialRegistrar == nil {
		return
	}
	handle := w.GetHandleByEntityID(entityID)
	if handle == types.InvalidHandle {
		return
	}
	transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, handle)
	if hasTransform && hasChunkRef {
		e.spatialRegistrar.AddStaticToChunkSpatial(handle, chunkRef.CurrentChunkX, chunkRef.CurrentChunkY, int(transform.X), int(transform.Y))
	}
}

// handleDroppedItemSpatial registers/unregisters dropped item entities in chunk spatial
// so that VisionSystem can discover them via QueryRadius.
func (e *InventoryExecutor) handleDroppedItemSpatial(w *ecs.World, result *OperationResult) {
	if !result.Success || e.spatialRegistrar == nil {
		return
	}

	if result.SpawnedDroppedEntityID != nil {
		handle := w.GetHandleByEntityID(*result.SpawnedDroppedEntityID)
		if handle == types.InvalidHandle {
			return
		}
		transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
		chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, handle)
		if hasTransform && hasChunkRef {
			e.spatialRegistrar.AddStaticToChunkSpatial(handle, chunkRef.CurrentChunkX, chunkRef.CurrentChunkY, int(transform.X), int(transform.Y))
		}
	}

	if result.DespawnedDroppedEntityID != nil {
		handle := w.GetHandleByEntityID(*result.DespawnedDroppedEntityID)
		if handle == types.InvalidHandle {
			return
		}
		transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
		chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, handle)
		if hasTransform && hasChunkRef {
			e.spatialRegistrar.RemoveStaticFromChunkSpatial(handle, chunkRef.CurrentChunkX, chunkRef.CurrentChunkY, int(transform.X), int(transform.Y))
		}
	}
}

// checkNestedCascade checks if any updated container is a nested one (owner_id = item_id).
// If the item's visual depends on hasNestedItems and that changed, update the parent item's resource
// and add the parent container to UpdatedContainers.
func (e *InventoryExecutor) checkNestedCascade(w *ecs.World, playerID types.EntityID, result *OperationResult) {
	if !result.Success {
		return
	}

	for _, updatedInfo := range result.UpdatedContainers {
		nestedOwnerID := updatedInfo.Container.OwnerID

		// Find which parent container holds the item with ItemID == nestedOwnerID
		parentInfo := e.findParentContainer(w, playerID, updatedInfo.Owner, nestedOwnerID)
		if parentInfo == nil {
			continue
		}

		// Find the item in parent container
		hasNestedItems := len(updatedInfo.Container.Items) > 0
		parentDirty := false

		ecs.MutateComponent[components.InventoryContainer](w, parentInfo.Handle, func(c *components.InventoryContainer) bool {
			for i := range c.Items {
				if c.Items[i].ItemID == nestedOwnerID {
					itemDef, ok := itemdefs.Global().GetByID(int(c.Items[i].TypeID))
					if !ok {
						break
					}
					newResource := itemDef.ResolveResource(hasNestedItems)
					if c.Items[i].Resource != newResource {
						c.Items[i].Resource = newResource
						c.Version++
						parentDirty = true
					}
					break
				}
			}
			return parentDirty
		})

		if parentDirty {
			updatedParent, _ := ecs.GetComponent[components.InventoryContainer](w, parentInfo.Handle)
			parentInfo.Container = &updatedParent
			// Add parent to updated list if not already there
			alreadyIncluded := false
			for _, existing := range result.UpdatedContainers {
				if existing.Handle == parentInfo.Handle {
					alreadyIncluded = true
					break
				}
			}
			if !alreadyIncluded {
				result.UpdatedContainers = append(result.UpdatedContainers, parentInfo)
			}
		}
	}
}

// collectClosedContainerRefs checks if any updated container is a hand that now holds
// a container item. If so, the item's nested container should be closed on the client.
func (e *InventoryExecutor) collectClosedContainerRefs(w *ecs.World, result *OperationResult) {
	if !result.Success {
		return
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)

	for _, info := range result.UpdatedContainers {
		if info.Container.Kind != constt.InventoryHand || len(info.Container.Items) == 0 {
			continue
		}
		item := info.Container.Items[0]
		itemDef, ok := itemdefs.Global().GetByID(int(item.TypeID))
		if !ok || itemDef.Container == nil {
			continue
		}
		// Item in hand is a container — check if it has a nested inventory
		if _, found := refIndex.Lookup(constt.InventoryGrid, item.ItemID, 0); found {
			appendClosedNestedRefIfPresent(w, result, item.ItemID)
		}
	}
}

// findParentContainer finds the container that holds an item with the given itemID
func (e *InventoryExecutor) findParentContainer(
	w *ecs.World,
	playerID types.EntityID,
	owner *components.InventoryOwner,
	itemID types.EntityID,
) *ContainerInfo {
	if owner != nil {
		for _, link := range owner.Inventories {
			// Skip nested containers themselves
			if link.OwnerID == itemID {
				continue
			}

			container, ok := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
			if !ok {
				continue
			}

			for _, item := range container.Items {
				if item.ItemID == itemID {
					return &ContainerInfo{
						Handle:    link.Handle,
						Container: &container,
						Owner:     owner,
					}
				}
			}
		}
	}

	// Nested container may be inside an opened world-object root container
	// which is not part of player's InventoryOwner links.
	openState, hasOpenState := ecs.TryGetResource[ecs.OpenContainerState](w)
	if !hasOpenState {
		return nil
	}

	rootOwnerID, hasRoot := openState.GetOpenedRoot(playerID)
	if !hasRoot {
		return nil
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, rootOwnerID, 0)
	if !found || !w.Alive(rootHandle) {
		return nil
	}

	rootContainer, ok := ecs.GetComponent[components.InventoryContainer](w, rootHandle)
	if !ok {
		return nil
	}

	for _, item := range rootContainer.Items {
		if item.ItemID == itemID {
			return &ContainerInfo{
				Handle:    rootHandle,
				Container: &rootContainer,
				Owner:     owner,
			}
		}
	}

	return nil
}

// ConvertContainersToStates converts internal ContainerInfo list to systems.InventoryContainerState list.
// Exported for use by admin commands and other mechanics that need to send inventory updates.
func (e *InventoryExecutor) ConvertContainersToStates(w *ecs.World, containers []*ContainerInfo) []systems.InventoryContainerState {
	states := make([]systems.InventoryContainerState, 0, len(containers))
	for _, c := range containers {
		states = append(states, e.convertContainerToState(w, c))
	}
	return states
}

// ExecutePickupFromWorld picks up a dropped item from the world into a player's inventory.
// Wraps InventoryOperationService.ExecutePickupFromWorld with spatial cleanup and state conversion.
func (e *InventoryExecutor) ExecutePickupFromWorld(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	droppedEntityID types.EntityID,
	dstRef *netproto.InventoryRef,
) systems.InventoryOpResult {
	result := e.service.ExecutePickupFromWorld(w, playerID, playerHandle, droppedEntityID, dstRef)

	opResult := systems.InventoryOpResult{
		Success:           result.Success,
		ErrorCode:         result.ErrorCode,
		Message:           result.Message,
		UpdatedContainers: make([]systems.InventoryContainerState, 0, len(result.UpdatedContainers)),
	}

	e.handleDroppedItemSpatial(w, result)

	// Collect nested container refs that should be closed on the client
	e.collectClosedContainerRefs(w, result)
	opResult.ClosedContainerRefs = result.ClosedContainerRefs

	for _, container := range result.UpdatedContainers {
		containerState := e.convertContainerToState(w, container)
		opResult.UpdatedContainers = append(opResult.UpdatedContainers, containerState)
	}

	return opResult
}

func (e *InventoryExecutor) convertContainerToState(w *ecs.World, info *ContainerInfo) systems.InventoryContainerState {
	state := systems.InventoryContainerState{
		OwnerID:          info.Container.OwnerID,
		Kind:             uint8(info.Container.Kind),
		Key:              info.Container.Key,
		Version:          info.Container.Version,
		Width:            info.Container.Width,
		Height:           info.Container.Height,
		Items:            make([]systems.InventoryItemState, 0, len(info.Container.Items)),
		HandMouseOffsetX: info.Container.HandMouseOffsetX,
		HandMouseOffsetY: info.Container.HandMouseOffsetY,
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	for _, item := range info.Container.Items {
		itemState := systems.InventoryItemState{
			ItemID:    item.ItemID,
			TypeID:    item.TypeID,
			Resource:  item.Resource,
			Quality:   item.Quality,
			Quantity:  item.Quantity,
			W:         item.W,
			H:         item.H,
			X:         item.X,
			Y:         item.Y,
			EquipSlot: item.EquipSlot,
		}

		// Check if this item has a nested container via index (O(1))
		if _, found := refIndex.Lookup(constt.InventoryGrid, item.ItemID, 0); found {
			itemState.NestedRef = &netproto.InventoryRef{
				Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
				OwnerId:      uint64(item.ItemID),
				InventoryKey: 0,
			}
		}

		state.Items = append(state.Items, itemState)
	}

	return state
}
