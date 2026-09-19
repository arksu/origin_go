# Proposal

## Why

Campfires currently discard the quality provenance of their construction materials: build completion leaves the campfire at quality zero, so its exhausted `ash` outcome is also quality zero. Future campfire recipes will use multiple branches and need a deterministic material-derived result.

## What Changes

- Compute a completed campfire's quality as the arithmetic mean of the quality of all consumed `branch` inputs, weighted by item count and rounded down to an integer.
- Preserve that computed quality on the completed campfire through save/restore.
- Continue passing the campfire's quality to its dropped `ash` exhaustion outcome.
- Keep a quality-zero fallback for legacy campfires whose construction inputs are unavailable.

## Capabilities

### New Capabilities

- `campfire-material-quality`: Defines branch-quality provenance from campfire construction through the dropped ash outcome.

### Modified Capabilities

- None.

## Impact

- Build completion quality calculation and its tests.
- Campfire object quality persistence and burner exhaustion output.
- Build, reload, and ash-drop regression tests.
