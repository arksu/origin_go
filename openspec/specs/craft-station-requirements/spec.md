# craft-station-requirements Specification

## Purpose

This capability defines how crafting uses the linked station's current conditions and station-local resources without taking ownership of the station's autonomous runtime. It provides a stable requirement contract that can later include operators, terrain, and nearby objects.

## Requirements

### Requirement: Craft recipes declare station requirements

A craft recipe MAY declare station requirements containing a required capability, an optional required state, zero or more typed conditions, and zero or more resources to consume when the cycle completes. A recipe with no station requirements MUST retain the existing linked-object and inventory behavior.

#### Scenario: Meat requires a burning cooking station

- **WHEN** a recipe declares capability `cooking` and state `burning`
- **THEN** the recipe MUST be eligible only while the linked station satisfies both requirements

#### Scenario: Recipe without station requirements remains portable

- **WHEN** a recipe declares no station requirements
- **THEN** the recipe MUST be able to run without a linked station, subject to its existing inputs, stamina, discovery, and skill requirements

### Requirement: Requirement evaluation is extensible by source

The crafting requirement system MUST evaluate requirements through a source-aware interface. Version 1 MUST support station-local capability, state, scalar, and local-resource checks. The interface MUST allow later providers for operator, terrain/tile, and nearby-object or dependent-station checks without changing the crafting operation lifecycle.

#### Scenario: Station-local provider evaluates a recipe

- **WHEN** a linked station exposes the requested capability, state, and local values
- **THEN** the evaluator MUST return a passing evaluation without mutating the station or inventory

#### Scenario: Unsupported future source is not silently accepted

- **WHEN** a v1 recipe contains a condition whose source provider is not enabled
- **THEN** the evaluator or definition validation MUST return a distinct unsupported-requirement result and MUST NOT treat the condition as satisfied

#### Scenario: Dependent station checks can be added later

- **WHEN** a future provider finds another nearby station and delegates evaluation of that station's requirements
- **THEN** the evaluator contract MUST support the nested evaluation with cycle detection and a bounded traversal depth

### Requirement: Requirements are checked before every cycle

The server MUST evaluate all station requirements before starting each craft cycle. This check MUST be read-only and MUST NOT consume station resources, inventory inputs, stamina, or outputs.

#### Scenario: Missing fire prevents cycle start

- **WHEN** a player is linked to a campfire that does not satisfy the required `burning` state
- **THEN** the next craft cycle MUST NOT start and no resource or inventory mutation MUST occur

### Requirement: Requirements are checked again at cycle completion

The server MUST evaluate the same station requirements again when a cycle reaches completion. The completion check MUST use the current station state rather than the state observed at cycle start.

#### Scenario: Station changes during the cycle

- **WHEN** all requirements pass at cycle start but one requirement fails before completion
- **THEN** the cycle MUST be canceled without consuming declared station resources, consuming craft inputs, or creating outputs

### Requirement: Successful completion commits all cycle mutations atomically

After the completion requirement check passes, the server MUST commit station-resource consumption, craft-input consumption, stamina consumption, and output creation as one logical cycle completion. A failed commit MUST NOT leave a partial cycle where only some of those mutations were applied.

#### Scenario: Station resource is consumed with the output

- **WHEN** a recipe requires one unit of station-local `thread` per cycle and all final checks pass
- **THEN** exactly one unit of `thread`, the required craft inputs, and the cycle's stamina MUST be consumed and the configured output MUST be created

#### Scenario: Final resource shortage prevents partial completion

- **WHEN** a required station resource is unavailable at cycle completion
- **THEN** the cycle MUST produce no output and MUST NOT consume craft inputs or other station resources

### Requirement: Autonomous station resources are not craft consumptions

A station resource consumed by an autonomous station rule, such as fuel consumed while burning, MUST NOT be implicitly consumed by a recipe merely because the recipe requires the corresponding station state.

#### Scenario: Cooking checks fire but does not consume fuel

- **WHEN** a recipe requires a campfire in state `burning` and completes successfully
- **THEN** the recipe MUST not deduct campfire fuel unless the recipe explicitly declares a separate craft consumption for that resource

### Requirement: Craft availability reports station failure

When craft availability is sent to a client, station requirements MUST contribute to the existing availability flags and MUST expose enough requirement data for the client to distinguish a missing linked station from an unsuitable station state or resource.

#### Scenario: Craft list reflects extinguished station

- **WHEN** the craft window is open and the linked station is no longer burning
- **THEN** the next craft-list snapshot MUST mark the recipe as not currently startable and include the station requirement data needed for client presentation
