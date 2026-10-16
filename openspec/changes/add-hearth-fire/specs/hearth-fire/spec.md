# Hearth Fire Spec Delta

## Purpose

Let characters acquire a personal home through construction, with authoritative runtime ownership, synchronous Hearth reads and saving, independent lifecycle checkpoints, protected world presence, and interruptible travel that respects permanent death and normal world-entry recovery.

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
- **THEN** the completed site's object SHALL immediately become that finisher's active hearth in runtime and the same call SHALL save construction before returning success
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

Permanent death SHALL immediately mark the owner PendingTerminal and make its runtime home ineligible; the surface owner SHALL then revoke the latest association. After releasing the source shard lock, the current update or attachment call SHALL synchronously finish conversion to an ordinary unlit campfire and its save under the surface lock before returning, including death on another layer. Work SHALL NOT carry over to another tick. An unloaded hearth SHALL be converted without activation. Hearth loading and saving SHALL use five-second contexts, without nesting shard locks or adding an atomic corpse-and-character guarantee. Hearth inheritance and descendant spawning SHALL NOT be added.

#### Scenario: Death before acquiring a home
- **WHEN** a character permanently dies without an active hearth
- **THEN** ordinary terminal character cleanup SHALL proceed without a Hearth conversion or creation
- **AND** known NoHome SHALL become Terminal without Hearth database or chunk access

#### Scenario: Death underground
- **WHEN** a character permanently dies underground while its hearth is on the surface
- **THEN** no active runtime home SHALL remain for that character immediately
- **AND** the surface world's execution path SHALL convert the old hearth into an ordinary unlit campfire
- **AND** the source call and its shard-manager update SHALL wait for Hearth cleanup and commit

#### Scenario: Death with an unloaded hearth
- **WHEN** permanent death occurs while the hearth is not loaded by any player
- **THEN** its runtime ownership SHALL become ineligible immediately and its conversion and synchronous save SHALL execute through the surface owner
- **AND** later loading during that server run SHALL not restore an active hearth or expose its old green flame

#### Scenario: Persistence is delayed during death
- **WHEN** a character permanently dies while database saving is delayed
- **THEN** terminal notification SHALL make runtime ownership ineligible before the owning Hearth save
- **AND** the current update or attachment call SHALL wait for the synchronous Hearth operation without retaining a living character or adding a corpse persistence barrier

#### Scenario: Shutdown overlaps attachment death
- **WHEN** server shutdown begins while an admitted login or transfer is completing permanent death
- **THEN** shutdown SHALL wait for the full owning call, including Hearth loading and saving, before stopping any shard's chunk manager
- **AND** a valid unloaded home SHALL remain loadable until terminal completion finishes

#### Scenario: The saved home no longer has a convertible object
- **WHEN** the current home source is missing, deleted, changed or quarantined
- **THEN** cleanup SHALL remove only the matching association and SHALL NOT recreate or overwrite that object

#### Scenario: Death arrives after an admitted construction completion
- **WHEN** an already admitted final construction cycle replaces the home after PendingTerminal is marked
- **THEN** surface completion SHALL revoke and convert the latest home
- **AND** repeated terminal completion SHALL perform no additional save

### Requirement: Terminal deletion cannot orphan an active hearth

REST character deletion SHALL execute the account-scoped soft delete and Hearth cleanup in one surface-shard critical section. Only a confirmed single-row deletion SHALL mark terminal ownership and perform ordinary-campfire conversion and synchronous saving before returning HTTP 204. Zero affected rows SHALL preserve the existing 204 no-op without touching ownership. The existing authorization and online-deletion policy SHALL remain unchanged; cleanup SHALL NOT disconnect the character, remove an online body, fabricate a corpse or emit character-location death FX. An invalid saved association to a deleted character SHALL NOT restore an active home.

#### Scenario: Offline character is explicitly deleted
- **WHEN** an authorized character deletion removes an offline owner who has an active hearth
- **THEN** runtime ownership SHALL be revoked and the hearth SHALL be converted through its owning world
- **AND** the surface owner SHALL perform Hearth reads and cleanup saving synchronously with five-second contexts without fabricating a character death event

#### Scenario: Offline ownership is unknown after deletion
- **WHEN** a confirmed REST deletion has Unknown runtime ownership
- **THEN** cleanup SHALL read the saved association with an independent five-second context, joining the object by both region and ID without filtering out the deleted owner
- **AND** a valid home SHALL use existing terminal conversion, an absent association SHALL become Terminal without conversion, and an unavailable or invalid source SHALL receive exact association-only cleanup
- **AND** known CurrentHome, NoHome and Terminal SHALL retain runtime authority without this lookup; broader repair of invalid links hidden by known NoHome remains separate

