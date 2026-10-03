# Spec Delta

## Purpose

Provide server-authoritative combat actions with explicit commitment, resource payment, independent cooldowns, directed input, and presentation shared by the performer and observers.

## ADDED Requirements

### Requirement: Axe actions use validated definitions
The server SHALL load separate area and single-target axe actions and weapon parameters from definitions. Both actions SHALL use a 90-degree sector, range 18, windup 600 ms, recovery 400 ms, cooldown 2000 ms, and start cost 60 stamina. The starter stone axe SHALL provide base damage 6; action multipliers SHALL be 1.0 and 1.5. Invalid values, references, or incompatible execution settings SHALL fail loading with the offending file identified.

#### Scenario: Approved preset loads
- **WHEN** the phase-1 definitions load
- **THEN** both actions SHALL require an equipped compatible axe and SHALL use the approved values without attribute-based timing acceleration

#### Scenario: Invalid combat content
- **WHEN** a definition contains nonfinite or negative damage/cost, nonpositive range or timing, an unknown weapon reference, or automatic execution repeat combined with combat
- **THEN** loading SHALL fail before the definition becomes usable

### Requirement: Direction selection starts one server-validated attempt
Actions/menu and hotbar SHALL arm the same direction-target action. One primary map click SHALL request one strike in the clicked world direction, without choosing a victim or approaching the point. Arming and pointer motion SHALL charge nothing. The server SHALL validate the current selection, session, world, direction, weapon, actor state, stamina, cooldown, and absence of conflicting execution before accepting a start.

#### Scenario: Select then aim
- **WHEN** an eligible player selects an axe action and primary-clicks a nonzero direction
- **THEN** the server SHALL lock the normalized direction from its current actor position to that point and start one windup without moving to or locking onto the clicked entity

#### Scenario: Degenerate or stale direction request
- **WHEN** a click has no usable direction, refers to an old selection/session/world, or is a duplicate consumed attempt
- **THEN** it SHALL cause no attack, resource charge, cooldown, combat event, or ordinary-click fallback

#### Scenario: No simultaneous work
- **WHEN** a player with active craft, build, context, carry, or other noncombat execution requests an axe action
- **THEN** combat SHALL be rejected without replacing that work or spending combat resources

### Requirement: Accepted starts pay exactly once
The server SHALL atomically charge the full action cost, start that action's cooldown, and enter windup only after all start checks pass. Exact payment to zero SHALL be allowed. Strike, miss, recovery, and external interruption SHALL neither charge again nor refund the cost. Existing stamina regeneration delay SHALL be updated by the accepted expenditure.

#### Scenario: Exact cost
- **WHEN** a player has exactly 60 stamina and an otherwise valid start
- **THEN** the attack SHALL start with zero stamina and SHALL still resolve unless externally interrupted

#### Scenario: Insufficient cost
- **WHEN** stamina is 59 at start validation
- **THEN** the attempt SHALL be rejected without payment, cooldown, or a combat event

#### Scenario: Miss and interruption preserve payment
- **WHEN** an accepted attack misses or is interrupted during windup or recovery
- **THEN** its paid 60 stamina SHALL remain spent and no partial refund or second charge SHALL occur

### Requirement: Axe execution is committed through recovery
An accepted axe action SHALL progress through windup, one strike resolution, and recovery. Voluntary cancellation, repeated activation, another combat/noncombat action, or equipment change SHALL NOT shorten or replace it or queue future work. Both a hit and a miss SHALL require recovery. Completion SHALL return to idle without automatic repeat.

#### Scenario: Escape and switching cannot cancel
- **WHEN** Escape, the same hotbar action, another action, crafting, building, or a context action is requested during windup or recovery
- **THEN** the existing cycle SHALL continue, no new execution SHALL start or be queued, and the requester SHALL receive authoritative state or a busy reason

#### Scenario: Click does not queue another strike
- **WHEN** another directional attack click arrives before recovery completes
- **THEN** it SHALL NOT produce an immediate or later strike, even after cooldown expires

### Requirement: Cooldowns are independent of recovery
Each action SHALL have its own cooldown beginning at accepted start. Recovery SHALL block the next gameplay execution until it ends, while cooldown SHALL block only reuse of its action. Cooldown time SHALL continue during recovery and interruption and SHALL NOT reset when equipment changes after recovery.

#### Scenario: Alternate attacks
- **WHEN** area attack starts at t=0 and is not interrupted
- **THEN** strike SHALL become due at t=0.6, recovery SHALL end at t=1.0, the single-target action SHALL be eligible at t=1.0, and area attack SHALL be eligible again at t=2.0, subject to other requirements

