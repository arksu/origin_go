# Spec Delta

## Purpose

Let players see actions described by defs performed by themselves and visible characters, with each complete clip fitted to the server-defined action duration and observers joining the current execution phase. Concrete action and animation choices are data, while synchronization is shared.

## ADDED Requirements

### Requirement: Concrete action presentation is defined exclusively by data

Def files SHALL be the sole authored source of action-to-animation bindings, actor/clip references, ordered equipment variants, facing policy, pose eligibility, blend parameters, and output-frame parameters. Protocol, server synchronization, client synchronization, actor playback, and preview logic SHALL interpret generic identities and declarative rules without concrete action names, clip names, tool names, action-specific APIs, or per-binding branches. An additional binding for an already supported timed execution source SHALL require only def changes and catalog publication, without protocol or runtime source changes. Generated catalogs SHALL derive from these defs rather than introduce another authored mapping.

#### Scenario: A second binding is added through a def
- **WHEN** a new def binds another supported timed action to an available compatible clip
- **THEN** the existing server and client SHALL select and synchronize it without changing protocol definitions or runtime source code

#### Scenario: Presentation settings are changed in data
- **WHEN** a def changes variant order, equipment conditions, facing policy, blend duration, or frame dimensions within supported limits
- **THEN** the common mechanism SHALL apply those settings after loading the matching published catalog without action-specific code changes

### Requirement: Definition errors fail before publication or activation

Def loading and publication SHALL reject unsupported schema versions, unknown fields, duplicate binding keys, conflicting execution-source selectors, invalid rule primitives, invalid numeric parameters, and missing or incompatible referenced actor clips. Server startup SHALL reject invalid server-side definitions rather than silently activate a partial registry. Client publication SHALL validate the complete projected catalog against its referenced asset manifests before replacing the public catalog. Runtime clients encountering a structurally valid but unknown binding SHALL preserve ordinary presentation without affecting gameplay.

#### Scenario: A def refers to an absent clip
- **WHEN** publication encounters a binding whose referenced clip is absent from its actor manifest
- **THEN** publication SHALL fail with the offending def and reference identified, and the previous public catalog SHALL remain usable

#### Scenario: Two defs select the same execution source
- **WHEN** definitions contain conflicting bindings for the same source selector
- **THEN** loading SHALL fail explicitly rather than choose one according to file order

### Requirement: Executing actions own their public animation state

The server SHALL resolve an executing cycle's generic source identity through defs and publish animation state only for a mapped source. The state SHALL identify the performer incarnation, opaque animation binding, current cycle revision, authoritative elapsed and total ticks, tick duration, server-time sampling point, and an optional facing target. An idle state SHALL explicitly clear the current action animation. Target selection and approaching SHALL NOT by themselves start an action clip. A client SHALL NOT select an arbitrary public animation or use playback to determine gameplay completion.

#### Scenario: A supported action starts
- **WHEN** a character begins a supported action cycle
- **THEN** the performer and observers who can see that character SHALL receive its current public animation state

#### Scenario: A target is still being approached
- **WHEN** a player selects or approaches a target before an action cycle starts
- **THEN** an action animation SHALL NOT begin solely because that target was selected

### Requirement: The complete clip fits the actual tick duration

For each supported action cycle, the client SHALL fit the complete loaded source clip into `total ticks × server tick duration`. The playback speed multiplier SHALL equal `source clip duration / action cycle duration`, including both acceleration and deceleration. The source clip SHALL NOT be trimmed or segmented to align a visible impact. Playback duration SHALL use the actual timing supplied for that cycle rather than a hard-coded animation duration, tick rate, or the source clip's nominal frame rate. Blending and reduced visual sampling rates SHALL NOT change the underlying cycle timeline.

#### Scenario: A longer clip is accelerated
- **WHEN** a three-second clip is used for an action lasting twenty ticks of one hundred milliseconds each
- **THEN** the complete clip SHALL occupy two seconds of the action timeline at a playback speed multiplier of 1.5

#### Scenario: A shorter clip is slowed down
- **WHEN** a one-second clip is used for a two-second action cycle
- **THEN** the complete clip SHALL occupy two seconds of the action timeline at a playback speed multiplier of 0.5

#### Scenario: A later cycle has a different duration
- **WHEN** a supported action's next cycle uses a different tick count or tick duration
- **THEN** that cycle's playback duration SHALL be recomputed from the new authoritative timing

