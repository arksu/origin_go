# Design

## Context

See `proposal.md` for motivation. This change crosses definition loading, inventory execution, station targeting, and alert transport.

Observed implementation:

- `internal/craftdefs/types.go` has fixed `Inputs` and `Outputs`; `loader.go` validates exactly one of `itemKey`/`itemTag`, positive counts, quality weights, and referenced output definitions.
- `internal/game/inventory/crafting.go` shares preview and consumption through `consumeCraftInputsInternal`, which validates against cloned containers before publishing changes. `craftInputMatchesItemDef` already implements tag matching. `craftOrderedInventoryLinks` uses root grids, nested grids, then hand; group and item slice order are preserved.
- `CanFitCraftOutputsOneCycle` simulates current inventory placement using `craft.Outputs`, without assuming consumed input will free space. `HandleCraftCycleComplete` checks final station state, previews inputs/quality, checks stamina/space, captures a rollback snapshot, consumes, and gives `craft.Outputs` through `GiveItem`.
- `buildCraftList` and start handling also check fit from the declared outputs. Every runtime use must change for mapped recipes, while serialization of those outputs remains the UI preview.
- `resolveRequiredLinkedObject` returns no target when the exact object key is absent. The loader already permits capability/state-only requirements without that key, but the runtime cannot use them successfully. `data/crafts/README.md` still documents the older exact-key restriction. Existing station specs require cycle-boundary revalidation and atomic completion; those contracts remain applicable.
- `CraftWindow.vue` already renders tag icons and output-key icons from `/assets/game/items/`. `data/items/food.jsonc` and existing assets contain all requested source/target/preview keys; the generic `raw_meat` item has no `raw_meat` tag and is not itself a recipe source.
- `S2C_MiniAlert` carries severity, reason code, and TTL only. The client formats reason codes by splitting underscores; the store already accepts an optional explicit message, but the network handler cannot receive one today.

## Goals / Non-Goals

**Goals:** Keep one source of input matching truth; make runtime output resolution explicit; validate all failures before consumption; preserve standard rollback, quality, nested-container updates, and discovery behavior.

**Non-Goals:** Multiple source rows or input units for mapped recipes; in-place item transformations; persistent reservations across a timed cycle; different placement policies; burner logic changes; per-species recipe UI or client-side mapping; new dependencies.

## Decisions

### 1. Optional map with a narrow validated source contract

Add `OutputByInputKey map[string]string` with JSON name `outputByInputKey`. Distinguish absent/nil from a supplied empty map: an empty map must enter mapped resolution and report a missing entry, never fixed-output fallback. Retain ordinary `outputs` validation because they remain valid preview records, including their positive counts and existing item keys.

For non-nil maps validate exactly one tagged input with count one, all ordinary input rules, nonblank source/target keys, both definitions, and each source's membership in that input tag. Match the configured tag generically; Roasted meat supplies `raw_meat`. Do not require coverage of all items with the tag, so later content omissions produce the requested diagnostic. Sort source keys for deterministic load-error reporting; do not silently rewrite map keys into different catalog identities.

The user explicitly chose the one-row/one-unit restriction. Supporting more rows would require new source precedence rules with no benefit to this recipe.

### 2. Expose selection from the existing input pass

Extend the shared input preparation path to expose the selected source key (and the selected item identity for consistency checks) alongside its quality aggregates and staged container changes. Derive this information at the existing positive-quantity match/consume point, using `craftInputMatchesItemDef` and `craftOrderedInventoryLinks`; do not scan separately for a mapped item or iterate the map to choose inventory input.

Use a short-lived prepared input result during completion: standard input consumption publishes its already validated container changes after all final checks. Keep preview and consume public helpers as thin wrappers around the shared preparation/commit logic. The plan is local to the synchronous completion call and must not be retained across ticks. This avoids separately selecting one species for fit and another for consumption without introducing inventory locking or long-lived reservations.

At start and in list snapshots, use read-only preparation. At completion, prepare afresh so current inventory order determines the consumed source. Invalid/unknown item definitions retain the existing input-validation behavior; a selected tagged definition lacking a map entry is the distinct map failure.

### 3. Explicit resolved outputs feed both placement and creation

Introduce a small runtime output resolver using the prepared input result. Fixed recipes resolve their declared outputs. Mapped recipes resolve the source key through the map and item registry into one concrete output with count one. Fail distinctly for a missing map entry or invalid target; never fall back to preview output.

Factor the existing fit simulation so it accepts resolved outputs explicitly. Route mapped recipe availability, start, completion, and continuation through this resolver. The final `GiveItem` loop must use exactly the resolved outputs that passed fit. Keep `craft.Outputs` only for recipe serialization and fixed-recipe resolution, not mapped fit or creation. Avoid manufacturing a modified copy of `CraftDef` with overwritten outputs, which obscures preview/runtime semantics.

Keep the existing strict fit policy: validate target dimensions and content/hand restrictions against space available before consumption; do not assume input removal frees a destination and do not introduce drop fallback. `beef` therefore checks `roasted_beef`, and `raw_pork` checks `roast_pork`, even if placeholder dimensions/restrictions differ.

