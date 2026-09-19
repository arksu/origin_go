# Proposal

## Why

Administrators cannot inspect the authoritative live state of a world object without server-side debugging. In particular, a campfire's remaining burner fuel exists only in its runtime behavior state and is not visible through the current chat commands.

## What Changes

- Add the administrator chat command `/info`, which arms a one-shot object-inspection click.
- Arm the game client for the next primary click, resolve the clicked live world object, and send the requesting administrator a detailed, deterministic chat report of that object's current runtime state.
- Include the object's current behavior-owned state and other mutable object state, such as a burner's `fuel`, without emitting immutable object-template configuration.
- Report invalid or disappeared targets clearly and clear the pending inspection so a later normal click is not consumed.

## Capabilities

### New Capabilities
- `admin-object-inspection`: Lets an administrator inspect a clicked world object's authoritative mutable runtime state through chat.

### Modified Capabilities
- None.

## Impact

- Admin command parsing and its per-player pending-click resources.
- Client-side one-shot object picking, existing interaction-command routing, and server-side transient-state cleanup.
- Runtime object-state formatting and focused client/server tests; no new wire protocol or object-template fields are required.
