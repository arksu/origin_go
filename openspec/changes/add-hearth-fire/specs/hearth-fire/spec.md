# Hearth Fire Spec Delta

## Purpose

Give each character a durable personal home, with server-authoritative creation, replacement, protected world presence, and interruptible travel that respects permanent death and normal world-entry recovery.

## ADDED Requirements

### Requirement: One active hearth belongs to one character

An active hearth SHALL belong to one character, not an account. The database SHALL enforce at most one active hearth per character and at most one owner per hearth object. Unfinished construction sites SHALL NOT count as active hearths or reserve ownership.

#### Scenario: Database rejects duplicate ownership
- **WHEN** a write would associate two active hearths with one character or one hearth with two characters
- **THEN** the database SHALL reject the duplicate association

#### Scenario: Many unfinished sites
- **WHEN** a character has an active hearth and starts several hearth construction sites
- **THEN** all otherwise-valid sites SHALL be allowed and the active home SHALL remain unchanged

### Requirement: First entry creates the hearth exactly once

A character's first durably prepared world entry SHALL create its automatic hearth exactly at the player spawn position. The complete player and hearth footprints SHALL both fit there. Later entry, reconnect, replacement, or absence of an active association SHALL NOT create another automatic hearth after first-entry initialization.

#### Scenario: First suitable spawn
- **WHEN** a new character enters the world for the first time
- **THEN** one active hearth SHALL be created exactly at the accepted player position before that prepared spawn is published

#### Scenario: Player-only fit is insufficient
- **WHEN** a proposed first-spawn position fits the player but intersects forbidden terrain or an obstacle across the hearth footprint
- **THEN** that position SHALL NOT be accepted for first-entry initialization

#### Scenario: Retry after commit or rollback
- **WHEN** first-entry preparation is retried after a disconnect or restart
- **THEN** a previously committed initialization SHALL be reused without a second automatic hearth
- **AND** a rolled-back initialization SHALL remain eligible for a new first-entry attempt

### Requirement: Manual construction is available immediately

All characters SHALL be able to construct a hearth without a skill or discovery prerequisite. The recipe SHALL require exactly five branches and no other materials. Other characters SHALL be allowed to contribute materials and construction work under the existing cooperative building rules.

#### Scenario: New character starts a hearth
- **WHEN** a new character selects hearth construction at a valid location
- **THEN** no additional skill or discovery SHALL be required and the site SHALL require five branches

#### Scenario: Helpers contribute
- **WHEN** several characters contribute to a hearth site
- **THEN** their valid material and work contributions SHALL advance the same shared site

### Requirement: Construction charges each worker for their branches

Processing one branch SHALL take two seconds and cost eight stamina through ordinary construction mechanics. One worker processing all five branches SHALL require ten seconds of work and forty stamina. Cooperative work SHALL accelerate completion normally, with each worker paying only for the branches they process.

#### Scenario: Solo construction
- **WHEN** one character processes all five branches without interruption
- **THEN** work SHALL total ten seconds and forty stamina

#### Scenario: Shared costs
- **WHEN** one builder processes two branches and another processes three
- **THEN** their construction costs SHALL be sixteen and twenty-four stamina respectively

### Requirement: The actual finisher receives the hearth

Ownership SHALL be determined by the character successfully processing the final required branch, not the site initiator or material provider. That finisher identity SHALL remain fixed across pending finalization, retries, and restart recovery. Finishing a helper's site SHALL replace the finisher's home, not the initiator's home.

#### Scenario: Helper finishes an offline initiator's site
- **WHEN** a helper completes a site placed by another character who is offline
- **THEN** the helper SHALL receive the new hearth and only the helper's previous hearth SHALL be replaced

#### Scenario: Initiator already died
- **WHEN** a living helper finishes a site whose initiator permanently died
- **THEN** the result SHALL be the helper's active hearth, not an ordinary campfire

#### Scenario: Another helper retries completed work
- **WHEN** a fully processed site awaits finalization and another character triggers a retry or the server restarts
- **THEN** the original finisher SHALL remain the owner of the result

### Requirement: Replacement converts the previous hearth in place

Completing a new hearth SHALL atomically make it the finisher's active home and transform the previous active hearth into an ordinary unlit campfire. The previous object's identity and position SHALL remain unchanged. Ordinary campfire rules SHALL then apply; lighting it SHALL NOT restore personal-hearth ownership.

#### Scenario: New home becomes active
- **WHEN** a character completes a new hearth while owning one elsewhere
- **THEN** the new hearth SHALL be the only active home and the old object SHALL remain at its old position as an unlit campfire

