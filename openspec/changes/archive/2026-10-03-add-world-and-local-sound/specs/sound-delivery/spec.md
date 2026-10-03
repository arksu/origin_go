# Sound Delivery Spec Delta

## Purpose

Deliver distant world sounds using server-authoritative hearing rules while keeping frequent quiet sounds entirely on the client, with authored audio settings and bounded processing under load.

## ADDED Requirements

### Requirement: Sound profiles declare delivery and audibility

Each sound SHALL have one canonical authored profile declaring a stable key, explicit `world` or `local` mode, finite positive `loudness` in world-distance units, separate playback volume, assets and playback settings. Server and client catalogs SHALL derive from those profiles. Playback volume and user volume controls SHALL NOT determine delivery mode or audibility distance.

#### Scenario: A user reduces playback volume
- **WHEN** a listener changes their master or sound-effects volume
- **THEN** world recipient selection SHALL remain unchanged while audible playback follows the selected volume

#### Scenario: A profile is changed
- **WHEN** a valid authored sound profile changes loudness or delivery settings and matching catalogs are published
- **THEN** the server and client SHALL apply the published settings without a second authored sound mapping

### Requirement: Invalid audio definitions fail before activation

Activation SHALL reject duplicate keys, unsupported modes, unknown fields, nonfinite or invalid numeric values, and unresolved sound references. Publication SHALL reject absent media and incompatible locomotion references before replacing active catalogs. Hearing SHALL be finite and positive; local settings SHALL satisfy finite `0 ≤ far gain ≤ near gain ≤ 1` and finite `shape > 0`. Invalid effective radius SHALL fail rather than be clamped. Errors SHALL identify the offending definition.

#### Scenario: A footstep sample does not exist
- **WHEN** publication encounters a local footstep profile referencing an absent audio file
- **THEN** publication SHALL fail explicitly and the previous published catalogs SHALL remain usable

#### Scenario: Hearing makes a radius invalid
- **WHEN** configured hearing and a sound's loudness produce an effective radius above the configured maximum
- **THEN** activation SHALL reject that configuration rather than start with truncated audibility

#### Scenario: Numeric settings could break attenuation
- **WHEN** hearing is nonpositive, a curve shape is zero, or gain bounds are invalid
- **THEN** activation SHALL reject those values before computing squared radius or logarithmic gain

### Requirement: Audibility combines sound loudness and player hearing

Every player SHALL have an effective hearing value, initially the shared explicit configurable base value `1.0`. A sound's effective radius SHALL equal `loudness × listener hearing`; a listener SHALL be eligible only when squared distance is strictly less than squared radius. Local clients SHALL receive their current hearing at world entry and refresh it on reconnect or transfer.

#### Scenario: A listener is at the radius boundary
- **WHEN** a listener is exactly at or beyond the effective radius of a sound
- **THEN** that listener SHALL receive no new world sound entry and initiate no new local playback for that sound

#### Scenario: A listener is inside the radius
- **WHEN** a listener with hearing `1.0` is 800 units from a world sound with loudness 1000
- **THEN** that listener SHALL be eligible for the sound subject to documented audio budgets

#### Scenario: The shared hearing setting changes
- **WHEN** base hearing changes from `1.0` to `1.5` within valid configuration limits
- **THEN** the effective radius of a sound with loudness 1000 SHALL become 1500 without editing its profile

### Requirement: World sounds propagate independently of visibility

World sound recipients SHALL be selected by distance and hearing among attached in-world listeners in the same shard and layer, independently of visibility of the performer or source. A point event SHALL retain its source position even after the source changes or disappears. The initial chop and tree-fall profiles SHALL have effective ranges exceeding ordinary player visibility at base hearing.

#### Scenario: Neither character nor tree is visible
- **WHEN** a connected listener cannot see a chopping character or tree but is within the chop sound's effective radius
- **THEN** the listener SHALL be eligible for world delivery without spawning or looking up either source object, subject to audio budgets and playback settings

#### Scenario: A successful fall removes its tree
- **WHEN** a successful terminal chopping effect despawns its target
- **THEN** the fall sound SHALL still be eligible for delivery from the captured target position

#### Scenario: The listener is on another layer
- **WHEN** a listener is close in coordinates but belongs to a different layer or shard
- **THEN** that listener SHALL NOT receive the world sound

