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

### Requirement: Station resource keys are validated

Station definitions and station requirements MUST reject empty resource keys, non-positive quantities, and references to unsupported resource value forms. Invalid station data MUST fail validation before it can enter gameplay.

#### Scenario: Invalid station resource is rejected

- **WHEN** station data declares a resource with an empty key or zero quantity
- **THEN** the definition loader MUST report a validation error identifying the station and resource
