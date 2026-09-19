# Design

## Context

See proposal.md for motivation. Build-site state already records every deposited stack as item key, quality, and count, but the current completed-object quality seam intentionally returns no computed value. Once completion removes build state, the final object has no remaining material provenance other than its own quality. Burner exhaustion already copies that object quality into its dropped item parameters.

## Goals / Non-Goals

**Goals:**

- Compute campfire quality before build state is removed.
- Weight the arithmetic mean by branch count and round down deterministically.
- Preserve the resulting quality through normal persistence, restoration, and ash replacement.

**Non-Goals:**

- Defining quality formulas for other buildable objects.
- Changing the current one-branch production recipe.
- Recovering construction material quality for legacy campfires.

## Decisions

### 1. Calculate only for completed campfires

Extend the existing completed-build quality seam to recognize the campfire result and aggregate only consumed `branch` stacks from its build state. The result is `sum(quality * count) / sum(count)` using a wide accumulator before conversion to the stored integer quality.

This keeps the policy at the transition that still has all material data and avoids retaining an otherwise redundant material ledger on the finished campfire. A generic quality formula for every buildable object is deferred because no other formula has been selected.

### 2. Reuse object quality as the provenance handoff

Pass the computed quality to the existing in-place build transform. Object persistence already stores entity quality, and the burner outcome already uses the source object's quality when preparing dropped ash.

This avoids adding burner-specific material state. Recomputing at exhaustion is impossible because completion removes the build behavior state.

### 3. Preserve zero for unverifiable history

If no consumed branch data is available, do not override the object quality. Legacy campfires therefore retain their existing zero value and produce zero-quality ash.

## Risks / Trade-offs

- [Future campfire recipes use branches across several stacks] → Aggregate every matching deposited stack and weight by count.
- [Large quality/count values overflow an intermediate sum] → Use a wider accumulator and clamp or reject impossible values before storing quality.
- [A non-branch material enters a future campfire recipe] → Exclude it from this campfire-specific formula unless a later design expands the rule.

## Migration Plan

1. Deploy calculation for newly completed campfires.
2. Preserve quality zero for existing persisted campfires with no material history.
3. Roll back by removing the campfire-specific completion override; existing object and ash qualities remain valid stored values.
