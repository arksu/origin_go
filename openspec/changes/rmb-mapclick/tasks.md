# Tasks

## 1. Unified input protocol

- [ ] 1.1 Add the primary/secondary button enum and MapClick button field, retire Interact and InteractionType, reserve the retired action tag/name and movement tags/names, and regenerate Go/browser bindings with `make proto` and `npm --prefix web_new run proto`; verify generated descriptors and protocol round-trip tests cover both buttons, omitted-button primary, and raw retired action tags.
- [ ] 1.2 Remove the obsolete interaction-type dependency from PendingInteraction and AutoInteractSystem while keeping the existing delayed pickup flow; verify migrated pickup assertions and AutoInteract tests still cover arrival-based pickup with no protocol interaction enum references in maintained source.

## 2. Direct put-down lifecycle

- [ ] 2.1 Add a server-only targeted-once ActionService entry point with shared definition/requirement/target validation, a fresh generation, and an explicit direct-attempt policy in transient action state; verify ActionService tests demonstrate no selecting state for a direct attempt and failure/completion ending idle, while ordinary activation and repeat/retry behavior remain unchanged.
- [ ] 2.2 Connect the direct entry point to the existing lift_down handler and deferred put-down flow, including rejection, timeout, Escape/requirement-loss cancellation, and exact coordinate placement; verify real lift/action integration tests retain valid carry on failure and preserve explicit menu-activated lift_down rejection/timeout retry selection.
- [ ] 2.3 Verify replacement and stale-completion safety for direct placement generations, repairing cleanup only if needed; add integration tests where RMB position A is replaced by B, a late A callback cannot relocate the object or clear B, and cancellation clears the pending transition, phantom, and owned movement without an action stamina charge.

## 3. Server button routing

- [ ] 3.1 Remove the Interact decoder/command branch and leave its command-ID slot vacant; split MapClick into primary/secondary routes with unsupported-button rejection before side effects, preserving primary admin/armed/pickup/link/movement order. Verify game routing tests ignore raw retired Interact and movement payloads and accept both supported MapClick buttons.
- [ ] 3.2 Integrate secondary cancellation and carry-first direct placement, and extract the old object-interaction body into a reusable live-target context helper; verify NetworkCommandSystem tests cover carry over object/dropped/ground/stale target, exact click coordinates, no fallback after placement rejection, and no newly started ground movement or hand-item dropping.
- [ ] 3.3 Migrate existing context tests to secondary MapClick and extend administrator/action routing coverage; verify zero/single/multiple context-action rules, the single-action menu override, secondary pickup once, cancellation in selecting/approaching/executing, all pending administrator command kinds remaining pending on RMB, and unchanged primary selection precedence.
- [ ] 3.4 Add end-to-end ECS command-to-action/lift regression coverage using the real ActionService and LiftService; verify an RMB command cancels the old action before starting one new placement attempt, failure leaves carry plus idle state, Escape stops the direct approach, and ordinary primary input can interrupt the direct approach safely.

## 4. Browser input

- [ ] 4.1 Extend sendMapClick with a typed default-primary button and remove sendInteract; update Render secondary routing to send coordinates, hit-tested target or zero, button, and modifiers for objects and ground. Verify the browser map-click suite observes one secondary packet and no extra CancelAction, inventory drop, or primary placement request in idle and all active action phases.
- [ ] 4.2 Route touch long-press through the same secondary sender, retain release-tap suppression, and close stale context menus for both secondary ground and object input; verify input/Render tests demonstrate long-press equivalence, preserved modifiers and coordinate rounding, no release duplicate, and no gameplay packet for middle-button camera panning.
- [ ] 4.3 Retain primary dropped-item, hand-item, armed targeting, and UI-consumed placement priorities in the existing browser regression suite; verify `npm --prefix web_new run test:map-click` and `npm --prefix web_new run test:actions` pass with both primary and secondary cases.

## 5. Integration and documentation

- [ ] 5.1 Update maintained protocol/input/context-flow documentation, including affected AGENTS.md references, and retain default-primary load-test behavior; verify the load-test MapClick test passes and a source/documentation search finds no maintained sendInteract or separate Interact packet instructions, excluding historical OpenSpec changes and intentional retired-input tests.
- [ ] 5.2 Run the combined checks `go test ./internal/network/proto ./internal/game ./internal/ecs/systems ./cmd/load_test`, `npm --prefix web_new run test:map-click`, `npm --prefix web_new run test:actions`, and `npm --prefix web_new run build`; verify they pass and record any environment/tooling limitation separately from product failures. Perform a browser smoke check for RMB ground cancellation, context menus, carrying object placement at the clicked point, rejected placement keeping carry without an armed action, and touch long-press without duplicate input.
