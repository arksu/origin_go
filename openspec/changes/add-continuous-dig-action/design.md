# Design

## Context

See `proposal.md` for motivation and `specs/` for the behavior contract. `plow_tile` already uses the tile-center approach, action-owned movement, a timed cycle, server-side target validation, and the `dig` cursor. Its `isRepeatable` setting leaves the cursor selecting after a cycle. `ActionService.Complete` currently clears the cycle and ends or rearms the action after each success. The shared cyclic-action lifecycle already supports continuing a cycle with a new index and animation revision; the context-action path uses that operation, but menu actions do not.

The shard has a standard `GiveItem` adapter that places generated items in eligible player inventory grids, then hand, and sends inventory and discovery updates. It does not drop an item when placement fails. `stone` is registered; `soil`, `clay`, and `sand` need definitions. Their PNGs, including `soil.png`, are present in the web asset tree. The configured default tick rate is 10 Hz, so 20 ticks equals two seconds at that rate.

## Goals / Non-Goals

**Goals:**

- Keep automatic execution repeat opt-in and separate from `isRepeatable`, so `plow_tile` and other current actions retain their behavior.
- Reuse the same action-owned target, cycle, cancellation, stamina, and inventory paths for digging.
- Keep each successful grant and 300-stamina charge paired within one authoritative shard update.

**Non-Goals:**

- Terrain depletion, regeneration, quality calculation beyond Q10, equipment or skill gating, or new character animation assets.
- Changing intentional world-item creation, such as objects that spawn dropped items, or rewriting unrelated inventory workflows.
- A new network command or action-specific client click path.

## Decisions

### Add an execution flag without changing selection repeat

Add an optional boolean `repeat` to `actiondefs.Execution`. Omitted and false retain one cycle per accepted click. Validate true only for positive-tick object or tile target actions. Keep `isRepeatable` as the existing post-attempt return-to-selection flag. Define `dig` with `execution: {"ticks":20,"stamina":300,"repeat":true}` and `isRepeatable: false`, so it becomes idle when its continuous attempt stops. `plow_tile` keeps its current definition and click-per-attempt semantics. The client action catalog needs no new field: the server already owns action state and sends cycle progress.

Alternative considered: reinterpret `isRepeatable` as automatic cycling. This would silently change plowing and every other repeatable action, so it is rejected.

### Continue the shared cycle after a successful nonterminal result

Extend the generic action handler result with a successful stop-after-cycle signal. `ActionService` handles a successful auto-repeat cycle by charging its declared stamina once, rechecking the same target and next-cycle affordability, then resetting elapsed ticks, incrementing the cycle index, updating the start tick, and calling the shared cyclic-action continuation path. Preserve the active action generation and target; do not clear/recreate the action between successful cycles. An explicit stop-after-cycle result, failed grant, invalid target, low stamina, or cancellation takes the terminal path, sends the normal finish/state update, and resets the cursor for `dig`. The terminal path must not turn an item granted into the hand into a canceled cycle: its effect and stamina cost are already complete.

Keep the ordinary tile-center arrival rule for the first cycle. On every later completion and before continuation, use the existing actual-position arrival check, handler target validation, and cost check. Make the next cycle visible through its new cycle identity and progress. An execution interrupted during a cycle has no item effect or action stamina cost for that cycle.

Alternative considered: have the dig handler schedule its own timer. That would duplicate cancellation, progress, animation, and action-generation safeguards.

### Make digging a small terrain-to-item handler

Add a `dig` handler registered with the shard's action handlers. Use loaded `GetTileID` and one map from the four `types.Tile*` constants to item keys. `ValidateTarget` reports unloaded and ineligible tiles through the existing action reasons. At each cycle completion, read current terrain again and select its mapped item, so a change to another eligible terrain yields its current resource while a change to ineligible terrain stops the attempt. Never call `SetTile` or mark a chunk dirty. Use Q10 as a dig-local constant until a later quality feature supersedes it.

Inject the existing shard `GiveItem` adapter into the handler rather than creating a second inventory placement path. Its outcome supplies success and `PlacedInHand`: failed placement stops without action stamina, and successful hand placement completes that cycle then stops. Delivery and `ActionService`'s exact stamina charge run synchronously in the shard update after the same affordability check, so no other action can spend stamina between grant and charge. Preserve the standard inventory/discovery notifications.

Alternative considered: create or drop a world item when inventory is full. This contradicts the requested cancellation behavior and bypasses the standard player grant order.

### Register the missing items and reuse assets

Add one-by-one inventory definitions for soil, clay, and sand with unique definition IDs and resources `items/soil.png`, `items/clay.png`, and `items/sand.png`. Reuse the existing stone definition and PNG. The `soil.png` file is already tracked in the repository; implementation should reuse it without overwriting it. Verify the other three assets remain available to the client.

## Risks / Trade-offs

- [A successful grant must never escape an unsuccessful stamina charge] -> Keep the affordability recheck, grant, and charge in the same shard update, use the same effective stamina bounds for both checks, and cover this ordering with a regression test.
- [Automatic repeat might accidentally alter `plow_tile`] -> Gate the loop strictly on `execution.repeat` and test that omitted/false flags still require another click.
- [A stale cycle could grant after cancel or retarget] -> Match the active action generation, target, and cycle identity before applying the effect, and test cancellation and late completion.
- [An item definition might point to a missing image] -> Confirm all four referenced PNGs are present when applying the change.

## Migration Plan

Load the new action and item definitions on server restart. No schema or protocol migration is needed. Existing actions omit `execution.repeat` and retain their current behavior. Rollback removes the new definitions and loop implementation; previously granted item instances require their item definitions to remain available until those inventories have been handled, so a live rollback should keep the three new item definitions and assets.
