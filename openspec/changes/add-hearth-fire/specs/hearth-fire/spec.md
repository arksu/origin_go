# Hearth Fire Spec Delta

## Purpose

Let characters acquire a personal home through construction, with authoritative runtime ownership, asynchronous saving, protected world presence, and interruptible travel that respects permanent death and normal world-entry recovery.

## ADDED Requirements

### Requirement: One active hearth belongs to one character

An active hearth SHALL belong to one character, not an account. The database SHALL enforce at most one active hearth per character and at most one owner per hearth object. Unfinished construction sites SHALL NOT count as active hearths or reserve ownership.

#### Scenario: Database rejects duplicate ownership
- **WHEN** a write would associate two active hearths with one character or one hearth with two characters
- **THEN** the database SHALL reject the duplicate association

#### Scenario: Many unfinished sites
- **WHEN** a character has an active hearth and starts several hearth construction sites
- **THEN** all otherwise-valid sites SHALL be allowed and the active home SHALL remain unchanged

### Requirement: Characters acquire hearths only through construction

A newly created character SHALL enter the world without an active hearth. Character creation, first entry, later login, reconnect, and the absence of an active home SHALL NOT create a hearth automatically. First entry SHALL use ordinary player spawn eligibility without imposing Hearth-specific footprint or terrain requirements. Completing construction SHALL be the only gameplay mechanism for acquiring the first or a subsequent active hearth.

#### Scenario: First entry has no home
- **WHEN** a new character enters the world for the first time
- **THEN** the character SHALL spawn normally without a Hearth object or active home association
- **AND** return SHALL remain unavailable until that character finishes a hearth

#### Scenario: Player spawn does not require a hearth-sized area
- **WHEN** an ordinary spawn candidate is valid for the player but would not fit a Hearth footprint
- **THEN** Hearth requirements SHALL NOT reject that player spawn

#### Scenario: Login without a home does not create one
- **WHEN** a character without an active hearth logs in or reconnects, including after a server restart
- **THEN** no hearth or active home association SHALL be created by world entry

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

Ownership SHALL be determined by the character successfully processing the final required branch, not the site initiator or material provider. The same runtime operation SHALL immediately make the completed object that finisher's first or replacement home. There SHALL be no paid completed site awaiting a database result. Finishing another character's site SHALL NOT change the initiator's home.

#### Scenario: Finisher acquires their first home
- **WHEN** a character with no active hearth successfully completes a hearth site
- **THEN** the completed site's object SHALL immediately become that finisher's active hearth in runtime and its ownership SHALL be scheduled for ordinary asynchronous persistence
- **AND** the character SHALL remain at their current position

#### Scenario: Helper finishes an offline initiator's site
- **WHEN** a helper completes a site placed by another character who is offline
- **THEN** the helper SHALL receive the new hearth and only the helper's previous hearth SHALL be replaced

#### Scenario: Initiator already died
- **WHEN** a living helper finishes a site whose initiator permanently died
- **THEN** the result SHALL be the helper's active hearth, not an ordinary campfire

#### Scenario: Two helpers reach the last branch
- **WHEN** two helpers attempt to process a site's last branch during the same server tick
- **THEN** only the first successful final cycle SHALL assign ownership and pay that cycle's cost
- **AND** the other attempt SHALL NOT re-finalize the object or replace its owner

### Requirement: Hearth policy remains feature-owned

Hearth ownership, construction completion policy, and persistence SHALL belong to the Hearth module and integrate through ordinary behavior capabilities and narrow generic hooks. Shared build, action, and object state SHALL NOT contain Hearth-specific fields or key branches. Construction SHALL NOT persist pending finisher metadata or require replay of a completed-site transition.

#### Scenario: Construction saves ordinary progress
- **WHEN** an unfinished Hearth site or an ordinary construction site is saved
- **THEN** its generic build progress SHALL contain no Hearth finisher field or feature-owned pending-completion sidecar

#### Scenario: Completed site is not a durable intermediate state
- **WHEN** a successful final branch is processed and an object snapshot is taken afterward
- **THEN** the snapshot SHALL represent the resulting Hearth rather than a fully processed site waiting for a database result

### Requirement: Replacement converts the previous hearth in place

