# Tasks

## 1. Extend timed action definitions and cycle lifecycle

- [x] 1.1 Add optional `execution.repeat` to action definitions and reject true for targetless or non-timed actions; verify loader tests cover omitted, false, valid true, and invalid true with file-named errors using `go test ./internal/actiondefs`.
- [x] 1.2 Make a successful repeating menu-action cycle continue on its accepted target with a new cycle index, reset progress, and no intermediate terminal finish; verify focused `internal/game` tests observe two effects from one click and distinct cycle progress.
- [x] 1.3 Add a handler success signal to stop after the current charged cycle, and preserve cancellation and generation guards; verify focused tests cover hand-stop, Escape/secondary-click cancellation, and stale completion without an extra effect.
- [x] 1.4 Keep `isRepeatable` independent of `execution.repeat`; verify existing plow tests still require a new click and both flag combinations have their specified terminal selecting/idle state with `go test ./internal/game -run 'Test(Action|Plow)'`.

## 2. Add dig definitions and item delivery

- [x] 2.1 Register soil, clay, and sand with unique item IDs and their existing PNG resources, retain the existing stone definition, and reuse `soil.png` without overwriting it; verify item-def loading and all four asset paths.
- [x] 2.2 Add the `dig` action definition with tile-center approach, the `dig` cursor, 20 ticks, 300 stamina, `execution.repeat: true`, and `isRepeatable: false`; register its handler and verify the production action registry loads with the new catalog entry using `go test ./internal/actiondefs ./internal/game`.
- [x] 2.3 Implement terrain eligibility and current-tile item mapping without writing terrain; verify focused tests cover all four allowed tiles, rejected/unloaded tiles, a tile changing to another allowed type, and no chunk-version change.
- [x] 2.4 Route dig rewards through the shared shard `GiveItem` adapter at Q10 and use its `PlacedInHand` result to stop immediately; verify inventory and discovery notifications, the fallback order, and one item per successful cycle in focused tests.

## 3. Verify stopping rules and integration

- [x] 3.1 Cover exact 300-stamina accounting for each successful cycle and no action cost or item on low stamina, full inventory plus hand, failed grant, blocked approach, or movement away; verify focused `internal/game` tests pass.
- [x] 3.2 Cover continuous same-tile digging, successful hand fallback followed by immediate idle state, terrain becoming ineligible, cancellation, and a new click after stopping; verify the action does not change tiles or begin a successor cycle after stop.
- [x] 3.3 Run `openspec validate add-continuous-dig-action --strict` and the affected Go test suites (`go test ./internal/actiondefs ./internal/itemdefs ./internal/game ./internal/game/inventory`); verify no unrelated working-tree files were changed.
