package systems

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

// AutoInteractSystem checks entities with PendingInteraction and executes the
// interaction once the entity is within range of the target.
// Priority 320: runs after TransformUpdateSystem (300) so positions are final.
type AutoInteractSystem struct {
	ecs.BaseSystem
	query                 *ecs.PreparedQuery
	inventoryExecutor     InventoryOperationExecutor
	inventoryResultSender InventoryResultSender
	logger                *zap.Logger
}

func NewAutoInteractSystem(
	inventoryExecutor InventoryOperationExecutor,
	inventoryResultSender InventoryResultSender,
	logger *zap.Logger,
) *AutoInteractSystem {
	return &AutoInteractSystem{
		BaseSystem:            ecs.NewBaseSystem("AutoInteractSystem", 320),
		inventoryExecutor:     inventoryExecutor,
		inventoryResultSender: inventoryResultSender,
		logger:                logger,
	}
}

func (s *AutoInteractSystem) Update(w *ecs.World, dt float64) {
	if s.query == nil {
		s.query = ecs.NewPreparedQuery(
			w,
			(1<<components.TransformComponentID)|
				(1<<components.PendingInteractionComponentID),
			0,
		)
	}

	type pendingAction struct {
		handle          types.Handle
		entityID        types.EntityID
		playerHandle    types.Handle
		pending         components.PendingInteraction
		playerTransform components.Transform
	}

	var actions []pendingAction

	s.query.ForEach(func(h types.Handle) {
		pending, ok := ecs.GetComponent[components.PendingInteraction](w, h)
		if !ok {
			return
		}

		// Validate target is still alive
		if !w.Alive(pending.TargetHandle) {
			ecs.RemoveComponent[components.PendingInteraction](w, h)
			return
		}

		playerTransform, hasTransform := ecs.GetComponent[components.Transform](w, h)
		if !hasTransform {
			return
		}

		targetTransform, hasTargetTransform := ecs.GetComponent[components.Transform](w, pending.TargetHandle)
		if !hasTargetTransform {
			ecs.RemoveComponent[components.PendingInteraction](w, h)
			return
		}

		// Check distance
		dx := playerTransform.X - targetTransform.X
		dy := playerTransform.Y - targetTransform.Y
		distSq := dx*dx + dy*dy
		rangeSq := pending.Range * pending.Range

		if distSq > rangeSq {
			// Not in range yet — check if still moving toward target
			mov, hasMov := ecs.GetComponent[components.Movement](w, h)
			if !hasMov || mov.State != constt.StateMoving {
				// Stopped moving but not in range — cancel
				ecs.RemoveComponent[components.PendingInteraction](w, h)
			}
			return
		}

		// In range — collect for deferred execution
		entityID, hasExt := w.GetExternalID(h)
		if !hasExt {
			ecs.RemoveComponent[components.PendingInteraction](w, h)
			return
		}

		actions = append(actions, pendingAction{
			handle:          h,
			entityID:        entityID,
			playerHandle:    h,
			pending:         pending,
			playerTransform: playerTransform,
		})
	})

	// Execute collected actions outside ForEach (may mutate archetypes)
	for _, a := range actions {
		s.executeInteraction(w, a.entityID, a.playerHandle, a.pending)
		ecs.RemoveComponent[components.PendingInteraction](w, a.handle)
	}
}

func (s *AutoInteractSystem) executeInteraction(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	pending components.PendingInteraction,
) {
	switch pending.Type {
	case netproto.InteractionType_PICKUP:
		s.executePickup(w, playerID, playerHandle, pending)
	default:
		s.logger.Debug("AutoInteract: unsupported interaction type",
			zap.Int32("type", int32(pending.Type)))
	}
}