### Requirement: Listener membership follows current world placement

World eligibility SHALL use the listener's committed position during the event's propagation tick. Attach, disconnect, relocation, rollback and world transfer SHALL update eligibility consistently, including movement hidden by visibility filters. Reused character identities and detached clients SHALL NOT receive a former listener's sound. Attached in-world observer sessions SHALL retain environmental hearing.

#### Scenario: An invisible listener crosses the hearing boundary
- **WHEN** a listener's committed movement crosses the sound radius while movement publication is suppressed by visibility
- **THEN** sound eligibility SHALL reflect the new position during that tick

#### Scenario: A teleport is rolled back
- **WHEN** a listener is relocated and that relocation is rolled back before propagation
- **THEN** sound eligibility SHALL use the restored position and world membership

#### Scenario: A former identity is reused
- **WHEN** a character disconnects and its identifier is reused for a new character
- **THEN** queued recipient work SHALL NOT deliver using the former character's connection or incarnation

### Requirement: The server determines world distance gain

For an eligible world recipient the server SHALL compute `t = distance / effective radius` and `distance_gain = 1 - 3t² + 2t³`. The client SHALL apply that gain exactly once, multiplied by authored playback volume and user volume controls. A present gain of zero SHALL remain zero. Sound playback SHALL NOT depend on source-object visibility.

#### Scenario: A recipient is halfway through the range
- **WHEN** the listener is at half of a world sound's effective radius
- **THEN** the server SHALL provide distance gain `0.5` and the client SHALL NOT apply another distance attenuation

#### Scenario: The received gain is zero
- **WHEN** a valid sound entry explicitly contains distance gain zero
- **THEN** the client SHALL preserve zero instead of replacing it with the missing-gain default

### Requirement: World routing scales with nearby listeners

Per-event world routing SHALL query nearby listener regions and SHALL NOT scan all connected players or all world objects. Adding listeners outside the queried regions or unrelated world objects SHALL NOT increase candidate counts for the same event. This change SHALL register zero additional ECS systems and introduce no dedicated hearing component.

#### Scenario: Remote population grows
- **WHEN** a warmed benchmark adds listeners outside the regions queried by a fixed sound
- **THEN** candidate and recipient counts SHALL stay unchanged without a hidden per-event full-player traversal

#### Scenario: World object population grows
- **WHEN** a warmed benchmark adds trees and items without changing nearby listeners
- **THEN** candidate counts for that sound SHALL stay unchanged

#### Scenario: The audio feature is enabled
- **WHEN** the matching server activates the sound feature
- **THEN** its ECS system registration count SHALL equal the count before this change

### Requirement: Audio work and delivery are bounded

World events, queried regions, candidate checks, admitted entries per shard tick, per-recipient entries, batch bytes, effective radius and client voices SHALL have validated budgets. Overflow SHALL select sounds by stable priority and creation order, stop work at the exhausted budget and count discarded audio without delaying gameplay. Service events SHALL NOT form a backlog across ticks. Dense fan-out SHALL be measured separately from spatial query cost.

#### Scenario: A crowd exceeds its entry budget
- **WHEN** a recipient is eligible for more entries than its configured batch budget permits
- **THEN** the retained entries SHALL follow priority and creation order, excluded entries SHALL be counted, and gameplay SHALL continue

#### Scenario: Fall feedback competes with chop noise
- **WHEN** a higher-priority tree-fall event competes with lower-priority chop events for a full audio budget
- **THEN** priority selection SHALL retain fall feedback ahead of lower-priority chop entries

#### Scenario: Audio transport is congested
- **WHEN** an audio batch cannot be admitted to its bounded audio transport queue
- **THEN** the batch SHALL be discarded and counted without changing critical gameplay-state delivery policy

#### Scenario: Dense propagation exhausts its work budget
- **WHEN** a crowd and many sounds exceed the global candidate or queried-region budget, including recipients whose batches are already full
- **THEN** propagation SHALL stop before exceeding that budget, record truncated queries and discarded events, and SHALL NOT continue scanning solely to count unheard recipients

### Requirement: Audio admission protects gameplay queue capacity

