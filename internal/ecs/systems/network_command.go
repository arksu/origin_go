package systems

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"
	"time"

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
	SendContainerClosed(entityID types.EntityID, ref *netproto.InventoryRef)
	SendInventoryUpdate(entityID types.EntityID, states []*netproto.InventoryState)
}

// InventorySnapshotSender sends full inventory snapshots to a client (used on login/reattach)
type InventorySnapshotSender interface {
	SendInventorySnapshots(w *ecs.World, entityID types.EntityID, handle types.Handle)
	SendCharacterProfileSnapshot(w *ecs.World, entityID types.EntityID, handle types.Handle)
}

// InventoryOperationExecutor executes inventory operations
type InventoryOperationExecutor interface {
	ExecuteOperation(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, op *netproto.InventoryOp) InventoryOpResult
	ExecutePickupFromWorld(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, droppedEntityID types.EntityID, dstRef *netproto.InventoryRef) InventoryOpResult
}

// AdminCommandHandler processes admin chat commands (e.g. /give, /spawn).
// Returns true if the text was recognized as an admin command.
type AdminCommandHandler interface {
	HandleCommand(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, text string) bool
	ExecutePendingSpawn(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, targetX, targetY float64)
}

// InventoryOpResult represents the result of an inventory operation
type InventoryOpResult struct {
	Success           bool
	ErrorCode         netproto.ErrorCode
	Message           string
	UpdatedContainers []InventoryContainerState

	// ClosedContainerRefs lists nested container refs that should be closed on the client
	// (e.g. when a container item is picked up into the hand).
	ClosedContainerRefs []*netproto.InventoryRef
}

// InventoryContainerState represents the state of an inventory container for sending to client
type InventoryContainerState struct {
	OwnerID types.EntityID
	Kind    uint8
	Key     uint32
	Version uint64
	Title   string
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

type OpenContainerError struct {
	Code    netproto.ErrorCode
	Message string
}

// OpenContainerCoordinator handles world-object container open/close mechanics.
type OpenContainerCoordinator interface {
	HandleOpenRequest(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, ref *netproto.InventoryRef) *OpenContainerError
	HandleCloseRequest(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, ref *netproto.InventoryRef) *OpenContainerError
	BroadcastInventoryUpdates(w *ecs.World, actorID types.EntityID, updated []*netproto.InventoryState)
	CloseRefsForOpenedPlayers(w *ecs.World, refs []*netproto.InventoryRef)
}

type ContextMenuSender interface {
	SendContextMenu(entityID types.EntityID, menu *netproto.S2C_ContextMenu)
}

type NetworkCommandSystem struct {
	ecs.BaseSystem

	playerInbox       *network.PlayerCommandInbox
	serverInbox       *network.ServerJobInbox
	logger            *zap.Logger
	chatDelivery      ChatDeliveryService
	chatLocalRadiusSq float64

	// Inventory operation handling
	inventoryExecutor       InventoryOperationExecutor
	inventoryResultSender   InventoryResultSender
	inventorySnapshotSender InventorySnapshotSender

	// Admin command handling
	adminHandler AdminCommandHandler

	// Vision system for forcing vision updates after inventory operations
	visionSystem *VisionSystem

	openContainerService OpenContainerCoordinator
	contextActionService ContextActionResolver
	contextMenuSender    ContextMenuSender
	contextPendingTTL    time.Duration

	// Reusable buffers to avoid allocations
	playerCommands       []*network.PlayerCommand
	serverJobs           []*network.ServerJob
	pendingContextQuery  *ecs.PreparedQuery
	pendingContextRemove []types.Handle
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
		contextPendingTTL:     15 * time.Second,
		playerCommands:        make([]*network.PlayerCommand, 0, 256),
		serverJobs:            make([]*network.ServerJob, 0, 64),
		pendingContextRemove:  make([]types.Handle, 0, 64),
	}
}

