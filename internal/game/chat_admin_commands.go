package game

import (
	"fmt"
	"strconv"
	"strings"

	"origin/internal/ecs"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	defaultGiveCount   uint32 = 1
	defaultGiveQuality uint32 = 10
)

// ChatAdminCommandHandler processes admin slash-commands received from chat.
// Implements systems.AdminCommandHandler.
type ChatAdminCommandHandler struct {
	inventoryExecutor     *inventory.InventoryExecutor
	inventoryResultSender systems.InventoryResultSender
	chatDelivery          systems.ChatDeliveryService
	logger                *zap.Logger
}

func NewChatAdminCommandHandler(
	inventoryExecutor *inventory.InventoryExecutor,
	inventoryResultSender systems.InventoryResultSender,
	chatDelivery systems.ChatDeliveryService,
	logger *zap.Logger,
) *ChatAdminCommandHandler {
	return &ChatAdminCommandHandler{
		inventoryExecutor:     inventoryExecutor,
		inventoryResultSender: inventoryResultSender,
		chatDelivery:          chatDelivery,
		logger:                logger,
	}
}

// HandleCommand parses and executes an admin command.
// Returns true if the text was recognized as an admin command (even if it failed).
func (h *ChatAdminCommandHandler) HandleCommand(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	text string,
) bool {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return false
	}

	switch parts[0] {
	case "/give":
		h.handleGive(w, playerID, playerHandle, parts[1:])
		return true
	default:
		return false
	}
}

// handleGive processes: /give <item_key> [count] [quality]
func (h *ChatAdminCommandHandler) handleGive(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	args []string,
) {
	if len(args) == 0 {
		h.sendSystemMessage(playerID, "usage: /give <item_key> [count] [quality]")
		return
	}

	itemKey := args[0]
	count := defaultGiveCount
	quality := defaultGiveQuality

	if len(args) >= 2 {
		if v, err := strconv.ParseUint(args[1], 10, 32); err == nil && v > 0 {
			count = uint32(v)
		} else {
			h.sendSystemMessage(playerID, "invalid count: "+args[1])
			return
		}
	}

	if len(args) >= 3 {
		if v, err := strconv.ParseUint(args[2], 10, 32); err == nil {
			quality = uint32(v)
		} else {
			h.sendSystemMessage(playerID, "invalid quality: "+args[2])
			return
		}
	}

	result := h.inventoryExecutor.GiveItem(w, playerID, playerHandle, itemKey, count, quality)
	if !result.Success {
		h.sendSystemMessage(playerID, "give failed: "+result.Message)
		h.logger.Warn("Admin /give failed",
			zap.Uint64("player_id", uint64(playerID)),
			zap.String("item_key", itemKey),
			zap.String("reason", result.Message))
		return
	}

	// Send inventory update to client
	if len(result.UpdatedContainers) > 0 {
		h.sendInventoryUpdate(w, playerID, result)
	}

	h.sendSystemMessage(playerID, fmt.Sprintf("gave %s x%d q%d — %s", itemKey, count, quality, result.Message))

	h.logger.Info("Admin /give executed",
		zap.Uint64("player_id", uint64(playerID)),
		zap.String("item_key", itemKey),
		zap.Uint32("count", count),
		zap.Uint32("quality", quality),
		zap.String("result", result.Message))
}

// sendInventoryUpdate converts updated containers to proto and sends to the client.
func (h *ChatAdminCommandHandler) sendInventoryUpdate(
	w *ecs.World,
	playerID types.EntityID,
	result *inventory.GiveItemResult,
) {
	if h.inventoryResultSender == nil {
		return
	}

	states := h.inventoryExecutor.ConvertContainersToStates(w, result.UpdatedContainers)

	updated := make([]*netproto.InventoryState, 0, len(states))
	for _, st := range states {
		updated = append(updated, buildInventoryStateFromContainerState(st))
	}

	response := &netproto.S2C_InventoryOpResult{
		OpId:    0,
		Success: true,
		Updated: updated,
	}
	h.inventoryResultSender.SendInventoryOpResult(playerID, response)
}

// sendSystemMessage sends a server-originated chat message to the player.
func (h *ChatAdminCommandHandler) sendSystemMessage(playerID types.EntityID, text string) {
	if h.chatDelivery == nil {
		return
	}
	h.chatDelivery.SendChatMessage(playerID, netproto.ChatChannel_CHAT_CHANNEL_LOCAL, 0, "[Server]", text)
}

// buildInventoryStateFromContainerState converts a systems.InventoryContainerState to proto.
func buildInventoryStateFromContainerState(cs systems.InventoryContainerState) *netproto.InventoryState {
	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind(cs.Kind),
		OwnerId:      uint64(cs.OwnerID),
		InventoryKey: cs.Key,
	}

	invState := &netproto.InventoryState{
		Ref:      ref,
		Revision: cs.Version,
	}

	switch cs.Kind {
	case uint8(netproto.InventoryKind_INVENTORY_KIND_GRID):
		gridItems := make([]*netproto.GridItem, 0, len(cs.Items))
		for _, item := range cs.Items {
			gridItems = append(gridItems, &netproto.GridItem{
				X:    uint32(item.X),
				Y:    uint32(item.Y),
				Item: buildItemInstanceFromState(item),
			})
		}
		invState.State = &netproto.InventoryState_Grid{
			Grid: &netproto.InventoryGridState{
				Width:  uint32(cs.Width),
				Height: uint32(cs.Height),
				Items:  gridItems,
			},
		}

	case uint8(netproto.InventoryKind_INVENTORY_KIND_HAND):
		handState := &netproto.InventoryHandState{}
		if len(cs.Items) > 0 {
			handState.Item = buildItemInstanceFromState(cs.Items[0])
			handState.HandPos = &netproto.HandPos{
				MouseOffsetX: int32(cs.HandMouseOffsetX),
				MouseOffsetY: int32(cs.HandMouseOffsetY),
			}
		}
		invState.State = &netproto.InventoryState_Hand{
			Hand: handState,
		}

	case uint8(netproto.InventoryKind_INVENTORY_KIND_EQUIPMENT):
		equipItems := make([]*netproto.EquipmentItem, 0, len(cs.Items))
		for _, item := range cs.Items {
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

func buildItemInstanceFromState(item systems.InventoryItemState) *netproto.ItemInstance {
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
