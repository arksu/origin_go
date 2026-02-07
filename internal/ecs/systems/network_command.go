package systems

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	// NetworkCommandSystemPriority runs first to process incoming commands
	NetworkCommandSystemPriority = 0
)

// ChatDeliveryService NetworkCommandSystem processes player commands from the network layer
// This is the bridge between network I/O and ECS game state
// Commands are drained from the inbox at the start of each tick
// ChatDeliveryService provides methods to send chat messages to clients
type ChatDeliveryService interface {
	SendChatMessage(entityID types.EntityID, channel netproto.ChatChannel, fromEntityID types.EntityID, fromName, text string)
	BroadcastChatMessage(entityIDs []types.EntityID, channel netproto.ChatChannel, fromEntityID types.EntityID, fromName, text string)
}

// InventoryResultSender sends inventory operation results to clients
type InventoryResultSender interface {
	SendInventoryOpResult(entityID types.EntityID, result *netproto.S2C_InventoryOpResult)
	SendContainerOpened(entityID types.EntityID, state *netproto.InventoryState)
}

// InventoryOperationExecutor executes inventory operations
type InventoryOperationExecutor interface {
	ExecuteOperation(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, op *netproto.InventoryOp) InventoryOpResult
	ExecutePickupFromWorld(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, droppedEntityID types.EntityID, dstRef *netproto.InventoryRef) InventoryOpResult
}

// AdminCommandHandler processes admin chat commands (e.g. /give).
// Returns true if the text was recognized as an admin command.
type AdminCommandHandler interface {
	HandleCommand(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, text string) bool
}

// InventoryOpResult represents the result of an inventory operation
type InventoryOpResult struct {
	Success           bool
	ErrorCode         netproto.ErrorCode
	Message           string
	UpdatedContainers []InventoryContainerState
}

// InventoryContainerState represents the state of an inventory container for sending to client
type InventoryContainerState struct {
	OwnerID types.EntityID
	Kind    uint8
	Key     uint32
	Version uint64
	Width   uint8
	Height  uint8
	Items   []InventoryItemState

	// HandMouseOffsetX/Y — only used when Kind == HAND and len(Items) == 1.
	HandMouseOffsetX int16
	HandMouseOffsetY int16
}

// InventoryItemState represents an item in inventory for sending to client
type InventoryItemState struct {
	ItemID    types.EntityID
	TypeID    uint32
	Resource  string
	Quality   uint32
	Quantity  uint32
	W, H      uint8
	X, Y      uint8
	EquipSlot netproto.EquipSlot

	// NestedRef points to the nested inventory if this item is a container.
	// The actual state is sent separately on open/change, not inlined.
	NestedRef *netproto.InventoryRef
}

type NetworkCommandSystem struct {
	ecs.BaseSystem

	playerInbox       *network.PlayerCommandInbox
	serverInbox       *network.ServerJobInbox
	logger            *zap.Logger
	chatDelivery      ChatDeliveryService
	chatLocalRadiusSq float64

	// Inventory operation handling
	inventoryExecutor     InventoryOperationExecutor
	inventoryResultSender InventoryResultSender

	// Admin command handling
	adminHandler AdminCommandHandler

	// Vision system for forcing vision updates after inventory operations
	visionSystem *VisionSystem

	// Reusable buffers to avoid allocations
	playerCommands []*network.PlayerCommand
	serverJobs     []*network.ServerJob
}

// NewNetworkCommandSystem creates a new network command system
func NewNetworkCommandSystem(
	playerInbox *network.PlayerCommandInbox,
	serverInbox *network.ServerJobInbox,
	chatDelivery ChatDeliveryService,
	inventoryExecutor InventoryOperationExecutor,
	inventoryResultSender InventoryResultSender,
	visionSystem *VisionSystem,
	chatLocalRadius int,
	logger *zap.Logger,
) *NetworkCommandSystem {
	radiusSq := float64(chatLocalRadius * chatLocalRadius)
	return &NetworkCommandSystem{
		BaseSystem:            ecs.NewBaseSystem("NetworkCommandSystem", NetworkCommandSystemPriority),
		playerInbox:           playerInbox,
		serverInbox:           serverInbox,
		logger:                logger,
		chatDelivery:          chatDelivery,
		inventoryExecutor:     inventoryExecutor,
		inventoryResultSender: inventoryResultSender,
		visionSystem:          visionSystem,
		chatLocalRadiusSq:     radiusSq,
		playerCommands:        make([]*network.PlayerCommand, 0, 256),
		serverJobs:            make([]*network.ServerJob, 0, 64),
	}
}

