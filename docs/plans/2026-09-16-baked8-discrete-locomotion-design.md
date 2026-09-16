# Baked8 discrete locomotion design

## Goal

Make all locomotion pose transitions in `baked8` use discrete drawn frames.
The world position can still interpolate smoothly, but legs must not blend at
display-frame frequency when movement begins or ends.

## Behaviour

When movement begins, baked8 immediately enables the walk pose. While the
position interpolator reports any incomplete `stopProgress`, baked8 retains
that walk pose even if it still reports `walking` because visual position is
settling. At `stopProgress === 1`, or an ordinary stopped state, baked8 snaps
to idle. The existing continuous hybrid3d transitions remain unchanged.

## Validation

Regression tests cover immediate baked8 walk start and a stop progress received
while `walking` is still true. Both must preserve a discrete pose until the
final idle snap.
