# No-collider Lift arrival fallback

## Goal

Complete an in-progress Lift of a no-collider object after the player reaches the object's arrival distance, even if the collision system does not emit a phantom collision.

## Cause

The pending Lift system currently finalizes only when `CollisionResult.IsPhantom` is true. The movement system may instead clear its target when the final movement step reaches `StopDistance` or snaps to the target, leaving the Lift pending until timeout.

## Design

For `PickupNoCollider` transitions only, `LiftPlacementSystem` will also finalize when the player's transform is within `StopDistance` of the saved target point. Existing phantom collision completion remains intact, and put-down transitions are unchanged.

## Verification

Add system tests for nearby-arrival finalization and for preserving the phantom path, then run focused ECS and game package tests.
