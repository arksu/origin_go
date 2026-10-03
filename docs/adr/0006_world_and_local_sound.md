# ADR 0006: World propagation and client local sounds

- Status: Accepted
- Date: 2026-10-03
- Supersedes: [ADR 0002](0002_sound_events_via_visibility_state.md)

## Context

Chopping must emit from the server at the authored animation impact and reach
players outside source visibility. Frequent footsteps must stay on the client.
The server needs explicit work/transport limits and cannot add ECS systems.

## Decision

Canonical sound profiles separate `world` and `local` modes, reference-distance
`loudness` and sample `volume`. Hearing is an explicit shared value, initially 1;
strict eligibility is `d² < (loudness * hearing)²`.

The `tree_chop` animation binding owns phase `0.4` for both hand variants. Existing
validated cycle advancement executes its world marker once; gameplay effects
remain at completion. Successful fall feedback uses the point captured before
effects and survives tree despawn. There is no legacy completion chop path.

A shard-owned sparse listener grid receives updates from existing attach,
detach, relocation and final-transform hooks. World events are immutable points.
The service queries nearby listeners after `world.Update`, computes smoothstep
gain, and emits bounded per-listener batches with stream/time identity. Stable
priority and creation order select events, while independent global work limits
bound cells, candidates and admitted entries. Cutoff has no catch-up backlog.

A separate nonblocking audio queue preserves gameplay queue capacity; the socket
writer prioritizes gameplay. Clients validate each entry, epoch and freshness,
apply authoritative gain once, and revalidate delayed playback before it starts.

Footsteps use authored gait contacts and existing interpolated motion entirely
on the client. A logarithmic curve makes other footsteps quieter and fades them continuously
to zero at the radius; own sounds retain gain 1. Contact rebasing avoids catch-up on discontinuities.
Discovery uses only existing owner FX; no redundant server sound is sent.

Sound/animation/locomotion projections publish together through the existing
immutable catalog. Matching client/server/assets form one deploy/rollback unit.
No database migration, extra ECS system or hearing component is introduced.

## Consequences

World hearing is independent of visibility and source lifetime. Query cost scales
with nearby listeners up to explicit limits; remote listeners and unrelated
objects do not add candidate work. Dense crowds still require real fan-out,
which may drop low-priority audio under configured work or playback limits.
Queue acceptance does not guarantee audible delivery, and a stalled shared
socket can still affect latency. Occlusion and individual hearing modifiers
remain outside this iteration.

The [sound PRD](../prd/sound_event_spatial_audio_refactor.md) specifies fields,
limits, active flow and rollout. [Validation evidence](../../openspec/changes/add-world-and-local-sound/validation.md)
records integration checks and warmed benchmarks without promising production
throughput from a development-machine measurement.
