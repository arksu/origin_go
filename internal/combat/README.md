# Shared combat primitives

This package has no player, socket, inventory, ECS, or object-category dependency.
The game adapter supplies current collider geometry, receiver eligibility, weapon
parameters, and effective STR at impact.

Contact clips the authoritative rectangle against the convex cone, then compares
the closest clipped point with range. This handles containment, boundary tangency,
line/point degeneracy, and collider centers outside the sector. The linear
tolerance is 1e-7 world units. Ranking uses actual distance followed by ascending
stable entity ID; it never uses an epsilon comparator.

Selection excludes self before eligibility or geometry. The receiver adapter
must reject removed/replaced/depleted entities. Both attacks share the same
contact set; AoE returns every distinct receiver in stable ID order, while single
selection chooses the nearest intersection. There are no friendship or creature
category filters. Movement blockers are not automatically damage receivers and
never occlude an axe strike.

`game.CombatReceivers` registers adapters by generational handle. Register or
call UpdateExtent before increasing a hittable collider; the conservative maximum
extent expands spatial queries across loaded chunk grids. Removal must unregister
the adapter. Liveness/incarnation is checked again immediately before applying
damage. An adapter validates before mutation, and a failed ApplyHit must have no
partial effects. Authoritative event sequences are monotonic across that world's
dispatch stream; the last applied sequence prevents replay.

Raw damage uses committed weapon B/Quality, impact-time float64 STR, and the action
multiplier. Armor reduction uses Draw²/(Draw+A), evaluated in an overflow-resistant
equivalent form, with zero raw damage returning zero. No integer conversion, hard
cap, per-target division, or minimum damage is applied.

## Runtime adapter

`game.CombatService` runs on the shard thread after collision/transform updates.
Actor-owned CombatState keeps the current execution, per-action deadlines and
activity timestamp. Begin validates before exact stamina payment, emits a start
identity, and records windup/recovery deadlines using TimeState.Now. The update
marks each due strike resolved before callbacks, then applies hits in stable
entity-ID order. Due executions sort by deadline, execution sequence, then actor
ID. A step crossing both deadlines resolves the strike before finishing recovery.
All event times use the current TimeState.UnixMs and an increasing world sequence.

Busy recovery can finish at command processing only after the strike is resolved.
Voluntary cancellation cannot end commitment. Interrupt clears execution and
recovery while retaining payment, cooldowns, and damage already applied. Existing
stun/KO/death/teardown conditions use this path; ordinary damage does not.
LastCombatEventAt takes the maximum accepted start/strike/hit time only.

Player input validates connection/epoch/revision before the action adapter. The
adapter resolves equipped weapon identity; a non-player actor can call Begin
with supplied weapon parameters and a strength provider without inventory/socket
requirements. This runtime does not persist timers or protect disconnected actors.
