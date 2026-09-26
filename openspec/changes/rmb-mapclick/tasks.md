# Tasks

## 1. Unified input protocol

- [x] 1.1 Add the primary/secondary button enum and MapClick button field, retire Interact and InteractionType, reserve the retired action tag/name and movement tags/names, and regenerate Go/browser bindings with `make proto` and `npm --prefix web_new run proto`; verify generated descriptors and protocol round-trip tests cover both buttons, omitted-button primary, and raw retired action tags.
- [x] 1.2 Remove the obsolete interaction-type dependency from PendingInteraction and AutoInteractSystem while keeping the existing delayed pickup flow; verify migrated pickup assertions and AutoInteract tests still cover arrival-based pickup with no protocol interaction enum references in maintained source.

## 2. Direct put-down lifecycle

- [x] 2.1 Add a server-only targeted-once ActionService entry point with shared definition/requirement/target validation, a fresh generation, and an explicit direct-attempt policy in transient action state; verify ActionService tests demonstrate no selecting state for a direct attempt and failure/completion ending idle, while ordinary activation and repeat/retry behavior remain unchanged.
- [x] 2.2 Connect the direct entry point to the existing lift_down handler and deferred put-down flow, including rejection, timeout, Escape/requirement-loss cancellation, and exact coordinate placement; verify real lift/action integration tests retain valid carry on failure and preserve explicit menu-activated lift_down rejection/timeout retry selection.
- [x] 2.3 Verify replacement and stale-completion safety for direct placement generations, repairing cleanup only if needed; add integration tests where RMB position A is replaced by B, a late A callback cannot relocate the object or clear B, and cancellation clears the pending transition, phantom, and owned movement without an action stamina charge.

## 3. Server button routing

- [x] 3.1 Remove the Interact decoder/command branch and leave its command-ID slot vacant; split MapClick into primary/secondary routes with unsupported-button rejection before side effects, preserving primary admin/armed/pickup/link/movement order. Verify game routing tests ignore raw retired Interact and movement payloads and accept both supported MapClick buttons.
- [x] 3.2 Integrate secondary cancellation and carry-first direct placement, and extract the old object-interaction body into a reusable live-target context helper; verify NetworkCommandSystem tests cover carry over object/dropped/ground/stale target, exact click coordinates, no fallback after placement rejection, and no newly started ground movement or hand-item dropping.
- [x] 3.3 Migrate existing context tests to secondary MapClick and extend administrator/action routing coverage; verify zero/single/multiple context-action rules, the single-action menu override, secondary pickup once, cancellation in selecting/approaching/executing, all pending administrator command kinds remaining pending on RMB, and unchanged primary selection precedence.
- [x] 3.4 Add end-to-end ECS command-to-action/lift regression coverage using the real ActionService and LiftService; verify an RMB command cancels the old action before starting one new placement attempt, failure leaves carry plus idle state, Escape stops the direct approach, and ordinary primary input can interrupt the direct approach safely.

## 4. Browser input

- [x] 4.1 Extend sendMapClick with a typed default-primary button and remove sendInteract; update Render secondary routing to send coordinates, hit-tested target or zero, button, and modifiers for objects and ground. Verify the browser map-click suite observes one secondary packet and no extra CancelAction, inventory drop, or primary placement request in idle and all active action phases.
- [x] 4.2 Route touch long-press through the same secondary sender, retain release-tap suppression, and close stale context menus for both secondary ground and object input; verify input/Render tests demonstrate long-press equivalence, preserved modifiers and coordinate rounding, no release duplicate, and no gameplay packet for middle-button camera panning.
- [x] 4.3 Retain primary dropped-item, hand-item, armed targeting, and UI-consumed placement priorities in the existing browser regression suite; verify `npm --prefix web_new run test:map-click` and `npm --prefix web_new run test:actions` pass with both primary and secondary cases.

## 5. Integration and documentation

- [x] 5.1 Update maintained protocol/input/context-flow documentation, including affected AGENTS.md references, and retain default-primary load-test behavior; verify the load-test MapClick test passes and a source/documentation search finds no maintained sendInteract or separate Interact packet instructions, excluding historical OpenSpec changes and intentional retired-input tests.
- [x] 5.2 Run the combined checks `go test ./internal/network/proto ./internal/game ./internal/ecs/systems ./cmd/load_test`, `npm --prefix web_new run test:map-click`, `npm --prefix web_new run test:actions`, and `npm --prefix web_new run build`; verify they pass and record any environment/tooling limitation separately from product failures. Perform a browser smoke check for RMB ground cancellation, context menus, carrying object placement at the clicked point, rejected placement keeping carry without an armed action, and touch long-press without duplicate input.

## Verification — 2026-09-26

- Passed: `GOCACHE=/private/tmp/origin-go-rmb-cache go test ./internal/network/proto ./internal/game ./internal/ecs/systems ./cmd/load_test`.
- Passed: `npm --prefix web_new run test:map-click` (three tests covering primary/secondary routing, modifiers, hand items, long-press release suppression, and middle-button panning), `npm --prefix web_new run test:actions`, and `npm --prefix web_new run build`.
- Browser smoke passed on an isolated local integration fixture using the production InputController, Render input callbacks, PlayerCommandController, protobuf encoding/decoding, NetworkCommandSystem, ActionService, and LiftService. RMB ground canceled selecting lift with no movement; RMB on a collider canceled lift and opened its context menu; carrying plus RMB on an object placed at `(210, 145)` rather than that object's center `(200, 150)`; invalid placement preserved carry with `LIFT_PUTDOWN_INVALID` and idle/empty cursor; touch long-press plus release sent exactly one secondary packet and canceled selection.
- Smoke scope: the fixture supplied test entities/context actions and simulated approach completion through LiftPlacementSystem. It did not connect to the running game or its database. Temporary fixture files, server, and browser tab were removed after verification. Deferred timeout, replacement, late callback, Escape, primary interruption, and explicit lift_down retry are covered by the permanent Go tests.
- Environment: the default Go build cache was inaccessible inside the sandbox; the writable temporary cache resolved this. Binding the isolated loopback test server required sandbox escalation and succeeded. These were tooling restrictions, not product test failures.
- Build warnings remain for protobufjs `eval`, mixed static/dynamic network imports, and a vendor chunk above 600 kB; the production build completed successfully.
