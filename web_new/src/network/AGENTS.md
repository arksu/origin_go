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
6. ← receive one S2C_ServerConstants before any world bootstrap/Pong
7. If fail: disconnect, state = error
```

**Important**: `ws.onopen` alone does NOT mean connected — must wait for `S2C_AuthResult.success = true`.

### Server constants and game calendar

- Successful authentication is followed by one required `S2C_ServerConstants` per
  connection. Geometry, tick rate, directional capability, and calendar scale are
  server-owned and immutable during its run. Do not embed fallback values in the
  client or read migrated constants from `PlayerEnterWorld`.
- Constants survive world entry/leave and layer transfers. Connection start,
  disconnect, auth failure, and error clear constants, dependent readiness, and
  calendar synchronization. Accept data only from the current authenticated
  socket. Identical duplicates are harmless; changed constants are a protocol error.
- Renderer initialization before handshake must remain safe; parameter-dependent
  calculations and world bootstrap require valid constants. Boolean false is a
  valid directional capability. Different valid values on reconnect are supported.
- Every authenticated Pong includes optional whole `runtimeSecondsTotal`, paired
  with its existing wall timestamp. Process `TimeSync` first, then update the game
  calendar from the received runtime only. `GameCalendarSync` exposes a reactive,
  immutable snapshot; it must not compensate delivery age or advance from local
  clocks. Preserve exact int64 precision and distinguish absent runtime from
  explicit zero. Keep the existing ping cadence. `DayTime.vue` reads the snapshot
  directly and does not start a timer or animate between Pongs.
- Runtime zero is the calendar epoch; offline time pauses. A new connection can
  accept lower runtime after crash rollback. Legacy Pongs still feed wall time.
- This bootstrap migration requires matching server/client deployment: removed
  enter-world tags 3/4/5/10 and names are reserved. See
  `docs/features/game_calendar.md` for the complete contract and accepted precision.

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
- `authResult`, `serverConstants`, `pong`
- `chunkLoad`, `chunkUnload`
- `playerEnterWorld`, `playerLeaveWorld`
- `objectSpawn`, `objectDespawn`, `objectMove`
- `movementMode`, `characterVisual`, `characterActionAnimation`
- `inventoryUpdate`, `inventoryOpResult`
- `containerOpened`, `containerClosed`
- `chat`, `contextMenu`, `miniAlert`
- `cyclicActionProgress`, `cyclicActionFinished`
- `attackResult`
- `sound`, `fx`
- `characterProfile`, `playerStats`, `expGained`
- `craftList`
- `buildState`, `buildStateClosed`
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
| `characterProfile` | Store full character profile snapshot (attributes + exp) |
| `playerStats` | Update stamina/energy state |
| `expGained` | Apply experience delta in store |
| `contextMenu` | Open context menu state in `gameStore` |
| `miniAlert` | Push center-screen transient alert in `gameStore` |
| `craftList` | Replace craft recipe list snapshot in `gameStore` |
| `buildState` | Replace active build-site snapshot (`entityId`, `buildName`, rows`) in `gameStore` |
| `buildStateClosed` | Close build-state window for the matching target |

### Action animation contract

`objectSpawn.actionAnimation` and `characterActionAnimation` carry the same plain
state. Decode with `types/actionAnimation.ts`, gate by stream epoch and character
generation, and compare exact uint64 revisions independently of equipment.
Equal-revision fresh samples may correct timing only for the same key/duration.
Missing optional spawn state clears presentation for legacy-server compatibility.
The canonical store entity owns accepted state; the facade forwards it to the
current ObjectView. Renderer readiness must never replay a captured old snapshot.
`TimeSync.estimateServerNowMs()` is sampled once per render update for all actors;
movement interpolation keeps its existing clock. See
`docs/features/action-animation-sync.md` for the generic def and phase contract.

### Combat result contract

- `AttackResultReceiver` validates combat notifications without changing health
  or creating unknown entities. It accepts at most 512 unique targets,
  preserves exact uint64 IDs, and checks finite nonnegative damage. Its monotonic
  event watermark advances only after validating the full packet. World entry,
  leave, and connection changes reset it; stale epochs and duplicate/older events
  are ignored. Every observer receives the full hit list; an empty list is a miss.
- Only accepted full packets reach `GameFacade.showDamageNumbers` with exact
  decimal target IDs and authoritative damage. The renderer uses existing views
  or a bounded 2-second despawn-anchor cache; IDs outside the exact numeric render
  range and unknown targets are skipped. Stream resets clear transient numbers
  and cached anchors. See `docs/features/damage_numbers.md` for presentation and
  measurement details.

### Character Profile + Craft Contract

- Server sends `S2C_CharacterProfile` after `S2C_PlayerEnterWorld`.
- Profile payload is a **full snapshot** of attributes + experience.
- Client should treat missing/invalid values as fail-safe `1` at store boundary.
- Client receives `S2C_CraftList` only after opening the craft UI (`C2S_OpenWindow("craft")`) and while it remains open.
- Client receives `S2C_BuildState` for build-site UI refreshes after open/put/take/progress server actions; inventory hand changes arrive via inventory messages.

## Protocol

### Protobuf Files

Located in `proto/` directory:
- `packets.js` — Generated from `packets.proto`
- `packets.d.ts` — TypeScript definitions

Generated via `protobufjs-cli`:
```bash
npm run proto
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
    mapClick: { x: 1000, y: 2000, targetEntityId: 0 },
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
| `index.ts` | Module exports + packet send helpers (`openWindow`, craft start, etc.) |
| `proto/` | Protobuf generated code |

### Confirmed melee execution

`sendActivateAction` sends direction-target actions immediately with current nonzero epoch and no aim, after WASD release/suppression. Server heading is authoritative. Geometry is already in the action catalog.

`ActionExecutionReceiver` validates ActionStateChanged epoch and exact uint64 generation before changing UI/cooldown snapshots. Explicit facing zero is valid; stale/duplicate generations cannot replay or extend the sector, conflicting action IDs/angles are rejected. Critical server FIFO is executing → finished → idle → next executing. Canceled finished clears the owner sector; completed finished keeps its remaining 1000 ms TTL through idle. Session/world/epoch changes and owner despawn clear receiver state and presentation. Actor animation bindings do not control this feedback.