Audio SHALL have bounded nonblocking admission isolated from gameplay queue capacity and an observable accepted or dropped result. Pending gameplay messages SHALL take writer priority over queued audio. An audio-only burst SHALL NOT occupy gameplay queue slots or cause critical-send failure from queue saturation. Existing critical failure policy SHALL remain in effect for actual gameplay congestion.

#### Scenario: A sound burst arrives before a critical state update
- **WHEN** audio exceeds its queue capacity while the gameplay queue has capacity for a critical update
- **THEN** excess audio SHALL be dropped and the critical update SHALL be admitted without closing the connection because of audio queue saturation

#### Scenario: Gameplay and audio are waiting together
- **WHEN** both gameplay messages and an audio batch are pending for a connection
- **THEN** the writer SHALL select pending gameplay messages before the audio batch

### Requirement: World sound batches carry freshness and stream identity

The server SHALL flush bounded per-listener world batches on their emission tick, carrying stream epoch, server-time anchor, source coordinates, sound key and authoritative gain. World entry SHALL supply the configured freshness window. Clients SHALL reject expired or wrong-stream audio on receipt and before delayed playback, validate entries independently, and clear pending audio on reset.

#### Scenario: A batch arrives after transfer
- **WHEN** a world sound batch belongs to the listener's previous world-stream epoch
- **THEN** the client SHALL ignore it without playing audio in the new world

#### Scenario: Playback arrives too late
- **WHEN** a batch's server-time anchor is older than the configured freshness window
- **THEN** the client SHALL discard and count the stale batch rather than replay delayed action noise

#### Scenario: The sample finishes loading after expiry
- **WHEN** an initially fresh event waits for its sample and expires or changes world stream before playback can start
- **THEN** the client SHALL discard it instead of letting asynchronous loading start an obsolete voice

#### Scenario: One entry is malformed
- **WHEN** one entry in a current valid batch has invalid coordinates, key or gain
- **THEN** the client SHALL reject that entry without preventing playback of valid entries in the same batch

### Requirement: New clients handle the supported legacy input

Matching new servers and clients SHALL be deployed together for batch output. A new client receiving a legacy single world sound without gain SHALL preserve legacy coordinate/radius attenuation. Gain-present entries SHALL use authoritative gain. Server packets naming a local profile SHALL be ignored so migrated local feedback is not duplicated. Old-client support for new batches SHALL NOT be assumed.

#### Scenario: A legacy world sound has no gain
- **WHEN** a new client receives a supported single world-sound message with coordinates and radius but no gain
- **THEN** it SHALL use the previous client distance calculation for that message

#### Scenario: A legacy server also sends local feedback audio
- **WHEN** a new client has already produced local discovery feedback and receives a server sound packet for the local feedback profile
- **THEN** it SHALL ignore that sound packet rather than play the feedback twice

### Requirement: Frequent local sounds require no server sound work

Local sounds SHALL originate solely from client presentation or existing owner feedback, with no server sound scheduling, propagation or sound packets. Footsteps SHALL use authored locomotion contacts and existing interpolated movement for the player and already-known characters. Local playback SHALL NOT request extra movement updates or create unknown characters.

#### Scenario: Many characters walk
- **WHEN** known characters cross authored footstep contacts while walking
- **THEN** clients SHALL produce eligible steps while the server produces zero additional sound events or sound packets for those contacts

#### Scenario: A character is unknown to the client
- **WHEN** a nearby character has no current client world representation
- **THEN** the client SHALL NOT invent its footsteps or request its movement solely for local audio

### Requirement: Other footsteps fade continuously with distance

Inside local audibility radius, own-source gain SHALL be `1`. Other-source footstep gain SHALL default to `0.9 × (1 - ln(1 + 4t) / ln(5))`, with `t = distance / effective radius`. Near/far gains and curve shape SHALL be authored validated settings. Footstep gain SHALL approach zero continuously at the effective radius. Outside the radius no new local one-shot SHALL start. Local hearing SHALL use entry-provided hearing and each sound's loudness.

#### Scenario: Another character is co-located
- **WHEN** another known character produces an eligible footstep at zero distance
- **THEN** default distance gain SHALL be `0.9`, distinguishing it from the player's own gain `1`

#### Scenario: Another character is halfway through local range
- **WHEN** another character produces a footstep at half its effective radius
- **THEN** default gain SHALL be approximately `0.286`, before profile and user volume factors