#### Scenario: Old campfire is lit
- **WHEN** somebody later lights that ordinary campfire
- **THEN** it SHALL behave as a campfire and SHALL NOT become a personal hearth again

### Requirement: Permanent death clears the active home

Permanent death SHALL atomically convert the owner's active hearth into an ordinary unlit campfire and remove its active ownership association. This SHALL work even if the player dies on another layer or the hearth's chunk is not active. Hearth inheritance and descendant spawning SHALL NOT be added.

#### Scenario: Death underground
- **WHEN** a character permanently dies underground while its hearth is on the surface
- **THEN** the surface hearth SHALL become an ordinary unlit campfire and no active home association SHALL remain for that character

#### Scenario: Death with an unloaded hearth
- **WHEN** permanent death occurs while the hearth is not loaded by any player
- **THEN** its saved state SHALL become an unlit campfire and later loading SHALL not restore an active hearth

### Requirement: Terminal deletion cannot orphan an active hearth

Any terminal deletion of a character SHALL leave no active hearth association for that deleted owner. An existing hearth SHALL undergo the same ordinary-campfire conversion and unlinking, without changing the existing authorization or deletion policy. This cleanup SHALL NOT invent a character-location death event for an absent body.

#### Scenario: Offline character is explicitly deleted
- **WHEN** an authorized character deletion removes an offline owner who has an active hearth
- **THEN** the hearth SHALL become an ordinary unlit campfire and its ownership association SHALL be removed

### Requirement: Hearth placement uses the entire surface footprint

A hearth SHALL occupy one tile by one tile with identical construction and collision bounds. Every overlapped tile SHALL be passable surface dry land. Underground layers, water including shallow water, swamp, and bog SHALL be forbidden. No additional soil-type prerequisite SHALL be imposed.

#### Scenario: Ordinary dry terrain
- **WHEN** the full footprint occupies passable surface dry land
- **THEN** terrain eligibility SHALL allow placement regardless of dry-land soil type

#### Scenario: Footprint touches forbidden terrain
- **WHEN** any part of the footprint overlaps water, shallow water, swamp, bog, or an underground layer
- **THEN** placement SHALL be rejected even if the anchor itself is valid

### Requirement: Only the owner and animals pass through the hearth

The owner SHALL be able to walk through their active hearth. Other players SHALL collide with it, and animals SHALL pass through it freely. Owner passage SHALL also apply to authoritative player spawn and return checks without disabling collisions with unrelated objects.

#### Scenario: Owner and non-owner movement
- **WHEN** the owner and another player attempt to cross the hearth footprint
- **THEN** the owner SHALL pass and the other player SHALL be blocked

#### Scenario: Animal crosses
- **WHEN** an animal crosses the hearth footprint
- **THEN** the hearth SHALL NOT obstruct that animal

#### Scenario: Unrelated obstacle at the home
- **WHEN** the owner's destination also overlaps an unrelated solid obstacle
- **THEN** ignoring the owner's hearth SHALL NOT ignore that obstacle

### Requirement: The hearth footprint stays reserved against placement

No construction or put-down of a carried world object SHALL overlap an active hearth's footprint, even when performed by the owner. Validation SHALL consider the complete placed footprint rather than only its anchor. Movement passage permission SHALL NOT grant structural placement permission.

#### Scenario: Owner builds across their home
- **WHEN** the owner attempts a construction whose footprint intersects the hearth
- **THEN** placement SHALL be rejected

#### Scenario: Carried object crosses the boundary
- **WHEN** a player attempts to put down a chest or log whose anchor is outside but footprint intersects the hearth
- **THEN** placement SHALL be rejected and the valid carry state SHALL remain intact

### Requirement: Active hearths are eternal and protected

An active hearth SHALL burn permanently without fuel. It SHALL be immune to all damage, including owner damage, and SHALL offer no destruction or removal action to anyone. It SHALL NOT be liftable or directly movable. Replacement and terminal-owner conversion SHALL be its supported lifecycle transitions.

#### Scenario: Damage and removal attempts
- **WHEN** the owner or another character attempts to damage, remove, or lift an active hearth
- **THEN** the hearth SHALL remain unchanged and active

#### Scenario: No fuel maintenance
- **WHEN** time passes or the owner disconnects
- **THEN** the hearth SHALL continue burning without consuming fuel

### Requirement: Hearth appearance does not reveal owner status

