# Character Action Animations Spec Delta

## MODIFIED Requirements

### Requirement: Concrete action presentation is defined exclusively by data

Def files SHALL be the sole authored source of action-to-animation bindings, actor/clip references, ordered equipment variants, facing policy, pose eligibility, blend parameters, output-frame parameters, and animation sound-cue identity, normalized phase and source selection. Protocol, server synchronization, client synchronization, actor playback, and preview logic SHALL interpret generic identities and declarative rules without concrete action names, clip names, tool names, action-specific APIs, or per-binding branches. An additional binding for an already supported timed execution source SHALL require only def changes and catalog publication, without protocol or runtime source changes. Generated catalogs SHALL derive from these defs rather than introduce another authored mapping. Referenced sound profiles SHALL supply delivery and playback settings without duplicating cue timing.

#### Scenario: A second binding is added through a def
- **WHEN** a new def binds another supported timed action to an available compatible clip
- **THEN** the existing server and client SHALL select and synchronize it without changing protocol definitions or runtime source code

#### Scenario: Presentation settings are changed in data
- **WHEN** a def changes variant order, equipment conditions, facing policy, blend duration, or frame dimensions within supported limits
- **THEN** the common mechanism SHALL apply those settings after loading the matching published catalog without action-specific code changes

#### Scenario: A sound cue changes phase
- **WHEN** a valid animation def changes its sound cue's normalized phase and matching catalogs are activated
- **THEN** the common execution mechanism SHALL use the new phase without adding action-specific timing in behavior or client code

### Requirement: Definition errors fail before publication or activation

Def loading and publication SHALL reject unsupported schema versions, unknown fields, duplicate binding keys, conflicting execution-source selectors, invalid rule primitives, invalid numeric parameters, and missing or incompatible referenced actor clips. Server startup SHALL reject invalid server-side definitions rather than silently activate a partial registry. Client publication SHALL validate the complete projected catalog against its referenced asset manifests before replacing the public catalog. Runtime clients encountering a structurally valid but unknown binding SHALL preserve ordinary presentation without affecting gameplay. Sound-cue validation SHALL reject duplicate cue IDs, nonfinite or out-of-range phases, non-increasing phase order, unsupported or unavailable source selectors, and unresolved sound profiles. Action sound-cue phases SHALL belong to `(0,1]`; omitting sound cues SHALL remain valid.

#### Scenario: A def refers to an absent clip
- **WHEN** publication encounters a binding whose referenced clip is absent from its actor manifest
- **THEN** publication SHALL fail with the offending def and reference identified, and the previous public catalog SHALL remain usable

#### Scenario: Two defs select the same execution source
- **WHEN** definitions contain conflicting bindings for the same source selector
- **THEN** loading SHALL fail explicitly rather than choose one according to file order

#### Scenario: A cue references an unknown sound
- **WHEN** an animation def contains a cue whose sound key is absent from the matching sound catalog
- **THEN** activation or publication SHALL fail explicitly before replacing valid definitions

#### Scenario: A cue phase is invalid
- **WHEN** an action cue uses zero, a value greater than one, a nonfinite value, or non-increasing phase order
- **THEN** definition validation SHALL reject the offending cue with a meaningful error

## ADDED Requirements

### Requirement: The chop marker is authored at sixty percent

The production `tree_chop` animation def SHALL declare a `chop` world sound cue at normalized phase `0.6`, sourced from the target position and shared by its hand variants. The server SHALL emit it at the first validated cycle tick at or beyond that phase. Slight delivery latency SHALL be accepted; client playback SHALL NOT replace server emission with an animation callback.

#### Scenario: A twenty-tick chop cycle advances
- **WHEN** the active validated chop cycle progresses from eleven to twelve of twenty ticks
- **THEN** the server SHALL create one chop sound event at the target position

#### Scenario: The phase lies between server ticks
- **WHEN** phase `0.6` belongs to a thirteen-tick chop cycle
- **THEN** the event SHALL be created on tick eight, the first tick at or beyond the authored phase

#### Scenario: The performer is invisible to a listener
- **WHEN** a valid chop cue crosses its marker but an eligible listener has no animation state for the performer
- **THEN** the listener SHALL remain eligible for world delivery according to the sound-delivery contract and its audio budgets

