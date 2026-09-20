# Design

## Context

See [proposal.md](proposal.md) for motivation and the delta specs for the behavioral contract. The campfire definition currently starts its station as `burning`; burner initialization immediately gives it initial fuel and a server-runtime deadline. The existing `burner` behavior already owns context actions and persisted burner state, while the context-action service already provides target approach, one-cycle progress, cancellation, and completion through `ActiveCyclicAction`.

## Goals / Non-Goals

**Goals:**

- Keep the campfire's configured initial fuel but leave its deadline unarmed until ignition succeeds.
- Reuse the existing burner behavior and cyclic-action lifecycle so no network command or client protocol is introduced.
- Persist both unlit/unarmed and lit/armed states correctly across object reload and server restart.
- Notify station consumers after ignition so linked craft availability reflects the burning state.
- Make every applicable burner transition use one object-agnostic appearance convention so clients render its unlit and burning states correctly.

**Non-Goals:**

- Adding player-supplied tinder, tool requirements, new sounds, or a relighting flow after fuel exhaustion.
- Changing fuel capacity, refueling semantics for burning burners, fuel duration, cooking recipes, or the ash outcome.
- Reinterpreting already persisted active campfires as newly built unlit campfires.
- Adding per-object visual-resource fields to burner configuration, or allowing burner behavior to branch on a concrete object key such as `campfire`.

## Decisions

### Extend the existing burner behavior with ignition

The burner behavior will expose `Light my fire` only for an unlit burner with its configured initial fuel waiting to be activated. It will hide `Add fuel` in that state, ensuring that the menu has exactly the one required action. The same behavior will validate, start, and complete the cyclic action.

This keeps ignition adjacent to the stored fuel and its activation deadline. A separate campfire-only behavior would duplicate target, persistence, and burner-state checks and add another ordered behavior to the object definition.

### Represent an unarmed burner with no fuel deadline

For a newly spawned campfire in the `unlit` station state, burner initialization will persist the configured fuel amount with an unarmed deadline. The burner runtime and restore reconciliation will skip consumption and exhaustion while that deadline is unarmed. On successful ignition, the behavior will set the first deadline from the authoritative server-runtime clock.

The existing deadline is already durable burner state, so this needs no database or protocol schema migration. Persisted burners with an existing deadline remain active under their previous semantics; a restored legacy object with no burner state follows the current definition only when it is reconstructed.

### Complete ignition through one normal target-linked cycle

Selecting `Light my fire` creates an `ActiveCyclicAction` targeting the campfire and sets the player to the normal interacting state. It uses the established single-cycle duration used by simple target actions (10 ticks), with no repeated cycle. At completion, revalidate that the campfire is still unlit and unarmed, then consume the exact 50-stamina ignition cost before changing any campfire state. Ignition deliberately does not use the shared long-action reserve rule: a player with exactly 50 stamina can pay this explicitly specified cost.

If the cycle is interrupted, the target changes state, or stamina is insufficient at completion, cancel without changing fuel, burner deadline, or station state. Successful completion arms the deadline, changes the station to `burning`, marks the object state dirty, and publishes the existing station-state-changed event so craft availability refreshes.

### Leave the campfire definition as the initial-state authority

The campfire station definition will declare `unlit` as its initial state while retaining the configured initial fuel. The burner initialization will derive whether to arm its deadline from the spawned station state rather than adding a campfire-specific persistence field or a new behavior configuration flag.

This is sufficient because the current burner-backed object is the campfire, and it avoids configuration that can disagree with station state. If a future burner needs delayed ignition with different rules, it can introduce an explicit configuration extension then.

### Derive burner appearance resources from common object state

Whenever burner behavior transitions a station-backed burner between the shared `unlit` and `burning` states, it will update `Appearance.Resource` by composing the immutable object-definition key with that state: `{object}/unlit` or `{object}/burning`. For example, a campfire's client resource data declares `campfire/unlit` and `campfire/burning`, but the behavior never contains either literal or receives either path through its configuration. It needs only the current object's definition key and the common target state.

The transition will publish the existing entity-appearance event only when the derived resource differs from the current appearance. That event already sends an object-spawn upsert to visible clients, so no protocol message or client state store is added. Client resource data remains responsible for mapping the conventional resource names to visuals; the generic burner behavior is responsible only for deriving and publishing the resource name.

## Risks / Trade-offs

- [An unarmed zero deadline could be treated as overdue] → Guard both runtime burning and restore catch-up so only armed deadlines can consume fuel or trigger exhaustion.
- [Station state changes may not refresh dependent craft UI] → Publish the existing station-state-changed event after a successful ignition.
- [A burner transition may select a missing client resource] → Require each burner-backed object that uses the shared states to register both `{object}/unlit` and `{object}/burning`; cover the generic derivation with a non-campfire fixture and the campfire resource registration with an integration assertion.
- [Generic behavior could acquire object-specific exceptions] → Keep the appearance helper limited to the immutable object key and the two shared state values; reject new resource-path fields or concrete object-key branches in burner configuration and tests.
- [An action may be requested from stale UI state] → Validate unlit/unarmed state at action execution, during cyclic validity checks, and at completion.
- [Existing saved campfires could lose burning state] → Preserve any persisted burner deadline and station snapshot; only new spawn initialization creates the unarmed state.

## Migration Plan

1. Deploy the definition and behavior changes together.
2. Newly built campfires start unlit and require ignition; existing persisted campfires retain their saved station and burner state.
3. Roll back by restoring the previous definition and behavior logic; no persisted-data migration is required.