#### Scenario: Server steps cross multiple deadlines
- **WHEN** one update reaches both a strike deadline and recovery completion
- **THEN** the server SHALL resolve the strike once before finishing the action, preserving the original cooldown deadline

### Requirement: External interruption has explicit semantics
An external stun, knockout, death, or actor teardown SHALL have an explicit interruption entry point. Interruption SHALL discard an unresolved strike and remaining recovery without resumption, refund, or cooldown reset. An ordinary damage notification SHALL NOT interrupt an axe action. Phase 1 SHALL verify this entry point without introducing a status runtime or changing real-character health rules.

#### Scenario: Interrupt before impact
- **WHEN** the test harness invokes external interruption during windup
- **THEN** no later hit SHALL occur and the original cooldown SHALL continue

#### Scenario: Interrupt after impact
- **WHEN** interruption occurs during recovery
- **THEN** existing damage SHALL remain applied, recovery SHALL end, and the interrupted action SHALL never resume

#### Scenario: Ordinary incoming damage
- **WHEN** an ordinary damage notification is supplied without an interrupting condition
- **THEN** the accepted windup or recovery SHALL continue

### Requirement: Combat equipment cannot change during execution
Every inventory path that would change combat equipment SHALL reject that mutation before effects during windup, strike, or recovery, including indirect swaps, transfers, and drops. Phase-1 combat equipment SHALL include both weapon hand slots. Unrelated inventory operations SHALL retain their normal rules. Rejected mutations SHALL NOT queue or cancel the attack.

#### Scenario: Swap bypass
- **WHEN** a swap initiated from a grid would replace either equipped weapon hand during recovery
- **THEN** the entire mutation SHALL fail with equipment and inventory revisions unchanged

#### Scenario: Ordinary inventory use
- **WHEN** a grid-to-grid rearrangement does not affect combat equipment during windup
- **THEN** it SHALL use the existing inventory rules without canceling or shortening the attack

### Requirement: Combat events have stable identity and order
Accepted starts, actual strikes including misses, and each applied hit SHALL have server-issued identities tied to the actor incarnation and execution. Reprocessing SHALL NOT duplicate their effects. Events SHALL be ordered by authoritative event time and server sequence; one event's mutations SHALL be visible to the next. Client requests and animation frames SHALL NOT supply authoritative event identity or impact timing.

#### Scenario: Replayed request after completion
- **WHEN** the same attempt is resent after its action and cooldown end
- **THEN** it SHALL NOT pay, start, emit events, or hit again

#### Scenario: Several hits in one update
- **WHEN** multiple executions become due together
- **THEN** the server SHALL use stable event ordering and apply each result before resolving the next event

### Requirement: Runtime combat activity records only combat events
The server SHALL update the attacker's runtime LastCombatEventAt for accepted start, actual strike including miss, and applied hits, taking the maximum with the previous value. Movement, aim changes, recovery, rejections, and noncombat health changes SHALL NOT update it. Object-like test targets SHALL NOT acquire a character logout timer. Persistence and combat-aware despawn are deferred to phase 7.

#### Scenario: Miss updates activity
- **WHEN** an axe starts at t=0 and misses at t=0.6
- **THEN** its attacker's activity time SHALL become 0.6 and SHALL NOT advance merely because recovery ends

#### Scenario: Older event arrives
- **WHEN** an older event is encountered after a newer combat event
- **THEN** LastCombatEventAt SHALL NOT move backward

### Requirement: Clients display authoritative combat state
The performer SHALL see aim, committed direction, phase, hit/miss, target HP, stamina, and each action's cooldown readiness. Visible observers SHALL receive public execution state and results. Current-state snapshots SHALL recover an ongoing phase without replaying its start. Epoch, incarnation, revision, and event identity SHALL prevent stale updates or duplicate feedback from replacing current state.

#### Scenario: Observer enters during recovery
- **WHEN** another client begins seeing a performer in recovery
- **THEN** it SHALL display the current phase and remaining time without restarting windup or replaying historical hit feedback

#### Scenario: Authoritative HP and feedback
- **WHEN** the server reports one hit or a miss
- **THEN** clients SHALL display that result once and SHALL NOT change HP or stamina from local collision or animation callbacks

#### Scenario: Fractional display
- **WHEN** a combat HP or damage value is fractional
- **THEN** display SHALL use one decimal place and show a positive value below 0.1 as `<0.1`, retaining the exact server value for calculations

#### Scenario: Old world or incarnation
- **WHEN** a delayed execution update belongs to a previous world or replaced actor incarnation
- **THEN** the client SHALL ignore it without recreating an actor or changing the current execution