Completing a new hearth SHALL atomically change runtime ownership and convert the previous hearth, if any, into an ordinary unlit campfire. The previous object's identity and position SHALL remain unchanged, whether its chunk is active or cached. An unloaded old object SHALL be reconciled before becoming visible again. Ordinary campfire rules SHALL then apply; lighting it SHALL NOT restore personal-hearth ownership.

#### Scenario: New home becomes active
- **WHEN** a character completes a new hearth while owning one elsewhere
- **THEN** the new hearth SHALL be the only active home and the old object SHALL remain at its old position as an unlit campfire

#### Scenario: Old campfire is lit
- **WHEN** somebody later lights that ordinary campfire
- **THEN** it SHALL behave as a campfire and SHALL NOT become a personal hearth again

#### Scenario: Previous home is unloaded
- **WHEN** a character successfully completes a replacement while the previous home's chunk is unloaded
- **THEN** only the new home SHALL be active in runtime
- **AND** loading the previous object SHALL expose an ordinary unlit campfire, not restore the old home

### Requirement: Permanent death clears the active home

Permanent death SHALL immediately revoke the owner's active runtime home. Conversion to an ordinary unlit campfire SHALL run through the surface world's owning execution path, including death on another layer; an unloaded hearth SHALL be reconciled before visibility. Saving SHALL remain asynchronous without a new atomic corpse-and-character guarantee. Hearth inheritance and descendant spawning SHALL NOT be added.

#### Scenario: Death before acquiring a home
- **WHEN** a character permanently dies without an active hearth
- **THEN** ordinary terminal character cleanup SHALL proceed without a Hearth conversion or creation

#### Scenario: Death underground
- **WHEN** a character permanently dies underground while its hearth is on the surface
- **THEN** no active runtime home SHALL remain for that character immediately
- **AND** the surface world's execution path SHALL convert the old hearth into an ordinary unlit campfire

#### Scenario: Death with an unloaded hearth
- **WHEN** permanent death occurs while the hearth is not loaded by any player
- **THEN** its runtime ownership SHALL be revoked and its conversion SHALL be scheduled for saving
- **AND** later loading during that server run SHALL not restore an active hearth or expose its old green flame

#### Scenario: Persistence is delayed during death
- **WHEN** a character permanently dies while database saving is delayed
- **THEN** death and runtime home revocation SHALL NOT wait for the database or retain a living character for a Hearth operation

### Requirement: Terminal deletion cannot orphan an active hearth

Any accepted terminal deletion of a character SHALL revoke its active runtime home and schedule ordinary-campfire conversion and ownership saving. This SHALL preserve the existing authorization and deletion policy and SHALL NOT invent a character-location death event for an absent body. An invalid saved association to a deleted character SHALL NOT restore an active home.

#### Scenario: Offline character is explicitly deleted
- **WHEN** an authorized character deletion removes an offline owner who has an active hearth
- **THEN** runtime ownership SHALL be revoked and the hearth SHALL be converted through its owning world
- **AND** the change SHALL use asynchronous saving without fabricating a character death event

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

#### Scenario: Home changes while transfer is queued
- **WHEN** the active hearth changes after transfer admission but before destination relocation
- **THEN** the target world SHALL resolve the new active home before spawning the character
- **AND** queued coordinates SHALL NOT relocate the character to the retired home

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

Normal world-entry spawning SHALL try the saved player position, the current exact hearth if one exists, nearby positions around the saved player position, then existing random-spawn fallback. Without an active hearth, the home stage SHALL be skipped without creating an object or failing merely because ownership is absent. Existing reattachment of a still-live detached body SHALL stay first and unchanged. Login recovery SHALL NOT turn voluntary return into a nearby-position search.

#### Scenario: Saved position is blocked and no home exists
- **WHEN** normal login cannot use the saved position and the character has no active hearth
- **THEN** normal login SHALL try nearby positions around the saved position and then existing random spawn if necessary
- **AND** no hearth SHALL be created

#### Scenario: Saved position is blocked but home is free
- **WHEN** normal login cannot use the saved position and the active hearth position is free
- **THEN** the character SHALL spawn at that hearth, including on its surface layer

