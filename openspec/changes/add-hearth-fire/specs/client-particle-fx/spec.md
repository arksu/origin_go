# Spec Delta

## MODIFIED Requirements

### Requirement: Smoke renders while a campfire is burning
The client SHALL render a rising, drifting continuous particle smoke effect attached to a campfire's view whenever the campfire's appearance resource is the burning state, and SHALL NOT attach that continuous smoke effect for any other campfire state. Smoke SHALL rise from the fire area, drift with the ambient wind, and animate continuously while the state holds. A separate explicit `skull_smoke` conversion event SHALL remain independent of this state-attached effect.

#### Scenario: Burning campfire shows smoke
- **WHEN** the client renders a campfire whose appearance resource is `campfire/burning`
- **THEN** a continuous smoke particle effect SHALL be visible attached to that campfire's view, rendering above its flame layer

#### Scenario: Unlit campfire shows no smoke
- **WHEN** a campfire's appearance resource is not the burning state
- **THEN** no state-attached continuous smoke effect SHALL be rendered for that campfire
- **AND** an explicit `skull_smoke` conversion event SHALL be handled separately without making that state continuously emit smoke

#### Scenario: Smoke fades out when fire goes out
- **WHEN** a burning campfire's appearance changes to the unlit state and its view is rebuilt
- **THEN** the smoke effect SHALL stop spawning immediately, while its existing puffs continue to drift and fade out before removal

## ADDED Requirements

### Requirement: Shared skull smoke renders once at the event position
The client SHALL render the reusable `skull_smoke` effect once at the authoritative world position of a permanent character death or an active hearth's conversion to an ordinary unlit campfire. Owner death and replacement by a newly completed hearth SHALL use the same effect. Rendering SHALL NOT require the source object view to survive the transition.

#### Scenario: Permanent character death emits smoke at the character
- **WHEN** a character permanently dies and the client receives the accepted death effect event
- **THEN** `skull_smoke` SHALL appear at that character's event position

#### Scenario: Hearth replacement emits smoke at the old hearth
- **WHEN** completing a new hearth converts the previous active hearth into an ordinary unlit campfire
- **THEN** the accepted conversion event SHALL render `skull_smoke` at the old hearth's position

#### Scenario: Owner death emits smoke at the converted hearth
- **WHEN** permanent owner death converts the owner's active hearth into an ordinary unlit campfire
- **THEN** the accepted conversion event SHALL render the same `skull_smoke` effect at that hearth's position independently of the character death effect

#### Scenario: Source view replacement does not erase the event
- **WHEN** a corpse or campfire appearance replaces the source view while its accepted skull-smoke event is active
- **THEN** the effect SHALL retain its event position and remaining lifetime without restarting

### Requirement: Transient world effects reach current observers without replay
The server SHALL send one skull-smoke event to the current observers of a runtime death or Hearth conversion, using the event position and each recipient's world-entry epoch. The client SHALL accept known effects with finite positions only for its current entry. Later visibility, loading, rebuilding, or reconnecting SHALL NOT replay the event. No application-level effect retransmission or durable history SHALL be required.

#### Scenario: Current observers receive the event
- **WHEN** a visible character permanently dies or a visible active hearth is converted
- **THEN** all players who can see that source at the event time SHALL receive its skull-smoke event
- **AND** players who cannot see that source SHALL NOT receive it merely because they own the hearth or are nearby on another world layer

#### Scenario: Later visibility does not replay smoke
- **WHEN** a player later loads, sees, or rebuilds the resulting corpse or ordinary unlit campfire
- **THEN** the client SHALL NOT infer or replay a historical skull-smoke event from that object's state

#### Scenario: Runtime transition emits only one event
- **WHEN** a character death or Hearth conversion has already occurred and a later persistence acknowledgment or visibility refresh is processed
- **THEN** the server SHALL NOT emit another skull-smoke event for that transition

#### Scenario: A previous world-entry event is ignored
- **WHEN** a received transient event belongs to a previous world entry
- **THEN** the client SHALL NOT render it in the current world

#### Scenario: Invalid events create no presentation
- **WHEN** a transient event has an unknown effect key, invalid world position, or invalid world-entry epoch
- **THEN** the client SHALL ignore it without creating objects or particles

### Requirement: Skull smoke has a finite two-second lifetime
Skull smoke SHALL use the eight approved frames once, at 250 milliseconds per frame, and disappear 2000 milliseconds after local playback starts. Its local monotonic deadline SHALL continue while rendering is suspended. Delivery delay SHALL NOT require server-clock synchronization. Visibility changes and source-view rebuilding SHALL NOT restart playback; shared assets SHALL be prepared through the existing loader.

#### Scenario: Smoke finishes within two seconds
- **WHEN** a locally playing skull-smoke event reaches its 2000 millisecond deadline
- **THEN** its animation SHALL have ended and every sprite for that effect SHALL be gone

#### Scenario: Hidden or suspended effects still expire
- **WHEN** skull smoke leaves the viewport or rendering is suspended beyond its expiry
- **THEN** resuming visibility or rendering SHALL show no particles from that expired event
- **AND** the event SHALL NOT replay missed frames

#### Scenario: Delayed delivery starts ordinary local playback
- **WHEN** a valid current-entry event is delivered after a transport delay
- **THEN** the client SHALL play its one-shot animation using the local two-second lifetime
- **AND** no server-authored timestamp or effect-event sequence SHALL be required

#### Scenario: Missing assets do not create a historical queue
- **WHEN** an event arrives before its shared assets are ready
- **THEN** the client MAY drop that cosmetic event rather than queue an unbounded backlog
- **AND** loading the assets later SHALL NOT replay previously dropped events

### Requirement: Transient effects have bounded resources and clear on reset
The client SHALL bound concurrent transient world effects and use shared asset loading without an unbounded event queue. Capacity exhaustion SHALL drop cosmetic work without affecting gameplay. World entry or leave, disconnect, session changes, and renderer destruction SHALL clear active transient effects and invalidate pending completions from the previous world or session. The renderer SHALL accept an effect key and event position without Hearth-specific state.

#### Scenario: Capacity exhaustion remains cosmetic
- **WHEN** the transient-effect capacity limit is reached
- **THEN** the client SHALL enforce that limit without unbounded queuing or resource growth
- **AND** gameplay state and later valid events SHALL remain usable

#### Scenario: World and session cleanup removes transient effects
- **WHEN** world entry or leave, disconnect, a session change, or renderer destruction resets presentation
- **THEN** all active transient world effects SHALL be removed and their resources released

#### Scenario: Pending asset loads cannot revive the previous world
- **WHEN** an asset load for an old transient event completes after its world or session was reset
- **THEN** that completion SHALL NOT create effect sprites in either the old world or the new world
