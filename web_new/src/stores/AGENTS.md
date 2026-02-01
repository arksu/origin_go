# Pinia Stores

## Architecture Overview

Stores use Pinia with Composition API pattern (`defineStore` + `ref`/`computed`). Two main stores: `auth` for authentication, `game` for game state.

```
authStore — Authentication, JWT token
    ↓
gameStore — Game session, connection, world state
    ↓
  ├─ chunks: Map<chunkKey, ChunkData>
  ├─ entities: Map<entityId, GameObjectData>
  └─ player: position, entityId
```

## authStore

**Purpose**: Manage authentication token and JWT expiration.

**State**:
| State | Type | Description |
|-------|------|-------------|
| `token` | `string` | JWT from localStorage |

**Computed**:
- `isAuthenticated` — Has token AND not expired
- `isTokenExpired` — JWT `exp` claim check

**Actions**:
- `setToken(token)` — Save to state + localStorage
- `logout()` — Clear token
- `init()` — Check expiration on app start, link to API client

**JWT Parsing**:
```typescript
// Parses JWT payload client-side (no verification)
parseJwtPayload(token) → { exp?: number }
```

**Usage**:
```typescript
const authStore = useAuthStore()
authStore.setToken(jwt)
if (authStore.isAuthenticated) { /* proceed */ }
```

## gameStore

**Purpose**: Single source of truth for all game state. Pure data (no Pixi objects).

### State Categories

#### Session
| State | Description |
|-------|-------------|
| `wsToken` | WebSocket auth token (from HTTP /auth/login) |
| `characterId` | Selected character ID |

#### Connection
| State | Description |
|-------|-------------|
| `connectionState` | `disconnected` / `connecting` / `authenticating` / `connected` / `error` |
| `connectionError` | Error code + message |

#### Player
| State | Description |
|-------|-------------|
| `playerEntityId` | Entity ID assigned by server |
| `playerName` | Character name |
| `playerPosition` | `{ x, y, heading }` — server position (not interpolated) |

#### World
| State | Type | Description |
|-------|------|-------------|
| `worldParams` | `WorldParams \| null` | `coordPerTile`, `chunkSize`, `streamEpoch` |
| `chunks` | `Map<string, ChunkData>` | Loaded chunk data by key `"x,y"` |
| `entities` | `Map<number, GameObjectData>` | All game objects by entity ID |

### Types

```typescript
interface Position {
  x: number
  y: number
  heading: number  // 0-7 for 8 directions
}

interface EntityMovement {
  position: Position
  velocity: { x: number; y: number }
  moveMode: number
  targetPosition?: { x: number; y: number }
  isMoving: boolean
}

interface GameObjectData {
  entityId: number
  objectType: number      // 1 = player, 6 = resource, etc
  resourcePath: string    // Spine/asset path
  position: { x: number; y: number }
  size: { x: number; y: number }
  movement?: EntityMovement
}

interface ChunkData {
  x: number
  y: number
  tiles: Uint8Array       // Raw tile type data
  version: number         // Chunk version for cache invalidation
}
```

### Actions

#### Session
- `setGameSession(token, charId)` — After character selection
- `clearGameSession()` — On disconnect/logout

#### Connection
- `setConnectionState(state, error?)` — Updated by GameConnection

#### Player
- `setPlayerEnterWorld(entityId, name, coordPerTile, chunkSize, streamEpoch)` — On `S2C_PlayerEnterWorld`
- `setPlayerLeaveWorld()` — Clear all player/world state
- `updatePlayerPosition(position)` — On `S2C_ObjectMove` for player

#### Chunks
- `loadChunk(x, y, tiles, version)` — On `S2C_ChunkLoad`
- `unloadChunk(x, y)` — On `S2C_ChunkUnload`

#### Entities
- `spawnEntity(data)` — On `S2C_ObjectSpawn`
- `despawnEntity(entityId)` — On `S2C_ObjectDespawn`
- `updateEntityMovement(entityId, movement)` — On `S2C_ObjectMove`

#### Reset
- `reset()` — Full cleanup (logout, disconnect, leave world)

### Key Patterns

**Store = Source of Truth, Not Render**:
```typescript
// CORRECT: Store holds data
const entity = gameStore.entities.get(entityId)
entity.movement = newMovement  // Update server data

// INCORRECT: Don't put Pixi objects in store
// gameStore.sprite = new PIXI.Sprite()  // ❌
```

**Computed State**:
```typescript
const isConnected = computed(() => connectionState.value === 'connected')
const isInGame = computed(() => isConnected.value && playerEntityId.value !== null)
```

**Map-based Collections**:
- `chunks` — Key: `"${x},${y}"`, Value: `ChunkData`
- `entities` — Key: `entityId`, Value: `GameObjectData`

Maps used for O(1) lookup and easy Vue reactivity.

## Data Flow

### Server → Store
```
S2C_ChunkLoad → handlers.ts → gameStore.loadChunk()
                                ↓
                         gameFacade.loadChunk()  // render update
```

### Component → Store → Server
```
GameView click → InputController → PlayerCommandController
                                         ↓
                                   gameConnection.send(C2S_PlayerAction)
```

## Files

| File | Purpose |
|------|---------|
| `authStore.ts` | JWT authentication state |
| `gameStore.ts` | Game world state (chunks, entities, player) |
