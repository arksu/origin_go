# Phase 1 combat protocol

The paired server advertises `S2C_PlayerEnterWorld.combat_supported` only when its
test context is enabled. Without support, send ordinary legacy inputs only.
Existing field numbers, tick animation fields, and retired reservations remain intact.

1. Send `C2S_ActivateAction` with action_id, stream_epoch, and a nonzero increasing
   uint64 request_revision for combat.
2. Await `S2C_ActionStateChanged` selecting, which supplies selection_generation,
   generation, and combat_range. Arming is free.
3. Preview aim locally. Send one primary MapClick at the aimed world point with
   combat_attempt containing action_id, selection_generation, request_revision,
   and stream_epoch. Activation and clicks share the same revision counter.
4. The accepted cycle is windup, strike, recovery, idle. Do not send an aim stream,
   choose a victim, or drive damage from local animation.

The shard validates connection ownership, layer, epoch and detached state before
consuming the revision. All well-formed fresh attempts consume it, including
gameplay rejections. A consumed or stale tagged click never becomes ordinary
movement. Replay state is bounded to the current connection/epoch high-water mark
on the actor incarnation. Queue CommandID is independent. A new epoch starts a
new request stream; old epochs cannot reset its counter.

Public `CombatExecutionState` carries generation/revision, execution_id, phase,
locked_direction, elapsed_ms/duration_ms, strike_at_ms/recovery_end_ms,
server_time_ms, range and sector_angle_degrees. Empty action/phase idle clears
presentation. `S2C_CombatOwnerState` separately carries private per-action
ready_at_ms values; these are informational and must not disable action icons.

`S2C_CombatResult` identifies the strike event and its execution. Each hit has a
distinct event_sequence, raw_damage/damage and receiver snapshot. Recipients see
only target details permitted by their visibility. HP is double throughout.
`CombatTargetState` has generation/revision, hp/max_hp and depleted. Object spawn
can include current execution/target snapshots without replaying historical hits.

Combat animation adds elapsed_ms/duration_ms, locked_direction and execution_id
to the existing animation snapshot. Legacy sources retain tick timing. All uint64
values must remain lossless (protobuf Long/string), never arithmetic on JS Number.
Gate updates by epoch/incarnation, compare revisions, and deduplicate event IDs.
This request protection does not persist combat timers across disconnect/transfer
or restart; that remains phase 7.
## Compatibility and commitment

Legacy dig/plow/craft actions retain their tick-based payment, repetition and
cancellation policies. A combat selection is free and cancelable. After a valid
direction commit, Escape, other action selections, secondary clicks, crafting,
building, context actions and lift shortcuts cannot replace the execution or
queue work. Explicit external interruption retains payment and cooldown.

During windup and recovery, primary clicks preserve administrator targeting
precedence, then route only to coordinate movement. Tagged combat clicks are
always consumed, including stale or rejected requests. Neither path can become
a pickup, object link or held-item drop. Directional movement retains its own
revision and expiry protocol.

Windup uses an effective Crawl movement mode through the movement step before
impact; it does not overwrite the selected mode. Ordinary stamina, carry and
collision restrictions still apply. Recovery restores ordinary effective
movement. Movement at zero stamina stops without canceling the paid attack.

Both equipment hand slots are locked through recovery. Inventory transactions
validate source, destination and swap participants before mutation, including
same-container moves and drops. Grid-only rearrangement remains available.
