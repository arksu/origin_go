package network

import (
	"sync"
	"sync/atomic"
	"time"

	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// CommandType identifies the type of player command
type CommandType uint16

const (
	CmdMoveTo CommandType = iota + 1
	CmdMoveToEntity
	CmdInteract
	CmdSelectContextAction
	CmdChat
	CmdInventoryOp
	CmdOpenContainer
	CmdCloseContainer
)

// PlayerCommand represents an intent from a client to be processed by ECS
type PlayerCommand struct {
	ClientID    uint64
	CharacterID types.EntityID
	CommandID   uint64 // Monotonic per-connection for idempotency
	CommandType CommandType
	Payload     any
	ReceivedAt  time.Time
	Layer       int
}

// ChatCommandPayload contains chat message data
type ChatCommandPayload struct {
	Channel netproto.ChatChannel
	Text    string
}

// Server job type constants
const (
	JobSendInventorySnapshot uint16 = iota + 1
	JobSendCharacterProfileSnapshot
)

// ServerJob represents an internal job to be processed by ECS
type ServerJob struct {
	JobType   uint16
	TargetID  types.EntityID
	Payload   any
	CreatedAt time.Time
	Layer     int
}

// InventorySnapshotJobPayload is the payload for JobSendInventorySnapshot
type InventorySnapshotJobPayload struct {
	Handle types.Handle
}

// CharacterProfileSnapshotJobPayload is the payload for JobSendCharacterProfileSnapshot.
// It currently transports attributes only, but represents player-profile snapshot flow.
type CharacterProfileSnapshotJobPayload struct {
	Handle types.Handle
}

// CommandQueueConfig holds configuration for command queues
type CommandQueueConfig struct {
	MaxQueueSize                int // Maximum commands in queue before overflow (default: 500)
	MaxPacketsPerSecond         int // Per-client rate limit (default: 40)
	MaxCommandsPerTickPerClient int // Fairness limit per tick (default: 20)
}

// DefaultCommandQueueConfig returns default configuration
func DefaultCommandQueueConfig() CommandQueueConfig {
	return CommandQueueConfig{
		MaxQueueSize:                500,
		MaxPacketsPerSecond:         40,
		MaxCommandsPerTickPerClient: 20,
	}
}

// OverflowError indicates queue overflow
type OverflowError struct{}

func (e OverflowError) Error() string { return "command queue overflow" }

// RateLimitError indicates rate limit exceeded
type RateLimitError struct{}

func (e RateLimitError) Error() string { return "rate limit exceeded" }

// DuplicateCommandError indicates duplicate command
type DuplicateCommandError struct{}

func (e DuplicateCommandError) Error() string { return "duplicate command" }

// PlayerCommandInbox is a double-buffer queue for player commands
// Network threads write to writeBuffer (with short mutex)
// ECS reads from readBuffer (lock-free during tick)
type PlayerCommandInbox struct {
	writeBuffer []*PlayerCommand
	readBuffer  []*PlayerCommand
	writeMu     sync.Mutex

	// Per-client state for rate limiting and deduplication
	clientState   map[uint64]*clientCommandState
	clientStateMu sync.RWMutex

	config CommandQueueConfig

	// Metrics
	totalReceived  atomic.Uint64
	totalDropped   atomic.Uint64
	totalProcessed atomic.Uint64
}

// clientCommandState tracks per-client command state
type clientCommandState struct {
	lastProcessedCommandID uint64
	commandTimestamps      []time.Time // Ring buffer for rate limiting
	timestampIndex         int
}

// NewPlayerCommandInbox creates a new player command inbox
func NewPlayerCommandInbox(config CommandQueueConfig) *PlayerCommandInbox {
	return &PlayerCommandInbox{
		writeBuffer: make([]*PlayerCommand, 0, config.MaxQueueSize),
		readBuffer:  make([]*PlayerCommand, 0, config.MaxQueueSize),
		clientState: make(map[uint64]*clientCommandState),
		config:      config,
	}
}

// Enqueue adds a command to the write buffer
// Called from network thread - uses short mutex
// Returns error if queue overflow or rate limit exceeded
func (q *PlayerCommandInbox) Enqueue(cmd *PlayerCommand) error {
	// Check rate limit first (before acquiring write lock)
	if err := q.checkRateLimit(cmd.ClientID); err != nil {
		return err
	}

	// Check for duplicate command
	if q.isDuplicate(cmd.ClientID, cmd.CommandID) {
		return DuplicateCommandError{}
	}

	q.writeMu.Lock()
	defer q.writeMu.Unlock()

	// Check queue overflow
	if len(q.writeBuffer) >= q.config.MaxQueueSize {
		q.totalDropped.Add(1)
		return OverflowError{}
	}

	q.writeBuffer = append(q.writeBuffer, cmd)
	q.totalReceived.Add(1)

	// Record timestamp for rate limiting
	q.recordCommandTimestamp(cmd.ClientID)

	return nil
}

// checkRateLimit checks if client has exceeded rate limit
func (q *PlayerCommandInbox) checkRateLimit(clientID uint64) error {
	q.clientStateMu.RLock()
	state, exists := q.clientState[clientID]
	q.clientStateMu.RUnlock()

	if !exists {
		return nil // New client, no rate limit yet
	}

	now := time.Now()
	windowStart := now.Add(-time.Second)

	count := 0
	for _, ts := range state.commandTimestamps {
		if ts.After(windowStart) {
			count++
		}
	}

	if count >= q.config.MaxPacketsPerSecond {
		return RateLimitError{}
	}

	return nil
}

// recordCommandTimestamp records command timestamp for rate limiting
func (q *PlayerCommandInbox) recordCommandTimestamp(clientID uint64) {
	q.clientStateMu.Lock()
	defer q.clientStateMu.Unlock()

	state, exists := q.clientState[clientID]
	if !exists {
		state = &clientCommandState{
			commandTimestamps: make([]time.Time, q.config.MaxPacketsPerSecond),
		}
		q.clientState[clientID] = state
	}

	state.commandTimestamps[state.timestampIndex] = time.Now()
	state.timestampIndex = (state.timestampIndex + 1) % len(state.commandTimestamps)
}

// isDuplicate checks if command is a duplicate or stale
func (q *PlayerCommandInbox) isDuplicate(clientID uint64, commandID uint64) bool {
	q.clientStateMu.RLock()
	defer q.clientStateMu.RUnlock()

	state, exists := q.clientState[clientID]
	if !exists {
		return false
	}

	// Ignore if commandID <= lastProcessedCommandID
	return commandID <= state.lastProcessedCommandID
}

// Drain swaps buffers and returns commands for processing
// Called at start of ECS tick - lock-free read after swap
// Returns commands grouped by client with fairness limits applied
func (q *PlayerCommandInbox) Drain() []*PlayerCommand {
	q.writeMu.Lock()
	// Swap buffers
	q.readBuffer, q.writeBuffer = q.writeBuffer, q.readBuffer
	// Clear write buffer for next tick (reuse capacity)
	q.writeBuffer = q.writeBuffer[:0]
	q.writeMu.Unlock()

	if len(q.readBuffer) == 0 {
		return nil
	}

	// Apply fairness: limit commands per client per tick
	result := q.applyFairness(q.readBuffer)

	return result
}

// applyFairness limits commands per client and carries over excess to next tick
func (q *PlayerCommandInbox) applyFairness(commands []*PlayerCommand) []*PlayerCommand {
	// Group by client
	byClient := make(map[uint64][]*PlayerCommand)
	for _, cmd := range commands {
		byClient[cmd.ClientID] = append(byClient[cmd.ClientID], cmd)
	}

	result := make([]*PlayerCommand, 0, len(commands))
	overflow := make([]*PlayerCommand, 0)

	limit := q.config.MaxCommandsPerTickPerClient

	for clientID, clientCmds := range byClient {
		if len(clientCmds) <= limit {
			result = append(result, clientCmds...)
		} else {
			// Take up to limit, carry over rest
			result = append(result, clientCmds[:limit]...)
			overflow = append(overflow, clientCmds[limit:]...)
		}
		_ = clientID
	}

	// Put overflow back into write buffer for next tick
	if len(overflow) > 0 {
		q.writeMu.Lock()
		q.writeBuffer = append(overflow, q.writeBuffer...)
		q.writeMu.Unlock()
	}

	return result
}

// MarkProcessed marks a command as processed for deduplication
func (q *PlayerCommandInbox) MarkProcessed(clientID uint64, commandID uint64) {
	q.clientStateMu.Lock()
	defer q.clientStateMu.Unlock()

	state, exists := q.clientState[clientID]
	if !exists {
		state = &clientCommandState{
			commandTimestamps: make([]time.Time, q.config.MaxPacketsPerSecond),
		}
		q.clientState[clientID] = state
	}

	if commandID > state.lastProcessedCommandID {
		state.lastProcessedCommandID = commandID
	}

	q.totalProcessed.Add(1)
}

// RemoveClient removes client state (on disconnect)
func (q *PlayerCommandInbox) RemoveClient(clientID uint64) {
	q.clientStateMu.Lock()
	delete(q.clientState, clientID)
	q.clientStateMu.Unlock()
}

// Stats returns queue statistics
func (q *PlayerCommandInbox) Stats() (received, dropped, processed uint64) {
	return q.totalReceived.Load(), q.totalDropped.Load(), q.totalProcessed.Load()
}

// ServerJobInbox is a double-buffer queue for internal server jobs
type ServerJobInbox struct {
	writeBuffer []*ServerJob
	readBuffer  []*ServerJob
	writeMu     sync.Mutex

	config CommandQueueConfig

	totalReceived  atomic.Uint64
	totalDropped   atomic.Uint64
	totalProcessed atomic.Uint64
}

// NewServerJobInbox creates a new server job inbox
func NewServerJobInbox(config CommandQueueConfig) *ServerJobInbox {
	return &ServerJobInbox{
		writeBuffer: make([]*ServerJob, 0, config.MaxQueueSize),
		readBuffer:  make([]*ServerJob, 0, config.MaxQueueSize),
		config:      config,
	}
}

// Enqueue adds a job to the write buffer
func (q *ServerJobInbox) Enqueue(job *ServerJob) error {
	q.writeMu.Lock()
	defer q.writeMu.Unlock()

	if len(q.writeBuffer) >= q.config.MaxQueueSize {
		q.totalDropped.Add(1)
		return OverflowError{}
	}

	q.writeBuffer = append(q.writeBuffer, job)
	q.totalReceived.Add(1)
	return nil
}

// Drain swaps buffers and returns jobs for processing
func (q *ServerJobInbox) Drain() []*ServerJob {
	q.writeMu.Lock()
	q.readBuffer, q.writeBuffer = q.writeBuffer, q.readBuffer
	q.writeBuffer = q.writeBuffer[:0]
	q.writeMu.Unlock()

	if len(q.readBuffer) == 0 {
		return nil
	}

	q.totalProcessed.Add(uint64(len(q.readBuffer)))
	return q.readBuffer
}

// Stats returns queue statistics
func (q *ServerJobInbox) Stats() (received, dropped, processed uint64) {
	return q.totalReceived.Load(), q.totalDropped.Load(), q.totalProcessed.Load()
}
