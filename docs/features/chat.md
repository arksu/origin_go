# Local Chat Delivery Architecture
One-sentence summary: Design separates network validation, gameplay chat processing, and network delivery via event bus to guarantee deterministic, testable message flow.

## Scope & Goals
- MVP = LOCAL channel only, but architecture must extend to other channels/anti-spam later.
- Enforce SOLID boundaries: network parsing, chat domain logic, delivery service, metrics/observability.
- Keep gameplay shard isolation (per-layer) and avoid cross-thread state mutations.

## Proposed Flow
1. **Network layer**: `handlePacket` adds `case *ClientMessage_Chat` → `handleChatMessage`.
   - Validate authenticated client, non-empty text, channel == LOCAL, length <= `maxChatLen`, min interval since last send.
   - Normalize text (trim, strip CR, collapse newlines) and track per-client spam state.
   - Build `ChatRequest` struct `{Layer, CharacterID, Channel, Text, Timestamp}`.
   - Publish to event bus topic `gameplay.chat.request.local` (PriorityMedium) + metrics (`chat_requests_total{result}`).
2. **Chat system (ECS worker)** subscribes asynchronously with worker pool per shard.
   - On event: resolve `senderHandle = world.GetHandleByEntityID`, ensure entity alive in same layer.
   - Fetch components: `Transform` for position, `Appearance` (name), optional `Faction` for future rules.
   - Query recipients: iterate players in same layer via ECS query (cached view) or `VisionSystem` neighbor index. Use squared distance vs `chatLocalRadius` from config.
   - Compose delivery payload once `{channel, fromID, fromName, text, sequenceID}` to reuse for all recipients.
   - Log debug summary and emit metrics (`chat_local_messages_total`, `chat_local_recipients_total`).
3. **Delivery service**: dedicated interface owned by shard linking EntityID ↔ network.Client.
   - Provide `SendChatTo(entityID, payload)` and `BroadcastChat(entityIDs, payload)`.
   - Backed by shard's `clients` map; respects player's connection state and stream epoch.
   - Handles marshaling to `netproto.S2C_ChatMessage` + `ServerMessage`, reusing buffers where possible.
4. **Client echo**: include sender in recipient set, even if no nearby players.

## Event Bus & Concurrency
- Add topic namespace `gameplay.chat.request.*` for incoming requests and `gameplay.chat.delivered` for auditing.
- Chat handler runs async to avoid blocking network threads; use bounded worker count (per shard) to avoid flooding.
- On handler failure (missing components, etc.) publish structured warn + increment `chat_rejected_total{reason}`.

## Validation & Error Responses
- Invalid request (channel unsupported, empty text, too long, spam) → immediate `S2C_Error` with `ERROR_CODE_INVALID_REQUEST` and reason string.
- Missing sender entity/handle: log warn and send `S2C_Error` "player not in world" (protect against stale sessions).
- If publish fails due to event bus overflow, fall back to direct error response to client to avoid silent drops.

## Config & Constants
- `game.chat_local_radius` (int) with default 1000 (update config struct + yaml + docs).
- `chat.max_len` (default 256) and `chat.min_interval_ms` (default 400) stored in GameConfig → accessible from game/shard.
- Add derived `chatLocalRadiusSq` cached per shard for distance comparisons.

## Observability & Safety
- Structured logs: `sender_id`, `layer`, text length, recipients count, duration.
- Metrics: requests accepted/rejected, per-reason counters, queue depth from event bus, delivery latency histogram (request→deliver timestamp difference).
- Set up SLO alerts on rejection % and delivery latency p95.

## Implementation Checklist
1. Update proto/docs (ensure `S2C_ChatMessage` defined) + regenerate Go code.
2. Extend config (+ docs/spec) with chat settings; plumb into shard/game structs.
3. Network: implement `handleChatMessage` with validation, rate limiting hooks, event publishing.
4. ECS: add `systems.ChatSystem` subscribing to event bus, performing recipient discovery, using delivery service abstraction.
5. Delivery service: expose methods on shard or dedicated component to send to clients; ensure thread-safe read-only access.
6. Metrics/logging: counters for request/recipient counts, warning logs on rejects, debug logs gated via logger level.
7. Tests: unit tests for text validation and recipient selection; integration test covering in-radius delivery expectations.
