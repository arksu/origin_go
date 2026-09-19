# Proposal

## Why

The client currently recognizes `/info` and `/destroy` in chat and arms a special selection flag. A map click should describe user input independently of administrator commands; the server should interpret it using its own pending state. The unused client `MoveToEntity` sender also leaves a redundant protocol and server routing path.

## What Changes

- Introduce `MapClick { x, y, target_entity_id }` for ordinary primary map clicks; zero target means empty ground. Keep modifiers on the enclosing player action.
- Preserve ordinary movement to the clicked coordinates regardless of the target ID, and preserve ordinary dropped-item pickup through server interpretation.
- Handle pending `/spawn`, `/tp`, `/info`, and `/destroy` before normal click behavior on the server; consume the click without also moving or picking up items.
- Remove command strings, selection flags, and arm/cancel methods from client input/chat handling.
- **BREAKING**: Replace the `MoveTo` wire action with `MapClick` and delete `MoveToEntity`, its sender, queue command, and server handler. Reserve retired protobuf action numbers and names; migrate all repository senders and generated bindings together.
- Preserve explicit `Interact` for ordinary context interaction; it no longer selects targets for pending `/info` or `/destroy`.

## Capabilities

### New Capabilities
- `map-click-input`: Neutral map-click transport, ordinary input semantics, server-only administrator click dispatch, and retirement of legacy movement wire actions.

### Modified Capabilities
- `admin-object-inspection`: Require server-only selection through `MapClick`, without client command recognition or special selection state.

## Impact

Affected areas are `api/proto/packets.proto`, generated Go/TypeScript bindings, `web_new` rendering/input/chat wiring, server player-action routing and network command processing, administrator pending state lifecycle, and `cmd/load_test` plus its documentation. Tests must cover both ordinary clicks and administrator interception. Existing uncommitted `/destroy` work is an input to this proposal and must be preserved. This change does not redesign object persistence or inventory deletion.
