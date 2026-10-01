# s2c-packet-batching Specification

## Purpose

Reduce WebSocket messages in initial visibility-spawn bursts, ordinary movement, and periodic carried-object following while delivering the same entity state. Occasional updates remain independent, and restoration must not duplicate initial spawns through appearance notifications.

## Requirements

### Requirement: Ordinary movement is aggregated per observer per simulation update
The server SHALL send each observing client at most one message containing its visible ordinary-movement entries from one simulation update. Two or more entries SHALL use `S2C_ObjectMoveBatch`; one entry SHALL use the existing single movement message, and zero entries SHALL produce no message. Each observer SHALL receive only its visible subset. This requirement SHALL NOT require combining ordinary movement with periodic carried-object following or occasional movement transitions.

#### Scenario: Multiple moving entities visible to one observer
- **WHEN** three entities visible to a client produce ordinary movement updates in one simulation update
- **THEN** the client receives exactly one `S2C_ObjectMoveBatch` message containing three entries, not three separate movement messages

#### Scenario: Movement visible to several observers
- **WHEN** several moving entities have different observing clients
- **THEN** each client receives one message containing exactly its visible subset of those movement entries

#### Scenario: Movement with no observers
- **WHEN** an entity moves and no client can see it
- **THEN** no movement entry for that entity is sent to any client

### Requirement: Periodic carried-object following is aggregated separately
The server SHALL send each observing client at most one message for the visible carried-object position updates produced by one periodic follow pass. Two or more entries SHALL use `S2C_ObjectMoveBatch`; one entry SHALL use a single movement message. An unchanged carried-object position SHALL NOT produce a periodic movement entry. Periodic following SHALL NOT send both an individual movement message and a batch entry for the same relocation. Ordinary movement and periodic following SHALL be allowed to produce separate messages in the same tick.

#### Scenario: Several players carry objects while moving
- **WHEN** a client sees 100 ordinary movers and 100 carried objects whose positions change in the same tick
- **THEN** it receives one ordinary-movement batch and one carried-object movement batch, with all 200 entries delivered once across those periodic streams
- **AND** these streams produce two messages for that client, excluding independent gameplay transitions

#### Scenario: Carriers remain stationary
- **WHEN** no carried object's position changes during the periodic follow pass
- **THEN** that pass produces no movement message

#### Scenario: Pickup or drop occurs without a position change
- **WHEN** an explicit pickup or drop requires a carry-relation transition at the same coordinates
- **THEN** the existing immediate transition is still delivered and is not suppressed as a stationary periodic update

### Requirement: Visibility spawn bursts are aggregated per observer
The server SHALL send each observer at most one spawn message for each nonempty visibility-update result, forced visibility-update result, or reattachment snapshot after filtering invalid entries. Two or more valid entries SHALL use `S2C_ObjectSpawnBatch`; one entry SHALL use the existing single spawn message. The message SHALL contain each valid target in that result once. Empty results SHALL produce no message. Chunks entering visibility in different updates SHALL be allowed separate messages; occasional appearance upserts SHALL NOT be required to join these bursts.

#### Scenario: Client enters the world
- **WHEN** a client's first visibility update discovers N valid entities, where N is greater than one
- **THEN** the client receives one `S2C_ObjectSpawnBatch` containing all N entities once

#### Scenario: Another chunk becomes visible after login
- **WHEN** a loaded chunk contributes several newly visible valid entities in one visibility update
- **THEN** the observer receives their spawn entries in one message, even though the client is already in the world

#### Scenario: Reattachment restores known objects
- **WHEN** a client reattaches with several currently visible valid entities in its snapshot
- **THEN** the snapshot is delivered in one spawn batch with one entry per target

#### Scenario: Forced visibility update discovers several objects
- **WHEN** a forced visibility update discovers several newly visible valid entities
- **THEN** their spawn entries are delivered in one message

### Requirement: Initial appearance changes do not duplicate visibility spawns
Appearance changes made while an object has no observers SHALL update its authoritative state without scheduling a network appearance notification. The decision SHALL use visibility at publication time, so an observer appearing before asynchronous delivery SHALL NOT turn that earlier unobserved change into an extra upsert. The object's later initial spawn SHALL carry its final appearance. Appearance changes to already observed objects SHALL retain their existing upsert delivery and dispatch-time visibility checks.

#### Scenario: Chunk restoration resolves mature trees and filled containers
- **WHEN** restoration changes the appearance of N initially unobserved objects and the next visibility update makes them visible to one client, with no unrelated appearance changes
- **THEN** the initial spawn delivery contains all valid objects with their final resources
- **AND** the restoration changes produce no additional appearance-upsert messages after the initial spawn delivery

#### Scenario: Appearance change on a visible object
- **WHEN** an observed object's appearance changes and an observer still sees it at delivery time
- **THEN** that observer receives the existing appearance-upsert spawn update, which can remain a single message

### Requirement: Batch entries retain existing payload and epoch semantics
Each spawn entry SHALL preserve the existing spawn fields, including its own `stream_epoch`, character visual and action-animation state, name, and carry relation. The server SHALL stamp spawn entries with the receiving client's current stream epoch, and the client SHALL apply the existing epoch check independently to each entry. The spawn batch SHALL NOT introduce a second outer epoch. Movement entries SHALL preserve all existing fields, including position, heading, velocity, mode, target, timestamp, current sequence value, teleport flag, and carry relation; batching SHALL NOT add a movement epoch or change sequence generation.