#### Scenario: Deletion overlaps admitted construction
- **WHEN** REST deletion overlaps an already admitted Hearth completion
- **THEN** the surface lock SHALL order the whole construction save before the soft delete and cleanup of its latest home, or order deletion before construction refusal
- **AND** REST SHALL NOT commit a soft delete outside this critical section

#### Scenario: The delete statement fails
- **WHEN** the account-scoped character DELETE returns a database error
- **THEN** REST SHALL preserve HTTP 500 and SHALL NOT start Hearth cleanup without confirmation of deletion
- **AND** later Hearth lookup or cleanup errors SHALL retain the fatal policy

#### Scenario: Request cancellation or shutdown overlaps cleanup
- **WHEN** a confirmed deletion is completing Hearth cleanup
- **THEN** request cancellation SHALL NOT interrupt cleanup and HTTP success SHALL wait for its save
- **AND** the surface lock SHALL keep its chunk manager available until cleanup finishes
- **AND** a request acquiring the lock after the shard entered Stopping SHALL refuse before executing the delete statement

### Requirement: Hearth placement uses standard build tile rules

A hearth SHALL occupy one tile by one tile with identical construction and collision bounds. At placement admission, standard build validation SHALL check the full result collider footprint against recipe `disallowedTiles: [1, 3, 50, 53, 56, 255]` (DeepWater, ShallowWater, Swamp1/2/3 and Void). Other known tile IDs SHALL remain eligible. Ordinary build timing SHALL be preserved without an additional final terrain recheck or placement layer policy; existing Hearth construction completion SHALL remain surface-only.

#### Scenario: Allowed terrain
- **WHEN** the full footprint occupies loaded known tiles outside the recipe's forbidden list
- **THEN** tile eligibility SHALL allow placement

#### Scenario: Footprint touches forbidden terrain
- **WHEN** any part of the footprint overlaps a forbidden tile
- **THEN** placement admission SHALL be rejected even if the anchor itself is valid

#### Scenario: Footprint touches unavailable chunk data
- **WHEN** any tile required for the footprint is unavailable
- **THEN** placement admission SHALL be rejected without creating pending placement or phantom

#### Scenario: Exact footprint boundary
- **WHEN** the collider's maximum edge exactly touches an adjacent tile boundary
- **THEN** that adjacent tile SHALL NOT be included in the footprint check

### Requirement: Only the owner passes through the hearth

The owner SHALL be able to walk through their active hearth. Other players and animals SHALL collide with it. Owner passage SHALL also apply to authoritative player spawn and return checks without disabling collisions with unrelated objects.

#### Scenario: Owner and non-owner movement
- **WHEN** the owner and another player attempt to cross the hearth footprint
- **THEN** the owner SHALL pass and the other player SHALL be blocked

#### Scenario: Animal attempts to cross
- **WHEN** an animal attempts to cross the hearth footprint
- **THEN** the hearth SHALL block that animal

#### Scenario: Unrelated obstacle at the home
- **WHEN** the owner's destination also overlaps an unrelated solid obstacle
- **THEN** ignoring the owner's hearth SHALL NOT ignore that obstacle

### Requirement: The hearth footprint stays reserved against placement

No construction or manual put-down of a carried world object SHALL overlap an active hearth's footprint, even when performed by the owner. Validation SHALL consider the complete placed footprint rather than only its anchor. Movement passage permission SHALL NOT grant structural placement permission. Manual put-down SHALL validate at request and final application, before restoring the object's runtime. Forced drops during logout, death and failed transfers are outside the manual put-down requirement.

For manual put-down on every layer, the server SHALL use the saved original collider and its masks, or the optional `lift.placementSize` for colliderless objects (log_x 24x6, log_y 6x24 world units; default 2x2 with Layer/Mask 1). Dimensions SHALL be finite and positive when configured. These dimensions SHALL NOT change colliderless pickup, movement collision or approach phantom behavior.

Construction placement on every layer SHALL use the collider and masks of the current result definition at request admission before creating new pending/phantom state, and again at final placement before allocating an ID or creating the site. The final check SHALL resolve the current result definition again. Collision refusal SHALL use `BUILD_PENDING_FAILED`. A final refusal SHALL clear the current pending/phantom through ordinary cancellation without creating a site or allocating an ID. Standard recipe terrain rules SHALL remain admission-only.

