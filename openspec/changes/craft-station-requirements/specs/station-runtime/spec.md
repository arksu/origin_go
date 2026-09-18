# Spec Delta

## Purpose

This capability defines autonomous production-station state that exists independently from any one crafting operation. It lets world objects expose capabilities, state, and local resources while their own runtime rules update those values over time.

## ADDED Requirements

### Requirement: Station exposes runtime capabilities and state

Each station instance MUST expose a runtime state that includes its configured capabilities, current operational state, and station-local values or resources. A station capability identifies what the object can do; a station state identifies whether it is currently ready for a dependent operation.

#### Scenario: Burning campfire exposes cooking capability

- **WHEN** a campfire has fuel and its autonomous update has it burning
- **THEN** the campfire exposes the `cooking` capability and `burning` state to requirement evaluation

#### Scenario: Unlit campfire keeps its capability but not its ready state

- **WHEN** a campfire has no active fire
- **THEN** the campfire may still expose `cooking`, but it MUST NOT satisfy a requirement for state `burning`

### Requirement: Station resources are independent from crafting operations

Station-local resources MUST be updated by station runtime rules independently of whether a player has an active craft operation. Crafting MUST NOT be responsible for autonomous consumption such as fuel burning, cooling, or charge decay.

#### Scenario: Fuel burns without crafting

- **WHEN** a campfire is burning and no player is crafting at it
- **THEN** the station runtime MAY consume fuel and update the campfire state without creating or completing a craft operation

#### Scenario: Fuel exhaustion changes station readiness

- **WHEN** autonomous fuel consumption reaches the campfire's exhaustion condition
- **THEN** the campfire MUST leave the `burning` state and later craft checks MUST observe that change

### Requirement: Station runtime changes are visible at cycle boundaries

The authoritative station state at the start and completion of a craft cycle MUST be used for requirement evaluation. A station state change between those boundaries MUST be able to prevent completion of the cycle.

#### Scenario: Station extinguishes during cooking

- **WHEN** a meat-cooking cycle starts while the campfire is burning and the campfire extinguishes before cycle completion
- **THEN** the completion check MUST observe the non-burning state and the craft MUST NOT produce meat

### Requirement: Station runtime state is durable

The authoritative station state, scalar values, and station-local resource quantities MUST survive world-object persistence, chunk unload/reload, and server restart. Any station mutation that changes durable state MUST mark the owning world object for persistence through the existing object-state lifecycle.

#### Scenario: Fueled station survives a chunk reload

- **WHEN** a station has consumed part of its fuel and its owning chunk is persisted and activated again
- **THEN** the restored station MUST expose the same state, values, and remaining fuel that were saved

#### Scenario: Craft consumption marks station state dirty

- **WHEN** a successful craft completion consumes a station-local resource
- **THEN** the owning world object's persistent state MUST be marked dirty before the chunk persistence pass

### Requirement: Station resource keys are validated

Station definitions and station requirements MUST reject empty resource keys, non-positive quantities, and references to unsupported resource value forms. Invalid station data MUST fail validation before it can enter gameplay.

#### Scenario: Invalid station resource is rejected

- **WHEN** station data declares a resource with an empty key or zero quantity
- **THEN** the definition loader MUST report a validation error identifying the station and resource