The world object SHALL be named `Hearth Fire`, without the owner's name, and SHALL always show green flame regardless of online status. No last-login indication or hearth minimap marker SHALL be added for the owner or other players.

#### Scenario: Owner goes offline
- **WHEN** the owner disconnects while other players see the hearth
- **THEN** its name and green flame SHALL remain unchanged and no online-status indicator SHALL appear

#### Scenario: Minimap remains unchanged
- **WHEN** the owner's or another player's hearth enters the minimap area
- **THEN** no hearth marker SHALL be displayed

### Requirement: Return uses the ordinary timed action interface

`Travel to Hearth Fire` SHALL be available through the existing Actions menu and use ordinary progress and cooldown presentation. One accepted attempt SHALL take five seconds of interruptible progress and cost no stamina. Travel Weariness SHALL NOT be introduced. Without an active home, the action SHALL be rejected with a clear message.

#### Scenario: Ordinary successful channel
- **WHEN** an eligible character activates return and waits without interruption
- **THEN** the character SHALL travel after five seconds without a stamina charge

#### Scenario: No active hearth
- **WHEN** a character without an active home requests return
- **THEN** the server SHALL reject it and SHALL NOT create a hearth or choose an alternative travel destination

### Requirement: Return respects combat swimming and knockout

Return SHALL be forbidden during combat, swimming, and active knockout. It SHALL be allowed while standing in shallow water and while lying after authoritative knockout resolution. Eligibility SHALL be revalidated throughout progress and immediately before transfer; in particular combat SHALL be checked again before relocation.

#### Scenario: Combat starts during progress
- **WHEN** the character enters combat before relocation
- **THEN** return SHALL be canceled and the character SHALL not travel

#### Scenario: Swimming and shallow standing
- **WHEN** a swimming character requests return
- **THEN** it SHALL be rejected
- **AND** a character standing in shallow water SHALL remain eligible under this restriction

#### Scenario: Post-knockout lying
- **WHEN** knockout has ended and the character is still lying
- **THEN** return SHALL be eligible without standing first, subject to its other restrictions

### Requirement: Return requires uninterrupted gameplay inactivity

Progress SHALL cancel immediately on a movement command, any attempt to launch another gameplay action even if rejected, any received damage, combat, swimming, or disconnect. Commands need not cause displacement to interrupt. Chat and personal-window opening SHALL NOT interrupt. Invalid or stale network input SHALL NOT bypass ordinary session and input-validity guards.

#### Scenario: Movement cannot displace the player
- **WHEN** a current valid WASD or ground-movement command occurs during progress but collision or lying prevents movement
- **THEN** return SHALL still cancel before movement eligibility is decided

#### Scenario: Rejected other-action attempt
- **WHEN** another gameplay-action attempt occurs during progress and fails its requirements or cooldown check
- **THEN** return SHALL already be canceled and SHALL NOT resume after the rejection

#### Scenario: Damage without combat
- **WHEN** the character receives damage without entering combat
- **THEN** return SHALL cancel

#### Scenario: Chat and personal windows
- **WHEN** the player chats or opens a personal window without attempting gameplay work
- **THEN** return progress SHALL continue

#### Scenario: Connection closes during progress
- **WHEN** the connection closes before authorized transfer begins
- **THEN** progress SHALL cancel through ordinary action cleanup and the character SHALL remain at the source without starting return cooldown

### Requirement: Return resolves the current exact home

Return SHALL resolve the active hearth immediately before relocation, not freeze the home when progress begins. Changing the active home SHALL NOT alone cancel progress. The destination SHALL be the exact hearth position, checked for the player footprint only. A blocked destination SHALL fail clearly without searching nearby or using random spawn.

#### Scenario: Home changes during progress
- **WHEN** the active hearth changes while return progresses
- **THEN** the completed return SHALL target the new active hearth

#### Scenario: Destination is blocked
- **WHEN** the exact hearth destination cannot fit the player because of an obstacle
- **THEN** return SHALL fail with a clear message, no relocation, and no cooldown

#### Scenario: Carried footprint is larger
- **WHEN** a carrying player fits at the destination but the carried object's footprint would not
- **THEN** destination occupancy SHALL evaluate only the player footprint and SHALL NOT reject merely because of the carried object

### Requirement: Return transfers layers and carried objects

Return SHALL allow a cave or other underground origin, including transfer to the surface home on another layer. A carried world object SHALL travel with the character through the existing transfer and rollback rules. Failure SHALL preserve the source body and valid carry state rather than duplicate or discard them.