// SetAdminHandler sets the admin command handler for processing chat commands like /give.
func (s *NetworkCommandSystem) SetAdminHandler(handler AdminCommandHandler) {
	s.adminHandler = handler
}

// Update drains command queues and processes commands
// Called at the start of each ECS tick under shard lock
func (s *NetworkCommandSystem) Update(w *ecs.World, dt float64) {
	// Drain player commands
	s.playerCommands = s.playerCommands[:0]
	if commands := s.playerInbox.Drain(); commands != nil {
		s.playerCommands = append(s.playerCommands, commands...)
	}

	// Drain server jobs
	s.serverJobs = s.serverJobs[:0]
	if jobs := s.serverInbox.Drain(); jobs != nil {
		s.serverJobs = append(s.serverJobs, jobs...)
	}

	// Process player commands
	for _, cmd := range s.playerCommands {
		s.processPlayerCommand(w, cmd)
	}

	// Process server jobs
	for _, job := range s.serverJobs {
		s.processServerJob(w, job)
	}
}

// processPlayerCommand routes a player command to the appropriate handler
// This is a skeleton - actual command handlers will be added as needed
func (s *NetworkCommandSystem) processPlayerCommand(w *ecs.World, cmd *network.PlayerCommand) {
	// Validate entity exists
	handle := w.GetHandleByEntityID(cmd.CharacterID)
	if handle == 0 || !w.Alive(handle) {
		s.logger.Debug("Command for non-existent entity",
			zap.Uint64("client_id", cmd.ClientID),
			zap.Uint64("character_id", uint64(cmd.CharacterID)))
		return
	}

	// Route to command handlers
	switch cmd.CommandType {
	case network.CmdMoveTo:
		s.handleMoveTo(w, handle, cmd)
	case network.CmdMoveToEntity:
		s.handleMoveToEntity(w, handle, cmd)
	case network.CmdInteract:
		s.handleInteract(w, handle, cmd)
	case network.CmdChat:
		s.handleChat(w, handle, cmd)
	case network.CmdInventoryOp:
		s.handleInventoryOp(w, handle, cmd)
	case network.CmdOpenContainer:
		s.handleOpenContainer(w, handle, cmd)
	case network.CmdCloseContainer:
		// CloseContainer is client-side only (UI close), no server action needed
	default:
		s.logger.Warn("Unknown command type",
			zap.Uint64("client_id", cmd.ClientID),
			zap.Uint16("command_type", uint16(cmd.CommandType)))
	}

	// Mark as processed for deduplication
	s.playerInbox.MarkProcessed(cmd.ClientID, cmd.CommandID)
}

func (s *NetworkCommandSystem) handleMoveTo(w *ecs.World, playerHandle types.Handle, cmd *network.PlayerCommand) {
	// Type assert payload
	moveTo, ok := cmd.Payload.(*netproto.MoveTo)
	if !ok {
		s.logger.Error("Invalid payload type for MoveTo",
			zap.Uint64("client_id", cmd.ClientID))
		return
	}

	// Get movement component
	mov, ok := ecs.GetComponent[components.Movement](w, playerHandle)
	if !ok {
		s.logger.Error("Movement component not found",
			zap.Uint64("client_id", cmd.ClientID),
			zap.Uint64("entity_id", uint64(cmd.CharacterID)))
		return
	}

	// Only allow movement if not stunned
	if mov.State == constt.StateStunned {
		s.logger.Debug("Cannot move while stunned",
			zap.Uint64("client_id", cmd.ClientID),
			zap.Uint64("entity_id", uint64(cmd.CharacterID)))
		return
	}

	// Clear pending interaction on any new movement command
	ecs.RemoveComponent[components.PendingInteraction](w, playerHandle)

	// Set movement target
	ecs.WithComponent(w, playerHandle, func(mov *components.Movement) {
		mov.SetTargetPoint(int(moveTo.X), int(moveTo.Y))
	})
}

