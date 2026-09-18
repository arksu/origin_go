# Tasks

## 1. Declarative station and craft definitions

- [x] 1.1 Add a typed station definition to object data with capabilities, initial state, local values/resources, and autonomous-consumption configuration; add loader validation for empty keys, invalid states, non-positive quantities, duplicate entries, and malformed condition values; verify with focused `internal/objectdefs` loader tests.
- [x] 1.2 Add typed craft station requirements and explicit per-cycle consumptions to `internal/craftdefs`, preserving `requiredLinkedObjectKey` compatibility; validate capability/state/condition/consumption shapes and referenced station resource keys; verify with focused `internal/craftdefs` tests and the full `data/crafts` integration loader test.
- [x] 1.3 Document the JSON contracts and v1 scope in `data/objects/README.md`, `data/crafts/README.md`, and the relevant data examples; verify every documented field is accepted by the loaders and no operator/terrain/nearby-object requirement is enabled in production data.

## 2. Station runtime state

- [x] 2.1 Add an ECS station runtime component containing current state, capabilities, scalar values, and local resource quantities, plus typed mutation helpers that reject negative results; verify component tests cover initialization, read-only snapshots, and underflow rejection.
- [x] 2.2 Initialize station runtime state from object definitions or restored persistent state in the world-object factory without attaching station state to non-station objects; verify an object-factory test covers both fresh initialization and restored state.
- [x] 2.3 Extend the object-state codec and chunk activation path to serialize and restore station runtime state, while accepting existing state envelopes without a station payload; verify persistence tests cover chunk reload, restart-style deserialize/rehydrate, and dirty marking after every station mutation.
- [x] 2.4 Implement the autonomous station update system for station-owned rules such as fuel decay and state transitions, using the existing ECS time/tick resources; verify a burning campfire loses fuel and becomes non-burning at exhaustion while no craft operation exists.
- [x] 2.5 Register the station update system at a deterministic point in the shard update order and verify with a system-order/integration test that craft completion observes the current station state from the same world tick.

## 3. Extensible requirement evaluator

- [x] 3.1 Create the requirement context, condition/result types, normalized consumption plan, provider interface, and evaluator registry in a focused station-requirements package; verify evaluator calls do not mutate ECS, inventory, or station state.
- [x] 3.2 Implement the v1 station provider for capability membership, exact state, scalar comparisons, and station-local resource availability; verify passing, failing, malformed, and unsupported-condition cases with table-driven unit tests.
- [x] 3.3 Add provider-source dispatch, stable failure codes, and evaluation metadata for visited entities and bounded depth; verify unsupported providers fail closed and recursive dependent-station evaluations detect cycles without stack growth.
- [x] 3.4 Add the future provider seams for operator, terrain/tile, and nearby-object/dependent-station requirements without enabling those providers in v1; verify the evaluator can register a provider under a new source kind without changes to craft orchestration.

## 4. Craft lifecycle integration

- [x] 4.1 Extend craft-start validation to evaluate station requirements after resolving the linked object and before creating `ActiveCraft`/`ActiveCyclicAction`; verify a missing capability, wrong state, or missing local resource prevents action creation and causes no mutation.
- [x] 4.2 Re-evaluate station requirements at cycle completion using the current runtime state and cancel without mutation when the station changed during the cycle; verify a campfire extinguishing mid-cycle produces no output and does not consume inputs or station resources.
- [x] 4.3 Build a complete cycle-completion plan that includes station consumptions, craft inputs, stamina, and outputs, then apply it as one serialized logical commit; verify injected failures cannot leave partially consumed resources or partially created output.
- [x] 4.4 Preserve autonomous-resource separation so a recipe requiring `burning` does not deduct fuel unless it explicitly declares a craft consumption; verify cooking consumes no fuel while the station runtime continues its own fuel decay.
- [x] 4.5 Keep multi-cycle behavior correct: run the completion check and commit independently for every cycle, stop after a failed final check, and avoid consuming resources for cycles that never complete; verify `StartCraftMany` with a station extinguishing between cycles.

## 5. Craft snapshots and protocol

- [x] 5.1 Extend the craft recipe snapshot contract with station requirement descriptions and enough normalized availability/failure information to distinguish no linked station, unsuitable station state, and missing station resource; regenerate `internal/network/proto/packets.pb.go` from `api/proto/packets.proto` and verify protobuf generation plus focused serialization tests.
- [x] 5.2 Update `CraftingService` snapshot refreshes on link changes, autonomous station state changes, and craft completion so an open craft window reflects current station availability; verify snapshot tests cover link creation, fire extinguishing, and resource depletion.
- [x] 5.3 Keep client requests authoritative only by craft key: reject any client-provided station state or consumption values and verify the server ignores or rejects such unsupported fields in protocol tests.

## 6. Data fixtures and end-to-end verification

- [x] 6.1 Add a minimal fixture station and recipes covering a burning campfire requirement, a station-local thread consumption, and a recipe with no station requirement; verify all fixtures load and preserve the existing starter recipes.
- [x] 6.2 Add an end-to-end game test that links a player to the station, starts one cycle, changes autonomous station state before completion, and asserts cancellation; repeat with a passing final check and assert atomic resource/input/output results.
- [x] 6.3 Run focused package tests for `internal/objectdefs`, `internal/craftdefs`, evaluator/runtime packages, and `internal/game`; then run the repository's standard Go test command and record any unrelated pre-existing failures separately.
- [x] 6.4 Run strict OpenSpec validation for the completed change and review the resulting diff to confirm only the approved OpenSpec artifacts and implementation files are included.
