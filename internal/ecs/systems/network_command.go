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

type NetworkCommandSystem struct {
	ecs.BaseSystem

	playerInbox       *network.PlayerCommandInbox
	serverInbox       *network.ServerJobInbox
	logger            *zap.Logger
	chatDelivery      ChatDeliveryService
	chatLocalRadiusSq float64

	// Reusable buffers to avoid allocations
	playerCommands []*network.PlayerCommand
	serverJobs     []*network.ServerJob
}

// NewNetworkCommandSystem creates a new network command system
func NewNetworkCommandSystem(
	playerInbox *network.PlayerCommandInbox,
	serverInbox *network.ServerJobInbox,
	chatDelivery ChatDeliveryService,
	chatLocalRadius int,
	logger *zap.Logger,
) *NetworkCommandSystem {
	radiusSq := float64(chatLocalRadius * chatLocalRadius)
	return &NetworkCommandSystem{
		BaseSystem:        ecs.NewBaseSystem("NetworkCommandSystem", NetworkCommandSystemPriority),
		playerInbox:       playerInbox,
		serverInbox:       serverInbox,
		logger:            logger,
		chatDelivery:      chatDelivery,
		chatLocalRadiusSq: radiusSq,
		playerCommands:    make([]*network.PlayerCommand, 0, 256),
		serverJobs:        make([]*network.ServerJob, 0, 64),
	}
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

	s.logger.Debug("MoveToEntity action",
		zap.Uint64("client_id", cmd.ClientID),
		zap.Uint64("target_entity_id", moveToEntity.EntityId),
		zap.Bool("auto_interact", moveToEntity.AutoInteract))

	// TODO: Implement entity targeting and pathfinding
	// 1. Validate target entity exists and is reachable
	// 2. Set movement target to entity position
	// 3. If auto_interact, queue interaction when reached
}

func (s *NetworkCommandSystem) handleInteract(w *ecs.World, playerHandle types.Handle, cmd *network.PlayerCommand) {
	// Type assert payload
	interact, ok := cmd.Payload.(*netproto.Interact)
	if !ok {
		s.logger.Error("Invalid payload type for Interact",
			zap.Uint64("client_id", cmd.ClientID))
		return
	}

	s.logger.Debug("Interact action",
		zap.Uint64("client_id", cmd.ClientID),
		zap.Uint64("target_entity_id", interact.EntityId),
		zap.Int32("interaction_type", int32(interact.Type)))

	// TODO: Implement interaction system
	// 1. Validate target entity exists and is in range
	// 2. Check if interaction is valid for entity type
	// 3. Execute interaction (gather, open container, use, pickup, etc.)
}

func (s *NetworkCommandSystem) handleChat(w *ecs.World, playerHandle types.Handle, cmd *network.PlayerCommand) {
	payload, ok := cmd.Payload.(*network.ChatCommandPayload)
	if !ok {
		s.logger.Warn("Invalid chat command payload",
			zap.Uint64("client_id", cmd.ClientID))
		return
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

	senderName := senderAppearance.Name
	if senderName == "" {
		senderName = "Unknown"
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

	characterEntities := w.CharacterEntities()

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
