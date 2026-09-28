# Spec Delta

## Purpose

Batches the server→client object-state stream: movement updates and object spawn updates are delivered as one batch message per client per delivery occasion instead of one message per entity, cutting WebSocket frame count and per-message overhead while preserving the per-entity semantics (stream epoch, move sequence, teleport flag, carried-by relation) and per-entry error isolation the client already relies on.

## ADDED Requirements

### Requirement: Movement updates are delivered as one batch message per observer per tick
The server SHALL deliver, for each observing client, all movement updates of entities visible to that observer that were produced in the same server tick as a single `S2C_ObjectMoveBatch` message. Each entry SHALL carry the same per-entity data as the single movement message it replaces: entity id, position, heading, velocity, move mode, is-moving flag, optional target position, server timestamp, per-entity monotonically increasing move sequence, teleport flag, and carried-by entity id. Movement of entities visible to no observer SHALL NOT be sent.

#### Scenario: Multiple moving entities visible to one observer
- **WHEN** three entities visible to a client move during one server tick
- **THEN** the client receives exactly one `S2C_ObjectMoveBatch` message containing three entries, not three separate movement messages

#### Scenario: Movement visible to several observers
- **WHEN** a moving entity is visible to two clients
- **THEN** each client receives a batch message containing that entity's movement entry

#### Scenario: Movement with no observers
- **WHEN** an entity moves and no client can see it
- **THEN** no movement entry for that entity is sent to any client

### Requirement: Spawn updates are delivered as batch messages
The server SHALL deliver all object spawn updates — visibility-transition spawns, forced visibility updates, reattachment spawn bursts, and appearance-change upserts — as `S2C_ObjectSpawnBatch` messages. Each delivery occasion for one observer SHALL produce at most one spawn batch message; each entry SHALL carry the same data as the single spawn message it replaces: entity id, type id, resource path, position and size, carried-by entity id, character visual state, action animation state, display name and name color. Each spawn batch message SHALL carry the stream epoch of the receiving client's stream, and the client SHALL gate the whole message on it.

#### Scenario: Client enters the world
- **WHEN** a client's area of interest is populated by its first visibility update
- **THEN** the spawn burst for that client is delivered as one `S2C_ObjectSpawnBatch` message containing one entry per newly visible entity

#### Scenario: Appearance change on a visible object
- **WHEN** a visible object's appearance changes and all its observers are resolved
- **THEN** each observer receives an upsert spawn entry via `S2C_ObjectSpawnBatch`

#### Scenario: Stream epoch mismatch
- **WHEN** a spawn batch message arrives with a stream epoch different from the client's current stream
- **THEN** the client discards the entire message, as it does for single spawn messages

### Requirement: Entries are evaluated independently
The server SHALL validate and build each batch entry independently at dispatch time; an entry whose target is no longer alive, no longer visible to the observer, or whose snapshot cannot be built SHALL be skipped without affecting other entries in the batch. On the client, a failure while applying one entry SHALL be contained to that entry and SHALL NOT prevent the remaining entries of the same message from being applied.

#### Scenario: Target dies between visibility computation and dispatch
- **WHEN** an entity that became visible to an observer is destroyed before its spawn entry is built
- **THEN** that entry is omitted from the batch and all other entries are delivered

#### Scenario: Client fails to apply one entry
- **WHEN** applying one entry of a batch message raises an error on the client
- **THEN** the error is contained to that entry and the remaining entries of the message are still applied

### Requirement: Per-client message order and per-entry currency are preserved
The server SHALL enqueue messages to each client in dispatch order, and the per-entry currency rules SHALL be unchanged from single-message delivery: the client applies entries in message order, rejects movement entries whose move sequence is older than the last applied sequence for that entity, and treats a carried-to-not-carried transition as a position snap. The batching change SHALL NOT introduce new cross-message ordering guarantees beyond per-client FIFO.

#### Scenario: Stale movement entry
- **WHEN** a movement entry arrives whose move sequence is older than the client's last applied sequence for that entity
- **THEN** the client discards that entry as it does for single movement messages

#### Scenario: Message order end-to-end
- **WHEN** the server enqueues several messages to one client, including batch messages
- **THEN** the client observes them in the order they were enqueued

### Requirement: Batch delivery criticality matches current per-entry policy
A spawn batch message SHALL be sent on the critical send path when any of its entries carries an action animation (overflow of the client's send buffer then ends the connection), and on the droppable send path otherwise. Movement batch messages SHALL be sent on the droppable send path. Chunk load and chunk unload delivery SHALL be unchanged.

#### Scenario: Critical spawn batch overflows the send buffer
- **WHEN** a spawn batch containing an action-animation entry cannot be enqueued because the client's send buffer is full
- **THEN** the server closes that client's connection instead of silently dropping the batch

#### Scenario: Non-critical batch overflows the send buffer
- **WHEN** a movement batch or a spawn batch without action-animation entries cannot be enqueued because the send buffer is full
- **THEN** the message is dropped and the connection stays open, as with single messages today

### Requirement: Client continues to accept single spawn and movement messages
The client SHALL continue to accept and process single `S2C_ObjectSpawn`, `S2C_ObjectMove`, and `S2C_ObjectDespawn` messages with unchanged behavior, so frames already in flight during a server update and older server builds remain interoperable.

#### Scenario: Single movement message after a batch
- **WHEN** a single `S2C_ObjectMove` message arrives for a known entity
- **THEN** the client applies it through the same per-entity movement handling used for batch entries