#### Scenario: Saved position and home are blocked
- **WHEN** both exact positions are blocked
- **THEN** normal login SHALL first try near the saved player position, not near the hearth, and then existing random spawn if necessary

#### Scenario: Detached body still exists
- **WHEN** a reconnect can reattach the existing live body
- **THEN** it SHALL reattach in place without applying home fallback or creating another hearth

### Requirement: Runtime transitions do not wait for persistence

Successful construction SHALL apply its object and ownership changes as one synchronous operation in the owning surface world. Runtime state SHALL remain authoritative while saving is queued or in flight. Old saves, load results, and callbacks SHALL NOT change the latest runtime ownership, recreate a transformed site, or rebind a terminal owner. Saving SHALL use bounded asynchronous work without a durable operation journal or a database-confirmed publication barrier. Optimistic version checks SHALL reject stale runtime mutations and save acknowledgments.

#### Scenario: Database is slow after construction
- **WHEN** a character completes a Hearth while its save is delayed
- **THEN** the completed object SHALL already be usable as that character's only active runtime home
- **AND** the site SHALL NOT remain available for another construction completion

#### Scenario: Earlier save finishes after a newer home exists
- **WHEN** a save callback or ownership-load result reflects an earlier home after replacement in runtime
- **THEN** the newer runtime home SHALL remain active and the older result SHALL NOT restore ownership or the old green Hearth

#### Scenario: Completion followed by permanent death
- **WHEN** a character successfully finishes a Hearth and then permanently dies before the completion is saved
- **THEN** the Hearth SHALL cease to be an active runtime home
- **AND** no later save callback SHALL undo that death or reinstate the home during that server run

#### Scenario: Old source handle is reused
- **WHEN** asynchronous preparation completes after its source object was removed and its handle was reused
- **THEN** the result SHALL NOT mutate the replacement entity or assign it Hearth ownership

### Requirement: Database errors stop the entire server

The database SHALL be assumed available. Any actual database read, write or commit error, including an unknown commit outcome, SHALL stop the entire server. Hearth SHALL NOT retry SQL or add a recovery path for database errors. Expected absent-row results, optimistic-version conflicts, unknown metadata awaiting preparation and worker-queue saturation SHALL NOT be treated as database failures.

#### Scenario: A database operation fails
- **WHEN** a database read, write or commit returns an actual error
- **THEN** the entire server SHALL stop instead of continuing gameplay or retrying SQL
- **AND** the error SHALL NOT be interpreted as absence of a home

#### Scenario: A home query finds no association
- **WHEN** a home lookup reports expected absence through `sql.ErrNoRows`
- **THEN** the result SHALL establish `NoHome` and the server SHALL continue normally

### Requirement: Home reads are exclusive and complete once

The home index SHALL track at most one pending read per character with a loading flag and the observed optimistic version, without a lookup ID. Once submitted, a read SHALL NOT be canceled, including on logout, and its submitter SHALL deliver the result exactly once. Only a submission rejection that guarantees no job or result can follow SHALL release loading before completion; that release SHALL occur once, immediately after rejection. A stale result SHALL release loading without changing current ownership. Loading SHALL prevent eviction until that result is delivered. The delivery path SHALL NOT lose a result or redeliver it after another read begins.

#### Scenario: Concurrent callers request an unknown home
- **WHEN** callers request a home while its read is pending
- **THEN** only one caller SHALL be admitted to submit the read

#### Scenario: The worker queue rejects a read
- **WHEN** submission is rejected before any read job is accepted
- **THEN** the submitting caller SHALL release loading once without changing the home version
- **AND** a later request SHALL be able to submit the read

#### Scenario: A character leaves while its home is loading
- **WHEN** the character logs out before an accepted read completes
- **THEN** the read SHALL still deliver its sole result and SHALL retain the index entry until completion

#### Scenario: Runtime ownership changes while a read is pending
- **WHEN** a read result arrives after the runtime home version changed
- **THEN** the result SHALL release loading without replacing the current runtime home

### Requirement: Final-cycle preparation precedes costs

