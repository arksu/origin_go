# Proposal

## Why

Players need one Roasted meat recipe that converts each raw meat species into its corresponding cooked item at a burning cooking station. Crafting currently creates fixed declared outputs, so it cannot preserve species through a tag-based recipe while validating the actual result before spending inputs.

## What Changes

- Add optional `CraftDef.outputByInputKey`, restricted in v1 to exactly one input row with `itemTag` and `count: 1`, as confirmed by the user.
- Validate map keys, item definitions, and source membership in the input tag during definition loading. Missing coverage remains a distinct runtime error for the selected source.
- Reuse standard input matching, validation, deterministic inventory traversal, quality calculation, consumption, and output placement/creation. Resolve exactly one mapped output before consumption; never create preview outputs or mutate the source into another item.
- Add `Roasted meat` with the nine requested mappings, one `raw_meat` input, and `roasted_meat` preview output. Keep the requested generic consume/result icons.
- Support linked stations selected by `stationRequirements` without an exact object key; require capability `cooking` and state `burning` at cycle boundaries, preserving input when fire is unlit at completion.
- Surface the distinct missing-map error exactly as `Roast can't be processed: no info {source_item_key} in roast map`.

## Capabilities

### New Capabilities

- `craft-mapped-output`: Validated input-key mapping, deterministic source resolution, one mapped runtime output, pre-consumption placement, standard quality, and distinct failures.
- `roasted-meat-craft`: Recipe content, exact species mappings, burning cooking requirement, and generic preview icons.

### Modified Capabilities

- `craft-station-requirements`: Explicitly allow requirements to select the player's linked station without `requiredLinkedObjectKey`, while retaining exact-object restrictions when supplied and binding cycles to that station.

## Impact

- Definition types/loading and tests in `internal/craftdefs`; recipe content and documentation in `data/crafts`.
- Shared input preview/consumption and output fit helpers in `internal/game/inventory/crafting.go`; availability, start, completion, continuation, and link handling in `internal/game/crafting_service.go`.
- Additive mini-alert message transport in `api/proto/packets.proto`, regenerated bindings, and client forwarding in `web_new/src/network/handlers.ts`. Existing reason codes and fallback formatting remain compatible.
- Existing food definitions and icon assets already contain all requested keys; reuse them. No new dependencies, database migration, item mutation mechanism, or autonomous burner changes.
