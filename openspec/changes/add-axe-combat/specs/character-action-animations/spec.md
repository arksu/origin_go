# Spec Delta

## MODIFIED Requirements

### Requirement: Executing actions own their public animation state

The server SHALL resolve an executing cycle's generic source identity through defs and publish animation state only for a mapped source. The state SHALL identify the performer incarnation, opaque animation binding, current cycle revision, authoritative progress and duration, server-time sampling point, and facing information. Legacy sources SHALL express progress with elapsed/total ticks and tick duration and MAY include a facing target. An idle state SHALL explicitly clear the current action animation. Target selection and approaching SHALL NOT by themselves start an action clip. A client SHALL NOT select an arbitrary public animation or use playback to determine gameplay completion.

Combat SHALL be an additional generic execution source resolved by definitions. Its public state SHALL carry the execution identity, authoritative elapsed/duration milliseconds, and a locked facing direction independent of movement. One animation traversal SHALL cover windup plus recovery; phase/strike state SHALL remain server-owned. Idle/interruption SHALL explicitly clear it. Existing tick-timed sources SHALL retain their wire fields and behavior.

#### Scenario: A supported action starts
- **WHEN** a character begins a supported action cycle
- **THEN** the performer and observers who can see that character SHALL receive its current public animation state

#### Scenario: A target is still being approached
- **WHEN** a player selects or approaches a target before an action cycle starts
- **THEN** an action animation SHALL NOT begin solely because that target was selected

#### Scenario: Combat source joins the public stream
- **WHEN** a mapped axe execution starts
- **THEN** the performer and visible observers SHALL receive the same execution timeline and locked direction without deriving impact from clip frames

### Requirement: The complete clip fits the actual tick duration

For each legacy tick-timed action cycle, the client SHALL fit the complete loaded source clip into `total ticks × server tick duration`. The playback speed multiplier SHALL equal `source clip duration / action cycle duration`, including both acceleration and deceleration. The source clip SHALL NOT be trimmed or segmented to align a visible impact. Playback duration SHALL use the actual timing supplied for that cycle rather than a hard-coded animation duration, tick rate, or the source clip's nominal frame rate. Blending and reduced visual sampling rates SHALL NOT change the underlying cycle timeline.

For a combat source with authoritative duration milliseconds, the complete clip SHALL instead fit that duration (windup plus recovery) with uniform playback scaling. Millisecond timing SHALL take precedence only for that advertised source; legacy tick timing SHALL remain valid. Strike time SHALL be the gameplay windup deadline, independent of the clip's visible impact frame.

#### Scenario: A longer clip is accelerated
- **WHEN** a three-second clip is used for an action lasting twenty ticks of one hundred milliseconds each
- **THEN** the complete clip SHALL occupy two seconds of the action timeline at a playback speed multiplier of 1.5

#### Scenario: A shorter clip is slowed down
- **WHEN** a one-second clip is used for a two-second action cycle
- **THEN** the complete clip SHALL occupy two seconds of the action timeline at a playback speed multiplier of 0.5

#### Scenario: A later cycle has a different duration
- **WHEN** a supported action's next cycle uses a different tick count or tick duration
- **THEN** that cycle's playback duration SHALL be recomputed from the new authoritative timing

#### Scenario: Axe clip is presentation only
- **WHEN** a combat execution lasts 1000 ms with a strike due at 600 ms
- **THEN** the full clip SHALL span 1000 ms and the hit SHALL still resolve at the server strike deadline regardless of its visible impact frame

### Requirement: Animation sound cues preserve gameplay timing

Authored audio phases SHALL NOT change action validation, tick counts, costs, successful effects, cancellation, repeatability or performer progress UI. Full clips SHALL retain uniform fitting to the authoritative duration. For legacy noncombat cycles, effects SHALL remain at gameplay completion even if audio occurs earlier. Missing public animation state from an older server SHALL not prevent existing gameplay or the standalone local preview.

The successful-effect-at-cycle-completion rule SHALL apply to legacy noncombat cycles. Combat effects SHALL instead occur at their server-owned strike deadline, which may precede recovery/animation completion. Authored animation and sound phases SHALL NOT move that deadline or cause duplicate damage.

#### Scenario: The visible impact precedes cycle completion
- **WHEN** a legacy noncombat source clip's visible impact and its authored sound marker occur before gameplay completion
- **THEN** the complete clip SHALL still be scaled uniformly, audio SHALL follow its authored marker, and successful gameplay effects SHALL remain at cycle completion

#### Scenario: An older server omits animation state
- **WHEN** a character-spawn snapshot contains no public animation state
- **THEN** the character SHALL use existing base presentation, ordinary actions SHALL remain functional, and the local animation preview SHALL remain usable

#### Scenario: The final chopping cycle succeeds
- **WHEN** the terminal chopping effect succeeds after its phase `0.4` chop cue
- **THEN** one chop cue SHALL have been emitted earlier and successful tree-fall feedback SHALL be produced once from the captured target position, without an extra end-cycle chop sound

#### Scenario: The terminal effect fails
- **WHEN** the terminal effect fails after an earlier validated chop marker
- **THEN** the earlier chop event SHALL remain valid and no successful tree-fall feedback SHALL be emitted

#### Scenario: Combat impact precedes animation completion
- **WHEN** a combat clip or optional sound cue reaches a visible impact at a time different from its strike deadline
- **THEN** the gameplay hit SHALL remain tied exclusively to the server strike deadline

### Requirement: Clients recover phase from the authoritative timeline

Clients SHALL derive playback phase from authoritative progress at a server-time sampling point and their current estimate of server time. Progress SHALL use ticks for legacy cycles and elapsed/duration milliseconds for combat executions. Receipt time SHALL NOT restart a clip from its beginning. Phase SHALL be bounded from zero to one for the published cycle. A supported clip's sampled time SHALL equal its source duration multiplied by that phase. A corrected estimate of server time SHALL affect subsequent phase calculations without restarting the action or its blend. Clients displaying the same cycle at the same estimated server time SHALL compute the same normalized phase, independent of frame rate and message arrival delay.

#### Scenario: A player enters visibility during a cycle
- **WHEN** an observer first sees a character halfway through a supported cycle
- **THEN** the observer SHALL join the cycle near its halfway phase rather than start the source clip at frame zero

#### Scenario: A delayed update arrives
- **WHEN** an active state sampled at five of twenty ticks arrives three tick periods later
- **THEN** the client SHALL derive the phase from eight of twenty tick periods on the authoritative timeline, rather than five ticks from receipt time

#### Scenario: Rendering pauses and resumes
- **WHEN** a client resumes rendering after its character was culled or its tab stopped producing frames
- **THEN** playback SHALL resume at the current phase without replaying missed cycles or accumulating frame-time drift

#### Scenario: Combat snapshot uses millisecond timing
- **WHEN** a combat execution sampled at 400 of 1000 ms arrives 200 ms later
- **THEN** the client SHALL join at phase 0.6 using the server-time estimate, without restarting windup or using legacy tick fields for that execution
