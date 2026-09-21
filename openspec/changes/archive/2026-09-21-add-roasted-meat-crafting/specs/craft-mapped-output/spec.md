# Spec Delta

## Purpose

Allow a tagged craft input to determine one concrete output while preserving standard inventory selection, quality, and failure behavior.

## ADDED Requirements

### Requirement: Mapped output definitions have one unambiguous tagged source

Craft definitions SHALL support optional `outputByInputKey`, mapping source item keys to target item keys. When supplied, a recipe MUST have exactly one input row with a non-empty `itemTag`, no `itemKey`, and `count: 1`. Existing input validation, including quality weights, MUST still apply. Every map source and target key MUST be non-empty and reference an existing item definition. Every source MUST have the sole input's tag; a source outside that tag MUST be rejected at load time. For the Roasted meat recipe this tag MUST be `raw_meat`. Validation errors MUST identify the recipe and invalid map entry or input shape.

#### Scenario: Invalid map entry is rejected on load

- **WHEN** a map contains an empty or whitespace-only key/value, an unknown source or target item, or a source without the declared input tag
- **THEN** loading MUST fail with a definition error before the recipe enters gameplay

#### Scenario: Ambiguous input shape is rejected on load

- **WHEN** a mapped recipe declares an exact-key input, more than one input row, or an input count other than one
- **THEN** loading MUST reject the recipe

#### Scenario: Incomplete map remains a runtime-diagnosable condition

- **WHEN** a valid map omits an item carrying the input tag, including an explicitly supplied empty map
- **THEN** loading MUST NOT require exhaustive tag coverage and the runtime MUST NOT fall back to declared preview outputs

### Requirement: Source selection follows standard craft input ordering

Mapped crafts MUST use the existing craft input validation, tag matching, and deterministic inventory traversal to select the first matching item for one unit of consumption. Traversal MUST retain root grids, then nested grids, then hand, preserving existing inventory-link order within each group and item order within containers. Mapping coverage MUST NOT filter or reorder candidates. The selected source key, consumed unit, and quality contribution MUST refer to the same item.

#### Scenario: Multiple matching meats are present

- **WHEN** beef precedes raw pork in the standard input traversal
- **THEN** one beef unit MUST be selected and its mapped result MUST be used, regardless of map iteration order

#### Scenario: First matching item has no mapping

- **WHEN** the first matching tagged item has no map entry and a later matching item has one
- **THEN** the craft MUST fail for the first item and MUST NOT skip it to consume the later item

#### Scenario: Stack supplies one unit

- **WHEN** the selected matching item has quantity greater than one
- **THEN** a successful cycle MUST consume exactly one unit and preserve the remaining stack's identity, type, and quality

### Requirement: Declared outputs are preview metadata for mapped crafts

When `outputByInputKey` is supplied, the declared `outputs` list and its counts MUST serve only as UI preview metadata and MUST NOT drive runtime output creation or placement. Each successful cycle MUST create exactly one unit of the mapped target using the standard output mechanism. It MUST consume the source through the standard input mechanism and MUST NOT change the source's item type in place. Recipes omitting `outputByInputKey` MUST retain their fixed-output behavior.

#### Scenario: Preview count does not control runtime count

- **WHEN** a valid mapped recipe declares preview output count five
- **THEN** a successful cycle MUST create only one mapped output unit and zero preview-only items

#### Scenario: Fixed recipe remains unchanged

- **WHEN** a recipe omits `outputByInputKey`
- **THEN** runtime output keys and quantities MUST continue to come from its declared outputs

### Requirement: Mapped result is validated before input consumption

Before consuming input, the craft MUST resolve the selected source key, its map entry, the target item definition, and valid output placement. Availability, start checks, and completion checks MUST use the mapped target for output fit. All source, map, target, quality, stamina, station, and placement validation failures MUST preserve input and produce no output. Completion MUST re-resolve from the current inventory rather than use a stale source or target from cycle start. Standard placement rules MUST remain in effect, including checking available space before input removal and rejecting the cycle if it cannot fit. Successful mutations and any rollback MUST retain the standard atomic craft completion contract.

#### Scenario: Beef placement uses roasted beef

- **WHEN** the selected input is `beef`
- **THEN** placement MUST be checked for `roasted_beef`, using its size and placement restrictions, rather than for `beef` or `roasted_meat`

#### Scenario: Pork placement uses roast pork

- **WHEN** the selected input is `raw_pork`
- **THEN** placement MUST be checked for `roast_pork`, using its size and placement restrictions, rather than for `raw_pork` or `roasted_meat`

#### Scenario: Preview fits but mapped result does not

- **WHEN** the preview item would fit but the mapped target would not fit under standard inventory rules
- **THEN** availability MUST be non-startable, a start request MUST fail, and a completion attempt MUST cancel without consuming input

#### Scenario: Mapped result fits but preview does not

- **WHEN** the mapped target fits but the preview item would not fit
- **THEN** preview placement MUST NOT block the craft if all other requirements pass

#### Scenario: Inventory changes during the cycle

- **WHEN** the first matching input at completion differs from the first matching input at start
- **THEN** final resolution, placement, consumption, and creation MUST all use the current first matching input

#### Scenario: Final target validation or creation fails

- **WHEN** final target resolution or placement fails, or standard output creation fails during commit
- **THEN** the cycle MUST leave no consumed input, deducted craft stamina or station resources, created output, or successful-craft reward

### Requirement: Mapped output uses standard craft quality

Output quality MUST be calculated using the recipe's standard craft quality mechanism and selected input contribution. Mapping MUST only select the output item type and MUST NOT introduce a separate quality formula.

#### Scenario: One weighted source determines quality

- **WHEN** the selected input has quality 37, count one, and quality weight one under `weighted_avg_floor`
- **THEN** the mapped output MUST have quality 37

### Requirement: Missing map entry has a distinct player-visible error

Selecting a source without a map entry MUST fail with a distinct error and the exact message `Roast can't be processed: no info {source_item_key} in roast map`, replacing the placeholder with the actual source key. The source key MUST be displayed unchanged, including underscores. The error MUST NOT be reported as missing input or no output space. Read-only availability MUST be non-startable without repeatedly emitting alerts; explicit start and completion failures MUST surface the error. No input MUST be consumed.

#### Scenario: Missing rabbit mapping reports the actual source

- **WHEN** an explicit craft attempt selects `rabbit_meat` with no map entry
- **THEN** the player MUST see `Roast can't be processed: no info rabbit_meat in roast map` and the rabbit meat MUST remain unchanged
