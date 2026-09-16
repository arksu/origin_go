# Admin `/pos` Command Design

## Goal

Allow an administrator to enter `/pos` in local chat and receive a private
server message with the administrator's current coordinates, formatted as
`pos: 345, 679`.

## Design

Extend `ChatAdminCommandHandler` with a `/pos` branch. The handler will read
the caller's `components.Transform` from the ECS world, convert its `X` and
`Y` values to integer coordinates, and return the result through the existing
`sendSystemMessage` path. This preserves the current command interception
behavior, so the command is never delivered as a normal chat message.

If the player has no transform, the command will still be recognized and the
handler will return a private server message explaining that the position is
unavailable.

## Verification

Add focused handler tests that verify `/pos` is recognized, returns the exact
required format, and replies privately to the requesting entity. Cover the
missing-transform response as well.
