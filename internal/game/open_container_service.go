package game

import (
	"context"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// OpenContainerService listens to link events and manages object container open/close lifecycle.
type OpenContainerService struct {
	shardManager *ShardManager
}

func NewOpenContainerService(shardManager *ShardManager) *OpenContainerService {
	return &OpenContainerService{
		shardManager: shardManager,
	}
}

func (s *OpenContainerService) Subscribe(eventBus *eventbus.EventBus) {
	eventBus.SubscribeSync(ecs.TopicGameplayLinkCreated, eventbus.PriorityMedium, s.handleLinkCreated)
	eventBus.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityMedium, s.handleLinkBroken)
}

func (s *OpenContainerService) handleLinkCreated(_ context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.LinkCreatedEvent)
	if !ok {
		return nil
	}

	shard := s.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}

	w := shard.World()
	if !w.Alive(event.PlayerHandle) || !w.Alive(event.ObjectHandle) {
		return nil
	}

	if !hasBehavior(w, event.ObjectHandle, "container") {
		return nil
	}

	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, event.ObjectHandle)
	if !hasOwner {
		return nil
	}

	open := ecs.GetResource[ecs.OpenContainers](w)
	if open == nil {
		return nil
	}

	// Close any leftovers for safety (relink should already produce LinkBroken).
	closed := open.CloseAll(event.PlayerID)
	if len(closed) > 0 {
		s.sendClosed(shard, event.PlayerID, closed)
	}

	for _, link := range owner.Inventories {
		if link.OwnerID != event.ObjectID {
			continue
		}

		container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !hasContainer {
			continue
		}

		open.Open(event.PlayerID, ecs.OpenContainerEntry{
			Handle:  link.Handle,
			OwnerID: container.OwnerID,
			Kind:    container.Kind,
			Key:     container.Key,
		})

		state := buildInventoryStateFromContainer(w, container)
		if state != nil {
			shard.SendContainerOpened(event.PlayerID, state)
		}
	}

	return nil
}

func (s *OpenContainerService) handleLinkBroken(_ context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.LinkBrokenEvent)
	if !ok {
		return nil
	}

	shard := s.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}

	w := shard.World()
	open := ecs.GetResource[ecs.OpenContainers](w)
	if open == nil {
		return nil
	}

	closed := open.CloseAll(event.PlayerID)
	if len(closed) > 0 {
		s.sendClosed(shard, event.PlayerID, closed)
	}

	return nil
}

func (s *OpenContainerService) sendClosed(shard *Shard, playerID types.EntityID, entries []ecs.OpenContainerEntry) {
	for _, entry := range entries {
		ref := &netproto.InventoryRef{
			Kind:         netproto.InventoryKind(entry.Kind),
			OwnerId:      uint64(entry.OwnerID),
			InventoryKey: entry.Key,
		}
		shard.SendContainerClosed(playerID, ref)
	}
}

func hasBehavior(w *ecs.World, h types.Handle, key string) bool {
	info, ok := ecs.GetComponent[components.EntityInfo](w, h)
	if !ok {
		return false
	}
	for _, b := range info.Behaviors {
		if b == key {
			return true
		}
	}
	return false
}

func buildInventoryStateFromContainer(w *ecs.World, container components.InventoryContainer) *netproto.InventoryState {
	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind(container.Kind),
		OwnerId:      uint64(container.OwnerID),
		InventoryKey: container.Key,
	}

	invState := &netproto.InventoryState{
		Ref:      ref,
		Revision: container.Version,
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)

	switch container.Kind {
	case constt.InventoryGrid:
		gridItems := make([]*netproto.GridItem, 0, len(container.Items))
		for _, item := range container.Items {
			gridItems = append(gridItems, &netproto.GridItem{
				X:    uint32(item.X),
				Y:    uint32(item.Y),
				Item: buildItemInstance(item, refIndex),
			})
		}
		invState.State = &netproto.InventoryState_Grid{
			Grid: &netproto.InventoryGridState{
				Width:  uint32(container.Width),
				Height: uint32(container.Height),
				Items:  gridItems,
			},
		}

	case constt.InventoryHand, constt.InventoryDroppedItem:
		handState := &netproto.InventoryHandState{}
		if len(container.Items) > 0 {
			handState.Item = buildItemInstance(container.Items[0], refIndex)
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
				Item: buildItemInstance(item, refIndex),
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

func buildItemInstance(item components.InvItem, refIndex *ecs.InventoryRefIndex) *netproto.ItemInstance {
	instance := &netproto.ItemInstance{
		ItemId:   uint64(item.ItemID),
		TypeId:   item.TypeID,
		Resource: item.Resource,
		Quality:  item.Quality,
		Quantity: item.Quantity,
		W:        uint32(item.W),
		H:        uint32(item.H),
	}

	if refIndex != nil {
		if _, nestedFound := refIndex.Lookup(constt.InventoryGrid, item.ItemID, 0); nestedFound {
			instance.NestedRef = &netproto.InventoryRef{
				Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
				OwnerId:      uint64(item.ItemID),
				InventoryKey: 0,
			}
		}
	}

	return instance
}
