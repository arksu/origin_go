# Design

## Context

See `proposal.md` for the motivation and scope. The current server already has a linked-player/object model in ECS, declarative craft definitions, and a `CraftingService` that starts a cyclic action and resolves inventory inputs and outputs at cycle completion. The current station constraint is only an object-key check; there is no autonomous station runtime or generic requirement evaluator.

## Goals / Non-Goals

**Goals:**

- Add an authoritative runtime model for station capabilities, state, scalar values, and local resources.
- Keep autonomous station updates independent from `CraftingService`.
- Introduce a source-aware requirement evaluator with station-local providers in v1.
- Preserve existing recipes and `requiredLinkedObjectKey` behavior.
- Check requirements at cycle start and cycle completion.
- Commit station-resource consumption, craft inputs, stamina, and outputs atomically at successful completion.
- Leave an extension seam for operator, terrain/tile, nearby-object, and dependent-station requirements.

**Non-Goals:**

- Implement operator, terrain/tile, nearby-object, or dependent-station providers in v1.
- Implement a reservation system; no resource is held during the active cycle.
- Define a general-purpose production graph, power network, or fluid network.
- Replace the existing link system or invent client-side authority for station state.

## Decisions

### 1. Use a dedicated typed station definition and runtime component

Station configuration belongs in a typed station section of the object definition rather than an unvalidated entry in the generic behavior map. Object loading validates capabilities, states, resource keys, and numeric ranges. When a station object is spawned, its configured station data initializes a station runtime component on that entity.

The runtime component is authoritative for current state and local values. A station system updates it from elapsed game time and station rules. Crafting reads the component but does not update it except through an explicit station-resource commit declared by the recipe.

This keeps static configuration separate from mutable entity state and follows the existing object-definition plus ECS-component pattern. A generic behavior-map approach was rejected because it would defer schema errors and force unrelated systems to decode raw JSON.

### 2. Represent requirements as declarative predicates plus explicit consumptions

Craft definitions add a station-requirement list. Each requirement can identify a capability, an exact state, typed conditions, and explicit per-cycle consumptions. Autonomous consumption is configured on the station, not inferred from a craft requirement.

The evaluator returns a read-only evaluation result containing pass/fail information, a stable failure code, and a normalized consumption plan. Evaluation never mutates ECS state, inventory, or station resources. This permits the same contract to be used for start checks, completion checks, and craft-list availability.

### 3. Use provider registration for future requirement sources

The evaluator owns a registry of source providers. The v1 station provider handles capability, state, scalar, and local-resource predicates. Future providers can handle operator, terrain/tile, nearby-object, and dependent-station predicates through the same context.

The context includes the world, actor, linked station, and bounded evaluation metadata such as visited entities and depth. Nested evaluation is therefore possible without making the crafting service know how spatial or dependency checks work. The evaluator must reject unsupported providers and detect recursive dependency chains rather than silently passing them.

### 4. Make cycle completion a two-phase logical transaction

At cycle start, the crafting service evaluates station requirements and existing craft preconditions without mutation, then creates the active cyclic action. At cycle completion, it evaluates station requirements again, rechecks inventory and output capacity, computes the output, and commits all mutations as one logical operation.

The completion path must not call independent mutating operations in an order that can leave a partial result. It should first build a complete commit plan, then apply station-resource consumption, input consumption, stamina consumption, and output creation under the shard's serialized update. If any precondition fails, it returns a failure before applying mutations.

No reservation is kept during the cycle. This is intentionally simple for v1 and means a station can become unavailable while a cycle is running; the final evaluation then cancels the cycle without output or consumption.

### 5. Keep the existing linked-object constraint as a compatibility layer

`requiredLinkedObjectKey` remains valid for existing recipes and continues to select/validate the linked target. New station requirements apply after the linked object has been resolved. A future migration may express the object selector through a richer requirement source, but v1 does not remove or reinterpret the existing field.

### 6. Extend craft snapshots without making the client authoritative

The server remains authoritative for requirement evaluation and cycle completion. Craft-list snapshots include station requirement descriptions and availability flags/reason data so the client can present why a recipe is unavailable. A client request still contains only the craft key; it does not submit station state or consumption results.

## Risks / Trade-offs

- **[Risk]** A station can change state between the final evaluation and commit. **Mitigation:** perform final evaluation and the complete commit in the same serialized world update; use a runtime version/generation if the ECS mutation path later becomes multi-step.
- **[Risk]** Existing inventory helpers may mutate inputs and outputs separately. **Mitigation:** introduce a craft completion plan/commit boundary and test failure injection to prove no partial completion.
- **[Risk]** Generic conditions can become an untyped expression language. **Mitigation:** keep v1 condition kinds explicit and validated; add providers by source rather than accepting arbitrary scripts.
- **[Risk]** Nested nearby-station requirements can form cycles. **Mitigation:** carry visited entity IDs and a maximum evaluation depth from the first evaluator call.
- **[Risk]** Station state may need persistence across chunk unloads or server restart. **Mitigation:** keep runtime state attached to the world-object state boundary and make persistence integration an explicit follow-up if current object persistence cannot yet serialize station data.
- **[Risk]** Exposing every internal station value to the client can create a large protocol surface. **Mitigation:** send normalized requirement descriptions and stable availability/failure codes, not the entire runtime component.

## Migration Plan

1. Add the new station and requirement definitions with empty/default values so existing objects and recipes remain valid.
2. Keep current recipes working through `requiredLinkedObjectKey`; migrate individual station recipes only after the runtime and evaluator tests pass.
3. Roll out station runtime updates and craft completion commit behind the existing server feature path; recipes without station requirements continue on the existing behavior.
4. If rollout must be reverted, disable station-requiring recipes and retain the compatibility path for recipes that use only the existing linked-object field.

## Open Questions

None that change the approved v1 architecture. Operator, terrain, nearby-object, and dependent-station semantics are intentionally deferred but have an explicit provider seam.
