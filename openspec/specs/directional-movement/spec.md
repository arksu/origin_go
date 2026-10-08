# directional-movement Specification

## Purpose

Provide continuous, screen-relative keyboard movement while the server remains authoritative over positions, speed, collisions, and gameplay restrictions. Bound the lifetime of held input so stale commands cannot move a player after control changes.

## Requirements

### Requirement: Physical WASD keys select screen-relative movement
While gameplay keyboard input is enabled, physical W, A, S, and D positions SHALL select up, left, down, and right relative to the screen, independent of keyboard layout. Simultaneous keys SHALL combine into one direction, with opposite directions canceling. The resulting direction SHALL be transformed to world coordinates and normalized there so all combinations use the same allowed world speed. Camera position, zoom, rendering frame rate, and operating-system key repeat SHALL NOT change the requested speed or direction.

#### Scenario: Screen directions in an isometric world
- **WHEN** the player holds W, D, or W+D
- **THEN** the projected movement SHALL point up, right, or up-right respectively, with equal world speed in each case

#### Scenario: Opposing keys
- **WHEN** W and S are held together, and then D is also pressed
- **THEN** the first combination SHALL request no movement and the second SHALL request rightward movement only

#### Scenario: Russian layout and repeated keydown
- **WHEN** the keyboard layout is Russian and the operating system repeats a held physical W key
- **THEN** the direction SHALL remain up and repeats SHALL NOT create additional input changes or increase packet rate

### Requirement: Directional input has a distinct additive wire contract
Directional movement SHALL use a dedicated `MoveDirection` player action containing a world direction, a nonzero input revision, and the current world stream epoch. The client SHALL normalize every nonzero world direction to unit length before serialization, subject to the precision of the wire float fields; each serialized component SHALL remain within -1 through 1. Refreshes SHALL repeat that normalized vector. Zero direction SHALL be serialized as (0,0) without normalization and SHALL mean release of directional control. The server SHALL advertise support in the enter-world snapshot; a client SHALL send directional input only after that support and an active world session are established. Existing protocol fields, retired reservations, map-click clients, and movement broadcasts SHALL remain compatible.

#### Scenario: Updated peers
- **WHEN** an updated client enters a world that advertises directional movement
- **THEN** it SHALL be able to start and stop WASD movement using the dedicated action without sending synthetic map clicks

#### Scenario: Diagonal direction is normalized on the wire
- **WHEN** the player holds W+D and the client sends the initial direction or a refresh
- **THEN** the decoded direction SHALL have unit length within float precision and components within -1 through 1, preserving the projected up-right direction without failing the server component-range check

#### Scenario: Mixed versions
- **WHEN** an updated client connects to a server without the support field, or an older client connects to an updated server
- **THEN** existing mouse movement SHALL continue to work, and the updated client SHALL leave WASD disabled when support is absent

### Requirement: Server validates direction and session before effects
The server SHALL reject non-finite direction values, values outside the supported component range of -1 through 1, revision zero, and input from an inactive connection, detached/dead player, wrong world, or obsolete stream epoch before changing movement, action state, or pending interactions. Every valid nonzero direction SHALL be normalized by the server and SHALL NOT supply speed, distance, or an authoritative position.

#### Scenario: Malformed vector
- **WHEN** a packet contains NaN, infinity, an out-of-range component, or revision zero
- **THEN** it SHALL produce no movement, action cancellation, interaction change, or extension of held input

#### Scenario: Obsolete connection or world
- **WHEN** a queued direction from the previous connection or stream epoch is processed after reconnect, teleport, or world transfer
- **THEN** it SHALL have no effect on the current player

#### Scenario: Client tries to increase speed
- **WHEN** the client sends any valid nonzero vector, including one with length greater than one
- **THEN** movement SHALL use the server-normalized direction and the server-selected movement speed

### Requirement: Revisions prevent superseded input from restarting movement
The client SHALL advance its input revision whenever the effective direction changes, including release, and SHALL repeat the same revision only to confirm an unchanged hold. An equal revision SHALL refresh only the identical currently active directional intent. A lower revision, a changed vector under an equal revision, or a repeated revision already stopped, rejected by gameplay restrictions, expired, or superseded SHALL NOT reactivate movement. Serial comparison SHALL handle uint32 wrap with zero reserved. A release SHALL affect only directional control and SHALL NOT cancel a point or object route installed later.

#### Scenario: Refresh does not restart an old hold
- **WHEN** a held direction has ended and another confirmation of that same revision arrives
- **THEN** the player SHALL remain under the newer control state

#### Scenario: Old release after mouse handoff
- **WHEN** a keyboard release is processed while movement is already owned by a point or object route
- **THEN** the route SHALL remain active

#### Scenario: Fresh input after a stop
- **WHEN** the player changes direction or presses a movement key again with a newer valid revision
- **THEN** the server SHALL evaluate it as fresh input under current gameplay restrictions

#### Scenario: Revision rollover
- **WHEN** a valid revision wraps from uint32 maximum to the next nonzero revision
- **THEN** it SHALL be treated as newer while delayed pre-wrap input remains obsolete

### Requirement: Held direction expires without fresh confirmation
The client SHALL send effective input changes immediately and refresh an unchanged nonzero direction every 200 ms while input remains enabled. The server SHALL expire directional control no later than the first simulation tick at or after 800 ms without a valid refresh received by the server. Queue residence SHALL consume this lifetime rather than extend it. Already expired nonzero commands SHALL NOT start movement when drained later; a later refresh of that retired revision SHALL NOT restart it. No timer SHALL send catch-up bursts after suspension.

