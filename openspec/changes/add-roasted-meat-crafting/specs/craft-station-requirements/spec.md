# Spec Delta

## ADDED Requirements

### Requirement: Station requirements can select a link without an exact object key

A recipe with `stationRequirements` MUST resolve the player's actual linked station even when `requiredLinkedObjectKey` is omitted. If an exact object key is supplied, its restriction MUST remain in force in addition to station requirements. A station-dependent cycle MUST be bound to its selected linked object; breaking the link, removing the object, or switching links before completion MUST prevent completion against the old target. Recipes without station requirements or exact linked-object requirements MUST retain portable crafting behavior.

#### Scenario: Capability-only recipe resolves the player's station

- **WHEN** a recipe declares cooking and burning requirements without an exact object key and the player is linked to a matching station
- **THEN** availability, cycle start, and completion MUST evaluate that linked station and allow the cycle if other checks pass

#### Scenario: Missing link has a station failure

- **WHEN** a recipe declares station requirements and the player has no linked station
- **THEN** availability MUST report a missing station and the recipe MUST NOT start

#### Scenario: Exact-object restriction is retained

- **WHEN** a recipe also requires exact object key `campfire` and the player is linked to a different cooking station
- **THEN** the recipe MUST remain blocked even if capability and state match

#### Scenario: Link changes during a station-dependent cycle

- **WHEN** the player unlinks from the starting station or switches to another station before completion
- **THEN** the active cycle MUST cancel without consuming its input or producing output, even if the old or new station is still burning