### Requirement: Clients recover phase from the authoritative timeline

Clients SHALL derive playback phase from authoritative tick progress at a server-time sampling point and their current estimate of server time. Receipt time SHALL NOT restart a clip from its beginning. Phase SHALL be bounded from zero to one for the published cycle. A supported clip's sampled time SHALL equal its source duration multiplied by that phase. A corrected estimate of server time SHALL affect subsequent phase calculations without restarting the action or its blend. Clients displaying the same cycle at the same estimated server time SHALL compute the same normalized phase, independent of frame rate and message arrival delay.

#### Scenario: A player enters visibility during a cycle
- **WHEN** an observer first sees a character halfway through a supported cycle
- **THEN** the observer SHALL join the cycle near its halfway phase rather than start the source clip at frame zero

#### Scenario: A delayed update arrives
- **WHEN** an active state sampled at five of twenty ticks arrives three tick periods later
- **THEN** the client SHALL derive the phase from eight of twenty tick periods on the authoritative timeline, rather than five ticks from receipt time

#### Scenario: Rendering pauses and resumes
- **WHEN** a client resumes rendering after its character was culled or its tab stopped producing frames
- **THEN** playback SHALL resume at the current phase without replaying missed cycles or accumulating frame-time drift

### Requirement: Server transitions delimit repeated cycles

Each new supported cycle SHALL receive a newer public animation revision and timing anchor. Duplicate delivery SHALL NOT restart playback or its blend. A client that reaches the end of a published cycle before receiving its successor SHALL hold that cycle's terminal sample until a newer cycle or idle state arrives; it SHALL NOT wrap into an unconfirmed cycle. A late newer cycle SHALL join its current phase rather than replay completed time.

#### Scenario: A repeating action starts its next cycle
- **WHEN** the server continues a mapped action into a new cycle
- **THEN** the new revision and tick timing SHALL define the next traversal of the source clip

#### Scenario: The next-cycle update is delayed
- **WHEN** a client has reached the end of its last confirmed cycle and no successor has arrived
- **THEN** it SHALL hold the terminal sample and SHALL NOT begin another cycle on a local modulo timer

#### Scenario: The same cycle is delivered twice
- **WHEN** a client receives duplicate state for an already applied cycle
- **THEN** neither the playback phase nor the blend-in SHALL restart

### Requirement: Visibility snapshots and updates converge on current state

Public action-animation updates SHALL be delivered only to the performer and observers who currently see the performer. A character's visibility-entry snapshot SHALL include its current animation state, including idle, captured consistently with that character's incarnation. State changes racing with visibility entry SHALL converge on the newest state without resurrecting a completed action. Critical animation transitions SHALL NOT be silently lost at the server's outbound queue: enqueue failure SHALL terminate the affected connection so a reconnect can rebuild state.

#### Scenario: An unrelated player cannot see the performer
- **WHEN** a character starts a supported action and another player cannot see that character
- **THEN** that unrelated player SHALL NOT receive the character's animation update

#### Scenario: Cancellation races with visibility entry
- **WHEN** an action ends while an observer's character-spawn snapshot is being delivered
- **THEN** the observer SHALL end with the newest idle state rather than an indefinitely active stale action

#### Scenario: The critical outbound queue is full
- **WHEN** an animation transition cannot be enqueued for a connected recipient
- **THEN** that recipient's connection SHALL be closed rather than remain connected with silently missing animation state

### Requirement: Stale state cannot recreate or replace current characters

The client SHALL validate world-stream epoch, performer incarnation, state revision, and timing before applying an update. An update for an unknown or despawned performer SHALL NOT create that performer. A lower revision, a different incarnation, or an earlier timing sample of the same revision SHALL NOT overwrite newer state. A fresher timing sample of the same active revision SHALL correct its anchor without replaying the cycle. Despawn and world reset SHALL clear retained animation state, and re-entry SHALL use the new spawn snapshot.

#### Scenario: An old start arrives after cancellation
- **WHEN** a start update with a lower revision arrives after a newer idle state
- **THEN** the character SHALL remain idle

#### Scenario: The entity identifier is reused
- **WHEN** an animation update refers to a prior incarnation of a currently known entity
- **THEN** the update SHALL be ignored and the current character's state SHALL remain unchanged