### 4. Preserve the existing finalization transaction

At final completion, first verify the bound link and current station requirements. Prepare/validate inputs, resolve source/map/target, calculate standard quality, check stamina, and simulate resolved output placement before publishing input changes. Then retain the existing cycle snapshot and standard input consumption, stamina/resource consumption, and `GiveItem` creation. Any commit failure restores the snapshot before reporting cancellation. Send inventory/reward notifications only after success.

Use the prepared input quality aggregates with `computeCraftQuality`; do not add roast-specific quality logic. One source at weight one produces the standard weighted-average result. New output identity comes from `GiveItem`; consuming one unit must not rewrite the original item's type, including when a stack remains.

For craft-many, resolve anew for each cycle and revalidate station/inputs/output before starting the next cycle. An unlit final station cancels only the pending cycle; earlier completed outputs remain. Retain existing station-before-craft system ordering and cover fuel exhaustion in the completion tick with a regression scenario.

### 5. Resolve capability-only station links through the existing link state

Treat a craft as needing a linked target when it has either an exact linked-object requirement or nonempty station requirements. Resolve the player's current link in both cases; apply the object-key check only when an exact key exists. Use object cyclic-action targeting for either form. Share target resolution across list, start, active validity, completion, and continuation so flags and execution agree.

Bind the action to its starting station ID. At completion require that the player's current live link still points there, then use the station evaluator for capability/state. A changed/broken link cancels; a replacement burning station does not silently inherit the old cycle. Portable crafts without station requirements or exact keys keep their link-free behavior. Exact-key recipes retain their start-time object-key restriction and are bound to the live link like capability-only recipes: unlinking or switching stations mid-cycle cancels them too.

Hardcoding `requiredLinkedObjectKey: "campfire"` was considered but would restrict the requested capability-based recipe. Update catalog documentation to remove its stale exact-key-only claim. The new station spec delta makes the previously unspecified keyless link behavior explicit.

### 6. Preserve exact error text through an additive alert message

Represent the missing-entry result with a distinct reason code, proposed `CRAFT_ROAST_MAP_ENTRY_MISSING`, and its source key. Add an optional `message` field using the next available field number in `S2C_MiniAlert`; regenerate Go and client bindings through the existing `make proto` and client proto scripts. Extend the craft alert helper without changing existing callers' fallback behavior.

For this failure send the exact formatted message `Roast can't be processed: no info {source_item_key} in roast map`. Forward the optional message in `web_new/src/network/handlers.ts` to the store's existing `message` field. Plain text rendering preserves punctuation and underscores. Older clients can still format the reason code; updated clients display the exact message. Putting dynamic text into `reason_code` would break existing formatting and stable error classification, so keep them separate.

Read-only craft-list evaluation sends no alerts and marks the recipe non-startable when source/output resolution fails. Explicit start or failed completion emits the distinct reason and message rather than collapsing it into missing-input/no-space errors. No new recipe-map field needs to be transmitted to the UI.

### 7. Reuse existing content and record minor recipe defaults

Add a `roasted_meat` recipe with name `Roasted meat` and the exact inputs, preview output, station requirements, and nine mappings in the recipe spec. Reuse all existing food definitions and assets; do not rename `roast_pork`. Choose the next available positive craft definition ID when implementing.

Timing and stamina were not specified. Proposed catalog defaults are `ticksRequired: 10` and `staminaCost: 10`, with no extra skill/discovery gates and the default `weighted_avg_floor` quality formula. These are explicit tuning assumptions, not a new timing or quality mechanism; keep them in recipe configuration. The UI already supports both requested generic icons without component changes.

## Risks / Trade-offs

- [Mapped fit appears correct because all current meats are 1x1] → Test synthetic preview/target definitions with different sizes and content restrictions, in both fit directions, including `beef` and `raw_pork` mappings.
- [Preview and consumption drift] → Expose selection from the shared preparation pass and use its staged inputs for commit; test root/nested/hand ordering, multiple matches, stacks, and inventory changes during the timed cycle.
- [Empty map accidentally behaves like an absent map] → Use presence/nil semantics rather than length and test both empty and omitted maps.
- [Shared inventory or link changes regress older recipes] → Retain focused fixed-output, exact-key station, portable craft, rollback, and nested-container tests.
- [Existing catalog integration fixtures may drift] → The integration fixture expected the removed `campfire_threaded_branch` recipe; the fixture was repaired to match the current catalog instead of restoring removed content, and unrelated failures are reported separately.
- [Recipe balancing defaults need later tuning] → Keep duration and stamina in JSONC and document the chosen values in the implementation summary.
- [Mixed client/server versions lack exact error text] → Add the field compatibly and deploy updated client assets with the server; reason-code fallback remains usable.

## Migration Plan

No database migration or existing item conversion is required. Ship definition/runtime support and the recipe together, regenerating protocol bindings and deploying the updated client for exact alert text. Existing recipes omit the map and keep their behavior. To roll back, remove the new recipe before reverting mapped-output support; already crafted species outputs use existing definitions and remain valid.