#### Scenario: Another character approaches the edge of footstep range
- **WHEN** another character moves toward the effective footstep radius
- **THEN** successive contact gains SHALL decrease toward zero, and crossing the radius SHALL NOT abruptly cut off a still-loud footstep gain

### Requirement: Local contact tracking avoids catch-up audio

Local cue timing SHALL follow accepted cycle identity or interpolated locomotion distance independently of model render rate and screen culling. Stops, snaps, late entry, loading, pauses and reset SHALL skip missed cues. Consumed action markers SHALL remain consumed across backward corrections in the same action incarnation and cycle. Action animation playback SHALL NOT locally duplicate a world-mode cue.

#### Scenario: The model is rendered less frequently
- **WHEN** a known moving character's pose is sampled at a reduced rendering rate
- **THEN** eligible local contacts SHALL follow movement progress without frame-rate drift or duplicate steps

#### Scenario: Movement jumps or presentation resumes
- **WHEN** a teleport, correction snap or long presentation pause skips several locomotion contacts
- **THEN** the client SHALL resume from the current contact phase without a burst of missed footsteps

#### Scenario: A known walker enters local hearing range
- **WHEN** a known character walks into the local radius after moving outside it
- **THEN** the client SHALL start tracking from the current contact phase without replaying contacts from outside the radius

#### Scenario: The locomotion clip changes
- **WHEN** movement switches between walk and carry-walk contact definitions
- **THEN** audio tracking SHALL rebase to the current gait phase without duplicate contacts at the transition

#### Scenario: An observer joins after a contact
- **WHEN** a character or its assets become available after a local cue's phase has passed
- **THEN** the client SHALL track subsequent contacts without replaying the passed cue

#### Scenario: An action progress correction moves backward
- **WHEN** a local cue has played and a timing correction moves the same accepted action cycle before that marker
- **THEN** advancing through that marker again SHALL NOT replay its local sound

### Requirement: Quiet discovery feedback preserves its existing trigger

Discovery audio SHALL be a local owner sound triggered by the existing owner-only discovery FX event, with the existing sample and balancing volume. Positive experience or LP deltas alone SHALL NOT trigger discovery audio. Migrating this sound SHALL remove its redundant server sound packet while preserving the existing FX, reward and experience behavior.

#### Scenario: A new item discovery is rewarded
- **WHEN** the player receives the existing discovery FX event and reward
- **THEN** the owner SHALL play discovery audio locally once and the server SHALL send no additional audio packet for it

#### Scenario: Experience is granted without discovery
- **WHEN** crafting or an administrator grants a positive experience or LP delta without the discovery FX event
- **THEN** the client SHALL NOT add discovery audio solely because of that delta

### Requirement: Concurrent playback instances retain independent gains

Playback SHALL apply gain independently to each playing instance, respect user mute/volume settings, and enforce configured voice limits. Reusable asset metadata SHALL be prepared outside frequent playback paths. Diagnostic playback logs SHALL be opt-in. One instance's distance SHALL NOT change the gain of another instance using the same sample.

#### Scenario: The same sample plays near and far
- **WHEN** two simultaneous instances use the same sample with different distance gains
- **THEN** each instance SHALL retain its own gain while it plays

#### Scenario: The user mutes sound effects
- **WHEN** the user mutes sound effects while eligible world and local events occur
- **THEN** both modes SHALL respect the setting without changing server audibility rules

### Requirement: Load behavior is observable and reproducible

Validation SHALL report events, queried regions, candidates, recipients, audio drops by reason, messages, bytes, allocations and propagation/encoding time. Reproducible warmed workloads SHALL cover sparse and dense populations, moving listeners and local-only footsteps, with hardware and workload recorded. Discarded audio SHALL NOT alter gameplay results.

#### Scenario: Dense load deliberately exceeds audio budgets
- **WHEN** a deterministic workload exceeds event, recipient or transport capacity
- **THEN** measured losses SHALL identify the exceeded budget or transport cause and gameplay outcomes SHALL match the audio-disabled run

#### Scenario: Quiet-only load is measured
- **WHEN** the workload contains only client footstep contacts
- **THEN** measured server audio events and audio packets SHALL remain zero