Both paths SHALL reuse one generic bounded spatial query and overlap check over loaded live `Transform`/`Collider` with ordinary movement mask compatibility, excluding the actor, carried object when applicable, and candidates with a phantom. Each service SHALL reuse its own 4096-handle buffer with 1024-cell and 4096-visit query budgets. Exact edge/corner contact SHALL be allowed. Hearth behavior, ownership and owner passage permission SHALL NOT affect this check; ordinary objects and construction sites SHALL also block intersecting placement. Existing chunk admission checks SHALL remain; neighboring chunk readiness and deferred activation SHALL NOT add placement restrictions. A missing spatial index or bounded spatial query failure SHALL reject placement without loading data.

#### Scenario: Construction crosses a hearth boundary
- **WHEN** the owner or another player requests construction whose anchor is outside the hearth but whose result footprint intersects it
- **THEN** placement SHALL be rejected without creating new pending/phantom state

#### Scenario: Ordinary site blocks construction
- **WHEN** construction's result footprint intersects an existing ordinary construction site's compatible collider
- **THEN** placement SHALL be rejected regardless of Hearth ownership

#### Scenario: Construction preview becomes obstructed
- **WHEN** a construction request was accepted but a compatible collider overlaps the result footprint before final placement
- **THEN** final placement SHALL fail with `BUILD_PENDING_FAILED`, clearing the current pending/phantom without allocating an ID or creating a site

#### Scenario: Replaced construction preview receives an older completion
- **WHEN** finalization receives an earlier preview's coordinates or build key that differ from the current pending placement
- **THEN** it SHALL return without creating a site or applying collision refusal and cleanup to the newer preview

#### Scenario: Carried object crosses the boundary
- **WHEN** a player attempts to put down a chest or log whose anchor is outside but footprint intersects the hearth
- **THEN** placement SHALL be rejected and the valid carry state SHALL remain intact

#### Scenario: Hearth appears during approach
- **WHEN** manual put-down was accepted but a Hearth overlaps the target before final application
- **THEN** the current attempt SHALL fail, clearing its pending/phantom while preserving the carried object's runtime and carry state

#### Scenario: Exact contact
- **WHEN** the placed footprint only touches an obstacle's boundary
- **THEN** that contact SHALL NOT reject construction or manual placement

#### Scenario: Hearth conversion retains ordinary collision
- **WHEN** the hearth becomes an ordinary campfire before manual placement finishes
- **THEN** its current collider SHALL still block intersecting placement according to ordinary masks

#### Scenario: Loaded collider extends across a chunk boundary
- **WHEN** a loaded obstacle's anchor lies in another chunk but its collider overlaps the placed footprint
- **THEN** construction or manual placement SHALL be rejected according to ordinary masks

#### Scenario: Stale put-down completion
- **WHEN** an older put-down attempt completes after a newer generation has started
- **THEN** the newer attempt and its phantom SHALL remain unchanged

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

Return SHALL use known runtime metadata and resolve the active hearth immediately before relocation, not freeze the home when progress begins. `Unknown` metadata SHALL fail closed without starting an asynchronous lookup. Changing the active home SHALL NOT alone cancel progress. The destination SHALL be the exact hearth position, checked for the player footprint only. A blocked destination SHALL fail clearly without searching nearby or using random spawn.

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

### Requirement: Runtime transitions precede persistence

Construction SHALL apply its object and ownership changes as one synchronous operation in the owning surface world before serialization and SQL. Its call SHALL return success only after the construction transaction commits. Known runtime state SHALL remain authoritative over loaded data, including known absence and terminal state. All Hearth reads and saves, including death and REST cleanup, SHALL execute synchronously under the owning surface shard lock with a five-second context per read or transaction. Cross-layer death cleanup SHALL complete after source unlock and before the current call returns without nesting shard locks. Hearth SHALL NOT add a save queue, SQL retry, asynchronous acknowledgment, durable operation journal or runtime rollback.

#### Scenario: Database is slow after construction
- **WHEN** a character completes a Hearth while its save is delayed
- **THEN** the completed object SHALL already be that character's only active runtime home
- **AND** the site SHALL NOT remain available for another construction completion
- **AND** the owning call SHALL wait for commit before reporting success, accepting whole-game-loop delay

#### Scenario: Saved ownership is older than runtime
- **WHEN** a read reflects an earlier home after replacement or revocation in runtime
- **THEN** the known runtime state SHALL remain unchanged and the saved result SHALL NOT restore ownership or the old green Hearth

