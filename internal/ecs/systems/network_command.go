package systems

import (
	"origin/internal/ecs"
	"origin/internal/network"

	"go.uber.org/zap"
)

const (
	// NetworkCommandSystemPriority runs first to process incoming commands
	NetworkCommandSystemPriority = 0
)

// NetworkCommandSystem processes player commands from the network layer
// This is the bridge between network I/O and ECS game state
// Commands are drained from the inbox at the start of each tick
type NetworkCommandSystem struct {
	ecs.BaseSystem

	playerInbox *network.PlayerCommandInbox
	serverInbox *network.ServerJobInbox
	logger      *zap.Logger

	// Reusable buffers to avoid allocations
	playerCommands []*network.PlayerCommand
	serverJobs     []*network.ServerJob
}

// NewNetworkCommandSystem creates a new network command system
func NewNetworkCommandSystem(
	playerInbox *network.PlayerCommandInbox,
	serverInbox *network.ServerJobInbox,
	logger *zap.Logger,
) *NetworkCommandSystem {
	return &NetworkCommandSystem{
		BaseSystem:     ecs.NewBaseSystem("NetworkCommandSystem", NetworkCommandSystemPriority),
		playerInbox:    playerInbox,
		serverInbox:    serverInbox,
		logger:         logger,
		playerCommands: make([]*network.PlayerCommand, 0, 256),
		serverJobs:     make([]*network.ServerJob, 0, 64),
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
			zap.Int64("character_id", int64(cmd.CharacterID)),
			zap.Uint16("command_type", uint16(cmd.CommandType)),
		)
		return
	}

	// Route command by type
	// TODO: Implement command routing to specific handlers
	// switch cmd.CommandType {
	// case network.CmdMoveItem:
	//     s.handleMoveItem(w, handle, cmd)
	// case network.CmdEquip:
	//     s.handleEquip(w, handle, cmd)
	// ...
	// }

	// Mark command as processed for deduplication
	s.playerInbox.MarkProcessed(cmd.ClientID, cmd.CommandID)
}

// processServerJob routes a server job to the appropriate handler
// This is a skeleton - actual job handlers will be added as needed
func (s *NetworkCommandSystem) processServerJob(w *ecs.World, job *network.ServerJob) {
	// Route job by type
	// TODO: Implement job routing to specific handlers
	// switch job.JobType {
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