func (s *NetworkCommandSystem) handleMoveToEntity(w *ecs.World, playerHandle types.Handle, cmd *network.PlayerCommand) {
	// Type assert payload
	moveToEntity, ok := cmd.Payload.(*netproto.MoveToEntity)
	if !ok {
		s.logger.Error("Invalid payload type for MoveToEntity",
			zap.Uint64("client_id", cmd.ClientID))
		return
	}

	// Clear any previous pending interaction
	ecs.RemoveComponent[components.PendingInteraction](w, playerHandle)

	// Validate target entity exists
	targetEntityID := types.EntityID(moveToEntity.EntityId)
	targetHandle := w.GetHandleByEntityID(targetEntityID)
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		s.logger.Debug("MoveToEntity: target entity not found",
			zap.Uint64("client_id", cmd.ClientID),
			zap.Uint64("target_entity_id", moveToEntity.EntityId))
		return
	}

	// Get movement component, check stunned
	mov, ok := ecs.GetComponent[components.Movement](w, playerHandle)
	if !ok {
		return
	}
	if mov.State == constt.StateStunned {
		return
	}

	// Get target position
	targetTransform, hasTransform := ecs.GetComponent[components.Transform](w, targetHandle)
	if !hasTransform {
		return
	}

	// Set movement target to entity handle (MovementSystem will track live position)
	ecs.WithComponent(w, playerHandle, func(m *components.Movement) {
		m.SetTargetHandle(targetHandle, int(targetTransform.X), int(targetTransform.Y))
	})

	// If autoInteract, determine interaction type and set PendingInteraction
	if moveToEntity.AutoInteract {
		_, isDroppedItem := ecs.GetComponent[components.DroppedItem](w, targetHandle)
		if isDroppedItem {
			ecs.AddComponent(w, playerHandle, components.PendingInteraction{
				TargetEntityID: targetEntityID,
				TargetHandle:   targetHandle,
				Type:           netproto.InteractionType_PICKUP,
				Range:          constt.DroppedPickupRadius,
			})
		}
	}

	s.logger.Debug("MoveToEntity action",
		zap.Uint64("client_id", cmd.ClientID),
		zap.Uint64("target_entity_id", moveToEntity.EntityId),
		zap.Bool("auto_interact", moveToEntity.AutoInteract))
}

func (s *NetworkCommandSystem) handleInteract(w *ecs.World, playerHandle types.Handle, cmd *network.PlayerCommand) {
	interact, ok := cmd.Payload.(*netproto.Interact)
	if !ok {
		s.logger.Error("Invalid payload type for Interact",
			zap.Uint64("client_id", cmd.ClientID))
		return
	}

	targetEntityID := types.EntityID(interact.EntityId)
	targetHandle := w.GetHandleByEntityID(targetEntityID)
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		s.logger.Debug("Interact: target entity not found",
			zap.Uint64("client_id", cmd.ClientID),
			zap.Uint64("target_entity_id", interact.EntityId))
		return
	}

	// Determine interaction type
	interactionType := interact.Type
	_, isDroppedItem := ecs.GetComponent[components.DroppedItem](w, targetHandle)

	// AUTO resolution: if target is a dropped item, treat as PICKUP
	if interactionType == netproto.InteractionType_AUTO && isDroppedItem {
		interactionType = netproto.InteractionType_PICKUP
	}

	switch interactionType {
	case netproto.InteractionType_PICKUP:
		if !isDroppedItem {
			s.logger.Debug("Interact: target is not a dropped item for PICKUP",
				zap.Uint64("client_id", cmd.ClientID))
			return
		}
		s.handlePickupInteract(w, playerHandle, cmd.CharacterID, targetEntityID, targetHandle)

	default:
		s.logger.Debug("Interact: unsupported interaction type",
			zap.Uint64("client_id", cmd.ClientID),
			zap.Int32("interaction_type", int32(interactionType)))
	}
}

