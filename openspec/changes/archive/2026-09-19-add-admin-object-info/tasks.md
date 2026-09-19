# Tasks

## 1. Server-side inspection lifecycle

- [x] 1.1 Add a per-player `PendingAdminObjectInfo` resource and wire `/info` command parsing, no-argument validation, prompt, mutual exclusion with `/spawn` and `/tp`, and player transient-state cleanup; verify focused admin-command tests cover arming, usage errors, replacement, and isolation between players.
- [x] 1.2 Extend the admin-command/network-command contract so a pending inspection consumes the next target-bearing interaction and the next targetless movement click; verify focused network-command tests prove an inspected target bypasses normal interaction and a ground click reports failure without moving the player.
- [x] 1.3 Implement a constrained, deterministic runtime-state chat formatter for a live object using only entity identity plus `ObjectInternalState` flags, behavior state, and mutable station state; verify admin-command tests report campfire `burner.fuel`, omit object-definition/template fields, handle absent state, disappeared targets, and serialization failures without panic.

## 2. Client one-shot object selection

- [x] 2.1 Add a renderer/facade one-shot admin-inspection mode that uses the existing topmost-visible-object picker on primary click, sends the existing interaction command for an object, sends the existing movement command for empty ground, and consumes the UI click; verify TypeScript type-check and a focused renderer test or testable helper cover object, ground, and mode-reset paths.
- [x] 2.2 Arm that mode only when the chat submitter sends exact `/info`, and cancel it on Escape and after the first click without changing ordinary click handling; verify `npm --prefix web_new run type-check` succeeds and manually confirm a normal primary click still moves when no inspection is armed.

## 3. Integration verification

- [x] 3.1 Run `go test ./internal/game ./internal/ecs/systems` and record successful focused server verification or any unrelated failures separately.
- [x] 3.2 Run `go test ./...` and `npm --prefix web_new run build`; manually verify `/info` then a campfire click prints current `burner.fuel` in chat and does not print burner configuration/template data.

Verification recorded on 2026-09-19: the Go suite and client production build passed during implementation. The user's in-game report for newly built campfire 485678 contained `behaviors.burner.fuel` and `next_fuel_burn_at_runtime_second`, without template configuration. The exhaustion persistence failure reported alongside that snapshot was handled separately.
