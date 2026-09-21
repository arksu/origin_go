# Spec Delta

## Purpose

Define the Roasted meat recipe so players can cook different meat species through one recipe while retaining the correct species-specific output.

## ADDED Requirements

### Requirement: Roasted meat uses the complete species map

The catalog MUST include a craft named `Roasted meat`, keyed `roasted_meat`, with one input `{ "itemTag": "raw_meat", "count": 1, "qualityWeight": 1 }` and preview output `{ "itemKey": "roasted_meat", "count": 1 }`. Its `outputByInputKey` MUST contain exactly these mappings:

| Source key | Target key |
| --- | --- |
| `beef` | `roasted_beef` |
| `fox_meat` | `roasted_fox_meat` |
| `rabbit_meat` | `roasted_rabbit_meat` |
| `boar_meat` | `roasted_boar_meat` |
| `bear_meat` | `roasted_bear_meat` |
| `raw_deer_meat` | `roasted_deer_meat` |
| `raw_mutton` | `roasted_mutton` |
| `raw_pork` | `roast_pork` |
| `raw_chicken_meat` | `roasted_chicken_meat` |

#### Scenario: Each meat produces its specified roast

- **WHEN** any source in the table is selected and all cycle checks pass
- **THEN** exactly one unit of that source MUST be consumed and one unit of its corresponding target MUST be created using standard craft quality
- **AND** the generic `roasted_meat` preview item MUST NOT be created

### Requirement: Roasted meat requires a burning linked cooking station

The recipe MUST declare `stationRequirements: [{ "capability": "cooking", "state": "burning" }]` without an exact `requiredLinkedObjectKey` restriction. A player MUST be linked to a station satisfying both fields before a cycle starts and at final completion. The recipe MUST NOT add a temperature threshold or implicit fuel consumption. Autonomous fuel consumption SHALL remain independent.

#### Scenario: Suitable linked station allows roasting

- **WHEN** the player is linked to any station exposing `cooking` in state `burning`, with sufficient input, stamina, and output space
- **THEN** the player MUST be able to start Roasted meat

#### Scenario: Missing or unsuitable link blocks roasting

- **WHEN** the player has no linked station, a linked object without cooking capability, or a linked cooking station in state `unlit`
- **THEN** Roasted meat MUST NOT start and input MUST remain unchanged

#### Scenario: Station becomes unlit before completion

- **WHEN** a roasting cycle starts while the station is burning and the station is `unlit` at the final check
- **THEN** the cycle MUST cancel without consuming input, deducting craft stamina or station resources, or creating output

#### Scenario: Repeated roasting preserves completed cycles

- **WHEN** an earlier cycle completes and the linked station becomes unlit before a later cycle completes
- **THEN** the completed roast MUST remain and the failed cycle's raw input MUST remain unconsumed

### Requirement: Roasted meat retains generic preview icons

The craft UI MUST show result icon `items/roasted_meat` and consume icon `items/raw_meat`. Preview metadata MUST remain generic regardless of the species selected by the server; actual inventory output MUST be the mapped item.

#### Scenario: Pork uses generic recipe preview and specific inventory result

- **WHEN** the UI displays Roasted meat while raw pork is the first matching input
- **THEN** its consume/result icons MUST remain `items/raw_meat` and `items/roasted_meat`
- **AND** a successful cycle MUST create `roast_pork` in inventory