func (s *AutoInteractSystem) executePickup(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	pending components.PendingInteraction,
) {
	if s.inventoryExecutor == nil {
		return
	}

	// Choose destination: hand if empty, otherwise grid
	dstRef := s.chooseDstContainer(w, playerID, playerHandle)
	if dstRef == nil {
		return
	}

	result := s.inventoryExecutor.ExecutePickupFromWorld(
		w, playerID, playerHandle, pending.TargetEntityID, dstRef,
	)

	// Send result to client
	if s.inventoryResultSender == nil {
		return
	}

	response := &netproto.S2C_InventoryOpResult{
		OpId:    0, // auto-pickup has no client-side opId
		Success: result.Success,
		Error:   result.ErrorCode,
		Message: result.Message,
		Updated: make([]*netproto.InventoryState, 0, len(result.UpdatedContainers)),
	}

	for _, container := range result.UpdatedContainers {
		invState := buildInventoryStateFromOpResult(container)
		response.Updated = append(response.Updated, invState)
	}

	s.inventoryResultSender.SendInventoryOpResult(playerID, response)

	if result.Success {
		s.logger.Debug("AutoInteract: pickup success",
			zap.Uint64("player_id", uint64(playerID)),
			zap.Uint64("target_entity_id", uint64(pending.TargetEntityID)))
	}
}

// chooseDstContainer selects the destination container for auto-pickup:
// grid first, then hand if empty.
func (s *AutoInteractSystem) chooseDstContainer(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
) *netproto.InventoryRef {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)

	// Check grid first
	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if hasOwner {
		for _, link := range owner.Inventories {
			if link.Kind == constt.InventoryGrid && link.OwnerID == playerID {
				return &netproto.InventoryRef{
					Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
					OwnerId:      uint64(playerID),
					InventoryKey: link.Key,
				}
			}
		}
	}

	// Fallback to hand if empty
	handHandle, handFound := refIndex.Lookup(constt.InventoryHand, playerID, 0)
	if handFound && w.Alive(handHandle) {
		handContainer, hasHand := ecs.GetComponent[components.InventoryContainer](w, handHandle)
		if hasHand && len(handContainer.Items) == 0 {
			return &netproto.InventoryRef{
				Kind:    netproto.InventoryKind_INVENTORY_KIND_HAND,
				OwnerId: uint64(playerID),
			}
		}
	}

	return nil
}

// buildInventoryStateFromOpResult converts InventoryContainerState to proto InventoryState.
// Mirrors NetworkCommandSystem.buildInventoryStateProto but works with InventoryOpResult data.
func buildInventoryStateFromOpResult(container InventoryContainerState) *netproto.InventoryState {
	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind(container.Kind),
		OwnerId:      uint64(container.OwnerID),
		InventoryKey: container.Key,
	}

	invState := &netproto.InventoryState{
		Ref:      ref,
		Revision: container.Version,
	}

	switch constt.InventoryKind(container.Kind) {
	case constt.InventoryGrid:
		gridItems := make([]*netproto.GridItem, 0, len(container.Items))
		for _, item := range container.Items {
			gridItems = append(gridItems, &netproto.GridItem{
				X:    uint32(item.X),
				Y:    uint32(item.Y),
				Item: buildItemInstanceFromState(item),
			})
		}
		invState.State = &netproto.InventoryState_Grid{
			Grid: &netproto.InventoryGridState{
				Width:  uint32(container.Width),
				Height: uint32(container.Height),
				Items:  gridItems,
			},
		}

	case constt.InventoryHand:
		handState := &netproto.InventoryHandState{}
		if len(container.Items) > 0 {
			handState.Item = buildItemInstanceFromState(container.Items[0])
			handState.HandPos = &netproto.HandPos{
				MouseOffsetX: int32(container.HandMouseOffsetX),
				MouseOffsetY: int32(container.HandMouseOffsetY),
			}
		}
		invState.State = &netproto.InventoryState_Hand{
			Hand: handState,
		}

	case constt.InventoryEquipment:
		equipItems := make([]*netproto.EquipmentItem, 0, len(container.Items))
		for _, item := range container.Items {
			equipItems = append(equipItems, &netproto.EquipmentItem{
				Slot: item.EquipSlot,
				Item: buildItemInstanceFromState(item),
			})
		}
		invState.State = &netproto.InventoryState_Equipment{
			Equipment: &netproto.InventoryEquipmentState{
				Items: equipItems,
			},
		}
	}

	return invState
}

func buildItemInstanceFromState(item InventoryItemState) *netproto.ItemInstance {
	instance := &netproto.ItemInstance{
		ItemId:   uint64(item.ItemID),
		TypeId:   item.TypeID,
		Resource: item.Resource,
		Quality:  item.Quality,
		Quantity: item.Quantity,
		W:        uint32(item.W),
		H:        uint32(item.H),
	}
	if item.NestedRef != nil {
		instance.NestedRef = item.NestedRef
	}
	return instance
}
