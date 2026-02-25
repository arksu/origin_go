# web_new — Game Client Architecture

## Overview

Modern Vue 3 + PixiJS v8 game client. Combines isometric rendering from `web_old` with networking from `web`.

## Directory Structure

```
src/
├── api/           # HTTP API for auth/characters
├── components/    # Reusable Vue UI components
├── config/        # App configuration (URL, debug flags)
├── game/          # PixiJS rendering (AGENTS.md inside)
├── network/       # WebSocket + Protobuf (AGENTS.md inside)
├── router/        # Vue Router setup
├── stores/        # Pinia state management (AGENTS.md inside)
├── types/         # Global TypeScript types
├── utils/         # General utilities
└── views/         # Vue page components
```

## Key Architectural Rules

### 1. Vue-Pixi Separation

**No direct PIXI imports in Vue components.**

```
✅ Vue → gameFacade → Render → PIXI
❌ Vue import * as PIXI from 'pixi.js'
```

`GameFacade` is the only entry point from Vue to game rendering.

### 2. Store = Data Only

Pinia stores hold **plain data** (POJOs), never PIXI objects.

```
✅ store.entities: Map<number, GameObjectData>
❌ store.sprite: PIXI.Sprite
```

### 3. Single Direction Data Flow

```
Server → Network → gameStore → gameFacade → PIXI Render
                ↓
         Vue Components (read-only for display)
```

### 4. Interaction UX Rules

- RMB on object is the entry point for context interactions.
- Context menu data comes from server (`S2C_ContextMenu`), client only renders and sends selection.
- Mini alerts are center-screen transient UI from server reason codes (`S2C_MiniAlert`).
- Character profile comes from server (`S2C_CharacterProfile`) as full snapshot (attributes + exp) and is shown in UI windows.
- Craft recipes come from server (`S2C_CraftList`) and are already filtered by server requirements; client only renders the provided list.
- Craft UI must send `C2S_OpenWindow("craft")` / `C2S_CloseWindow("craft")` so the server can gate craft-list recomputation and pushes.
- Build-state UI is also server-authoritative (`S2C_BuildState` rows/counters + `buildName` title); client should not recompute build progress locally.
- Keep anti-spam logic in store/UI (`debounce`, `coalesce`, max visible items), not in Pixi layer.

### Build UI Rules

- Build recipe list window and build-site state window are separate UI states.
- Build-state window click semantics are input-mode dependent:
  - hand has item → put to build-site
  - empty hand + row click → take back one deposited item from that row
- Object icon helpers must handle both visual JSON shapes used in `src/game/objects`: direct `{layers}` and nested variant maps.

## Stack

| Category | Technology | Version |
|----------|------------|---------|
| Framework | Vue | 3.x |
| Language | TypeScript | 5.x |
| Build | Vite | 6.x |
| State | Pinia | 2.x |
| Rendering | PixiJS | 8.x |
| Protocol | Protobuf | 7.x |
| HTTP | Axios | — |

## Entry Points

### main.ts

```typescript
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
```

Also initializes:
- `authStore.init()` — Check JWT expiration
- `registerMessageHandlers()` — Network message routing

### App.vue

Minimal root component. Router handles page transitions.

## Key Modules

### api/

HTTP client with interceptors:
- `client.ts` — Axios instance, error classification
- `auth.ts` — Login/register endpoints
- `characters.ts` — Character list/create endpoints

### game/

Full AGENTS.md documentation inside. Key exports:
- `gameFacade` — Vue interface to rendering
- `moveController` — Movement interpolation
- `cameraController` — Camera control
- `playerCommandController` — Input → network

### network/

Full AGENTS.md documentation inside. Key exports:
- `gameConnection` — WebSocket lifecycle
- `messageDispatcher` — Message routing
- `timeSync` — Server time estimation

### stores/

Full AGENTS.md documentation inside. Key stores:
- `authStore` — JWT authentication
- `gameStore` — Game world state

### views/

| View | Route | Purpose |
|------|-------|---------|
| `LoginView.vue` | `/login` | User authentication |
| `RegisterView.vue` | `/register` | New account |
| `CharactersView.vue` | `/characters` | Character selection |
| `GameView.vue` | `/game` | Main game canvas |

### config/

`index.ts` exports:
- `API_BASE_URL` — HTTP API endpoint
- `WS_URL` — WebSocket endpoint
- `DEBUG` — Debug flags
- `CLIENT_VERSION` — Protocol version

## Build Commands

```bash
npm run dev     # Vite dev server (regenerates proto first)
npm run build   # Production build (regenerates proto first)
npm run proto   # Generate protobuf JS + TS typings
```

## Communication Between Layers

### Vue → Game
```typescript
// GameView.vue
import { gameFacade } from '@/game'

onMounted(() => {
  gameFacade.init(canvas)
})
```

### Game → Network
```typescript
// PlayerCommandController.ts
import { gameConnection } from '@/network'

gameConnection.send({ playerAction: ... })
```

### Network → Store
```typescript
// handlers.ts
import { useGameStore } from '@/stores/gameStore'

messageDispatcher.on('chunkLoad', (msg) => {
  gameStore.loadChunk(x, y, tiles, version)
})
```

### Store → Game
```typescript
// handlers.ts
import { gameFacade } from '@/game'

gameFacade.loadChunk(x, y, tiles)
```

## Sub-AGENTS.md Files

Detailed documentation in each module:
- `game/AGENTS.md` — Rendering system
- `game/terrain/AGENTS.md` — Terrain generation
- `network/AGENTS.md` — Network layer
- `stores/AGENTS.md` — State management