The final construction cycle SHALL validate and prepare the runtime data needed for immediate completion before consuming its branch or stamina. Unknown ownership SHALL be resolved by direct `GetCharacterHearth`, and an unloaded previous-home chunk SHALL be awaited through direct `WaitPreloaded`, each with its own five-second context under the surface shard lock. Successful preparation SHALL hydrate the index and complete construction in the same call without an extra action cycle, retry result or Hearth preparation queue. The old chunk SHALL be retained using a blocking pin over its existing I/O gate and pin counter; the mutex SHALL be released before waiting for the loader. These chosen waits MAY delay the whole game loop because the shard manager waits for every layer before the next tick; serial latency is cumulative and movement/cancel commands remain queued until the call returns. Waiting or rejected preparation SHALL leave that cycle's materials, stamina, processed progress, and ownership unchanged. Expected chunk unavailability during shutdown SHALL refuse completion before costs; genuine database/I/O errors SHALL remain fatal. A successful payment SHALL NOT leave a completed site waiting for a database response.

#### Scenario: Unknown ownership is resolved during the final cycle
- **WHEN** the final cycle reads unknown ownership successfully and the old-home source is ready or no home exists
- **THEN** the index SHALL be hydrated and construction SHALL complete in the same call without another action cycle

#### Scenario: Previous home chunk is not ready
- **WHEN** the last branch is reached before the required previous-home chunk is ready
- **THEN** the owning call SHALL retain the chunk and await its existing loader without consuming the branch or stamina
- **AND** successful loading SHALL allow completion in the same call without another action cycle or player click
- **AND** the I/O mutex SHALL NOT remain held while the loader needs it

#### Scenario: Preparation becomes stale
- **WHEN** the prepared source or active ownership no longer matches immediately before final processing
- **THEN** the final cycle SHALL be revalidated or rejected without applying costs or an outdated ownership change

### Requirement: Recovery accepts asynchronous save rollback

After a process failure, recovery SHALL use saved character, object, and ownership data; recent unsaved progress, homes, or terminal changes MAY roll back. These snapshots need not form one atomic checkpoint. Invalid associations SHALL yield no active home. A saved Hearth without valid ownership SHALL become an unlit campfire before visibility. Recovery SHALL NOT infer an owner, upgrade a campfire, or replay an unsaved final cycle. Ordinary material and stamina durability SHALL remain unchanged.

#### Scenario: Process fails before completion is saved
- **WHEN** the process fails after runtime completion but before its data is saved
- **THEN** recovery MAY restore earlier saved progress or ownership instead of the unsaved completion
- **AND** it SHALL NOT fabricate the lost finisher or replay the final cycle

#### Scenario: Process fails before death is saved
- **WHEN** death and home revocation exist only in memory when the process fails
- **THEN** recovery MAY use the earlier saved character and home state
- **AND** the feature SHALL NOT claim every runtime terminal change survived the restart

#### Scenario: Association points to invalid saved data
- **WHEN** saved ownership points to a missing object, an ordinary campfire, another unsupported type, or a deleted character
- **THEN** the association SHALL NOT provide a home or return destination
- **AND** recovery SHALL NOT recreate the object, upgrade its type, or infer a replacement owner

#### Scenario: Saved Hearth has no valid association
- **WHEN** a saved Hearth has no valid owning association after a partial save or crash
- **THEN** it SHALL be converted to an ordinary unlit campfire before becoming visible
- **AND** it SHALL NOT acquire an owner from nearby players, a build initiator, or a later retry caller

#### Scenario: Ordinary shutdown cannot finish saving
- **WHEN** ordinary shutdown ends before all latest Hearth data has been saved
- **THEN** the unsaved portion MAY be lost under the same recovery rules
- **AND** shutdown SHALL NOT require a new Hearth operation drain or claim a fully atomic world checkpoint

### Requirement: Deployment starts from the approved clean database

This feature SHALL provide its schema changes without legacy-character backfill, inheritance migration, or automatic world cleanup. The project owner's planned clean-database preparation SHALL be a deployment prerequisite, not a gameplay or application-side compatibility mechanism.

#### Scenario: Feature is deployed
- **WHEN** the feature is prepared for release
- **THEN** schema and generated persistence contracts SHALL be deployed coherently and the application SHALL NOT run a legacy-player backfill or clear the database itself