#### Scenario: Release never reaches the server
- **WHEN** a connection silently stalls or the final release command is dropped
- **THEN** directional movement SHALL end by the first tick at or after the 800 ms deadline from the last accepted fresh receipt

#### Scenario: Backlogged movement command
- **WHEN** a direction waits in the command queue beyond its lifetime
- **THEN** it SHALL neither move the player nor cancel an action, and a refresh of that same revision SHALL not turn it into a new hold

#### Scenario: Expiry while blocked
- **WHEN** collision has stopped the player, directional confirmations cease, and the input deadline passes
- **THEN** the held direction SHALL expire while the player is stationary, and subsequently removing the obstacle SHALL not resume movement without fresh input

#### Scenario: Normal command volume
- **WHEN** one direction is held without changes
- **THEN** the client SHALL send at most five scheduled refreshes per second, independent of frame rate and key repeat

### Requirement: Directional movement obeys existing world rules
Directional motion SHALL use existing server speed modes, movement restrictions, collision resolution, carry restrictions, stamina accounting, world boundaries, and chunk transitions. Directional input SHALL NOT create a click destination, pickup request, link intent, or point-arrival event. An existing active link SHALL be broken through its normal lifecycle before manual movement starts. Stationary collision contact SHALL NOT spend movement stamina or trigger an interaction.

#### Scenario: Modes and diagonal speed
- **WHEN** the player moves in any WASD combination while walking, running, crawling, swimming, or carrying an object
- **THEN** allowed speed and movement cost SHALL follow the same server rules as ordinary movement

#### Scenario: Held direction reaches a wall
- **WHEN** collision fully blocks movement while fresh input remains held
- **THEN** the authoritative position SHALL stop, the movement broadcast SHALL report no actual movement, and the held direction SHALL remain available for subsequent collision checks

#### Scenario: Obstacle disappears
- **WHEN** an obstacle blocking a still-valid held direction disappears
- **THEN** movement SHALL resume through normal collision resolution without requiring key repeat or a new key press

#### Scenario: Restricted player
- **WHEN** a stun, knockout, death, or inability to move prevents or ends manual movement
- **THEN** the player SHALL not move, the old directional revision SHALL be retired, and recovery SHALL require fresh input

#### Scenario: No implicit world interaction
- **WHEN** WASD movement contacts an object or passes over a dropped item
- **THEN** it SHALL NOT automatically link, pick up, place, or activate that object

### Requirement: Focus and lifecycle changes clear keyboard control
The client SHALL stop active directional control when focus enters an input, textarea, select, editable content, or a blocking game modal; when the window blurs or the document becomes hidden; when the world or connection ends; and when the renderer is destroyed. WASD SHALL not capture text input, composition events, or Ctrl/Alt/Meta shortcuts. Shift SHALL not change the selected speed mode. Reset SHALL clear repeat timers and suppress held keys until a fresh physical press; focus return or world re-entry SHALL NOT resume movement automatically. Release events SHALL still clear tracked keys even when their targets are editable.

#### Scenario: Open chat while walking
- **WHEN** a player holds W and focuses chat
- **THEN** keyboard movement SHALL be released, typing SHALL not move the player, and closing chat SHALL not resume W automatically

#### Scenario: Keyup occurs inside an input
- **WHEN** focus changes while a movement key is held and its keyup targets an editable element
- **THEN** that key SHALL be cleared rather than remaining stuck

#### Scenario: Hide or destroy the game
- **WHEN** the player changes browser tabs, loses window focus, disconnects, or leaves the game view
- **THEN** no active keyboard timer SHALL continue sending directions, and returning SHALL require fresh movement input

#### Scenario: Key release was missed outside the window
- **WHEN** the player released a key while the window was unfocused, its keyup was not delivered, and a fresh non-repeated press occurs after returning
- **THEN** the new press SHALL be accepted without reviving other suppressed keys or requiring an extra press

#### Scenario: Browser shortcut
- **WHEN** Ctrl, Alt, or Meta is used with a movement key, or text composition is active
- **THEN** the game SHALL not start or continue keyboard movement from that shortcut or composition

### Requirement: Directional motion uses existing presentation and bounded work
Authoritative direction starts, turns, and stops SHALL use the existing movement stream for both the player and observers. Directional movement SHALL NOT create a destination marker; an existing one-shot ring from a prior point route MAY finish fading. A blocked held direction SHALL not appear as permanent walking or extrapolate through a wall. Implementations SHALL avoid per-frame network input and unbounded timer, listener, queue, or world-lifecycle accumulation. Existing interpolation and locomotion transition durations SHALL remain unchanged in this change; measured visual latency SHALL be reported separately from server stopping latency.

#### Scenario: Start from a click route
- **WHEN** WASD replaces a route with a visible target marker
- **THEN** the next authoritative directional movement update SHALL omit a destination, create no new ring, and allow the prior ring to finish its one-shot fade

#### Scenario: Player and observer stop
- **WHEN** directional motion stops or becomes fully blocked
- **THEN** both clients SHALL converge to the collision-resolved server position and standing animation without continued extrapolation

#### Scenario: Repeated world entry
- **WHEN** the client repeatedly enters and leaves a world
- **THEN** one active game session SHALL still have only one set of keyboard listeners and at most one refresh timer
