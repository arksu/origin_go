# ADR 0002: Unified Object Behavior Runtime

## Status
Accepted

## Context

Object behavior logic was split across multiple places:
- runtime recompute (`ObjectBehaviorSystem`),
- context actions (`ContextActionService`),
- cyclic action callbacks (tree chop flow),
- key validation in `objectdefs`.

This caused behavior logic for one object type to be spread across files/packages and made extensions error-prone.

## Decision

1. Introduce a single behavior contract in `internal/types/behavior.go`.
2. Move behavior implementations to `internal/game/behaviors`.
3. Use one unified runtime registry (`internal/game/behaviors/registry.go`) for:
   - behavior execution routing,
   - behavior key validation in object definition loading.
4. Support optional capabilities (runtime/actions/cyclic/lifecycle), not a monolithic interface.
5. Enforce fail-fast startup validation in registry:
   - action-capable behavior must declare action specs,
   - provider/validator requires execute capability,
   - cyclic actions require cyclic handler.
6. Add lifecycle init hooks for object behavior state and call them for:
   - spawn,
   - restore,
   - transform.

## Consequences

### Positive
- Behavior logic for a given object type is colocated.
- A single registry is the source of truth for validation and execution.
- Contract violations fail early at startup.
- Lifecycle initialization is explicit and consistent.

### Trade-offs
- `types` contract uses generic `any` world/action payload fields to avoid cyclic imports.
- More explicit conversion is required at behavior call sites.
