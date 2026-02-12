package game

import (
	"fmt"
	"strconv"
	"strings"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	defaultGiveCount    uint32 = 1
	defaultGiveQuality  uint32 = 10
	defaultSpawnQuality uint32 = 10
)

// AdminSpawnChunkProvider gives the admin handler access to chunk data for spawn validation.
type AdminSpawnChunkProvider interface {
	GetEntityActiveChunks(entityID types.EntityID) []*core.Chunk
	GetChunk(coord types.ChunkCoord) *core.Chunk
	AddStaticToChunkSpatial(handle types.Handle, chunkX, chunkY, x, y int)
}

// AdminVisionForcer forces immediate vision recalculation for an observer.
type AdminVisionForcer interface {
	ForceUpdateForObserver(w *ecs.World, observerHandle types.Handle)
}

// ChatAdminCommandHandler processes admin slash-commands received from chat.
// Implements systems.AdminCommandHandler.
type ChatAdminCommandHandler struct {
	inventoryExecutor     *inventory.InventoryExecutor
	inventoryResultSender systems.InventoryResultSender
	chatDelivery          systems.ChatDeliveryService
	entityIDAllocator     inventory.EntityIDAllocator
	chunkProvider         AdminSpawnChunkProvider
	visionForcer          AdminVisionForcer
	eventBus              *eventbus.EventBus
	logger                *zap.Logger
}

