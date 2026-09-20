# Spec Delta

## ADDED Requirements

### Requirement: Burner appearance uses common object-state resources
When burner behavior initializes or changes a station-backed burner's shared `unlit` or `burning` state, the system SHALL set the object's client appearance resource to `{object}/unlit` or `{object}/burning`, respectively, and publish the existing appearance update when that resource changes. `{object}` SHALL be derived from the immutable object-definition key. Burner behavior and burner configuration MUST NOT contain a concrete object key, object-specific resource path, or per-object appearance field for this transition. Each object that uses these shared burner states SHALL register matching client resources under those conventional names.

#### Scenario: Unlit burner receives its conventional resource
- **WHEN** a newly constructed burner-backed object with definition key `campfire` is in the `unlit` state
- **THEN** its client appearance resource SHALL be `campfire/unlit`

#### Scenario: Ignition derives the burning resource without object-specific configuration
- **WHEN** a burner-backed object with definition key `test-hearth` transitions from `unlit` to `burning`
- **THEN** its client appearance resource SHALL become `test-hearth/burning` and no burner configuration value SHALL supply that resource path

### Requirement: Exhaustion failures remain scheduled for retry
An exhausted burner whose configured outcome has not succeeded SHALL remain unlit and retry that outcome on a subsequent scheduled tick. Successful exhaustion SHALL stop further callbacks for that burner. A retry SHALL NOT consume additional fuel or duplicate a completed outcome.

#### Scenario: Persistence fails once
- **WHEN** the first exhaustion attempt fails and a later scheduled attempt succeeds
- **THEN** the burner SHALL stay unlit between attempts and SHALL create the outcome only once

## MODIFIED Requirements

### Requirement: Burner accepts configured fuel abilities
The system SHALL allow a world-object burner to declare an ordered-independent set of accepted fuel ability keys, a positive fuel capacity, a positive integer `ticksPerFuel`, and an initial fuel amount no greater than capacity. Each fuel unit SHALL provide the configured number of server ticks. The burner SHALL accept an item only when it has at least one accepted ability with a positive value. Seconds-based burner configuration SHALL NOT be supported.

#### Scenario: Campfire accepts a branch
- **WHEN** a player uses a branch in hand on an active campfire configured to accept `fuel`
- **THEN** the burner SHALL accept the branch as one fuel unit

#### Scenario: Campfire rejects coal-only fuel
- **WHEN** a player uses an item that has only a `coal` ability on a campfire configured to accept only `fuel`
- **THEN** the burner SHALL not consume the item or change its fuel reserve

#### Scenario: Zero fuel duration is rejected
- **WHEN** a burner definition declares zero `ticksPerFuel`
- **THEN** definition loading SHALL fail with an error identifying the invalid duration

### Requirement: Burner duration uses server runtime only
The system SHALL burn one stored fuel unit after each configured number of server ticks while the burner is active. Fuel advancement SHALL use scheduled behavior callbacks. A burner with initial fuel that has not been activated SHALL preserve that fuel without scheduling consumption. Wall-clock time and the accumulated seconds counter SHALL NOT advance burning independently of ticks. The system SHALL preserve the reserve and next tick deadline across server restart or world-object unload/reload and reconcile elapsed tick intervals when processing resumes. Seconds-based saved schedules SHALL NOT be supported.

#### Scenario: Campfire initial duration
- **WHEN** a campfire has five initial fuel units, a duration of 1,440 ticks per unit, and is successfully ignited
- **THEN** it SHALL remain fueled for 7,200 server ticks from ignition unless it receives more fuel

#### Scenario: Unignited initial fuel is preserved
- **WHEN** a newly constructed campfire has five initial fuel units and remains unlit
- **THEN** advancing server ticks SHALL not reduce its fuel reserve or create an exhaustion outcome

#### Scenario: Server downtime does not burn fuel
- **WHEN** an active burner is saved and the server is stopped for wall-clock time
- **THEN** its remaining fuel duration SHALL be unchanged when the server resumes

#### Scenario: Seconds advance without ticks
- **WHEN** the accumulated seconds counter advances but the current server tick does not
- **THEN** the burner SHALL not consume fuel

#### Scenario: Deferred callback catches up
- **WHEN** a callback or restored object is processed after multiple fuel intervals have elapsed in server ticks
- **THEN** those elapsed intervals SHALL be consumed exactly once and the next deadline SHALL preserve the original interval boundaries
