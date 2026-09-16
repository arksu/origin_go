# No-collider Lift Down target placement

## Goal

Place every lifted object at the position selected by the player, including no-collider logs.

## Cause

Lift Down saves the clicked target position and uses a phantom collider to stop the player. During finalization, no-collider objects overwrite that saved target with the player's stopping position, so logs appear under the player rather than at the clicked point.

## Design

Use `PendingLiftTransition.TargetX` and `TargetY` as the placement position for every Lift Down transition. The phantom remains responsible only for movement completion and does not change the object's destination.

## Verification

Add a focused regression test asserting that both collider and no-collider pending transitions resolve to their requested target coordinates, then run the Lift package tests.
