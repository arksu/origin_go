package game

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
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
	alertSender           AdminAlertSender
	entityIDAllocator     inventory.EntityIDAllocator
	chunkProvider         AdminSpawnChunkProvider
	visionForcer          AdminVisionForcer
	teleportExecutor      AdminTeleportExecutor
	behaviorRegistry      contracts.BehaviorRegistry
	eventBus              *eventbus.EventBus
	logger                *zap.Logger
}

// AdminAlertSender sends direct error/warning packets to a single player.
type AdminAlertSender interface {
	SendError(entityID types.EntityID, errorCode netproto.ErrorCode, message string)
	SendWarning(entityID types.EntityID, warningCode netproto.WarningCode, message string)
}

// AdminTeleportExecutor handles full player teleport transfer flow.
type AdminTeleportExecutor interface {
	RequestAdminTeleport(playerID types.EntityID, sourceLayer int, targetX, targetY int, targetLayer *int) error
}

func NewChatAdminCommandHandler(
	inventoryExecutor *inventory.InventoryExecutor,
	inventoryResultSender systems.InventoryResultSender,
	chatDelivery systems.ChatDeliveryService,
	alertSender AdminAlertSender,
	entityIDAllocator inventory.EntityIDAllocator,
	chunkProvider AdminSpawnChunkProvider,
	visionForcer AdminVisionForcer,
	behaviorRegistry contracts.BehaviorRegistry,
	eventBus *eventbus.EventBus,
	logger *zap.Logger,
) *ChatAdminCommandHandler {
	return &ChatAdminCommandHandler{
		inventoryExecutor:     inventoryExecutor,
		inventoryResultSender: inventoryResultSender,
		chatDelivery:          chatDelivery,
		alertSender:           alertSender,
		entityIDAllocator:     entityIDAllocator,
		chunkProvider:         chunkProvider,
		visionForcer:          visionForcer,
		behaviorRegistry:      behaviorRegistry,
		eventBus:              eventBus,
		logger:                logger,
	}
}