#### Scenario: Stream epoch mismatch
- **WHEN** a spawn entry in a batch has an epoch different from the client's current stream
- **THEN** the client ignores that entry through the same check used for a single spawn
- **AND** any valid entries in that batch continue to be evaluated independently

#### Scenario: Immediate relocation has its existing default sequence
- **WHEN** a movement entry carries the sequence and teleport values produced by the existing relocation path
- **THEN** its single or batched delivery preserves those values without inventing a new sequence policy

### Requirement: Entries are evaluated independently
The server SHALL validate and build spawn entries independently at dispatch time. A spawn target that is dead, no longer matches its recorded identity, no longer visible, or cannot yield a valid snapshot SHALL be skipped without affecting other entries. Movement routing SHALL preserve the existing visibility filtering of authored movement entries. On the client, a failure while applying one entry SHALL be contained to that entry and SHALL NOT prevent remaining entries from being applied.

#### Scenario: Target dies between visibility computation and dispatch
- **WHEN** an entity that became visible to an observer is destroyed before its spawn entry is built
- **THEN** that entry is omitted from the batch and all other entries are delivered

#### Scenario: All spawn targets are invalid at dispatch
- **WHEN** every target in a spawn result fails validation or is no longer visible
- **THEN** no empty spawn message is sent

#### Scenario: Client fails to apply one entry
- **WHEN** applying one entry of a batch message raises an error on the client
- **THEN** the error is contained to that entry and the remaining entries of the message are still applied

### Requirement: Batch processing preserves existing entry handling and enqueue order
The client SHALL process batch entries in their listed order through the same entry handling as single messages. Existing movement sequence filtering, teleport handling, carry-drop snapping, and downstream store/carry updates SHALL remain unchanged. This change SHALL NOT add a stronger guarantee that a stale movement entry is rejected before every state mutation. Snapshot delivery SHALL preserve existing bootstrap and animation-state ordering, and clients SHALL observe messages in server enqueue order; no new ordering across asynchronous producers is required.

#### Scenario: Stale movement entry
- **WHEN** an older movement entry is delivered once as a single message and once as a batch entry from equivalent client state
- **THEN** both deliveries have the same movement-controller and downstream store/carry effects

#### Scenario: Carried object is dropped
- **WHEN** a movement entry changes a known object from carried to not carried
- **THEN** single and batched handling both preserve the existing position-snap behavior

#### Scenario: Message order end-to-end
- **WHEN** the server enqueues several messages to one client, including batch messages
- **THEN** the client observes them in the order they were enqueued

### Requirement: Queued batches retain their authored data
Batch contents awaiting asynchronous delivery SHALL NOT be overwritten by subsequent simulation updates. This SHALL apply to ordinary movement, periodic carry-follow, and spawn target lists.

#### Scenario: The next update runs before delivery
- **WHEN** a batch is queued and its producer computes different entries for the next update before delivery completes
- **THEN** the earlier batch retains its original target list and authored movement values

### Requirement: Batch delivery criticality matches current per-entry policy
A spawn batch message SHALL be sent on the critical send path when any of its entries carries an action animation (overflow of the client's send buffer then ends the connection), and on the droppable send path otherwise. Movement batch messages SHALL be sent on the droppable send path. Chunk load and chunk unload delivery SHALL be unchanged.

#### Scenario: Critical spawn batch overflows the send buffer
- **WHEN** a spawn batch containing an action-animation entry cannot be enqueued because the client's send buffer is full
- **THEN** the server closes that client's connection instead of silently dropping the batch

#### Scenario: Non-critical batch overflows the send buffer
- **WHEN** a movement batch or a spawn batch without action-animation entries cannot be enqueued because the send buffer is full
- **THEN** the message is dropped and the connection stays open, as with single messages today

### Requirement: Single messages and occasional independent updates remain supported
The client SHALL continue processing single spawn, movement, and despawn messages with their existing behavior. The server SHALL retain single-message delivery where only one entry is sent. Occasional link, lift-stop, pickup, put-down, forced-drop, and transfer updates SHALL retain their existing immediate publication behavior without waiting for a periodic aggregation timer. Support for singles SHALL NOT imply that an old client can decode the new batch types.

#### Scenario: Single movement message after a batch
- **WHEN** a single `S2C_ObjectMove` message arrives for a known entity
- **THEN** the client applies it through the same per-entity movement handling used for batch entries

#### Scenario: An occasional transition accompanies periodic batches
- **WHEN** a link or lift transition occurs in a tick that also produces ordinary and carried-object movement batches
- **THEN** it can be delivered independently without violating the periodic batching requirements

### Requirement: Validation distinguishes fewer messages from missing updates
Validation reports SHALL distinguish WebSocket message count, bytes, and decoded entity-entry count. A run claiming complete entry counts or position tracking SHALL decode messages for its entire duration. Runs that deliberately stop decoding SHALL label entry counts as partial rather than report them as total delivered updates.

#### Scenario: Validation exceeds the first hundred messages
- **WHEN** a full-content validation run receives more than 100 messages, including batches
- **THEN** later batch entries still contribute to entry counts and player-position tracking

#### Scenario: A spawn burst is reduced without losing objects
- **WHEN** N valid entities, with N greater than one, are restored into one visibility result without unrelated updates
- **THEN** validation reports all N expected entity entries and one spawn message, with no restoration-triggered appearance duplicates
