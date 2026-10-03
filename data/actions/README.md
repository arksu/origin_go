# Menu actions

Each JSON file contains a version-1 `actions` array. `cooldown` is an optional top-level whole number of milliseconds (0–4294967295). Omitted or zero means no cooldown; invalid values fail loading with the source filename.

```json
"cooldown": 2000
```

For existing noncombat actions, cooldown begins only after successful completion, at the same point where action costs such as stamina are charged. Canceling during approach or execution, rejection, and failure do not start cooldown. It is independent per character and action. Each successfully completed automatic cycle starts a full cooldown before the next cycle can begin; canceling the sequence during this wait preserves the completed cycle's cooldown.

`plow_tile` has a 20-tick execution, lasting two seconds at the default 10 Hz. Its configured cooldown starts after successful completion and runs for the full duration.

Cooldown timestamps are saved with character snapshots and continue expiring offline. Apply `migrations/20261003_action_cooldowns.sql` to existing databases before deploying the server; fresh databases use `migrations/schema.sql`. The migration defaults to the `origin` schema; adapt that qualifier for installations using a different configured schema. Normal save/logout/shutdown guarantees apply, without a synchronous database write for every action.

The client receives cooldown duration in the catalog and current deadlines in action state snapshots. Every UI icon uses `ActionIcon` and `useActionPresentation`; no action-specific client timer is needed.

## Axe combat definitions

`axe_sweep` and `axe_strike` are loaded into the shared registry and sent to the client catalog. Their handlers and combat mechanics are not implemented yet. Definitions without handlers do not prevent startup; registered handlers must still reference existing definitions and have unique IDs.

Both actions require an item tagged `axe` in either `right_hand` or `left_hand`, with no skill requirement. Direction targets require these fields:

- `sector.range`: positive distance from the attacker center in world units, representable as a protocol float.
- `sector.angleDeg`: full sector width in degrees, greater than 0 and at most 360. The loader converts it once; runtime definitions and the network catalog contain radians only.
- `combat.hitMode`: `all` selects all eligible targets in the sector; `nearest` selects the nearest eligible target. Selection is server-owned and is not sent in the catalog.
- `combat.damageMultiplier`: positive finite action multiplier; weapon base damage is not stored in the action.
- `execution.ticks`: positive windup duration in server ticks, also the future visible strike-animation duration.
- `execution.recoveryTicks`: explicitly supplied non-negative duration in server ticks; recovery is not sent in the catalog.

Combat actions cannot declare a cursor, approach, or either repetition flag as true. Combat fields are not accepted on ordinary actions.

The preset is a 90-degree sector with range 18, windup 6 ticks, recovery 4 ticks, stamina cost 60 and independent 2000 ms cooldowns. At the default 10 Hz, the durations are 0.6 and 0.4 seconds; changing the tick rate changes these durations. The sweep multiplier is 1.0, the nearest-target strike multiplier is 1.5. The intended combat rule charges stamina and starts cooldown at accepted windup start, as specified in `docs/features/combat_protocol.md`; this iteration only loads and publishes definitions, without implementing that execution rule.