func (h *ChatAdminCommandHandler) SetTeleportExecutor(executor AdminTeleportExecutor) {
	h.teleportExecutor = executor
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
	case "/tp":
		h.handleTeleport(w, playerID, parts[1:])
		return true
	case "/online":
		h.handleOnline(w, playerID)
		return true
	case "/error":
		h.handleError(playerID, parts[1:])
		return true
	case "/warn":
		h.handleWarn(playerID, parts[1:])
		return true
	case "/stamina":
		h.handleStamina(w, playerID, playerHandle, parts[1:])
		return true
	case "/energy":
		h.handleEnergy(w, playerID, playerHandle, parts[1:])
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
	if result.DiscoveryLPGained > 0 {
		if expSender, ok := h.alertSender.(interface {
			SendExpGained(entityID types.EntityID, gained *netproto.S2C_ExpGained)
		}); ok {
			lp := result.DiscoveryLPGained
			expSender.SendExpGained(playerID, &netproto.S2C_ExpGained{
				EntityId: uint64(playerID),
				Lp:       &lp,
			})
		}
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

	// /spawn and /tp click mode are mutually exclusive.
	ecs.GetResource[ecs.PendingAdminTeleport](w).Clear(playerID)

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

// handleTeleport processes:
//
//	/tp
//	/tp <x> <y>
//	/tp <x> <y> <layer>
func (h *ChatAdminCommandHandler) handleTeleport(
	w *ecs.World,
	playerID types.EntityID,
	args []string,
) {
	if h.teleportExecutor == nil {
		h.sendSystemMessage(playerID, "teleport service unavailable")
		return
	}

	switch len(args) {
	case 0:
		// /tp click mode (current layer only)
		ecs.GetResource[ecs.PendingAdminSpawn](w).Clear(playerID)
		ecs.GetResource[ecs.PendingAdminTeleport](w).Set(playerID)
		h.sendSystemMessage(playerID, "Click on the map where you want to teleport.")
		h.logger.Info("Admin /tp pending click", zap.Uint64("player_id", uint64(playerID)))
		return
	case 1:
		h.sendSystemMessage(playerID, "missing y coordinate: usage /tp <x> <y> [layer]")
		return
	case 2, 3:
		x, err := strconv.Atoi(args[0])
		if err != nil {
			h.sendSystemMessage(playerID, "invalid x: "+args[0])
			return
		}
		y, err := strconv.Atoi(args[1])
		if err != nil {
			h.sendSystemMessage(playerID, "invalid y: "+args[1])
			return
		}

		var targetLayer *int
		if len(args) == 3 {
			layer, layerErr := strconv.Atoi(args[2])
			if layerErr != nil {
				h.sendSystemMessage(playerID, "invalid layer: "+args[2])
				return
			}
			targetLayer = &layer
		}

		// Any explicit /tp clears deferred states to avoid ambiguous click actions.
		ecs.GetResource[ecs.PendingAdminSpawn](w).Clear(playerID)
		ecs.GetResource[ecs.PendingAdminTeleport](w).Clear(playerID)

		if err := h.teleportExecutor.RequestAdminTeleport(playerID, w.Layer, x, y, targetLayer); err != nil {
			h.sendSystemMessage(playerID, "teleport failed: "+err.Error())
			return
		}

		if targetLayer != nil {
			h.sendSystemMessage(playerID, fmt.Sprintf("Teleport requested to (%d, %d) layer %d.", x, y, *targetLayer))
		} else {
			h.sendSystemMessage(playerID, fmt.Sprintf("Teleport requested to (%d, %d).", x, y))
		}
		return
	default:
		h.sendSystemMessage(playerID, "usage: /tp | /tp <x> <y> [layer]")
		return
	}
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
		EntityID:         newID,
		X:                targetX,
		Y:                targetY,
		Quality:          entry.Quality,
		Region:           region,
		Layer:            layer,
		InitReason:       contracts.ObjectBehaviorInitReasonSpawn,
		BehaviorRegistry: h.behaviorRegistry,
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

// ExecutePendingTeleport is called by NetworkCommandSystem when a player with a pending
// /tp click command clicks on map or entity.
func (h *ChatAdminCommandHandler) ExecutePendingTeleport(
	w *ecs.World,
	playerID types.EntityID,
	_ types.Handle,
	targetX, targetY float64,
) {
	pending := ecs.GetResource[ecs.PendingAdminTeleport](w)
	if !pending.Get(playerID) {
		return
	}
	pending.Clear(playerID)

	if h.teleportExecutor == nil {
		h.sendSystemMessage(playerID, "teleport service unavailable")
		return
	}

	targetXI := int(math.Round(targetX))
	targetYI := int(math.Round(targetY))

	if err := h.teleportExecutor.RequestAdminTeleport(playerID, w.Layer, targetXI, targetYI, nil); err != nil {
		h.sendSystemMessage(playerID, "teleport failed: "+err.Error())
		return
	}

	h.sendSystemMessage(playerID, fmt.Sprintf("Teleport requested to (%d, %d).", targetXI, targetYI))
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

// handleError processes: /error <text>
func (h *ChatAdminCommandHandler) handleError(playerID types.EntityID, args []string) {
	if len(args) == 0 {
		h.sendSystemMessage(playerID, "usage: /error <text>")
		return
	}
	if h.alertSender == nil {
		h.sendSystemMessage(playerID, "error sender unavailable")
		return
	}

	message := strings.Join(args, " ")
	h.alertSender.SendError(playerID, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, message)
	h.logger.Info("Admin /error executed",
		zap.Uint64("player_id", uint64(playerID)),
		zap.String("message", message))
}

// handleWarn processes: /warn <text>
func (h *ChatAdminCommandHandler) handleWarn(playerID types.EntityID, args []string) {
	if len(args) == 0 {
		h.sendSystemMessage(playerID, "usage: /warn <text>")
		return
	}
	if h.alertSender == nil {
		h.sendSystemMessage(playerID, "warning sender unavailable")
		return
	}

	message := strings.Join(args, " ")
	h.alertSender.SendWarning(playerID, netproto.WarningCode_WARN_INPUT_QUEUE_OVERFLOW, message)
	h.logger.Info("Admin /warn executed",
		zap.Uint64("player_id", uint64(playerID)),
		zap.String("message", message))
}

// handleStamina processes: /stamina <value>
func (h *ChatAdminCommandHandler) handleStamina(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	args []string,
) {
	if len(args) == 0 {
		h.sendSystemMessage(playerID, "usage: /stamina <value>")
		return
	}

	value, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		h.sendSystemMessage(playerID, "invalid stamina value: "+args[0])
		return
	}

	// Set stamina
	ecs.MutateComponent[components.EntityStats](w, playerHandle, func(stats *components.EntityStats) bool {
		stats.Stamina = value
		return true
	})

	// Mark for immediate client update
	ecs.MarkPlayerStatsDirty(w, playerID, 0)

	h.sendSystemMessage(playerID, fmt.Sprintf("stamina set to %.2f", value))
	h.logger.Info("Admin /stamina executed",
		zap.Uint64("player_id", uint64(playerID)),
		zap.Float64("stamina", value))
}

// handleEnergy processes: /energy <value>
func (h *ChatAdminCommandHandler) handleEnergy(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	args []string,
) {
	if len(args) == 0 {
		h.sendSystemMessage(playerID, "usage: /energy <value>")
		return
	}

	value, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		h.sendSystemMessage(playerID, "invalid energy value: "+args[0])
		return
	}

	// Set energy
	ecs.MutateComponent[components.EntityStats](w, playerHandle, func(stats *components.EntityStats) bool {
		stats.Energy = value
		return true
	})

	// Mark for immediate client update
	ecs.MarkPlayerStatsDirty(w, playerID, 0)

	h.sendSystemMessage(playerID, fmt.Sprintf("energy set to %.2f", value))
	h.logger.Info("Admin /energy executed",
		zap.Uint64("player_id", uint64(playerID)),
		zap.Float64("energy", value))
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

	if len(updated) == 0 {
		return
	}

	// Admin commands are server-initiated and not tied to a client op-id.
	// Send plain inventory update so client revisions are refreshed deterministically.
	h.inventoryResultSender.SendInventoryUpdate(playerID, updated)
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
	return systems.BuildInventoryStateProto(cs)
}