// handlePickupInteract attempts immediate pickup if in range, otherwise sets movement + PendingInteraction.
func (s *NetworkCommandSystem) handlePickupInteract(
	w *ecs.World,
	playerHandle types.Handle,
	playerID types.EntityID,
	targetEntityID types.EntityID,
	targetHandle types.Handle,
) {
	playerTransform, hasPlayerT := ecs.GetComponent[components.Transform](w, playerHandle)
	targetTransform, hasTargetT := ecs.GetComponent[components.Transform](w, targetHandle)
	if !hasPlayerT || !hasTargetT {
		return
	}

	dx := playerTransform.X - targetTransform.X
	dy := playerTransform.Y - targetTransform.Y
	distSq := dx*dx + dy*dy

	if distSq <= constt.DroppedPickupRadiusSq {
		// In range — immediate pickup (delegate to AutoInteractSystem-style logic)
		// Set PendingInteraction so AutoInteractSystem picks it up this tick
		ecs.RemoveComponent[components.PendingInteraction](w, playerHandle)
		ecs.AddComponent(w, playerHandle, components.PendingInteraction{
			TargetEntityID: targetEntityID,
			TargetHandle:   targetHandle,
			Type:           netproto.InteractionType_PICKUP,
			Range:          constt.DroppedPickupRadius,
		})
		return
	}

	// Out of range — move toward target and set pending interaction
	mov, hasMov := ecs.GetComponent[components.Movement](w, playerHandle)
	if !hasMov || mov.State == constt.StateStunned {
		return
	}

	ecs.RemoveComponent[components.PendingInteraction](w, playerHandle)

	ecs.WithComponent(w, playerHandle, func(m *components.Movement) {
		m.SetTargetHandle(targetHandle, int(targetTransform.X), int(targetTransform.Y))
	})

	ecs.AddComponent(w, playerHandle, components.PendingInteraction{
		TargetEntityID: targetEntityID,
		TargetHandle:   targetHandle,
		Type:           netproto.InteractionType_PICKUP,
		Range:          constt.DroppedPickupRadius,
	})
}

func (s *NetworkCommandSystem) handleChat(w *ecs.World, playerHandle types.Handle, cmd *network.PlayerCommand) {
	payload, ok := cmd.Payload.(*network.ChatCommandPayload)
	if !ok {
		s.logger.Warn("Invalid chat command payload",
			zap.Uint64("client_id", cmd.ClientID))
		return
	}

	// Intercept admin commands (starting with "/")
	if len(payload.Text) > 0 && payload.Text[0] == '/' {
		if s.adminHandler != nil && s.adminHandler.HandleCommand(w, cmd.CharacterID, playerHandle, payload.Text) {
			return
		}
	}

	senderTransform, hasTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	if !hasTransform {
		s.logger.Debug("Chat sender has no Transform component",
			zap.Int64("character_id", int64(cmd.CharacterID)))
		return
	}

	senderAppearance, hasAppearance := ecs.GetComponent[components.Appearance](w, playerHandle)
	if !hasAppearance {
		s.logger.Debug("Chat sender has no Appearance component",
			zap.Int64("character_id", int64(cmd.CharacterID)))
		return
	}

	senderName := "Unknown"
	if senderAppearance.Name != nil && *senderAppearance.Name != "" {
		senderName = *senderAppearance.Name
	}

	recipients := s.findChatRecipients(w, senderTransform.X, senderTransform.Y, cmd.CharacterID)

	if len(recipients) == 0 {
		s.logger.Debug("No recipients found for chat message",
			zap.Int64("sender_id", int64(cmd.CharacterID)),
			zap.Int("layer", cmd.Layer))
		return
	}

	s.chatDelivery.BroadcastChatMessage(
		recipients,
		netproto.ChatChannel_CHAT_CHANNEL_LOCAL,
		cmd.CharacterID,
		senderName,
		payload.Text,
	)

	s.logger.Debug("Chat message delivered",
		zap.Int64("sender_id", int64(cmd.CharacterID)),
		zap.Int("recipients", len(recipients)),
		zap.Int("text_len", len(payload.Text)))
}