func NewChatAdminCommandHandler(
	inventoryExecutor *inventory.InventoryExecutor,
	inventoryResultSender systems.InventoryResultSender,
	chatDelivery systems.ChatDeliveryService,
	entityIDAllocator inventory.EntityIDAllocator,
	chunkProvider AdminSpawnChunkProvider,
	visionForcer AdminVisionForcer,
	eventBus *eventbus.EventBus,
	logger *zap.Logger,
) *ChatAdminCommandHandler {
	return &ChatAdminCommandHandler{
		inventoryExecutor:     inventoryExecutor,
		inventoryResultSender: inventoryResultSender,
		chatDelivery:          chatDelivery,
		entityIDAllocator:     entityIDAllocator,
		chunkProvider:         chunkProvider,
		visionForcer:          visionForcer,
		eventBus:              eventBus,
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
	case "/spawn":
		h.handleSpawn(w, playerID, parts[1:])
		return true
	case "/online":
		h.handleOnline(w, playerID)
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

// handleSpawn processes: /spawn <object_key> [quality]
func (h *ChatAdminCommandHandler) handleSpawn(
	w *ecs.World,
	playerID types.EntityID,
	args []string,
) {
	if len(args) == 0 {
		h.sendSystemMessage(playerID, "usage: /spawn <object_key> [quality]")
		return
	}

	objectKey := args[0]
	quality := defaultSpawnQuality

	if len(args) >= 2 {
		if v, err := strconv.ParseUint(args[1], 10, 32); err == nil {
			quality = uint32(v)
		} else {
			h.sendSystemMessage(playerID, "invalid quality: "+args[1])
			return
		}
	}

	def, ok := objectdefs.Global().GetByKey(objectKey)
	if !ok {
		h.sendSystemMessage(playerID, "unknown object key: "+objectKey)
		return
	}

	pending := ecs.GetResource[ecs.PendingAdminSpawn](w)
	pending.Set(playerID, ecs.AdminSpawnEntry{
		ObjectKey: objectKey,
		DefID:     def.DefID,
		Quality:   quality,
	})

	h.sendSystemMessage(playerID, "Click on the map where the object should be spawned.")
	h.logger.Info("Admin /spawn pending",
		zap.Uint64("player_id", uint64(playerID)),
		zap.String("object_key", objectKey),
		zap.Uint32("quality", quality))
}

// ExecutePendingSpawn is called by NetworkCommandSystem when a player with a pending
// /spawn clicks on the map. It validates coordinates, creates the entity, and announces.
func (h *ChatAdminCommandHandler) ExecutePendingSpawn(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetX, targetY float64,
) {
	pending := ecs.GetResource[ecs.PendingAdminSpawn](w)
	entry, ok := pending.Get(playerID)
	if !ok {
		return
	}
	pending.Clear(playerID)

	// Validate target is within one of the player's active chunks
	chunk := h.findActiveChunkForPoint(playerID, targetX, targetY)
	if chunk == nil {
		h.sendSystemMessage(playerID, "Target location is outside your active chunks.")
		h.logger.Warn("Admin /spawn: target outside active chunks",
			zap.Uint64("player_id", uint64(playerID)),
			zap.Float64("x", targetX), zap.Float64("y", targetY))
		return
	}

	def, ok := objectdefs.Global().GetByID(entry.DefID)
	if !ok {
		h.sendSystemMessage(playerID, "object definition no longer valid")
		return
	}

	newID := h.entityIDAllocator.GetFreeID()
	chunkX := int(targetX) / constt.ChunkWorldSize
	chunkY := int(targetY) / constt.ChunkWorldSize

	// Resolve region/layer from the chunk
	region := chunk.Region
	layer := chunk.Layer

	hp := def.HP

	handle := gameworld.SpawnEntityFromDef(w, def, gameworld.DefSpawnParams{
		EntityID: newID,
		X:        targetX,
		Y:        targetY,
		Region:   region,
		Layer:    layer,
	})
	if handle == types.InvalidHandle {
		h.sendSystemMessage(playerID, "failed to spawn entity")
		return
	}
	ecs.AddComponent(w, handle, components.ChunkRef{
		CurrentChunkX: chunkX,
		CurrentChunkY: chunkY,
		PrevChunkX:    chunkX,
		PrevChunkY:    chunkY,
	})
	ecs.AddComponent(w, handle, components.ObjectInternalState{
		IsDirty: true,
	})

	// Add to chunk spatial
	if def.IsStatic {
		h.chunkProvider.AddStaticToChunkSpatial(handle, chunkX, chunkY, int(targetX), int(targetY))
	} else {
		coord := types.ChunkCoord{X: chunkX, Y: chunkY}
		if c := h.chunkProvider.GetChunk(coord); c != nil {
			c.Spatial().AddDynamic(handle, int(targetX), int(targetY))
		}
	}

	// Instantiate inventories if the definition has container behavior
	if h.isContainerDef(def) {
		links := h.spawnObjectInventories(w, newID, def)
		ecs.AddComponent(w, handle, components.InventoryOwner{
			Inventories: links,
		})
	}

	// Mark chunk dirty for persistence
	chunk.MarkRawDataDirty()
	ecs.MarkObjectBehaviorDirty(w, handle)

	// Force vision update for all players so the object appears immediately
	if h.visionForcer != nil {
		characterEntities := ecs.GetResource[ecs.CharacterEntities](w)
		for _, ce := range characterEntities.Map {
			if w.Alive(ce.Handle) {
				h.visionForcer.ForceUpdateForObserver(w, ce.Handle)
			}
		}
	}

	h.sendSystemMessage(playerID, fmt.Sprintf(
		"Object %s spawned at (%.0f, %.0f) id=%d hp=%d q=%d",
		entry.ObjectKey, targetX, targetY, newID, hp, entry.Quality,
	))
	h.logger.Info("Admin /spawn executed",
		zap.Uint64("player_id", uint64(playerID)),
		zap.String("object_key", entry.ObjectKey),
		zap.Uint64("new_id", uint64(newID)),
		zap.Float64("x", targetX),
		zap.Float64("y", targetY))
}

// handleOnline processes: /online - displays current online players count
func (h *ChatAdminCommandHandler) handleOnline(
	w *ecs.World,
	playerID types.EntityID,
) {
	characterEntities := ecs.GetResource[ecs.CharacterEntities](w)

	onlineCount := len(characterEntities.Map)
	message := fmt.Sprintf("Online players: %d", onlineCount)

	h.sendSystemMessage(playerID, message)
	h.logger.Info("Admin /online executed",
		zap.Uint64("player_id", uint64(playerID)),
		zap.Int("online_count", onlineCount))
}

// findActiveChunkForPoint returns the active chunk containing the given world point,
// or nil if the point is outside all of the player's active chunks.
func (h *ChatAdminCommandHandler) findActiveChunkForPoint(
	playerID types.EntityID,
	x, y float64,
) *core.Chunk {
	if h.chunkProvider == nil {
		return nil
	}

	chunks := h.chunkProvider.GetEntityActiveChunks(playerID)
	for _, chunk := range chunks {
		minX := float64(chunk.Coord.X * constt.ChunkWorldSize)
		minY := float64(chunk.Coord.Y * constt.ChunkWorldSize)
		maxX := minX + float64(constt.ChunkWorldSize)
		maxY := minY + float64(constt.ChunkWorldSize)
		if x >= minX && x < maxX && y >= minY && y < maxY {
			return chunk
		}
	}
	return nil
}

func (h *ChatAdminCommandHandler) isContainerDef(def *objectdefs.ObjectDef) bool {
	if def == nil || def.Components == nil || len(def.Components.Inventory) == 0 {
		return false
	}
	return def.HasBehavior("container")
}

func (h *ChatAdminCommandHandler) spawnObjectInventories(
	w *ecs.World,
	ownerID types.EntityID,
	def *objectdefs.ObjectDef,
) []components.InventoryLink {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	links := make([]components.InventoryLink, 0, len(def.Components.Inventory))

	for _, invDef := range def.Components.Inventory {
		kind := parseSpawnInventoryKind(invDef.Kind)
		containerHandle := w.SpawnWithoutExternalID()
		if containerHandle == types.InvalidHandle {
			continue
		}

		container := components.InventoryContainer{
			OwnerID: ownerID,
			Kind:    kind,
			Key:     invDef.Key,
			Version: 1,
			Width:   uint8(max(invDef.W, 0)),
			Height:  uint8(max(invDef.H, 0)),
			Items:   []components.InvItem{},
		}
		ecs.AddComponent(w, containerHandle, container)
		refIndex.Add(container.Kind, container.OwnerID, container.Key, containerHandle)
		links = append(links, components.InventoryLink{
			Kind:    container.Kind,
			Key:     container.Key,
			OwnerID: container.OwnerID,
			Handle:  containerHandle,
		})
	}

	return links
}

func parseSpawnInventoryKind(kind string) constt.InventoryKind {
	switch kind {
	case "hand":
		return constt.InventoryHand
	case "equipment":
		return constt.InventoryEquipment
	default:
		return constt.InventoryGrid
	}
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