#### Scenario: Terminal notification arrives during SQL
- **WHEN** a different layer marks the character terminal while the surface owner is waiting on SQL
- **THEN** PendingTerminal SHALL block new acquisition and return without changing the association from that notifying path
- **AND** the surface owner SHALL apply terminal cleanup to the latest association before allowing later acquisition

#### Scenario: Old source handle is reused
- **WHEN** owning work observes that a prepared source object was removed and its handle was reused
- **THEN** it SHALL NOT mutate the replacement entity or assign it Hearth ownership

### Requirement: Construction saves its final objects and association synchronously

After final-branch processing, transforms and runtime index replacement, construction SHALL serialize its final objects and use one existing `WithTx` call with a five-second context to save the new Hearth, the previous unlit campfire if present, `character_hearth` and the durable object-ID watermark. Ordinary object/chunk dirty flags SHALL remain set, and a previous raw chunk's pin SHALL remain held until saving returns. Serialization, SQL or commit errors and an association affected-row count other than one SHALL stop the entire server, without runtime rollback, SQL retry or a construction save queue. Character, inventory, material, stamina and corpse persistence SHALL remain independent.

#### Scenario: First construction commits
- **WHEN** a character completes their first Hearth
- **THEN** the new object, association and durable ID watermark SHALL be saved in one transaction before success is returned
- **AND** ownership SHALL require no asynchronous save acknowledgment

#### Scenario: Replacement includes an inactive previous home
- **WHEN** construction replaces a previous home held in the raw chunk cache
- **THEN** the same transaction SHALL save that object's campfire state together with the new Hearth and association
- **AND** its chunk pin SHALL remain held until saving returns, while ordinary object/chunk dirty flags remain intact

#### Scenario: Association write does not affect exactly one row
- **WHEN** `SetCharacterHearth` returns an affected-row count other than one during construction saving
- **THEN** the entire server SHALL stop without reporting construction success, retrying SQL or rolling runtime back

### Requirement: Database errors stop the entire server

The database SHALL be assumed available. Any actual Hearth database read, write or commit error, including an unknown commit outcome, SHALL stop the entire server. The account-scoped REST character DELETE itself SHALL retain HTTP 500 on SQL failure; this exception SHALL NOT apply to subsequent Hearth cleanup. Hearth SHALL NOT retry SQL or add a recovery path for database errors. Invalid query results SHALL also stop the server. Expected absent-row results and unknown metadata awaiting synchronous preparation SHALL NOT be treated as database failures.

#### Scenario: A database operation fails
- **WHEN** a Hearth database read, write or commit returns an actual error
- **THEN** the entire server SHALL stop instead of continuing gameplay or retrying SQL
- **AND** the error SHALL NOT be interpreted as absence of a home

#### Scenario: A home query finds no association
- **WHEN** a home lookup reports expected absence through `sql.ErrNoRows`
- **THEN** the result SHALL establish `NoHome` and the server SHALL continue normally

### Requirement: Home metadata preserves known runtime state

The owning surface shard SHALL serialize home reads and association mutations. `LoadIfUnknown` SHALL install copied metadata only while the character state is Unknown, preserving known NoHome, CurrentHome and terminal state. The index SHALL retain PendingTerminal and Terminal eligibility with a short mutex for cross-layer notification; it SHALL NOT hold that mutex across SQL, shard locks or ECS callbacks. Records SHALL remain for the process lifetime, accepting growth with encountered characters; no loading/version/dirty acknowledgment or retain/eviction protocol SHALL be required. Return and collision policy SHALL use eligible ownership, while the copied reverse snapshot used for activation SHALL also include PendingTerminal associations.

#### Scenario: A known character is loaded again
- **WHEN** login or construction already has known metadata
- **THEN** its home lookup SHALL skip SQL and preserve the current state

#### Scenario: An old association is offered for a known absent home
- **WHEN** loaded metadata names a home for a character already known to have NoHome
- **THEN** LoadIfUnknown SHALL preserve NoHome

#### Scenario: Pending terminal owner is inspected for activation
- **WHEN** an object belongs to a character whose terminal notification is pending
- **THEN** the reverse snapshot SHALL retain its association so activation does not mistake it for an orphan
- **AND** ordinary Owner lookup SHALL NOT grant that character return or passage eligibility

### Requirement: Hearth ownership is prepared before raw activation

