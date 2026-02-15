# Network Layer Architecture

## Overview

The network layer implements a **lock-free command queue pattern** to separate network I/O from ECS game state processing. This architecture ensures that network threads never block the game tick and that all game state
mutations happen deterministically within the ECS frame under shard lock.

## Core Principle

**Network threads write, ECS reads - zero contention on hot path**

- Network I/O threads enqueue player commands into a double-buffer queue
- ECS drains the queue at the start of each tick (lock-free read after buffer swap)
- All game state changes happen sequentially within the ECS tick
- Responses are published asynchronously via event bus

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Network Layer                                │
│  ┌──────────────┐                                                    │
│  │ Client Conn  │──┐                                                 │
│  └──────────────┘  │                                                 │
│  ┌──────────────┐  │  Enqueue()                                      │
│  │ Client Conn  │──┼──────────►┌─────────────────────────┐          │
│  └──────────────┘  │           │  PlayerCommandInbox     │          │
│  ┌──────────────┐  │           │  ┌────────────────┐    │          │
│  │ Client Conn  │──┘           │  │ Write Buffer   │◄───┼─ Network │
│  └──────────────┘              │  └────────────────┘    │   Thread  │
│                                │  ┌────────────────┐    │          │
│                                │  │ Read Buffer    │    │          │
│                                │  └────────────────┘    │          │
│                                └─────────────────────────┘          │
└─────────────────────────────────────────────────────────────────────┘
                                         │
                                         │ Drain() (buffer swap)
                                         ▼
┌─────────────────────────────────────────────────────────────────────┐
│                          ECS Layer                                   │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Shard Lock Acquired                                           │  │
│  │                                                                │  │
│  │  1. NetworkCommandSystem.Update()                             │  │
│  │     ├─ Drain PlayerCommandInbox                               │  │
│  │     ├─ Drain ServerJobInbox                                   │  │
│  │     └─ Route commands to handlers                             │  │
│  │                                                                │  │
│  │  2. ResetSystem.Update()                                      │  │
│  │  3. MovementSystem.Update()                                   │  │
│  │  4. CollisionSystem.Update()                                  │  │
│  │  5. TransformUpdateSystem.Update()                            │  │
│  │     └─ Publish events to EventBus (async)                     │  │
│  │                                                                │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                                         │
                                         │ PublishAsync()
                                         ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        Event Bus Workers                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │
│  │   Worker 1   │  │   Worker 2   │  │   Worker N   │             │
│  │              │  │              │  │              │             │
│  │ Send to      │  │ Send to      │  │ Send to      │             │
│  │ Client       │  │ Client       │  │ Client       │             │
│  └──────────────┘  └──────────────┘  └──────────────┘             │
└─────────────────────────────────────────────────────────────────────┘
```

## Components

### 1. PlayerCommandInbox

Double-buffer queue for player commands with built-in protections.

**Features:**

- **Double-buffer pattern**: Write buffer for network threads, read buffer for ECS
- **Rate limiting**: Per-client packets-per-second limit (default: 40 pps)
- **Overflow protection**: Queue size limit with drop-newest policy (default: 500)
- **Fairness**: Per-client commands-per-tick limit (default: 20)
- **Deduplication**: Monotonic command IDs prevent replay attacks
- **Zero-allocation drain**: Buffer swap is O(1) with no allocations

**Configuration:**

```go
type CommandQueueConfig struct {
MaxQueueSize            int     // Default: 500
MaxPacketsPerSecond     int     // Default: 40
MaxCommandsPerTickPerClient int // Default: 20
}
```

**Usage Pattern:**

```go
// Network thread (concurrent)
cmd := &PlayerCommand{
ClientID:    client.ID,
CharacterID: client.CharacterID,
CommandID:   nextCommandID,
CommandType: CmdMoveItem,
Payload:     data,
}
err := inbox.Enqueue(cmd)
if err != nil {
switch err.(type) {
case OverflowError:
// Send S2C_Warning WARN_INPUT_QUEUE_OVERFLOW
case RateLimitError:
// Send ERROR_PACKET_PER_SECOND_LIMIT_EXCEEDED and disconnect
case DuplicateCommandError:
// Ignore silently (already processed)
}
}

