# Implementation verification

Implemented the Roasted meat catalog recipe and optional mapped-output mechanic, including the confirmed single tagged input/count-one restriction. Runtime selection, quality, input commit, and output placement/creation share the standard crafting paths. Capability-only station requirements now resolve the player's live link. The exact missing-map message is carried by an additive optional mini-alert message field.

Recipe tuning: 10 ticks, 10 stamina, standard weighted-average quality, no additional skill/discovery requirements. All nine existing species definitions and generic preview icons are reused.

## Checks

- Definition and catalog tests passed: `go test ./internal/craftdefs -run 'TestMapped|TestLoadFromDirectory|TestLoadRoastedMeatCatalog'`.
- The full planned command was run before and after implementation: `go test ./internal/craftdefs ./internal/game/inventory ./internal/game/stationreq ./internal/game ./internal/network/proto`. Both runs failed only in the existing `TestLoadAllCrafts_RegistersStationRequirementFixtures`, which expects the absent `campfire_threaded_branch` recipe. This unrelated fixture/catalog mismatch predated the change (commit 21d8646 removed the recipe without updating the fixture).
- All five packages passed with only that confirmed baseline failure excluded: `go test ./internal/craftdefs ./internal/game/inventory ./internal/game/stationreq ./internal/game ./internal/network/proto -skip '^TestLoadAllCrafts_RegistersStationRequirementFixtures$'`.
- Additional focused tests passed for nested output content restrictions and preservation of quality-overflow errors.
- `npm run build` in `web_new` passed, including regenerated protocol bindings and TypeScript checking of the browser harness. Vite reports the existing large-chunk warning.
- Browser smoke test at `web_new/tests/roasted-meat.html` passed 13 checks: generic input/result icons and counts for beef/pork recipe metadata, successful icon decoding, exact error text through protobuf and the real network handler/store, legacy alert fallback, and both craft buttons disabled for an unlit station. The rendered icons were visually inspected. This harness uses client fixtures; server behavior is covered by the service tests, not a logged-in live-game session.
- `openspec validate add-roasted-meat-crafting --strict` and `git diff --check` passed.

## Regression coverage

Server tests cover all nine actual catalog mappings, new output identity, one-unit creation regardless of preview counts/rows, standard quality and discovery, stack preservation, root/nested/hand traversal, fixed-input precedence, source changes between start and completion, missing/empty maps, no skipping unmapped sources, target dimensions and placement restrictions, stale prepared inputs, and rollback on output creation failure.

Station tests cover differently keyed cooking stations, retained exact-key restrictions, portable crafts, missing/unsuitable links, link switching/removal, unlit final checks, repeated cycles, and same-tick fuel exhaustion.

Mapped-size tests exposed an existing unsigned-underflow bug in `PlacementService.FindFreeSpace`. A bounds guard now rejects zero-sized or oversized items before subtracting dimensions. This narrow shared-helper correction is necessary for mapped placement checks and standard output creation to agree.

Final review confirmed `craft.Outputs` is read only for preview serialization or fixed-output resolution; mapped creation uses the resolved target list. No database migration, source-item type mutation, or burner behavior change was introduced.

## Post-review repairs

- `TestLoadAllCrafts_RegistersStationRequirementFixtures` was repaired by removing its `campfire_threaded_branch` expectations; the full planned command now passes without `-skip`.
- The roast-specific player message moved out of the generic inventory layer: `inventory.CraftMapEntryMissingError` reports a neutral message (`no mapped output entry for source item {key}`), and `CraftingService` composes the spec-pinned roast text next to the `CRAFT_ROAST_MAP_ENTRY_MISSING` reason code. Player-visible text is unchanged.
- Removed the production-dead helpers `HasCraftInputs`, `ConsumeCraftInputs`, and `CanFitCraftOutputsOneCycle`; tests use the preview/commit and resolved-fit APIs directly.
- design.md decision 5 now records that exact-key recipes are bound to the live link like capability-only recipes (unlink/switch mid-cycle cancels), matching the implemented link-binding behavior.

## Delivery state

All 19 implementation tasks are complete. Changes are uncommitted; the OpenSpec change remains active and has not been archived.
