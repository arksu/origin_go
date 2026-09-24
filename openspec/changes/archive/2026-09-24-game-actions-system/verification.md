# Verification log

Automated checks for this change:

| Scenario | Evidence |
| --- | --- |
| Action definitions, validation, handler matching | `go test ./internal/actiondefs` |
| Enter-world list and state snapshots; skill, equipment, stamina, carry availability | `TestEnterWorldQueuesActionSnapshot`, `TestActionSnapshotJobSendsListAndState`, `TestActionRequirementsAndAvailabilityRefresh`, `TestNoColliderLiftCompletesAndLiftDownPlacementLifecycle` |
| Collider and no-collider lift, invalid target, target loss, cancellation, timeout | `TestColliderLiftApproachesAndCompletesAfterLink`, `TestColliderLiftRejectsNonLiftableAndTargetLoss`, `TestColliderLiftCancelStopsApproach`, `TestNoColliderLiftRequiresArmingAndCancelClearsApproach`, `TestNoColliderLiftTimeoutReturnsToSelection` |
| Put-down ghost state, valid/rejected placement, timeout, forced carry loss | Server transition and placement tests in `lift_action_handlers_test.go`; `GameView` watches authoritative `lift_down/selecting` state to drive the ghost. Test relocation uses a deterministic in-memory substitute for chunk storage. |
| Switch during approach/cycle, lost requirements, late callback, repeatability | `TestActionSwitchCancelsApproachAndLateCallback`, `TestActionLossDuringCycleAndUnavailableReplacement`, `TestActionRepeatAndTargetFailure`, `TestRepeatableTileActionAcceptsAnotherClick` |
| Administrator click precedence, invalid target without pickup/movement, ordinary click | `TestMapClickAdminPrecedesArmedAction`, `TestMapClickActionRoutingPrecedesPickupAndMovement`, `TestMapClickOrdinaryMovementAndPickup`, `npm run test:map-click` |
| Actions panel order, selected/unavailable state, inside/outside click, mouse/touch drag, mixed/stale hotbar entries | `npm run test:actions` |
| Escape action priority, unknown cursor fallback, Pixi hover cursor styles | `npm run test:actions` |

`go test ./...`, `go build ./...`, `npm run build`, `npm run type-check`, focused ESLint, `npm run test:actions`, `npm run test:map-click`, and `openspec validate game-actions-system --strict` pass. The repository-wide ESLint command reports pre-existing errors outside this change, including generated dependencies and protocol bindings. A live browser/server session with the rebuilt server has not yet been run; the existing development server and Vite process are already using the default ports.
