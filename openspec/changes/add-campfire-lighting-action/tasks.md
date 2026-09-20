# Tasks

## 1. Define unlit campfire initialization

- [ ] 1.1 Change the campfire definition and completed-build transform to create `unlit` campfires with their configured initial fuel, and verify the object-definition, spawn, and completed-build tests show an unlit campfire with five stored fuel units.
- [ ] 1.2 Update burner initialization, runtime burning, and restore catch-up to represent an unarmed initial-fuel deadline safely, and verify unlit burners neither consume fuel nor create exhaustion outcomes across runtime advancement and reload.

## 2. Implement the ignition action

- [ ] 2.1 Extend the burner context-action behavior to expose only `Light my fire` for the eligible unlit campfire and reject stale, duplicate, or incompatible action requests; verify action-list and validation unit tests.
- [ ] 2.2 Start one target-linked cyclic ignition action through the existing active-action lifecycle, using the normal simple target-action duration, and verify linked progress start plus cancellation leaves campfire fuel, state, and stamina unchanged.
- [ ] 2.3 Complete ignition by revalidating state, consuming exactly 50 stamina even at the exact-cost boundary, arming the burner deadline, changing the station to `burning`, marking it durable, and publishing the station-state event with explicit failure logging; verify success and insufficient-stamina tests.

## 3. Publish generic burner appearance states

- [ ] 3.1 Implement a shared burner appearance transition that derives `{object}/unlit` and `{object}/burning` solely from the immutable object-definition key and the common target state, updates `Appearance.Resource`, and emits the existing appearance event only on a resource change; verify a synthetic non-campfire burner derives its resource without a concrete object key, resource path, or appearance field in burner configuration.
- [ ] 3.2 Verify the campfire client resource data registers `campfire/unlit` and `campfire/burning`, and verify completed build plus successful ignition send the corresponding existing appearance upserts to visible clients.

## 4. Preserve downstream behavior

- [ ] 4.1 Verify an ignited campfire follows the existing server-runtime fuel schedule and still produces the existing ash exhaustion outcome with `go test ./internal/game/behaviors ./internal/ecs/systems ./internal/game`.
- [ ] 4.2 Verify the unlit campfire fails burning-station craft requirements and successful ignition refreshes linked craft availability with focused crafting-station tests.
- [ ] 4.3 Run the relevant Go test suite and `openspec validate add-campfire-lighting-action --strict`; record any unrelated existing failures separately.
