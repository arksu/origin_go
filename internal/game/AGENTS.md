# Game Package Architecture

This document describes the architecture and organization of the `internal/game` package.

## Overview

The `game` package is the core of the game server, responsible for managing game state, player connections, shards, and game loop execution.

## Package Structure

```
internal/game/
├── game.go              # Main game logic and game loop
├── game_auth.go          # Player authentication and spawning
├── shard.go              # Individual shard management
├── shard_manager.go      # Multi-shard coordination
├── errors.go             # Common error definitions
├── id.go                 # Entity ID management
├── inventory/            # Inventory system
│   ├── loader.go         # Inventory loading from database
│   ├── saver.go          # Inventory saving to database
│   ├── snapshot.go       # Inventory snapshot sending to clients
│   └── types.go          # Common inventory types
├── world/                # World and object management
│   ├── chunk_manager.go  # Chunk loading/unloading and spatial management
│   ├── chunk_manager_test.go
│   ├── object_builder.go # Object building logic
│   └── object_factory.go # Object factory pattern
├── events/               # Game event handling
│   └── game_events.go    # Network visibility and event dispatching
└── systems/              # Future game systems
```

## Core Components

### Game (`game.go`)
- **Purpose**: Main game server entry point and game loop management
- **Key Responsibilities**:
  - Game state management (Starting, Running, Stopping, Stopped)
  - Network message handling
  - Client connection/disconnection
  - Game loop execution and tick management
  - Statistics collection
- **Key Types**: `Game`, `GameState`, `GameStats`, `tickStats`

### Authentication (`game_auth.go`)
- **Purpose**: Player authentication, character selection, and world spawning
- **Key Responsibilities**:
  - Authentication token validation
  - Character loading and validation
  - World spawning and initial setup
  - Inventory snapshot sending
  - Player reattachment after disconnect
- **Key Functions**: `handleAuth()`, `spawnAndLogin()`, `reattachPlayer()`

### Shard Management (`shard.go`, `shard_manager.go`)
- **Purpose**: Multi-shard world management
- **Key Responsibilities**:
  - Shard creation and lifecycle management
  - Client-to-shard assignment
  - Entity AOI (Area of Interest) tracking
  - Command queue management
  - Character saving coordination
- **Key Types**: `Shard`, `ShardManager`, `ShardState`

### Entity ID Management (`id.go`)
- **Purpose**: Unique entity ID generation and management
- **Key Responsibilities**:
  - ID range allocation per shard
  - Database persistence of ID state
  - Collision prevention
- **Key Types**: `EntityIDManager`

## Sub-packages

### Inventory (`inventory/`)
- **Purpose**: Player inventory management
- **Components**:
  - `loader.go`: Loads inventory data from database
  - `saver.go`: Saves inventory data to database
  - `snapshot.go`: Sends inventory snapshots to clients
  - `types.go`: Common inventory data structures
- **Key Types**: `InventoryLoader`, `InventorySaver`, `SnapshotSender`

### World (`world/`)
- **Purpose**: World geometry, chunks, and object management
- **Components**:
  - `chunk_manager.go`: Chunk loading/unloading, spatial indexing
  - `object_factory.go`: Object creation using factory pattern
  - `object_builder.go`: Specific object building logic
- **Key Types**: `ChunkManager`, `ObjectFactory`, `ObjectBuilder`

### Events (`events/`)
- **Purpose**: Game event handling and network visibility
- **Components**:
  - `game_events.go`: Network visibility dispatcher for entity events
- **Key Types**: `NetworkVisibilityDispatcher`

### Context Actions (`context_action_service.go`)
- **Purpose**: Aggregate and execute context menu actions from object behaviors.
- **Key Responsibilities**:
  - Compute action list on RMB (`Interact`)
  - Resolve duplicate action IDs with first-wins policy
  - Respect object-def option `contextMenuEvenForOneItem` for single-action RMB behavior
  - Execute selected action only after `LinkCreated`
  - Emit `S2C_MiniAlert` for explicit execution failures
  - Manage cyclic action terminal events (`S2C_CyclicActionFinished`)
  - Force immediate vision updates after tree chop result (stump + spawned logs)
- **Key Types**: `ContextActionService`

## Data Flow

### Player Connection Flow
1. Client connects → `game.go:handleConnect()`
2. Client sends auth → `game_auth.go:handleAuth()`
3. Load character data → `inventory/loader.go:LoadPlayerInventories()`
4. Spawn in world → `game_auth.go:spawnAndLogin()`
5. Send inventory snapshots → `inventory/snapshot.go:SendInventorySnapshots()`
6. Setup event handlers → `events/game_events.go`

### Game Loop Flow
1. `game.go:gameLoop()` runs at configured tick rate
2. Updates all shards via `shard_manager.go:Update()`
3. Each shard processes its world and entities
4. Events are dispatched through event bus
5. Network visibility updates sent to clients

### Command Processing Flow
1. Client sends action → `game.go:handlePlayerAction()`
2. Command enqueued in shard's player inbox
3. Processed in shard's update cycle
4. Results sent back to client

## Key Design Patterns

### Factory Pattern
- `world/object_factory.go`: Creates different object types
- `world/object_builder.go`: Implements specific building logic

### Command Queue Pattern
- Separate network and ECS processing
- Rate limiting and overflow protection
- Per-shard command queues

### Event-Driven Architecture
- Central event bus for game events
- Async event processing
- Network visibility as event subscriber

### Spatial Partitioning
- Chunk-based world organization
- AOI (Area of Interest) management
- Efficient visibility calculations

## Configuration

Key configuration parameters:
- `Game.TickRate`: Game loop frequency
- `Game.Env`: `dev|stage|prod` runtime mode
- `Game.MaxEntities`: Maximum entities per shard
- `Game.ChunkLRUTTL`: Chunk cache TTL
- `Game.CommandQueueSize`: Command queue limits
- `Game.DisconnectDelay`: Player detach time
- `Game.WorkerPoolSize`: Event bus worker pool size
- `Game.ObjectBehaviorBudgetPerTick`: max dirty object behavior recomputes per tick (default `512`)
- `Game.InteractionPendingTimeout`: pending context action lifetime before cleanup (default `15s`)

## Object Behavior Runtime Notes

- `ObjectBehaviorSystem` is dirty-queue driven (no global per-tick scan in normal mode).
- Chunk activation (`world/chunk_manager.go`) forces initial behavior recompute for all loaded objects with behaviors (lazy init is not allowed).
- Inventory mutations that affect object-root containers must mark object behavior dirty, so appearance/flags stay in sync.

## Error Handling

Common error types defined in `errors.go`:
- Entity-related errors
- Chunk-related errors
- Inventory-related errors
- Authentication errors

## Future Extensions

The `systems/` directory is reserved for future game systems:
- Combat system
- Crafting system
- Movement system enhancements
- AI/NPC systems
- Quest system

## Dependencies

The game package depends on:
- `internal/ecs`: Entity component system
- `internal/network`: Network layer
- `internal/persistence`: Database layer
- `internal/eventbus`: Event handling
- `internal/types`: Shared type definitions
