# Network Layer

## Architecture Overview

Network layer handles WebSocket communication with game server using Protocol Buffers (protobuf). Single singleton `gameConnection` manages the connection lifecycle.

```
GameConnection (singleton)
    ├─ WebSocket (binary protobuf)
    ├─ State machine: disconnected → connecting → authenticating → connected
    ├─ Ping/Pong for keepalive and time sync
    └─ Routes messages → MessageDispatcher

MessageDispatcher (singleton)
    ├─ Type → handler mapping
    ├─ Debug message buffer (last 100)
    └─ Dispatches to handlers.ts

TimeSync (singleton)
    └─ Estimates server time from Ping/Pong RTT
```

## Connection Lifecycle

### States

| State | Description |
|-------|-------------|
| `disconnected` | Initial state, no connection |
| `connecting` | WebSocket connecting |
| `authenticating` | Connected, waiting for `S2C_AuthResult` |
| `connected` | Authenticated, game active |
| `error` | Connection failed or closed unexpectedly |

### Handshake Flow

```
1. HTTP POST /auth/login → get wsToken
2. new WebSocket(WS_URL)
3. ws.onopen → send C2S_Auth { token, clientVersion }
4. ← receive S2C_AuthResult
5. If success: start ping interval, state = connected
6. If fail: disconnect, state = error
```

**Important**: `ws.onopen` alone does NOT mean connected — must wait for `S2C_AuthResult.success = true`.

## Components

### GameConnection.ts

**Role**: Manages WebSocket connection and low-level message handling.

**Key Methods**:
- `connect(authToken)` — Initiate connection with wsToken
- `disconnect()` — Close connection, cleanup
- `send(payload)` — Send protobuf message
- `sendPing()` — Send C2S_Ping (auto-started after auth)

**Callbacks**:
- `onMessage(handler)` — Received server messages
- `onStateChange(handler)` — Connection state changes

**Ping/Pong**:
- Interval: `config.PING_INTERVAL_MS` (default 5000ms)
- Pong data feeds `TimeSync` for server time estimation
- RTT, jitter, offset exposed via `timeSync.getDebugMetrics()`

### MessageDispatcher.ts

**Role**: Routes decoded protobuf messages to typed handlers.

**Pattern**: Type-safe handler registration
```typescript
messageDispatcher.on('chunkLoad', (msg) => { /* ... */ })
messageDispatcher.on('objectMove', (msg) => { /* ... */ })
```

**Supported Types** (from `proto.IServerMessage`):
- `authResult`, `pong`
- `chunkLoad`, `chunkUnload`
- `playerEnterWorld`, `playerLeaveWorld`
- `objectSpawn`, `objectDespawn`, `objectMove`
- `inventoryUpdate`, `inventoryOpResult`
- `containerOpened`, `containerClosed`
- `error`, `warning`

**Debug Features**:
- `getDebugBuffer()` — Last 100 messages with timestamps
- `getUnknownMessageCount()` — Messages with no handler
- `clearDebugBuffer()` — Reset debug history

### TimeSync.ts

**Role**: Estimate server time on client for movement interpolation.

**Algorithm**:
1. Send `C2S_Ping { clientTimeMs }`
2. Receive `S2C_Pong { clientTimeMs, serverTimeMs }`
3. Calculate:
   - `rtt = now - clientSendMs`
   - `offset ≈ serverTime - clientSend - rtt/2`
4. EWMA smoothing to reduce jitter

**Constants**:
```typescript
EWMA_ALPHA = 0.2              // Offset smoothing
JITTER_EWMA_ALPHA = 0.1       // Jitter smoothing
BASE_INTERPOLATION_DELAY = 120ms
MIN_INTERPOLATION_DELAY = 80ms
MAX_INTERPOLATION_DELAY = 250ms
JITTER_MULTIPLIER = 2.5       // Extra delay per jitter ms
```

**API**:
- `onPong(clientSendMs, serverTimeMs)` — Feed pong data
- `estimateServerNowMs()` — Get current server time estimate
- `getInterpolationDelayMs()` — Recommended delay for buffering
- `getDebugMetrics()` — RTT, jitter, offset, sample count

Used by `MoveController` for time-based interpolation.

### handlers.ts

**Role**: Bridge between network messages and game/render state.

Called once on app init via `registerMessageHandlers()`:
```typescript
// In main.ts or App.vue
import { registerMessageHandlers } from '@/network/handlers'
registerMessageHandlers()
```

**Handler Pattern**:
1. Decode protobuf message
2. Update `gameStore` (Pinia state)
3. Call `gameFacade` methods (render updates)
4. Call `moveController` for movement-related messages

**Key Handlers**:

| Message | Action |
|---------|--------|
| `playerEnterWorld` | Set world params, init MoveController, set camera |
| `playerLeaveWorld` | Cleanup entities, reset MoveController |
| `chunkLoad` | `gameStore.loadChunk`, `gameFacade.loadChunk` |
| `chunkUnload` | `gameStore.unloadChunk`, `gameFacade.unloadChunk` |
| `objectSpawn` | Spawn entity in store + facade, init in MoveController |
| `objectDespawn` | Remove entity from store + facade + MoveController |
| `objectMove` | Feed to MoveController, update store position |

## Protocol

### Protobuf Files

Located in `proto/` directory:
- `packets.js` — Generated from `packets.proto`
- `packets.d.ts` — TypeScript definitions

Generated via `protobufjs-cli`:
```bash
npm run proto:generate
```

### Message Format

All messages wrapped in `ServerMessage` / `ClientMessage`:

```protobuf
message ServerMessage {
  uint64 sequence = 1;
  oneof payload {
    S2C_AuthResult authResult = 2;
    S2C_Pong pong = 3;
    S2C_ChunkLoad chunkLoad = 4;
    // ... etc
  }
}
```

### Binary Encoding

- WebSocket `binaryType = 'arraybuffer'`
- Protobuf binary encoding (not JSON)
- `ServerMessage.decode(new Uint8Array(buffer))`

## Usage Example

```typescript
import { gameConnection, messageDispatcher } from '@/network'
import { registerMessageHandlers } from '@/network/handlers'
import { useGameStore } from '@/stores/gameStore'

// 1. Register handlers once
registerMessageHandlers()

// 2. Listen for state changes
gameConnection.onStateChange((state, error) => {
  const gameStore = useGameStore()
  gameStore.setConnectionState(state, error)
})

// 3. Connect after HTTP auth
async function connectToGame(wsToken: string) {
  gameConnection.connect(wsToken)
}

// 4. Send player action
gameConnection.send({
  playerAction: proto.C2S_PlayerAction.create({
    moveTo: { x: 1000, y: 2000 },
    modifiers: 1, // SHIFT
  })
})
```

## Error Handling

**Connection Errors**:
- `CONNECTION_FAILED` — WebSocket couldn't connect
- `AUTH_FAILED` — Server rejected token
- `CONNECTION_CLOSED` — Unexpected close

All errors transition to `error` state. Handler must reset or retry.

**Handler Errors**:
- Wrapped in try/catch in MessageDispatcher
- Logged to console, doesn't crash dispatcher

## Files

| File | Purpose |
|------|---------|
| `GameConnection.ts` | WebSocket lifecycle, auth, ping/pong |
| `MessageDispatcher.ts` | Type-based message routing |
| `TimeSync.ts` | Server time estimation via RTT |
| `handlers.ts` | Message → state/render bridge |
| `types.ts` | Connection state types |
| `index.ts` | Module exports |
| `proto/` | Protobuf generated code |
