# Baked8 stop animation design

## Goal

Make a stopping character in `baked8` look like hand-drawn pixel art instead
of a high-frame-rate blend from a walk pose to idle.

## Behaviour

`hybrid3d` keeps its current continuous 300 ms stop blend. In `baked8`, the
last discrete walk pose remains unchanged while movement position settles. When
the shared movement `stopProgress` reaches `1`, the actor switches directly to
idle in a single rendered update.

## Data flow

`MoveController` remains the source of `stopProgress`; no network or movement
timing changes are needed. `ActorInstance.updatePose` branches only when the
selected actor render mode is `baked8`, preventing the continuous walk weight
from changing before the final stop state.

## Validation

Add a regression test proving that baked8 preserves its pose and revision for
intermediate stop progress, then switches to idle at completion. Existing
hybrid stop-blend tests continue to protect the continuous mode.
