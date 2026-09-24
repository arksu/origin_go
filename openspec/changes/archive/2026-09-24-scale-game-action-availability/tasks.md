# Tasks

## 1. Remove availability from the protocol

- [x] 1.1 Delete fields 11 and 12 from `ActionDefinition` without reserving them, then regenerate Go and browser bindings with `make proto` and `npm --prefix web_new run proto`; verify the generated definition has neither field and both generators succeed.

## 2. Simplify server action processing

- [x] 2.1 Build `S2C_ActionList` only from loaded definitions on enter-world bootstrap, remove availability signatures and refresh sends, and verify service tests receive one ordered catalog with no availability fields and no list resend after requirement changes or rejected activation.
- [x] 2.2 Restrict `ActionValidationSystem` to players with `ActiveGameAction`; verify a focused test shows idle players trigger no action requirement checks while active selection, approach, and timed execution still recheck and cancel on requirement or target loss.
- [x] 2.3 Preserve direct server checks at activation, target acceptance, handler start, and effect commit; verify tests for skill, equipment, stamina, and lift carry state reject or cancel correctly, emit the existing mini-alert on activation refusal, and never charge canceled work.
- [x] 2.4 Remove now-unused availability cache cleanup and callers while preserving re-entry bootstrap; verify leave/re-enter tests receive a fresh catalog and no stale active action state.

## 3. Make client actions uniformly selectable

- [x] 3.1 Remove availability gating, disabled styling, and reason text from `ActionsMenu`; verify the action UI test shows every server-listed icon is keyboard-accessible, clickable, draggable, and retains the active marker.
- [x] 3.2 Send activation from the panel for every catalog ID and close the panel immediately without waiting for a server response; verify a rejected lift_down request closes the panel and the existing mini-alert path displays its reason.
- [x] 3.3 Remove availability gating from hotbar activation while retaining catalog membership and loading checks; verify valid pinned actions send the same request as the panel and missing IDs send nothing after the list loads.

## 4. Verify the integrated change

- [x] 4.1 Remove unused availability-only code and test assertions in server and client, then verify a repository search finds no runtime reads of the removed protocol fields or availability signature.
- [x] 4.2 Run `go test ./...`, `go build ./...`, `npm --prefix web_new run test:actions`, `npm --prefix web_new run type-check`, `npm --prefix web_new run build`, and `openspec validate scale-game-action-availability --strict`; record results and any environment-limited command in this change.