// ECS tick (single-threaded under shard lock)
commands := inbox.Drain() // Lock-free after buffer swap
for _, cmd := range commands {
processCommand(cmd)
inbox.MarkProcessed(cmd.ClientID, cmd.CommandID)
}
```

### 2. ServerJobInbox

Double-buffer queue for internal server jobs (machine ticks, auto-merge, etc.).

**Features:**

- Same double-buffer pattern as PlayerCommandInbox
- No rate limiting (internal jobs are trusted)
- Overflow protection with drop-newest policy

**Usage Pattern:**

```go
// Any thread
job := &ServerJob{
JobType:  JobMachineOfflineTick,
TargetID: machineEntityID,
Payload:  tickData,
}
inbox.Enqueue(job)

// ECS tick
jobs := inbox.Drain()
for _, job := range jobs {
processJob(job)
}
```

**Current login/bootstrap jobs**:
- `JobSendInventorySnapshot`
- `JobSendCharacterAttributesSnapshot`

These jobs are consumed by `NetworkCommandSystem` on ECS tick thread, not directly from network goroutines.

### 3. NetworkCommandSystem

ECS system that bridges network and game logic.

**Priority:** 0 (runs first in ECS tick)

**Responsibilities:**

1. Drain both command queues at tick start
2. Validate entity existence
3. Route commands to appropriate handlers
4. Mark commands as processed for deduplication

**Integration:**

```go
// In shard initialization
s.world.AddSystem(systems.NewNetworkCommandSystem(
s.playerInbox,
s.serverInbox,
logger,
))
```

## Safety Guarantees

### 1. No Race Conditions

- **Write path**: Short mutex only during `Enqueue()` to append to write buffer
- **Read path**: Lock-free after buffer swap in `Drain()`
- **ECS state**: All mutations happen under shard lock, single-threaded

### 2. Overflow Protection

When queue is full:

- New commands are **dropped** (drop-newest policy)
- Client receives `S2C_Warning` with `WARN_INPUT_QUEUE_OVERFLOW`
- Metrics track dropped count for monitoring

### 3. Rate Limiting

Per-client sliding window (1 second):

- Tracks last N command timestamps (N = MaxPacketsPerSecond)
- Rejects commands exceeding limit
- Client receives `ERROR_PACKET_PER_SECOND_LIMIT_EXCEEDED` and is disconnected

### 4. Fairness

Per-tick per-client limit prevents one client from monopolizing the tick:

- Each client limited to `MaxCommandsPerTickPerClient` per tick
- Excess commands carried over to next tick (not dropped)
- Ensures fair processing across all clients

### 5. Idempotency

Command deduplication prevents replay attacks:

- Each command has monotonic `CommandID` per client
- Server tracks `lastProcessedCommandID` per client
- Commands with `CommandID <= lastProcessedCommandID` are ignored

## Performance Characteristics

### Hot Path (ECS Tick)

```
Operation                    Time/op     Allocs/op
Drain (empty)                ~50 ns      0
Drain (100 commands)         ~2 µs       0
Drain (1000 commands)        ~20 µs      0
Buffer swap                  ~10 ns      0
```

### Memory Usage

- **Per-inbox overhead**: ~1 KB (buffers + client state map)
- **Per-client state**: ~320 bytes (timestamps + metadata)
- **Command overhead**: ~80 bytes per command

### Scalability

- **Linear scaling** with command count
- **No lock contention** between network and ECS
- **Suitable for 1000+ commands per tick**

## Error Handling

### Network Thread Errors

```go
err := inbox.Enqueue(cmd)
switch err.(type) {
case OverflowError:
// Queue full - send warning to client
sendWarning(client, WARN_INPUT_QUEUE_OVERFLOW)

case RateLimitError:
// Rate limit exceeded - disconnect client
sendError(client, ERROR_PACKET_PER_SECOND_LIMIT_EXCEEDED)
client.Close()

case DuplicateCommandError:
// Already processed - ignore silently
return
}
```

### ECS Tick Errors

```go
func (s *NetworkCommandSystem) processPlayerCommand(w *ecs.World, cmd *PlayerCommand) {
// Validate entity exists
handle := w.GetHandleByEntityID(cmd.CharacterID)
if handle == 0 || !w.Alive(handle) {
s.logger.Debug("Command for non-existent entity",
zap.Uint64("client_id", cmd.ClientID))
return
}

// Route to handler
// ... command processing ...

// Mark as processed
s.playerInbox.MarkProcessed(cmd.ClientID, cmd.CommandID)
}
```

## Configuration

### Game Config

```yaml
game:
  command_queue_size: 500              # Max commands in queue
  max_packets_per_second: 40           # Per-client rate limit
  max_commands_per_tick_per_client: 20 # Fairness limit
