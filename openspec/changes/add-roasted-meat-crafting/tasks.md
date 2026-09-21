# Tasks

## 1. Definition contract and validation

- [ ] 1.1 Add optional `OutputByInputKey` to `internal/craftdefs/types.go` with absent-versus-empty-map semantics; verify decoding tests cover omitted and supplied empty maps and fixed-output compatibility.
- [ ] 1.2 Extend `internal/craftdefs/loader.go` to require exactly one tagged input with count one for mapped recipes and validate every nonblank source/target key, referenced definition, and source tag membership; verify table-driven loader tests cover every invalid case, ordinary quality-weight validation, valid generic tags, and incomplete map coverage.

## 2. Shared input preparation and resolved outputs

- [ ] 2.1 Refactor the existing input preview/consume path in `internal/game/inventory/crafting.go` to expose selected source identity/key and staged input changes with quality aggregates, keeping existing matching and traversal; verify tests cover root/nested/hand order, order within groups/containers, multiple matching species, stack quantity one, read-only preview, and fixed-recipe input behavior.
- [ ] 2.2 Add explicit runtime output resolution from the selected source to one mapped target, preserving fixed-output resolution for absent maps; verify tests cover missing/empty maps, invalid target definitions, first unmatched-map source before a later mapped source, and preview counts/lists having no effect on mapped output count.
- [ ] 2.3 Factor the standard fit helper to accept resolved outputs; verify synthetic beef/roasted_beef and raw_pork/roast_pork cases with distinct dimensions and placement rules, preview-fits/target-fails and target-fits/preview-fails cases, and unchanged strict pre-consumption space policy.

## 3. Craft service integration

- [ ] 3.1 Use shared input preparation and mapped output resolution for `buildCraftList` and explicit craft start checks; verify availability and start behavior agree on mapped fit, unresolved maps disable starting without snapshot alert spam, and neither path changes inventory or stamina.
- [ ] 3.2 Update completion to resolve source/map/target/quality/placement before publishing standard input consumption and to give only the resolved output; verify successful cycles create one new mapped item with standard quality, preserve remaining source stacks, and never create the generic preview item even when preview count exceeds one.
- [ ] 3.3 Keep completion rollback and notification ordering around standard input, stamina, resource, and output mutations; verify failure tests for missing mapping, unavailable target, unsupported quality, insufficient stamina, no space, and output creation failure preserve input and leave no partial rewards or outputs.
- [ ] 3.4 Re-resolve the selected input at completion and before subsequent craft-many cycles; verify a source-order change during a cycle changes both consumed source and mapped output consistently, and consecutive cycles can roast different species in standard traversal order.

## 4. Capability-based linked stations

- [ ] 4.1 Extend shared link resolution, availability flags, and cyclic action targeting to station requirements without an exact object key; verify keyless cooking recipes work at two differently keyed suitable stations, no-link/wrong-capability/unlit cases fail, and exact-key plus portable recipes retain existing behavior.
- [ ] 4.2 Bind active cycles to the original live link and recheck station requirements at completion and before continuation; verify unlink/relink/object-removal cancellation, burning-to-unlit final cancellation without consumption, same-tick autonomous fuel exhaustion, and preservation of earlier successful craft-many outputs.

## 5. Missing-map error transport

- [ ] 5.1 Add optional explicit message text to `S2C_MiniAlert` in `api/proto/packets.proto`, regenerate bindings with existing protocol scripts, and send a distinct missing-map reason plus the exact source-key message from craft start/completion failures; verify protobuf round-trip and service tests preserve punctuation/underscores and ordinary alerts remain compatible.
- [ ] 5.2 Forward the optional mini-alert message in `web_new/src/network/handlers.ts` into the store's existing message field; verify the UI displays `Roast can't be processed: no info rabbit_meat in roast map` unchanged and legacy messages still use reason-code fallback.

## 6. Recipe content and documentation

- [ ] 6.1 Add the `roasted_meat` / `Roasted meat` recipe to `data/crafts` with a unique positive ID, exact nine-entry mapping, raw_meat tag/count/weight, generic roasted_meat preview, cooking/burning requirements, and design defaults of 10 ticks and 10 stamina; verify catalog integration tests assert the full map, source tags, target definitions, default quality, and absence of exact-object, temperature, or implicit fuel constraints.
- [ ] 6.2 Update `data/crafts/README.md` with the optional map contract, one-row/one-unit restriction, preview-only outputs, source-tag validation, runtime missing-map error, and keyless station support; verify documented examples match the loader and new recipe.
- [ ] 6.3 Verify the existing craft UI renders `items/raw_meat` and `items/roasted_meat` previews with one input/output count, while actual output is species-specific; record a UI smoke check using beef and raw pork and reuse existing assets.

## 7. Integrated verification

- [ ] 7.1 Run table-driven end-to-end craft completion tests for all nine real mappings, including the exceptional `raw_pork` to `roast_pork` spelling, and verify input consumption, new output identity/count, standard quality, and discovery behavior.
- [ ] 7.2 Run `go test ./internal/craftdefs ./internal/game/inventory ./internal/game/stationreq ./internal/game ./internal/network/proto` and the client production build with regenerated protocol bindings; record results and distinguish pre-existing catalog-fixture failures from regressions without changing unrelated content.
- [ ] 7.3 Run `openspec validate add-roasted-meat-crafting --strict`, inspect the final diff for mapped runtime reads of preview outputs and unrelated edits, and record validation plus commit status in the implementation summary.
