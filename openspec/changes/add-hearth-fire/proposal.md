# Proposal

## Why

Origin needs a personal home and an interruptible way to return to it, integrated with permanent death and existing building, action, transfer, and persistence mechanisms. The approved decisions in `docs/features/hearth_fire.md`, Part 2, define the feature without importing unrelated Haven & Hearth systems. Runtime state is authoritative: construction and ownership change immediately in the owning world, while asynchronous database saves may lag. The database is assumed available; a database error must stop the entire server, without SQL retries or error-recovery logic. Small crash rollback and partial checkpoints are acceptable; lossless durability is not a goal.

## What Changes

- New characters enter the world without a hearth. Character creation, login, reconnect, and missing ownership never create one automatically; a character obtains their first eternal, fuel-free, green `Hearth Fire` through ordinary construction.
- Add immediate-access manual construction from five branches, at two seconds and eight stamina per branch. The actual finisher becomes the owner of their first hearth or replaces their existing home; unlimited unfinished sites remain allowed.
- Enforce one active hearth per character in the database. Completing another hearth converts the old one in place into an ordinary unlit campfire; permanent death does the same and removes the active association. Terminal character deletion must also clear that ownership so a deleted owner cannot leave an active hearth behind.
- Finalize construction, select the actual finisher, and change runtime ownership in the same surface-shard execution, without waiting for SQL. Reuse bounded persistence workers to save the latest ownership projection afterward; intermediate changes may be coalesced rather than journaled.
- Add a 1x1-tile hearth with surface-only construction completion, passable only to its owner and solid to other players and animals, immune to damage and removal, and not liftable. Disallow intersecting construction and put-down even by the owner. Reuse standard build admission over the full result footprint with recipe `disallowedTiles: [1, 3, 50, 53, 56, 255]`; preserve ordinary build timing without an additional final terrain recheck or placement layer policy.
- Add `Travel to Hearth Fire` to the existing Actions menu: five-second interruptible progress, zero stamina, and an existing persisted six-minute cooldown only after successful transfer. Allow underground and cross-layer origins, carried objects, and lying after knockout; successful post-knockout travel stands the character.
- Cancel travel on movement commands, any other gameplay-action attempt even if rejected, any damage, combat, swimming, or disconnect. Keep chat and personal-window opening non-interrupting. Resolve the current active hearth immediately before travel and reject a blocked exact destination without nearby fallback.
- Add the same action beside `Stand Up` after knockout ends. For ordinary login only, try saved position, current hearth if one exists, a location near the saved position, then existing random-spawn fallback. Preserve live-body reattachment and ordinary spawning for characters without a hearth.
- Add reusable world-position `skull_smoke`, visible for two seconds to event-time observers of a permanently dying character or a hearth becoming a campfire. Do not replay it on view rebuild or later entry, and do not add minimap markers.
- **BREAKING:** deployment requires the new database schema and the project owner's planned clean database. There is no legacy-character backfill or compatibility path; this change does not perform the database cleanup.

## Capabilities

### New Capabilities

- `hearth-fire`: Construction-only personal hearth acquisition and replacement, ownership, placement and collision rules, ordinary return, login fallback, post-knockout access, and permanent-death lifecycle.

### Modified Capabilities

- `game-actions`: Narrow Hearth-return interruption rules at input admission, including rejected or cooling-down other-action attempts and movement commands that produce no movement; preserve unrelated action rules and success-only shared cooldowns.
- `client-particle-fx`: Reusable finite world-event effects and observer delivery, including two-second skull smoke and a narrow distinction between unlit campfires' lack of continuous smoke and conversion-event smoke.

## Impact

- Persistence: add the single undeployed `20261012_character_hearth.sql` migration, coherent `migrations/schema.sql` and sqlc queries for unpartitioned `character_hearth`. Tasks 1.1-1.4 deliver these contracts and the isolated runtime metadata index; gameplay and background persistence remain unwired. No first-entry column/query, initialization marker, finisher sidecar, pending table, operation journal, or compatibility migration is needed. Reuse bounded worker queues and chunk I/O gates; keep ordinary chunk, character, inventory, and corpse saves independent. A small transaction saves the current home projection, but gameplay publication does not wait for its commit.
- Server: login fallback to an existing home, synchronous build completion and replacement, player transfer, health/death and terminal character-delete cleanup, action ingress and cooldown commits, collider filtering, put-down placement, object restore, and event-time visibility delivery. Hearth owns its runtime ownership index, dirty revisions, and SQL policy. Narrow registered hooks and current lifecycle helpers keep Hearth fields and key branches out of shared core code; no new general mutation-lock system, lifecycle coordinator, or global shutdown barrier is introduced. Existing character-deletion authorization and policy remain unchanged.
- Client: existing action/progress/cooldown presentation, knockout window, world-position FX dispatch and cleanup, hearth assets, and a shared skull-smoke preset. Existing `exp_gain` and continuous campfire smoke retain their behavior.
- Protocol: reuse action messages and the existing FX key/position contract, adding only `stream_epoch = 3` for world-entry freshness. Finite effects start on current-stream receipt and end after two seconds; no durable event IDs or server-clock replay framework is needed. Preserve fields 1/2 and regenerate Go/client outputs.
- No new dependencies, inheritance, Hearth Secret/kin, claims, scent/Red-Handed, Thingwall, Travel Weariness, fuel lifecycle, online-status indication, removal action, or minimap work. The object definition and client art are integrated as a preparatory stage; Hearth gameplay implementation remains pending (see `tasks.md` and feature section 2.12).