ChunkManager SHALL invoke one optional generic callback after its existing gate/state guards and before creating raw ECS entities. The surface runtime SHALL select current raw Hearth candidates, excluding deleted rows and object IDs already represented by live entities. It SHALL consult copied reverse snapshots first and use one existing GetCharacterHearthOwners query for unresolved keys, with a five-second context under the surface shard lock. Eligible offline owners SHALL also be hydrated through LoadIfUnknown; known runtime state SHALL win. Empty or fully known candidate sets SHALL need no SQL. Existing chunk generations, gates, dirty behavior and load workers SHALL remain unchanged. Any dead-observer AOI unregister path that can initiate activation SHALL hold its existing shard lock.

#### Scenario: A chunk contains no unresolved Hearth
- **WHEN** the current activation candidates contain no Hearth needing owner metadata
- **THEN** activation SHALL issue no Hearth owner query

#### Scenario: Several offline owners have Hearths in one chunk
- **WHEN** a chunk contains several unresolved Hearths with eligible offline owners
- **THEN** one batch query SHALL hydrate all those owners before entity creation or visibility

#### Scenario: Raw cache duplicates a live entity
- **WHEN** a current raw row has the same object ID as an already live entity, or the row is deleted
- **THEN** Hearth preparation SHALL skip that row without changing its live entity or writing a campfire conversion

#### Scenario: Owner state changes while the batch is read
- **WHEN** a PendingTerminal notification arrives during the synchronous batch read
- **THEN** activation SHALL honor current terminal metadata and SHALL NOT normalize that pending owner's Hearth as an orphan

### Requirement: Orphan normalization commits before activation

A true orphan Hearth SHALL be prepared as an ordinary unlit campfire using the same raw conversion helper as construction. The conversion SHALL preserve composite identity and position, apply the campfire definition and HP, clear old behavior data and set the current lastTick. All orphan copies for that activation SHALL be saved in one synchronous transaction with a five-second context using existing UpsertObject. Only after commit SHALL the runtime apply UpsertRawObject and create entities. SQL, commit or invalid-result failure SHALL stop the server without exposing partial normalization. No new SQL/schema, per-object query, worker, generation protocol or deferred-restore framework SHALL be introduced.

#### Scenario: Several Hearths have no valid owner
- **WHEN** a batch confirms several true orphan Hearth candidates
- **THEN** one transaction SHALL save all campfire copies before any raw-cache replacement or ECS activation
- **AND** each converted object SHALL retain its identity and position with campfire HP, cleared data and current lastTick

#### Scenario: Orphan saving fails
- **WHEN** the orphan transaction returns an SQL or commit error
- **THEN** the entire server SHALL stop and the activation callback SHALL NOT apply the orphan copies or expose their entities

#### Scenario: Owner died before restart
- **WHEN** a restored Hearth has no valid alive owner after restart
- **THEN** it SHALL be saved and exposed as an ordinary unlit campfire without inferring an owner or replaying construction

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

### Requirement: Recovery accepts independent checkpoints

After a process failure, recovery SHALL use saved character, object, and ownership data; recent progress or a Hearth operation interrupted before its synchronous commit MAY roll back. These snapshots need not form one atomic checkpoint. Invalid associations SHALL yield no active home. A saved Hearth without valid ownership SHALL become an unlit campfire before visibility. Recovery SHALL NOT infer an owner, upgrade a campfire, or replay an unsaved final cycle. Ordinary material and stamina durability SHALL remain unchanged.

#### Scenario: Process fails before construction commit
- **WHEN** the process fails after runtime completion but before its construction transaction commits
- **THEN** recovery MAY restore earlier saved progress or ownership instead of the unsaved completion
- **AND** it SHALL NOT fabricate the lost finisher or replay the final cycle

#### Scenario: Process fails before terminal commit
- **WHEN** death or home revocation exists only in memory because the process fails before synchronous terminal cleanup commits
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
- **WHEN** ordinary shutdown or process failure interrupts a pending owning operation before its Hearth transaction commits
- **THEN** the unsaved portion MAY be lost under the same recovery rules
- **AND** shutdown SHALL preserve ordinary final-save ordering without a Hearth dirty flush or new operation drain and SHALL NOT claim a fully atomic world checkpoint

### Requirement: Deployment starts from the approved clean database

This feature SHALL provide its schema changes without legacy-character backfill, inheritance migration, or automatic world cleanup. The project owner's planned clean-database preparation SHALL be a deployment prerequisite, not a gameplay or application-side compatibility mechanism.

#### Scenario: Feature is deployed
- **WHEN** the feature is prepared for release
- **THEN** schema and generated persistence contracts SHALL be deployed coherently and the application SHALL NOT run a legacy-player backfill or clear the database itself
