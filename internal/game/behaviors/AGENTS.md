# Behaviors Package

## Purpose

`internal/game/behaviors` is the single runtime location for object behavior logic.

This package owns:
- behavior implementations (`container`, `tree`, `player`),
- unified behavior registry,
- fail-fast contract validation for behavior capabilities,
- object-def behavior config validation/application (`ValidateAndApplyDefConfig`).

## Contracts

- Shared behavior interfaces and typed context/result types are defined in `/Users/park/projects/origin_go/internal/game/behaviors/contracts/behavior.go`.
- Contexts are fail-fast and strongly typed (`*ecs.World`, typed deps, typed cyclic action payload); avoid `any` in behavior call boundaries.
- Runtime systems must use the unified registry (`DefaultRegistry()` / `MustDefaultRegistry()`), not local behavior maps.
- Behavior keys in object defs are validated via this same runtime registry (single source of truth).

## Capability Model

Behaviors can implement only the capabilities they need:
- def config validation (`ValidateAndApplyDefConfig`) for objectdefs loader,
- runtime (`ApplyRuntime`),
- context actions (`ProvideActions`, `ValidateAction`, `ExecuteAction`),
- cyclic (`OnCycleComplete`),
- scheduled tick (`OnScheduledTick`),
- lifecycle init (`InitObject`).

## Fail Fast Rules

Registry creation validates behavior contracts and fails startup when:
- action provider/validator exists without execute capability,
- action capability exists without declared action specs,
- declared cyclic action (`StartsCyclic=true`) has no cyclic handler,
- scheduled tick capability exists without runtime capability,
- behavior keys/action declarations are invalid.

## Lifecycle Init

Lifecycle init hooks are expected to support:
- `spawn`,
- `restore`,
- `transform`.

Implementations must be idempotent and avoid clobbering valid restored runtime state.

## Tree Behavior Runtime Contract

- Tree growth is scheduler-driven: behavior stores `stage` and `next_growth_tick`, scheduler drives updates.
- Runtime flags must expose current growth stage as `tree.stageN`.
- Chop availability is stage-gated via def config (`allowedChopStages`).
- On terminal stage, tree behavior must cancel its scheduled tick.

## Implementation Notes

- Keep behavior-specific logic colocated inside this package; do not split one behavior across multiple packages.
- Keep helpers private unless they are needed by multiple behavior files.
- Prefer simple, explicit flows over generic abstraction layers.
- Shared build-state snapshot helpers may live here when that avoids `internal/game` ↔ `behaviors` import cycles.
