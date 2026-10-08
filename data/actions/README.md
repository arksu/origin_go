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

`axe_sweep` and `axe_strike` use the shared `MeleeActionHandler` in production. All loaded actions with a `combat` block are prepared before shard startup and registered through this generic handler; item selectors, geometry, hit mode and multipliers come from definitions. Definitions without handlers still do not prevent startup; registered handlers must reference existing definitions and have unique IDs.

Both actions require an item tagged `axe` in either `right_hand` or `left_hand`, with no skill requirement. Direction targets require these fields:

- `sector.range`: positive distance from the attacker center in world units, representable as a protocol float.
- `sector.angleDeg`: full sector width in degrees, greater than 0 and at most 360. The loader converts it once; runtime definitions and the network catalog contain radians only.
- `combat.hitMode`: `all` selects all eligible targets in the sector; `nearest` selects the nearest eligible target. Selection is server-owned and is not sent in the catalog.
- `combat.damageMultiplier`: positive finite action multiplier; weapon base damage is not stored in the action.
- `execution.ticks`: positive execution duration in server ticks, using the standard action progress bar. There is no recovery phase; the old `execution.recoveryTicks` field is rejected.

Combat actions cannot declare a cursor, approach, or either repetition flag as true. Combat fields are not accepted on ordinary actions.

## Server equipment parameters

`requirements.equipment[].damageSource` is an optional server-only boolean, defaulting to `false`. The loader rejects multiple marked requirements and markers on noncombat actions. Preparing a melee action requires exactly one marked requirement with an `itemKey` or `itemTag` selector and distinct valid `slots`. Both existing axe actions mark their weapon requirement; the other requirements still control action availability. The marker is not included in protobuf or the client catalog.

The generic equipment resolver selects only equipped items with the `melee` capability that match the source selector and slots. It calculates `Draw` for each candidate using the supplied effective STR, instance Quality, definition base damage and action multiplier. The greatest Draw wins; equal Draw is resolved by the smaller ItemID. Weapon kinds such as axes, swords, knives and pikes use the same calculation; bows have no melee capability unless explicitly defined with one. Equipped items with `armor` contribute independently, including items that also have `melee`.

`internal/game/combat_equipment_catalog.go` prepares an immutable item catalog and melee selectors outside the tick path. Catalogs can be shared across shards; prepared actions belong to their originating catalog. `internal/game/combat_equipment_resolver.go` binds to one World, component storages and InventoryRefIndex and must be called under the owning shard's lock. Recreate it when replacing these resources or definitions. Each call reads current equipment through the index, checks at most ten entries and uses stack arrays without allocations, logging or world queries. Armor is summed in protocol slot order. Missing equipment contributes zero armor; unavailable weapons and corrupted equipment return distinct errors without partial results. Backpack and cursor-hand items do not contribute. Weapon choices, quality and container handles are never cached between calls.

Each shard binds its equipment resolver, creature/object damage receivers and melee execution service to its own World. Living creatures are prepared after successful setup or attachment, including transfer and rollback; live detached bodies retain their preparation. Definition-backed object targets are prepared through the existing component setup path. Damage uses the current weapon Quality, STR and target armor at completion. Current STR comes from the character's base attributes through a `float64` boundary; status-based attribute modifiers remain a later change.

The preset is a 90-degree sector with range 18, execution 6 ticks (0.6 seconds at the default 10 Hz), stamina cost 60 and independent 2000 ms cooldowns. The sweep multiplier is 1.0, the nearest-target strike multiplier is 1.5. Stamina and cooldown commit on successful execution at 100%, including a miss; unfinished actions cost nothing. Another ready action can start immediately after completion, following `docs/features/combat_final.md`.

The shared action service supports direction-target execution: activation includes an explicit finite `aim_angle` and the current `stream_epoch`, fixes the normalized angle at start, and uses the existing timed cycle and cancellation flow. KO, lying and stun prohibit directed actions. Accepted manual movement cancels an unfinished directed action.

Combat completion prepares the entire hit batch before charging costs or changing health. Spatial work is limited to 1024 cell visits and 4096 raw collider membership visits, including duplicate memberships and fallback traversal. Contact storage holds 4096 entries; at most 512 eligible targets may be hit. Exceeding a limit rejects the whole completion without truncated results, damage or costs. The attacker is excluded by exact handle and EntityID before geometry. Eligible contacts are ordered by distance to the part of the collider in the sector, then EntityID; creatures and ordinary objects use their respective receivers. Corpses are ordinary damageable objects; dropped items, dead bodies and pending destruction targets are excluded.

All calculations and lethal object queue/pin reservations succeed before stamina and cooldown commit. A failure releases reservations and applies no health changes. A successful miss still commits costs. After cost commit, the service writes the complete prepared batch, finishes the action, then performs destruction quarantine and sends one `S2C_AttackResult` to the attacker and its current observers. The result includes every target, even if an observer does not know it; it never predicts health or creates client objects. Event IDs are shared across shards, monotonically allocated and never reused; the client validates the packet's epoch and IDs and retains only a highwater ID per stream. Hit FX, status effects and ranged combat remain separate changes. Existing animation bindings are used when available; this change does not add axe animation assets.
