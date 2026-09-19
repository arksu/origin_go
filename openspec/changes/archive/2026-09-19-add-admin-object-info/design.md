# Design

## Context

See proposal.md for motivation. Admin commands already keep per-player pending state for `/spawn` and coordinate-only `/tp`; the network command system consumes those states before regular movement. The current renderer can pick a visible object but ordinarily uses that picker for right-click interaction only. A mutable object's durable and behavior-owned values live in `ObjectInternalState`, whose `RuntimeObjectState.Behaviors` map contains the campfire's `BurnerBehaviorState` including `Fuel`.

## Goals / Non-Goals

**Goals:**

- Reuse the existing chat command, object picker, interaction packet, and server authority rather than introducing a protocol message.
- Make the client mode one-shot and make the server the sole authority for whether a selected entity is inspectable and what it reports.
- Produce a stable, readable serialization of mutable runtime state, including behavior keys and values.

**Non-Goals:**

- Editing object state, inspecting object definitions, exposing persistent storage records, adding a general developer UI, or changing ordinary left-click movement/interactions.
- Revealing template-derived configuration, visual resources, static metadata, or a full arbitrary ECS-component dump.

## Decisions

### 1. Arm the client locally for exact `/info`, then reuse object picking and `Interact`

When the chat submitter sends the exact no-argument `/info` command, the game view enables a transient primary-click selection mode on the renderer. That mode asks the existing object picker for the topmost visible entity and sends the existing `Interact` command for it. For an empty-ground click it sends the existing `MoveTo` command; the server uses the pending inspection state to consume that command as a failed selection. In either case it consumes the local click so it does not perform normal movement, drop a held item, or open normal interaction. Escape and the first primary click cancel the local mode.

The server checks its own pending inspection state in the `Interact` path before normal interaction and consumes it there. This preserves authority: a forged interaction cannot produce a report unless the server accepted a preceding `/info` from that player. The implementation must not rely on parsing a server chat prompt as an acknowledgment.

Alternative: send a new inspection protocol message or a server-to-client armed-state packet. That would make synchronization explicit but is unnecessary because the existing target-bearing interaction packet and server-side guard provide the required authority. Alternative: change all object clicks to `MoveToEntity`; that would alter ordinary gameplay behavior.

### 2. Add an isolated pending-inspection resource and resolve the actual target entity

Add a `PendingAdminObjectInfo` resource keyed by player ID, parallel to the existing pending spawn and teleport resources. `/info` clears spawn and teleport pending state before setting inspection; `/spawn` and both `/tp` forms clear inspection when they supersede it. Player cleanup on death/disconnect clears inspection as well.

The network system consumes a pending inspection when it receives a target-bearing interaction, and it consumes/reports a failed selection when it receives `MoveTo` while inspection is pending. The latter is the existing targetless click path and preserves ordinary movement for every player without pending inspection.

Alternative: leave inspection armed after a ground click. This makes recovery convenient but violates the stated one-shot behavior and can accidentally consume a later object click.

### 3. Report a constrained runtime-state snapshot through chat

The command handler obtains the selected entity's `ObjectInternalState` under the shard's normal command execution lock and creates a diagnostic DTO containing only the entity ID, state flags/dirty marker, and the current runtime state's behavior map and station mutable state. It serializes this DTO with deterministic JSON ordering and sends it only to the requesting administrator as one or more ordered server chat messages if needed for readability or transport limits.

No object definition lookup is performed for report content. In particular, the report must not add a definition key/name, configured behavior list, appearance resource, burner capacity, burn interval, or initial fuel. If runtime state is absent or cannot be serialized, return a clear diagnostic message rather than panic or emit partial template data.

Alternative: reflect and dump every ECS component. That may expose unrelated player/inventory data and makes the command unstable as implementation details evolve. Alternative: use object-definition data to label or enrich behavior output; it conflicts with the requested template-data boundary.

## Risks / Trade-offs

- [Client arms locally before server accepts `/info`] -> The server ignores target interactions without its pending entry; limit client arming to the exact no-argument command and cancel it on the first click or Escape.
- [A long future behavior state exceeds comfortable chat display length] -> Split only at serialization-safe boundaries into ordered messages, retaining the entity ID/header on each part.
- [A behavior introduces a non-JSON-serializable runtime value] -> Treat serialization failure as a reported diagnostic and add a regression test; do not use reflection to bypass it.
- [Pending click modes conflict] -> Clear all other admin click modes when arming one and test each replacement direction.

## Migration Plan

1. Add the pending resource, `/info` command handling, server interaction interception, cleanup, and constrained formatter.
2. Add the client one-shot selection mode using the existing picker and interaction command.
3. Run focused client and server tests, then deploy without data or protocol migration.
4. Roll back by removing the command and transient resource; no persisted object or network data requires migration.