func (s *NetworkCommandSystem) findChatRecipients(w *ecs.World, senderX, senderY float64, senderID types.EntityID) []types.EntityID {
	recipients := make([]types.EntityID, 0, 32)

	characterEntities := ecs.GetResource[ecs.CharacterEntities](w)

	for entityID := range characterEntities.Map {
		handle := w.GetHandleByEntityID(entityID)
		if handle == types.InvalidHandle || !w.Alive(handle) {
			continue
		}

		transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
		if !hasTransform {
			continue
		}

		dx := transform.X - senderX
		dy := transform.Y - senderY
		distSq := dx*dx + dy*dy

		if distSq <= s.chatLocalRadiusSq {
			recipients = append(recipients, entityID)
		}
	}

	return recipients
}

func (s *NetworkCommandSystem) handleInventoryOp(w *ecs.World, playerHandle types.Handle, cmd *network.PlayerCommand) {
	op, ok := cmd.Payload.(*netproto.InventoryOp)
	if !ok {
		s.logger.Error("Invalid payload type for InventoryOp",
			zap.Uint64("client_id", cmd.ClientID))
		return
	}

	s.logger.Debug("InventoryOp received",
		zap.Uint64("client_id", cmd.ClientID),
		zap.Uint64("op_id", op.OpId))

	// Check if inventory executor is configured
	if s.inventoryExecutor == nil {
		s.logger.Warn("Inventory executor not configured")
		s.sendInventoryError(cmd.CharacterID, op.OpId, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Inventory system not available")
		return
	}

	// Execute the operation
	result := s.inventoryExecutor.ExecuteOperation(w, cmd.CharacterID, playerHandle, op)

	// Build and send response
	response := &netproto.S2C_InventoryOpResult{
		OpId:    op.OpId,
		Success: result.Success,
		Error:   result.ErrorCode,
		Message: result.Message,
		Updated: make([]*netproto.InventoryState, 0, len(result.UpdatedContainers)),
	}

	// Convert updated containers to proto format
	for _, container := range result.UpdatedContainers {
		invState := s.buildInventoryStateProto(container)
		response.Updated = append(response.Updated, invState)
	}

	// Send result to client
	if s.inventoryResultSender != nil {
		s.inventoryResultSender.SendInventoryOpResult(cmd.CharacterID, response)
	}

	s.logger.Debug("InventoryOp completed",
		zap.Uint64("client_id", cmd.ClientID),
		zap.Uint64("op_id", op.OpId),
		zap.Bool("success", result.Success))

	// Force immediate vision update for successful drop/pickup operations to eliminate client delay
	if result.Success && s.visionSystem != nil {
		// Check if this was a drop or pickup operation
		if op.GetDropToWorld() != nil || (len(result.UpdatedContainers) > 0 &&
			// Check if any updated container is a dropped item (pickup operation)
			func() bool {
				for _, container := range result.UpdatedContainers {
					if constt.InventoryKind(container.Kind) == constt.InventoryDroppedItem {
						return true
					}
				}
				return false
			}()) {
			s.visionSystem.ForceUpdateForObserver(w, playerHandle)
		}
	}
}

func (s *NetworkCommandSystem) handleOpenContainer(w *ecs.World, playerHandle types.Handle, cmd *network.PlayerCommand) {
	ref, ok := cmd.Payload.(*netproto.InventoryRef)
	if !ok || ref == nil {
		s.logger.Error("Invalid payload type for OpenContainer",
			zap.Uint64("client_id", cmd.ClientID))
		return
	}

	// O(1) lookup via InventoryRefIndex
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	handle, found := refIndex.Lookup(constt.InventoryKind(ref.Kind), types.EntityID(ref.OwnerId), ref.InventoryKey)
	if !found || !w.Alive(handle) {
		s.logger.Debug("OpenContainer: container not found",
			zap.Uint64("client_id", cmd.ClientID),
			zap.Uint64("owner_id", ref.OwnerId))
		return
	}

	container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, handle)
	if !hasContainer {
		return
	}

	ownerID := types.EntityID(ref.OwnerId)

	// Authorization: if owner_id != player, check that item belongs to player's inventory
	if ownerID != cmd.CharacterID {
		owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
		if !hasOwner {
			return
		}
		allowed := false
		for _, link := range owner.Inventories {
			if link.OwnerID == ownerID {
				allowed = true
				break
			}
		}
		if !allowed {
			s.logger.Debug("OpenContainer: access denied",
				zap.Uint64("client_id", cmd.ClientID),
				zap.Uint64("owner_id", ref.OwnerId))
			return
		}
	}

	// Build and send S2C_ContainerOpened
	containerState := InventoryContainerState{
		OwnerID:          container.OwnerID,
		Kind:             uint8(container.Kind),
		Key:              container.Key,
		Version:          container.Version,
		Width:            container.Width,
		Height:           container.Height,
		Items:            make([]InventoryItemState, 0, len(container.Items)),
		HandMouseOffsetX: container.HandMouseOffsetX,
		HandMouseOffsetY: container.HandMouseOffsetY,
	}
	for _, item := range container.Items {
		itemState := InventoryItemState{
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
		// Check nested ref for items inside this container
		if _, nestedFound := refIndex.Lookup(constt.InventoryGrid, item.ItemID, 0); nestedFound {
			itemState.NestedRef = &netproto.InventoryRef{
				Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
				OwnerId:      uint64(item.ItemID),
				InventoryKey: 0,
			}
		}
		containerState.Items = append(containerState.Items, itemState)
	}

	invState := s.buildInventoryStateProto(containerState)

	if s.inventoryResultSender != nil {
		s.inventoryResultSender.SendContainerOpened(cmd.CharacterID, invState)
	}
}

func (s *NetworkCommandSystem) sendInventoryError(entityID types.EntityID, opID uint64, code netproto.ErrorCode, message string) {
	if s.inventoryResultSender == nil {
		return
	}
	response := &netproto.S2C_InventoryOpResult{
		OpId:    opID,
		Success: false,
		Error:   code,
		Message: message,
	}
	s.inventoryResultSender.SendInventoryOpResult(entityID, response)
}

func (s *NetworkCommandSystem) buildInventoryStateProto(container InventoryContainerState) *netproto.InventoryState {
	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind(container.Kind),
		OwnerId:      uint64(container.OwnerID),
		InventoryKey: container.Key,
	}

	invState := &netproto.InventoryState{
		Ref:      ref,
		Revision: container.Version,
	}

	// Build items based on container kind
	switch constt.InventoryKind(container.Kind) {
	case constt.InventoryGrid:
		gridItems := make([]*netproto.GridItem, 0, len(container.Items))
		for _, item := range container.Items {
			gridItems = append(gridItems, &netproto.GridItem{
				X:    uint32(item.X),
				Y:    uint32(item.Y),
				Item: s.buildItemInstanceProto(item),
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
			handState.Item = s.buildItemInstanceProto(container.Items[0])
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
				Item: s.buildItemInstanceProto(item),
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

// buildItemInstanceProto creates a proto ItemInstance from InventoryItemState
func (s *NetworkCommandSystem) buildItemInstanceProto(item InventoryItemState) *netproto.ItemInstance {
	instance := &netproto.ItemInstance{
		ItemId:   uint64(item.ItemID),
		TypeId:   item.TypeID,
		Resource: item.Resource,
		Quality:  item.Quality,
		Quantity: item.Quantity,
		W:        uint32(item.W),
		H:        uint32(item.H),
	}

	// Set nested ref if this item is a container (state sent separately)
	if item.NestedRef != nil {
		instance.NestedRef = item.NestedRef
	}

	return instance
}

// processServerJob routes a server job to the appropriate handler
// This is a skeleton - actual job handlers will be added as needed
func (s *NetworkCommandSystem) processServerJob(w *ecs.World, job *network.ServerJob) {
	// Route job by type
	// TODO: Implement job routing to specific handlers
	// case JobMachineOfflineTick:
	//     s.handleMachineOfflineTick(w, job)
	// case JobAutoDropOverflow:
	//     s.handleAutoDropOverflow(w, job)
	// ...
	// }
	_ = w
	_ = job
}

// Stats returns processing statistics
func (s *NetworkCommandSystem) Stats() (playerReceived, playerDropped, playerProcessed, serverReceived, serverDropped, serverProcessed uint64) {
	pr, pd, pp := s.playerInbox.Stats()
	sr, sd, sp := s.serverInbox.Stats()
	return pr, pd, pp, sr, sd, sp
}
