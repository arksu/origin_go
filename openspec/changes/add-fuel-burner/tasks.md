# Tasks

## 1. Definition contracts

- [ ] 1.1 Add validated positive numeric `abilities` to item definitions and verify loader tests reject empty keys and non-positive values while loading `branch` with `fuel: 1`.
- [ ] 1.2 Add validated `burner` behavior configuration to object definitions (`fuelAbilities`, capacity, duration, initial fuel, and exhaustion outcome) and verify invalid or incompatible configurations fail fast.
- [ ] 1.3 Configure campfire as a five-capacity, five-initial-unit burner accepting `fuel`, burning for 1,440 runtime seconds per unit, and producing `ash`; remove its legacy autonomous station fuel rule and verify all production definitions load.

## 2. Reusable burner runtime

- [ ] 2.1 Add persistent burner state for whole-unit reserve and next server-runtime burn boundary; verify fresh spawn, save/restore, and legacy campfire normalization preserve a valid schedule.
- [ ] 2.2 Implement the generic runtime burner advancement using `RuntimeSecondsTotal`, including multiple overdue boundaries, and verify one unit burns per configured duration without tick-rate dependence.
- [ ] 2.3 Reconcile burner state on chunk/object restore before interaction exposure and verify server downtime does not advance fuel while an unloaded object catches up accumulated server runtime.
- [ ] 2.4 Keep station-visible burning state coherent with burner fuel and verify a fuel-dependent craft cannot finalize after the linked burner exhausts.

## 3. Refuelling interaction

- [ ] 3.1 Register a reusable burner context action that accepts only a hand item with at least one configured fuel ability; verify incompatible coal-only items are not consumed by a `fuel`-only campfire.
- [ ] 3.2 Implement atomic accepted-item consumption and fuel addition, summing all matching ability values, and verify a multi-ability item contributes the expected sum.
- [ ] 3.3 Clamp added fuel to capacity while still consuming the accepted item and verify both a full campfire and an overflow case leave fuel at five.
- [ ] 3.4 Persist and notify refuelling state changes, and verify the action remains correct after link revalidation, inventory mutation, and chunk reload.

## 4. Exhaustion replacement

- [ ] 4.1 Implement the configurable burner exhaustion path that durably creates one dropped outcome item at the burner's location before removing the source object, and verify an exhausted campfire produces one dropped ash item.
- [ ] 4.2 Remove the exhausted object from ECS, chunk spatial state, persistence, and client visibility only after durable outcome creation; verify persistence failure leaves the burner retriable without losing ash or campfire.
- [ ] 4.3 Verify an already-expired burner restored from persisted chunk data is replaced before it becomes interactable, with no duplicate ash on repeated reconciliation.

## 5. Regression verification

- [ ] 5.1 Run focused Go tests for item/object definitions, burner behavior and runtime, station crafting, dropped-item persistence, and chunk restore; fix regressions found.
- [ ] 5.2 Run `go test ./...` and `go build ./...` and record any unrelated pre-existing failures separately from this change.
