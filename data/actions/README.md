# Menu actions

Each JSON file contains a version-1 `actions` array. `cooldown` is an optional top-level whole number of milliseconds (0–4294967295). Omitted or zero means no cooldown; invalid values fail loading with the source filename.

```json
"cooldown": 2000
```

Cooldown begins only after successful completion, at the same point where action costs such as stamina are charged. Canceling during approach or execution, rejection, and failure do not start cooldown. It is independent per character and action. Each successfully completed automatic cycle starts a full cooldown before the next cycle can begin; canceling the sequence during this wait preserves the completed cycle's cooldown.

`plow_tile` has a 20-tick execution, lasting two seconds at the default 10 Hz. Its configured cooldown starts after successful completion and runs for the full duration.

Cooldown timestamps are saved with character snapshots and continue expiring offline. Apply `migrations/20261003_action_cooldowns.sql` to existing databases before deploying the server; fresh databases use `migrations/schema.sql`. The migration defaults to the `origin` schema; adapt that qualifier for installations using a different configured schema. Normal save/logout/shutdown guarantees apply, without a synchronous database write for every action.

The client receives cooldown duration in the catalog and current deadlines in action state snapshots. Every UI icon uses `ActionIcon` and `useActionPresentation`; no action-specific client timer is needed.