### Requirement: Each cycle crosses each sound cue at most once

Cue execution SHALL be tied to the authoritative action incarnation and cycle identity. Each eligible world cue SHALL emit at most once when validated progress crosses its marker, including context and menu execution paths. A new repeated cycle SHALL reset its cue progress. Cancellation or replacement before a marker SHALL prevent that event; cancellation afterward SHALL NOT retract an already-created event.

#### Scenario: A processed tick is encountered again
- **WHEN** the same cycle progress is processed more than once
- **THEN** an already-crossed marker SHALL NOT produce another sound event

#### Scenario: Chopping repeats
- **WHEN** a successful chop cycle starts its successor
- **THEN** the successor SHALL be eligible for one new event at its own phase `0.6`

#### Scenario: Chopping is canceled before the marker
- **WHEN** the action is canceled before reaching phase `0.6`
- **THEN** no sound event SHALL be produced for that cycle's marker

#### Scenario: Chopping is canceled after the marker
- **WHEN** the action is canceled after a validated cue created its event
- **THEN** the event SHALL remain eligible for normal world delivery without another emission

#### Scenario: A mapped menu cycle advances
- **WHEN** a supported menu execution source crosses its authored world sound cue
- **THEN** the same once-per-cycle marker semantics SHALL apply as for a context action

### Requirement: Animation sound cues preserve gameplay timing

Authored audio phases SHALL NOT change action validation, tick counts, costs, successful effects, cancellation, repeatability or performer progress UI. Full clips SHALL retain uniform fitting to the authoritative duration. Effects SHALL remain at gameplay completion even if audio occurs earlier. Missing public animation state from an older server SHALL not prevent existing gameplay or the standalone local preview.

#### Scenario: The visible impact precedes cycle completion
- **WHEN** the source clip's visible impact and its authored sound marker occur before gameplay completion
- **THEN** the complete clip SHALL still be scaled uniformly, audio SHALL follow its authored marker, and successful gameplay effects SHALL remain at cycle completion

#### Scenario: An older server omits animation state
- **WHEN** a character-spawn snapshot contains no public animation state
- **THEN** the character SHALL use existing base presentation, ordinary actions SHALL remain functional, and the local animation preview SHALL remain usable

#### Scenario: The final chopping cycle succeeds
- **WHEN** the terminal chopping effect succeeds after its phase `0.6` chop cue
- **THEN** one chop cue SHALL have been emitted earlier and successful tree-fall feedback SHALL be produced once from the captured target position, without an extra end-cycle chop sound

#### Scenario: The terminal effect fails
- **WHEN** the terminal effect fails after an earlier validated chop marker
- **THEN** the earlier chop event SHALL remain valid and no successful tree-fall feedback SHALL be emitted

### Requirement: Cue ownership separates world and local playback

The server SHALL execute only world-mode animation cues. Client action playback SHALL execute eligible local cues from accepted cycle state without replaying passed markers on late entry or loading, and SHALL NOT reproduce world cues. Preview audio SHALL be explicit local presentation using the same definitions and SHALL NOT create server world events.

#### Scenario: The client samples the chop impact
- **WHEN** the client action pose reaches the world-mode chop marker
- **THEN** animation sampling SHALL NOT start an additional local chop sound

#### Scenario: A local cue is already in the past
- **WHEN** a client first accepts a cycle or finishes loading after its local sound marker
- **THEN** it SHALL skip that passed marker and track future eligible cues

#### Scenario: Preview audio is enabled
- **WHEN** a standalone animation preview explicitly enables sound inspection
- **THEN** it SHALL use authored cue metadata locally without sending gameplay sound events

## REMOVED Requirements

### Requirement: Visual synchronization does not change gameplay or sound timing

**Reason**: The approved chop behavior moves sound emission from cycle completion to an authored animation marker at phase `0.6`; prohibiting markers or preserving old sound timing would contradict that behavior.

**Migration**: Use the added requirements for authored chop markers, once-per-cycle execution, gameplay timing preservation and separate world/local cue ownership. Keep gameplay effects and full-clip fitting unchanged while removing legacy end-cycle chop emission.