// SetAdminHandler sets the admin command handler for processing chat commands like /give.
func (s *NetworkCommandSystem) SetAdminHandler(handler AdminCommandHandler) {
	s.adminHandler = handler
}

// SetInventorySnapshotSender sets the handler for sending full inventory snapshots on login/reattach.
func (s *NetworkCommandSystem) SetInventorySnapshotSender(sender InventorySnapshotSender) {
	s.inventorySnapshotSender = sender
}

func (s *NetworkCommandSystem) SetOpenContainerService(service OpenContainerCoordinator) {
	s.openContainerService = service
}

func (s *NetworkCommandSystem) SetContextActionService(service ContextActionResolver) {
	s.contextActionService = service
}

func (s *NetworkCommandSystem) SetContextMenuSender(sender ContextMenuSender) {
	s.contextMenuSender = sender
}

func (s *NetworkCommandSystem) SetContextPendingTTL(ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	s.contextPendingTTL = ttl
}

// Update drains command queues and processes commands
// Called at the start of each ECS tick under shard lock
func (s *NetworkCommandSystem) Update(w *ecs.World, dt float64) {
	s.cleanupPendingContextActions(w)

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
	case network.CmdSelectContextAction:
		s.handleSelectContextAction(w, handle, cmd)
	case network.CmdChat:
		s.handleChat(w, handle, cmd)
	case network.CmdInventoryOp:
		s.handleInventoryOp(w, handle, cmd)
	case network.CmdOpenContainer:
		s.handleOpenContainer(w, handle, cmd)
	case network.CmdCloseContainer:
		s.handleCloseContainer(w, handle, cmd)
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

	// Check for pending admin spawn — intercept click as spawn target
	if s.adminHandler != nil {
		pending := ecs.GetResource[ecs.PendingAdminSpawn](w)
		if _, hasPending := pending.Get(cmd.CharacterID); hasPending {
			s.adminHandler.ExecutePendingSpawn(w, cmd.CharacterID, playerHandle, float64(moveTo.X), float64(moveTo.Y))
			return
		}
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
	s.clearPendingInteractionIntents(w, playerHandle, cmd.CharacterID)

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

	// Check for pending admin spawn — use target entity's position as spawn target
	if s.adminHandler != nil {
		pending := ecs.GetResource[ecs.PendingAdminSpawn](w)
		if _, hasPending := pending.Get(cmd.CharacterID); hasPending {
			targetEntityID := types.EntityID(moveToEntity.EntityId)
			targetHandle := w.GetHandleByEntityID(targetEntityID)
			if targetHandle != types.InvalidHandle && w.Alive(targetHandle) {
				if t, hasT := ecs.GetComponent[components.Transform](w, targetHandle); hasT {
					s.adminHandler.ExecutePendingSpawn(w, cmd.CharacterID, playerHandle, t.X, t.Y)
					return
				}
			}
			// Target entity invalid — clear pending and let normal flow continue
			pending.Clear(cmd.CharacterID)
		}
	}

	// Clear any previous pending interaction
	s.clearPendingInteractionIntents(w, playerHandle, cmd.CharacterID)

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

	_, isDroppedItem := ecs.GetComponent[components.DroppedItem](w, targetHandle)
	isCollidingWithTarget := lastCollidedEntityForHandle(w, playerHandle) == targetEntityID
	isLinkedToTarget := false
	if link, hasLink := ecs.GetResource[ecs.LinkState](w).GetLink(cmd.CharacterID); hasLink && link.TargetID == targetEntityID {
		isLinkedToTarget = true
	}

	// If the player is already in contact (or already linked) with the same world object,
	// re-click should not restart path-following. Keep interaction intent only.
	if moveToEntity.AutoInteract && !isDroppedItem && (isCollidingWithTarget || isLinkedToTarget) {
		s.stopMovementAndEmit(w, playerHandle)
		s.setLinkIntent(w, cmd.CharacterID, targetEntityID, targetHandle)
		return
	}

	// Set movement target to entity handle (MovementSystem will track live position)
	ecs.WithComponent(w, playerHandle, func(m *components.Movement) {
		m.SetTargetHandle(targetHandle, int(targetTransform.X), int(targetTransform.Y))
	})

	// If autoInteract, determine interaction type and set PendingInteraction
	if moveToEntity.AutoInteract {
		if isDroppedItem {
			ecs.AddComponent(w, playerHandle, components.PendingInteraction{
				TargetEntityID: targetEntityID,
				TargetHandle:   targetHandle,
				Type:           netproto.InteractionType_PICKUP,
				Range:          constt.DroppedPickupRadius,
			})
		} else {
			// For non-dropped world objects autoInteract means explicit link intent.
			s.setLinkIntent(w, cmd.CharacterID, targetEntityID, targetHandle)
		}
	}

	s.logger.Debug("MoveToEntity action",
		zap.Uint64("client_id", cmd.ClientID),
		zap.Uint64("target_entity_id", moveToEntity.EntityId),
		zap.Bool("auto_interact", moveToEntity.AutoInteract))
}

func (s *NetworkCommandSystem) stopMovementAndEmit(w *ecs.World, playerHandle types.Handle) {
	movement, hasMovement := ecs.GetComponent[components.Movement](w, playerHandle)
	if !hasMovement || movement.State != constt.StateMoving {
		return
	}

	transform, hasTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	if !hasTransform {
		return
	}

	ecs.WithComponent(w, playerHandle, func(m *components.Movement) {
		m.ClearTarget()
	})

	// Force a movement batch entry with IsMoving=false in this tick.
	ecs.GetResource[ecs.MovedEntities](w).Add(playerHandle, transform.X, transform.Y)
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

	_, isDroppedItem := ecs.GetComponent[components.DroppedItem](w, targetHandle)
	if isDroppedItem {
		s.handlePickupInteract(w, playerHandle, cmd.CharacterID, targetEntityID, targetHandle)
		return
	}

	if _, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); !hasCollider {
		return
	}
	if s.contextActionService == nil {
		return
	}

	actions := s.computeContextActions(w, cmd.CharacterID, playerHandle, targetEntityID, targetHandle)
	switch len(actions) {
	case 0:
		// Product rule: no actions => full ignore.
		return
	case 1:
		if s.shouldOpenContextMenuForSingleAction(w, targetHandle) {
			s.sendContextMenu(cmd.CharacterID, targetEntityID, actions)
			return
		}
		// Fast path: exactly one action is auto-selected.
		s.startPendingContextAction(w, playerHandle, cmd.CharacterID, targetEntityID, targetHandle, actions[0].ActionID)
	default:
		s.sendContextMenu(cmd.CharacterID, targetEntityID, actions)
	}
}

