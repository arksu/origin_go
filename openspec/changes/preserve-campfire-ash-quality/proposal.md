# Proposal

## Why

An exhausted campfire currently creates `ash` with quality zero, losing the quality of the branches used to build it. The resulting item should retain the weakest contributing branch quality.

## What Changes

- Derive a constructed campfire's quality from the minimum quality of all consumed `branch` inputs.
- Preserve that quality through the existing burner exhaustion replacement so the dropped `ash` receives it.
- Define a safe quality-zero fallback for legacy or otherwise untraceable campfires.

## Capabilities

### New Capabilities

- `campfire-ash-quality`: Defines quality provenance from campfire construction through its ash outcome.

### Modified Capabilities

- None.

## Impact

- Build completion quality calculation and persisted object quality.
- Campfire burner exhaustion outcome parameters and dropped-item persistence.
- Build, burn, and restore regression tests.