#### Scenario: Underground return carrying a chest
- **WHEN** an eligible underground character successfully returns while carrying a chest
- **THEN** the character and chest SHALL arrive together at the surface hearth

#### Scenario: Transfer fails
- **WHEN** destination spawning fails after transfer preparation
- **THEN** ordinary rollback SHALL restore the source body and carry state without granting return success

### Requirement: Return cooldown commits only on successful relocation

Return SHALL use the existing independent persisted action cooldown of six minutes. It SHALL start only when the authoritative destination body successfully spawns, not at request admission, progress completion, or socket acknowledgement. Cancellation, rejection, timeout, and rolled-back transfer SHALL NOT start it. Offline time SHALL count under existing cooldown rules.

#### Scenario: Accepted transfer later fails
- **WHEN** a transfer request is accepted but the destination spawn fails
- **THEN** return cooldown SHALL NOT start

#### Scenario: Successful transfer and later reconnect
- **WHEN** return succeeds and the player reconnects while the six-minute deadline is unexpired
- **THEN** the ordinary action state SHALL preserve that deadline and remaining cooldown presentation

#### Scenario: Participant recovery after body relocation
- **WHEN** the target body has successfully relocated and a carry participant uses its ordinary restoration-failure recovery
- **THEN** the authoritative body relocation SHALL still count as return success and start the cooldown exactly once
- **AND** the participant SHALL retain its existing warning and recovery behavior

### Requirement: Post-knockout access uses the same return action

After active knockout ends, the knockout window SHALL show `Travel to Hearth Fire` beside `Stand Up`. It SHALL invoke the same progress, restrictions, and cooldown as the menu action. Successful return from lying SHALL stand the character; failed return or rollback SHALL preserve the original lying state.

#### Scenario: Button becomes available after knockout
- **WHEN** active knockout resolves and the character remains lying
- **THEN** the window SHALL expose the ordinary return action beside `Stand Up`, without automatic or instant travel

#### Scenario: Successful return stands the character
- **WHEN** a lying post-knockout character successfully returns
- **THEN** the destination character SHALL stand

#### Scenario: Failed return leaves the character lying
- **WHEN** return from post-knockout lying fails or rolls back
- **THEN** the source character SHALL remain lying

### Requirement: Login fallback differs from voluntary return

Normal world-entry spawning SHALL try the saved player position, the current exact hearth, nearby positions around the saved player position, then existing random-spawn fallback. Existing reattachment of a still-live detached body SHALL stay first and unchanged. Login recovery SHALL NOT turn voluntary return into a nearby-position search.

#### Scenario: Saved position is blocked but home is free
- **WHEN** normal login cannot use the saved position and the active hearth position is free
- **THEN** the character SHALL spawn at that hearth, including on its surface layer

#### Scenario: Saved position and home are blocked
- **WHEN** both exact positions are blocked
- **THEN** normal login SHALL first try near the saved player position, not near the hearth, and then existing random spawn if necessary

#### Scenario: Detached body still exists
- **WHEN** a reconnect can reattach the existing live body
- **THEN** it SHALL reattach in place without applying home fallback or creating another hearth

### Requirement: Hearth transitions are durable and replay safe

First initialization, home replacement, and terminal-owner cleanup SHALL be atomic durable transitions with server-authoritative ordering. Retry, restart, late callbacks, or older saves SHALL NOT duplicate initialization, steal finalizer ownership, restore a converted hearth, replace a newer home, or rebind a deleted owner.

#### Scenario: Late completion after a newer home
- **WHEN** an already-processed build completion is retried after a newer hearth became active
- **THEN** the newer home SHALL remain active and the earlier operation SHALL NOT reapply its ownership change

#### Scenario: Terminal owner and pending completion
- **WHEN** accepted completion and terminal deletion affect the same owner
- **THEN** their accepted authoritative order SHALL be preserved and the terminal state SHALL contain no active association for the deleted character

#### Scenario: Older chunk save arrives
- **WHEN** an older object snapshot is saved after a committed hearth-to-campfire conversion
- **THEN** it SHALL NOT restore the previous hearth type or active association

### Requirement: Deployment starts from the approved clean database

This feature SHALL provide its schema changes without legacy-character backfill, inheritance migration, or automatic world cleanup. The project owner's planned clean-database preparation SHALL be a deployment prerequisite, not a gameplay or application-side compatibility mechanism.

#### Scenario: Feature is deployed
- **WHEN** the feature is prepared for release
- **THEN** schema and generated persistence contracts SHALL be deployed coherently and the application SHALL NOT run a legacy-player backfill or clear the database itself
