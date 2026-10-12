# Proposal

## Why

Origin needs a durable personal home and an interruptible way to return to it, integrated with permanent death and existing building, action, transfer, and persistence mechanisms. The approved decisions in `docs/features/hearth_fire.md`, Part 2, now define the feature sufficiently to plan its implementation without importing unrelated Haven & Hearth systems.

## What Changes

- Create one eternal, fuel-free, green `Hearth Fire` exactly at a character's first world-entry spawn; validate that the location fits both the player and the full hearth footprint.
- Add immediate-access manual construction from five branches, at two seconds and eight stamina per branch. The actual finisher becomes the owner; unlimited unfinished sites remain allowed.
- Enforce one active hearth per character in the database. Completing another hearth converts the old one in place into an ordinary unlit campfire; permanent death does the same and removes the active association. Terminal character deletion must also clear that ownership so a deleted owner cannot leave an active hearth behind.
- Add a surface-only 1x1-tile hearth, passable to its owner and animals but solid to other players, immune to damage and removal, and not liftable. Disallow intersecting construction and put-down even by the owner; reject water, shallow water, swamp, and bog placement.
- Add `Travel to Hearth Fire` to the existing Actions menu: five-second interruptible progress, zero stamina, and an existing persisted six-minute cooldown only after successful transfer. Allow underground and cross-layer origins, carried objects, and lying after knockout; successful post-knockout travel stands the character.
- Cancel travel on movement commands, any other gameplay-action attempt even if rejected, any damage, combat, swimming, or disconnect. Keep chat and personal-window opening non-interrupting. Resolve the current active hearth immediately before travel and reject a blocked exact destination without nearby fallback.
- Add the same action beside `Stand Up` after knockout ends. For ordinary login only, try saved position, current hearth, a location near the saved position, then existing random-spawn fallback. Preserve live-body reattachment.
- Add reusable world-position `skull_smoke`, visible for two seconds to event-time observers of a permanently dying character or a hearth becoming a campfire. Do not replay it on view rebuild or later entry, and do not add minimap markers.
- **BREAKING:** deployment requires the new database schema and the project owner's planned clean database. There is no legacy-character backfill or compatibility path; this change does not perform the database cleanup.

## Capabilities

### New Capabilities

- `hearth-fire`: Personal hearth ownership, first-entry initialization, manual replacement, placement and collision rules, ordinary return, login fallback, post-knockout access, and permanent-death lifecycle.

### Modified Capabilities

- `game-actions`: Narrow Hearth-return interruption rules at input admission, including rejected or cooling-down other-action attempts and movement commands that produce no movement; preserve unrelated action rules and success-only shared cooldowns.
- `client-particle-fx`: Reusable finite world-event effects and observer delivery, including two-second skull smoke and a narrow distinction between unlit campfires' lack of continuous smoke and conversion-event smoke.

## Impact

- Persistence: deployment migration, `migrations/schema.sql`, sqlc queries, `character.first_world_entry_at`, unpartitioned `character_hearth`, and an optional `hearth_finisher_id` in existing build JSON. Reuse worker queues, chunk persistence pins/gates, and authoritative in-place lifecycle paths.
- Server: authentication/spawn, build completion, player transfer, health/death and terminal character-delete cleanup, action ingress and cooldown commits, collider filtering, put-down placement, object restore, and event-time visibility delivery. Existing character-deletion authorization and policy remain unchanged.
- Client: existing action/progress/cooldown presentation, knockout window, world-position FX dispatch and cleanup, hearth assets, and a shared skull-smoke preset. Existing `exp_gain` and continuous campfire smoke retain their behavior.
- Protocol: reuse action messages; extend the existing FX contract only as needed for world-entry freshness, preserving field numbers and regenerating Go/client outputs if changed.
- No new dependencies, inheritance, Hearth Secret/kin, claims, scent/Red-Handed, Thingwall, Travel Weariness, fuel lifecycle, online-status indication, removal action, or minimap work. This change contains planning artifacts only; implementation remains unstarted.