#### Scenario: A stale update arrives after world transfer
- **WHEN** an animation update belongs to a previous world-stream epoch
- **THEN** the client SHALL ignore it without modifying the current world's characters

### Requirement: Animation loading preserves the latest action state

The client SHALL retain the latest accepted animation state while character or equipment assets load. When required assets become ready, playback SHALL use the current authoritative phase and current state. Completion of an older load SHALL NOT restart an action that has been canceled, replaced, or despawned. Def-declared eligibility and equipment rules SHALL determine whether the action pose can currently be displayed. Suppression SHALL NOT rewrite the server's action timeline, and becoming eligible again SHALL reveal the current phase rather than restart playback.

#### Scenario: The actor is still loading
- **WHEN** a mapped action state arrives before its actor and def-required equipment assets are ready
- **THEN** readiness SHALL reveal the current phase of the latest still-active cycle

#### Scenario: Cancellation precedes load completion
- **WHEN** a mapped action is canceled before an earlier equipment load finishes
- **THEN** load completion SHALL NOT resurrect the canceled action pose

#### Scenario: The character is knocked out
- **WHEN** a character displaying an action pose becomes knocked out
- **THEN** the knockout pose SHALL take priority and no later stale action or asset result SHALL restore the old action pose

### Requirement: The first production binding is declared in defs

The first production def SHALL be declared entirely in data against the preserved complete hand-specific clips. Its ordered equipment variants SHALL declare compatible equipment visuals, a preferred-hand clip, and the alternative-hand variant. Its policies SHALL declare target facing, pose eligibility, blending, and an output frame covering the clip's pose while preserving the ground anchor. These choices SHALL be entries interpreted by the common mechanism, never special cases in protocol or runtime code. Unmapped gameplay actions SHALL leave ordinary presentation in control.

#### Scenario: Equipment matches only the alternative variant
- **WHEN** a character performs the bound action while compatible equipment matches only the later-declared variant
- **THEN** that variant's clip SHALL play at the authoritative cycle phase and duration

#### Scenario: Multiple declared variants match
- **WHEN** a character starts the bound action with equipment matching more than one declared variant
- **THEN** the first declared variant's clip SHALL be selected because of def ordering, without a hard-coded preference in code

#### Scenario: A newer server advertises an unknown binding
- **WHEN** a client receives a structurally valid animation key it does not support
- **THEN** it SHALL retain ordinary base presentation and continue processing subsequent messages

### Requirement: Render bounds and previews use the same definitions

The common renderer SHALL apply the selected binding's def-declared frame dimensions and origin while preserving native pixel density, the ground anchor, picking, and culling. Blend transitions SHALL retain sufficient bounds for the outgoing pose. Standalone preview SHALL select bindings and variants from the same definitions and use the common pose sampling path; local looping and scrubbing SHALL NOT require a concrete action API or a fixed source-frame count in code.

#### Scenario: A binding requests a larger frame
- **WHEN** a valid def selects a clip needing frame dimensions larger than ordinary presentation
- **THEN** the renderer SHALL use those declared bounds without clipping or moving the character's ground anchor

#### Scenario: A new binding is opened in the preview
- **WHEN** a second valid binding is added and its catalog is loaded
- **THEN** the existing preview SHALL offer that binding and sample its variants without changes to preview source code

### Requirement: Visual synchronization does not change gameplay or sound timing

Introducing public animation state SHALL preserve existing action validation, tick counts, costs, successful effects, cancellations, repeatability, performer progress UI, and sound delivery. The system SHALL NOT require a visible impact to coincide with an effect or sound, add impact markers, or move a server gameplay event to a source-clip frame. Missing public animation state from an older server SHALL mean no network-controlled action animation; it SHALL NOT prevent existing gameplay or the standalone local preview.

#### Scenario: The visible impact precedes cycle completion
- **WHEN** the source clip's visible impact occurs before its server action cycle completes
- **THEN** the complete clip SHALL still be scaled uniformly to that cycle and existing effect and sound timing SHALL remain unchanged

#### Scenario: An older server omits animation state
- **WHEN** a character-spawn snapshot contains no public animation state
- **THEN** the character SHALL use existing base presentation, ordinary actions SHALL remain functional, and the local animation preview SHALL remain usable
