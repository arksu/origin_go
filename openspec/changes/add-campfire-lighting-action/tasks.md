# Tasks

## 1. Define unlit campfire initialization

- [ ] 1.1 Change the campfire definition to spawn in `unlit` while retaining its configured initial fuel, and verify the object-definition and spawn tests show an unlit campfire with five stored fuel units.
- [ ] 1.2 Update burner initialization, runtime burning, and restore catch-up to represent an unarmed initial-fuel deadline safely, and verify unlit burners neither consume fuel nor create exhaustion outcomes across runtime advancement and reload.

## 2. Implement the ignition action

- [ ] 2.1 Extend the burner context-action behavior to expose only `Light my fire` for the eligible unlit campfire and reject stale, duplicate, or incompatible action requests; verify action-list and validation unit tests.
- [ ] 2.2 Start one target-linked cyclic ignition action through the existing active-action lifecycle, using the normal simple target-action duration, and verify progress start plus cancellation leaves campfire fuel, state, and stamina unchanged.
- [ ] 2.3 Complete ignition by revalidating state, consuming exactly 50 stamina, arming the burner deadline, changing the station to `burning`, marking it durable, and publishing the station-state event; verify success and insufficient-stamina tests.

## 3. Preserve downstream behavior

- [ ] 3.1 Verify an ignited campfire follows the existing server-runtime fuel schedule and still produces the existing ash exhaustion outcome with `go test ./internal/game/behaviors ./internal/ecs/systems ./internal/game`.
- [ ] 3.2 Verify the unlit campfire fails burning-station craft requirements and successful ignition refreshes linked craft availability with focused crafting-station tests.
- [ ] 3.3 Run the relevant Go test suite and `openspec validate add-campfire-lighting-action --strict`; record any unrelated existing failures separately.
