# Nearby no-collider Lift

## Goal

Allow a player to lift a no-collider object, such as a log, when they already stand within the movement system's arrival distance.

## Cause

No-collider Lift normally installs a temporary phantom collider and waits for the collision system to report the phantom crossing. When the player is already within `StopDistance`, the movement system clears its target without moving, so no phantom collision is reported and the pending Lift times out.

## Design

Use the same `StopDistance` boundary before starting the normal pending no-collider Lift flow. If the player is already within it, start the existing carry transition immediately. Otherwise retain the existing temporary phantom and movement flow unchanged.

## Verification

Add a focused Lift service regression test for the nearby case and run the package tests.