```

### Environment Variables

```bash
GAME_COMMAND_QUEUE_SIZE=500
GAME_MAX_PACKETS_PER_SECOND=40
GAME_MAX_COMMANDS_PER_TICK_PER_CLIENT=20
```

## Monitoring

### Metrics

```go
// Get queue statistics
received, dropped, processed := inbox.Stats()

// Log periodically
logger.Info("Command queue stats",
zap.Uint64("received", received),
zap.Uint64("dropped", dropped),
zap.Uint64("processed", processed),
)
```

### Alerts

- **High drop rate**: `dropped / received > 0.01` (1%)
- **Queue near capacity**: `len(writeBuffer) > MaxQueueSize * 0.8`
- **Rate limit violations**: Track per-client disconnect count

## Best Practices

### 1. Command Design

```go
// Good: Small, focused commands
type MoveItemCommand struct {
FromSlot int
ToSlot   int
Count    int
}

// Bad: Large, complex commands
type DoEverythingCommand struct {
MoveItems []MoveOp
CraftItems []CraftOp
TradeItems []TradeOp
// ... too much in one command
}
```

### 2. Handler Implementation

```go
// Good: Fast, deterministic handlers
func handleMoveItem(w *ecs.World, h types.Handle, cmd *MoveItemCommand) {
// Quick validation
if !validateSlots(cmd.FromSlot, cmd.ToSlot) {
return
}

// Direct state mutation
inventory := ecs.GetComponent[Inventory](w, h)
moveItem(inventory, cmd.FromSlot, cmd.ToSlot, cmd.Count)
}

// Bad: Slow, blocking handlers
func handleMoveItem(w *ecs.World, h types.Handle, cmd *MoveItemCommand) {
// Database query in ECS tick - BAD!
item := db.QueryItem(cmd.ItemID)

// Network call in ECS tick - BAD!
response := http.Get(validateURL)
}
```

### 3. Client Cleanup

```go
// On client disconnect
func (s *Server) onDisconnect(client *Client) {
// Remove client state from inbox
s.shard.PlayerInbox().RemoveClient(client.ID)
}
```

## Migration Guide

### From Direct Network Handlers

**Before:**

```go
func (s *Server) onMessage(client *Client, data []byte) {
// Parse packet
cmd := parseCommand(data)

// Direct state mutation - UNSAFE!
s.shard.mu.Lock()
processCommand(s.shard.world, cmd)
s.shard.mu.Unlock()
}
```

**After:**

```go
func (s *Server) onMessage(client *Client, data []byte) {
// Parse packet
cmd := parseCommand(data)

// Enqueue for ECS processing
err := s.shard.PlayerInbox().Enqueue(&PlayerCommand{
ClientID:    client.ID,
CharacterID: client.CharacterID,
CommandID:   client.nextCommandID,
CommandType: cmd.Type,
Payload:     cmd.Data,
})

if err != nil {
handleEnqueueError(client, err)
}
}
```

## Future Enhancements

### 1. Priority Queues

Add priority levels for commands:

- **High**: Combat actions, movement
- **Medium**: Inventory operations
- **Low**: Chat, emotes

### 2. Command Batching

Batch similar commands for efficiency:

- Multiple item moves → single inventory update
- Multiple chat messages → single broadcast

### 3. Adaptive Rate Limiting

Adjust rate limits based on server load:

- Reduce limits when tick time increases
- Increase limits when server is idle

### 4. Command Compression

Compress command payloads for bandwidth savings:

- Delta encoding for movement
- Dictionary compression for chat

## References

- ECS Systems Documentation: `internal/ecs/systems/AGENTS.md`
- Event Bus Architecture: `internal/eventbus/`
- Configuration: `internal/config/config.go`