func (s *NetworkCommandSystem) handleSelectContextAction(
	w *ecs.World,
	playerHandle types.Handle,
	cmd *network.PlayerCommand,
) {
	selectAction, ok := cmd.Payload.(*netproto.SelectContextAction)
	if !ok {
		s.logger.Error("Invalid payload type for SelectContextAction",
			zap.Uint64("client_id", cmd.ClientID))
		return
	}
	if selectAction.ActionId == "" {
		return
	}
	if s.contextActionService == nil {
		return
	}

	targetEntityID := types.EntityID(selectAction.EntityId)
	targetHandle := w.GetHandleByEntityID(targetEntityID)
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return
	}
	if _, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); !hasCollider {
		return
	}

	actions := s.computeContextActions(w, cmd.CharacterID, playerHandle, targetEntityID, targetHandle)
	if !containsContextAction(actions, selectAction.ActionId) {
		return
	}

	s.startPendingContextAction(
		w,
		playerHandle,
		cmd.CharacterID,
		targetEntityID,
		targetHandle,
		selectAction.ActionId,
	)
}

func containsContextAction(actions []ContextAction, actionID string) bool {
	for _, action := range actions {
		if action.ActionID == actionID {
			return true
		}
	}
	return false
}

func (s *NetworkCommandSystem) startPendingContextAction(
	w *ecs.World,
	playerHandle types.Handle,
	playerID types.EntityID,
	targetEntityID types.EntityID,
	targetHandle types.Handle,
	actionID string,
) {
	if actionID == "" {
		return
	}

	if link, hasLink := ecs.GetResource[ecs.LinkState](w).GetLink(playerID); hasLink && link.TargetID == targetEntityID {
		// Already linked: execute immediately without creating pending state.
		s.contextActionService.ExecuteAction(w, playerID, playerHandle, targetEntityID, targetHandle, actionID)
		return
	}

	targetTransform, hasTransform := ecs.GetComponent[components.Transform](w, targetHandle)
	if !hasTransform {
		return
	}

	mov, hasMov := ecs.GetComponent[components.Movement](w, playerHandle)
	if !hasMov || mov.State == constt.StateStunned {
		return
	}

	s.clearPendingInteractionIntents(w, playerHandle, playerID)
	s.setLinkIntent(w, playerID, targetEntityID, targetHandle)

	if lastCollidedEntityForHandle(w, playerHandle) == targetEntityID {
		s.stopMovementAndEmit(w, playerHandle)
	} else {
		ecs.WithComponent(w, playerHandle, func(m *components.Movement) {
			m.SetTargetHandle(targetHandle, int(targetTransform.X), int(targetTransform.Y))
		})
	}

	ttlMs := s.contextPendingTTL.Milliseconds()
	if ttlMs <= 0 {
		ttlMs = 15000
	}
	// Pending action exists only until link is created (or timeout/cancel path removes it).
	expireAt := ecs.GetResource[ecs.TimeState](w).UnixMs + ttlMs
	ecs.AddComponent(w, playerHandle, components.PendingContextAction{
		TargetEntityID: targetEntityID,
		TargetHandle:   targetHandle,
		ActionID:       actionID,
		ExpireAtUnixMs: expireAt,
	})
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
		ecs.RemoveComponent[components.PendingContextAction](w, playerHandle)
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
	ecs.RemoveComponent[components.PendingContextAction](w, playerHandle)

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

	if len(result.ClosedContainerRefs) > 0 {
		if s.inventoryResultSender != nil {
			// Always close on actor client as a hard guarantee, even if this ref was
			// opened through a path not tracked in OpenContainerState.
			for _, ref := range result.ClosedContainerRefs {
				s.inventoryResultSender.SendContainerClosed(cmd.CharacterID, ref)
			}
		}

		if s.openContainerService != nil {
			s.openContainerService.CloseRefsForOpenedPlayers(w, result.ClosedContainerRefs)
		}
	}

	if result.Success && s.openContainerService != nil && len(response.Updated) > 0 {
		s.openContainerService.BroadcastInventoryUpdates(w, cmd.CharacterID, response.Updated)
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

	if s.openContainerService == nil {
		s.sendInventoryError(cmd.CharacterID, 0, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Open container service not available")
		return
	}
	if openErr := s.openContainerService.HandleOpenRequest(w, cmd.CharacterID, playerHandle, ref); openErr != nil {
		s.sendInventoryError(cmd.CharacterID, 0, openErr.Code, openErr.Message)
	}
}

func (s *NetworkCommandSystem) handleCloseContainer(w *ecs.World, playerHandle types.Handle, cmd *network.PlayerCommand) {
	ref, ok := cmd.Payload.(*netproto.InventoryRef)
	if !ok || ref == nil {
		s.logger.Error("Invalid payload type for CloseContainer",
			zap.Uint64("client_id", cmd.ClientID))
		return
	}

	if s.openContainerService == nil {
		s.sendInventoryError(cmd.CharacterID, 0, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Open container service not available")
		return
	}
	if openErr := s.openContainerService.HandleCloseRequest(w, cmd.CharacterID, playerHandle, ref); openErr != nil {
		s.sendInventoryError(cmd.CharacterID, 0, openErr.Code, openErr.Message)
	}
}

func (s *NetworkCommandSystem) setLinkIntent(
	w *ecs.World,
	playerID types.EntityID,
	targetID types.EntityID,
	targetHandle types.Handle,
) {
	if playerID == 0 || targetID == 0 {
		return
	}

	linkState := ecs.GetResource[ecs.LinkState](w)
	createdAt := ecs.GetResource[ecs.TimeState](w).Now
	linkState.SetIntent(playerID, targetID, targetHandle, createdAt)
}

func (s *NetworkCommandSystem) clearLinkIntent(w *ecs.World, playerID types.EntityID) {
	if playerID == 0 {
		return
	}
	ecs.GetResource[ecs.LinkState](w).ClearIntent(playerID)
}

func (s *NetworkCommandSystem) clearPendingInteractionIntents(
	w *ecs.World,
	playerHandle types.Handle,
	playerID types.EntityID,
) {
	ecs.RemoveComponent[components.PendingInteraction](w, playerHandle)
	ecs.RemoveComponent[components.PendingContextAction](w, playerHandle)
	s.clearLinkIntent(w, playerID)
}

func (s *NetworkCommandSystem) computeContextActions(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetEntityID types.EntityID,
	targetHandle types.Handle,
) []ContextAction {
	if s.contextActionService == nil {
		return nil
	}
	return s.contextActionService.ComputeActions(w, playerID, playerHandle, targetEntityID, targetHandle)
}

func (s *NetworkCommandSystem) shouldOpenContextMenuForSingleAction(
	w *ecs.World,
	targetHandle types.Handle,
) bool {
	entityInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo {
		return true
	}

	registry := objectdefs.Global()
	if registry == nil {
		return true
	}

	def, found := registry.GetByID(int(entityInfo.TypeID))
	if !found {
		return true
	}
	return def.ContextMenuEvenForOneItemValue
}

func (s *NetworkCommandSystem) sendContextMenu(
	playerID types.EntityID,
	targetEntityID types.EntityID,
	actions []ContextAction,
) {
	if s.contextMenuSender == nil || len(actions) == 0 {
		return
	}

	menu := &netproto.S2C_ContextMenu{
		EntityId: uint64(targetEntityID),
		Actions:  make([]*netproto.ContextMenuAction, 0, len(actions)),
	}
	for _, action := range actions {
		menu.Actions = append(menu.Actions, &netproto.ContextMenuAction{
			ActionId: action.ActionID,
			Title:    action.Title,
		})
	}
	s.contextMenuSender.SendContextMenu(playerID, menu)
}

func (s *NetworkCommandSystem) cleanupPendingContextActions(w *ecs.World) {
	if s.pendingContextQuery == nil {
		s.pendingContextQuery = ecs.NewPreparedQuery(
			w,
			(1 << components.PendingContextActionComponentID),
			0,
		)
	}

	nowUnixMs := ecs.GetResource[ecs.TimeState](w).UnixMs
	linkState := ecs.GetResource[ecs.LinkState](w)
	s.pendingContextRemove = s.pendingContextRemove[:0]

	s.pendingContextQuery.ForEach(func(h types.Handle) {
		pending, ok := ecs.GetComponent[components.PendingContextAction](w, h)
		if !ok {
			return
		}
		playerID, hasExternalID := w.GetExternalID(h)
		if !hasExternalID {
			s.pendingContextRemove = append(s.pendingContextRemove, h)
			return
		}

		// Timeout path: pending action exceeded configured TTL.
		if pending.ExpireAtUnixMs > 0 && nowUnixMs >= pending.ExpireAtUnixMs {
			s.pendingContextRemove = append(s.pendingContextRemove, h)
			return
		}

		if pending.TargetHandle == types.InvalidHandle || !w.Alive(pending.TargetHandle) {
			s.pendingContextRemove = append(s.pendingContextRemove, h)
			return
		}

		targetHandle := w.GetHandleByEntityID(pending.TargetEntityID)
		if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
			s.pendingContextRemove = append(s.pendingContextRemove, h)
			return
		}

		// If linked to target, execution is handled by LinkCreated subscriber.
		if link, hasLink := linkState.GetLink(playerID); hasLink && link.TargetID == pending.TargetEntityID {
			return
		}

		mov, hasMov := ecs.GetComponent[components.Movement](w, h)
		if !hasMov || mov.State != constt.StateMoving {
			// Cancel path: stopped movement without reaching link state.
			s.pendingContextRemove = append(s.pendingContextRemove, h)
		}
	})

	for _, handle := range s.pendingContextRemove {
		ecs.RemoveComponent[components.PendingContextAction](w, handle)
	}
}

func lastCollidedEntityForHandle(w *ecs.World, playerHandle types.Handle) types.EntityID {
	cr, ok := ecs.GetComponent[components.CollisionResult](w, playerHandle)
	if !ok {
		return 0
	}
	if cr.PrevCollidedWith != 0 {
		return cr.PrevCollidedWith
	}
	if cr.HasCollision && cr.CollidedWith != 0 {
		return cr.CollidedWith
	}
	return 0
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
	return BuildInventoryStateProto(container)
}

// processServerJob routes a server job to the appropriate handler
func (s *NetworkCommandSystem) processServerJob(w *ecs.World, job *network.ServerJob) {
	switch job.JobType {
	case network.JobSendInventorySnapshot:
		s.handleInventorySnapshotJob(w, job)
	case network.JobSendCharacterProfileSnapshot:
		s.handleCharacterProfileSnapshotJob(w, job)
	default:
		s.logger.Warn("Unknown server job type", zap.Uint16("job_type", job.JobType))
	}
}

func (s *NetworkCommandSystem) handleInventorySnapshotJob(w *ecs.World, job *network.ServerJob) {
	payload, ok := job.Payload.(*network.InventorySnapshotJobPayload)
	if !ok {
		s.logger.Error("Invalid payload for inventory snapshot job")
		return
	}

	if !w.Alive(payload.Handle) {
		s.logger.Debug("Inventory snapshot job: entity no longer alive",
			zap.Uint64("entity_id", uint64(job.TargetID)))
		return
	}

	if s.inventorySnapshotSender != nil {
		s.inventorySnapshotSender.SendInventorySnapshots(w, job.TargetID, payload.Handle)
	}
}

func (s *NetworkCommandSystem) handleCharacterProfileSnapshotJob(w *ecs.World, job *network.ServerJob) {
	payload, ok := job.Payload.(*network.CharacterProfileSnapshotJobPayload)
	if !ok {
		s.logger.Error("Invalid payload for character profile snapshot job")
		return
	}

	if !w.Alive(payload.Handle) {
		s.logger.Debug("Character profile snapshot job: entity no longer alive",
			zap.Uint64("entity_id", uint64(job.TargetID)))
		return
	}

	if s.inventorySnapshotSender != nil {
		s.inventorySnapshotSender.SendCharacterProfileSnapshot(w, job.TargetID, payload.Handle)
	}
}

// Stats returns processing statistics
func (s *NetworkCommandSystem) Stats() (playerReceived, playerDropped, playerProcessed, serverReceived, serverDropped, serverProcessed uint64) {
	pr, pd, pp := s.playerInbox.Stats()
	sr, sd, sp := s.serverInbox.Stats()
	return pr, pd, pp, sr, sd, sp
}
