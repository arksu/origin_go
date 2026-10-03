# Audio implementation validation

Date: 2026-10-03. Server: Go 1.27.1, darwin/arm64, Apple M1 Pro,
benchmark GOMAXPROCS 10. Client/tooling: Node 26.10.0; actual in-app Chromium 154.
These are development-machine results, not a production throughput guarantee.

## Footstep near-zone correction

The user requested other-player footsteps at 90% gain through 16 absolute
world-coordinate units, followed by attenuation. The current canonical footstep
profile authors `near_distance: 16`, `near_gain: 0.9`, `far_gain: 0`, `shape: 4`.
The near zone stays fixed when hearing changes; the outer radius remains
`loudness * hearing` (120 at hearing 1). Own-source gain stays 1.

Other-source gain is `.9` for `d <= 16`, then follows the logarithmic curve
with `t = (d - 16) / (R - 16)`. At distance 68 it is approximately `.285654425`;
it approaches zero continuously at 120. No local playback starts at or beyond
that radius. The optional `near_distance` defaults to zero for existing local
profiles. Both parsers reject invalid near distances, and server hearing
validation rejects an effective radius that leaves no attenuation interval.

Checks passed: 21 client sound tests, 23 client catalog tests, 123 asset-pipeline
tests, client and pipeline type checks, and Go sound-definition/configuration/
action-animation-definition tests. Regression coverage includes the inclusive
16-unit boundary, continuity at both boundaries, absolute near distance under
hearing scaling, and successive other-character contacts across the plateau.
The current immutable sound projection is
`d2f632296185f1460ff3e2e3e33b934e1493ed4dde06504622939c48c0e2432f`.

## Historical first attenuation correction after playtesting

The user reported nearly equal own/other footsteps followed by sudden silence.
The original coefficients below (`0.90` near / `0.80` far) document the initial
run, not the current tuning. They left other footsteps at 80% gain immediately
before range exclusion. Runtime review found the correct raw world coordinates,
owner identity and independent per-ID gains; the problem was the authored curve.

The first correction used near `0.50`, far `0`, shape `4`, retaining
radius `120` at hearing `1`, sample volume `.35` and own gain `1`. Expected gain:

| Other-source distance | Gain relative to own footsteps |
| --- | ---: |
| 0 | .50 |
| 30 | .284661721 |
| 60 | .158696903 |
| 90 | .069323442 |
| 119 | .002078051 |
| 120 | 0 |

That correction needed no new runtime branch: authoring far gain zero made the
curve continuous at the exclusion radius. Updated sound
tests load the canonical profile, check monotonic attenuation and a near-zero
limit just inside the radius, and check successive other-character contacts at
increasing world distances. All 21 sound tests pass; catalog checks (23) and Go
sound definition checks pass. The new immutable sound projection is
`0dc0cd904be0b1ea2f3632756156b01e6cd3590af876448cbaed993fff54b6e0`.
Targeted actual-browser playback passed for own footsteps and other characters
at distances 0/30/60/90/119/120. At master `.5`, measured native volumes were
`.175` for own footsteps and `.0875 / .0498158012 / .0277719580 / .0121316023 /
.0003636589 / 0` for others. The exact radius produced no native playback ID.
Re-entering range replayed no old contacts; the next genuine contact played once
with the correct near-zero volume. No browser audio/runtime errors were reported.
Exact output: [footstep-attenuation-validation.json](footstep-attenuation-validation.json).
Use the harness's attenuation-only checkbox to reproduce this focused check.
Reload the game page to replace the already-loaded audio catalog in an open session.

## Automated checks

The combined command passed:

```sh
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-go-audio-cache go test ./internal/game/... ./internal/ecs/... ./internal/network/... ./internal/config ./internal/sounddefs ./internal/actionanimationdefs ./internal/cyclicaction
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-go-audio-cache go build -o /private/tmp/origin-audio-gameserver ./cmd/gameserver
```

Local WebSocket/HTTP integration tests require permission to bind loopback ports.
CGO is disabled because the installed macOS SDK/linker rejects the SDK's
`arm64e.x1` architecture when linking the game test binary. The pure-Go build and
all affected package tests pass; this is not evidence that the host's CGO linker
has been repaired. Network/protocol race tests also passed separately.

Each client command was run separately and passed:

- `npm --prefix web_new run test:sounds`: 20 tests, including independent gains,
  authoritative zero, malformed siblings, loading/epoch expiry, Howler native
  play-lock regression, local contacts/rebasing and FX-only discovery.
- `npm --prefix web_new run test:movement-input`: 20 tests.
- `npm --prefix web_new run test:actions`: passed.
- `npm --prefix web_new run test:action-animations`: 9 tests.
- `npm --prefix web_new run test:asset-pipeline`: 23 tests.
- `npm --prefix web_new run type-check`: passed.
- `npm --prefix tools/asset_pipeline test`: 117 tests.
- `npm --prefix tools/asset_pipeline run type-check`: passed.

Required tooling regressions passed: atlas roundtrip (1899 frames), Pixi
semantics and offline render comparison (zero differing pixels). Publication
is reproducible and validates sound/media/clip/menu-target references before
switching all three catalog references. A failed publication preserves the old
catalog. Go and client generated protocol bindings were regenerated; independent
scratch generation matched workspace output byte-for-byte. Final canonical
publication also left `asset-catalog.json` byte-identical.

Production Vite build also passed (`vite build --outDir /private/tmp/origin-audio-client-build`).
It reports nonfatal warnings about dependency `eval`, mixed static/dynamic imports
and large chunks. The build output directory is outside the project and was not
emptied by Vite. These warnings were not treated as failed audio validation.

## Server integration

`TestSoundLifecycle*` uses actual framed WebSocket connections, production
attach/reattach, transfer-source detach, immediate relocation, observer disconnect,
`Shard.Update`, cycle processing, batching and the isolated audio sender. The
suite passed three consecutive runs. Transfer coverage composes real detach,
spawn/attach and rollback hooks, including target capacity failure; it does not
run database-backed `executeTransfer` end to end.

The authored chop test loads `data/action_animations/tree.json`. With performer
and tree at `(200,100)` and listener at `(1000,100)`, the visibility maps are empty
and the listener is 800 units away. A 20-tick cycle sends one chop at tick 12,
then another at tick 32 after repeat. Effects remain at ticks 20 and 40. Terminal
success despawns the tree before a fall batch is sent from its captured point at
tick 40. Chop gain is approximately `.104`; fall gain is approximately `.393586`.
The listener does not need either source object on its client.

Other focused tests cover phase `.6` at tick 8/13 and phase `.14` at 7/50,
cancellation before/after contact, duplicate and stale starts, target replacement,
failed completion, layer/radius exclusion and detached/reused identity guards.
Source coordinates and queued packet bytes survive source despawn and later
scratch-buffer reuse. Movement tests check collision-adjusted positions before
visibility filtering, and immediate relocation affects the same tick's query.

The gameplay parity test runs two repeated cycles with audio disabled and with
continuous deliberate priority overload. Both produce exactly two effects,
stamina 66, identical action-state/finished packets and 40 progress packets.
Network tests exercise full audio queues, both queues ready, an audio wakeup
followed by gameplay, shutdown and sound bursts; critical gameplay retains its
own capacity and connection-close policy.

Exact server-produced packets can be exported for the browser harness:

```sh
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-go-audio-cache \
ORIGIN_AUDIO_FIXTURE_PATH=/private/tmp/origin-audio-server-fixture.json \
go test ./internal/game -run '^TestSoundLifecycleShardFlushesAuthoredChopBeforeCompletion$' -count=1
```

## Warmed propagation and encoding

```sh
CGO_ENABLED=0 GOCACHE=/private/tmp/origin-go-audio-cache \
go test ./internal/game -run '^$' -bench BenchmarkWorldSoundRouting \
  -benchmem -benchtime=500ms -count=3
```

Each workload warms four ticks before timing, uses canonical-shaped profiles,
commits listener positions and performs real protobuf encoding into owned bytes.
It includes metrics instrumentation, but excludes socket transport and client
playback. Remote listeners are near `(100000,100000)`, outside queried cells.
Unrelated objects have no listener membership. Moving listeners cross a cell
boundary every tick. No candidate list or full-world/player traversal is used.

Median of three runs; propagation and encoding are separate measured portions of
total tick cost. The total also includes event admission, listener movement where
applicable, and metrics. Full raw results are in [benchmarks.txt](benchmarks.txt).

| Workload | Total µs/tick | Propagation µs/tick | Encoding µs/tick | B/tick | Allocs/tick |
| --- | ---: | ---: | ---: | ---: | ---: |
| sparse | 5.64 | 1.09 | 2.63 | 1472 | 32 |
| remote1000 | 5.50 | 1.09 | 2.56 | 1472 | 32 |
| remote10000 | 5.49 | 1.11 | 2.54 | 1472 | 32 |
| objects10000 | 5.48 | 1.05 | 2.56 | 1472 | 32 |
| dense | 2210.00 | 797.12 | 1255.04 | 536000 | 4000 |
| overload | 2568.77 | 808.77 | 1515.24 | 694145 | 8000 |
| moving | 118.20 | 26.14 | 66.08 | 33784 | 520 |
| event_overload | 900.04 | 773.12 | 32.39 | 13504 | 32 |
| cell_overload | 357.78 | 295.06 | 0.00 | 0 | 0 |
| full_batches | 1574.27 | 1139.64 | 329.83 | 184000 | 4000 |

All work counts are deterministic across the three runs. Each message below is
one recipient batch, so messages equal recipients. Event drops include retained
or partially processed events discarded by work cutoff; they do not estimate
unvisited recipients. Per-recipient entry drops are separately counted.

| Workload | Offered events | Cells | Candidates | Entries | Messages | Encoded bytes | Event / entry drops |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| sparse | 1 | 64 | 8 | 8 | 8 | 240 | 0 / 0 |
| remote1000 | 1 | 64 | 8 | 8 | 8 | 240 | 0 / 0 |
| remote10000 | 1 | 64 | 8 | 8 | 8 | 240 | 0 / 0 |
| objects10000 | 1 | 64 | 8 | 8 | 8 | 240 | 0 / 0 |
| dense | 16 | 1024 | 16000 | 16000 | 1000 | 361000 | 0 / 0 |
| overload | 128 | 549 | 16384 | 16384 | 2000 | 378448 | 120 / 0 |
| moving | 4 | 256 | 512 | 512 | 128 | 12288 | 0 / 0 |
| event_overload | 2048 | 65536 | 8192 | 512 | 8 | 11336 | 1024 / 7680 |
| cell_overload | 1024 | 65536 | 0 | 0 | 0 | 0 | 569 / 0 |
| full_batches | 128 | 4197 | 65536 | 1000 | 1000 | 30000 | 63 / 64536 |

Workloads: sparse has 8 listeners/1 chop; remote1000 and remote10000 add only
remote listeners; objects10000 adds 10,000 unrelated entities. Dense has 1,000
listeners/16 chops; overload has 2,000/128; moving has 128/4. Event overload offers
2,048 events to the 1,024-event buffer. Cell overload uses 1,024 fall events with
no listeners. Full batches uses 1,000 listeners/128 events and a one-entry
per-recipient cap to force candidate cutoff even when no more entries fit.

Remote population and unrelated objects leave cells=64, candidates=8,
recipients=8 and allocations unchanged. Dense work is bounded by real fan-out;
overload admits exactly 16,384 entries, full-batch overload checks exactly 65,536
candidates, and cell overload visits exactly 65,536 cells. Separate cutoff
counters identify each exhausted budget without scanning skipped recipients.
The service clears every event after the tick, regardless of admission outcome.

No absolute production SLO is inferred. Retain the initial 256-unit cells,
1,024-event / 65,536-cell / 65,536-candidate / 16,384-entry limits; 64 entries and
16 KiB per recipient; 500 ms freshness and one pending audio batch. Observe work,
drop and timing metrics under actual population before changing these settings.

## Browser playback and local load

Reproduce the browser checks by running Vite and opening
`/tests/audio-browser.html`, then clicking Run. The harness uses actual Howler,
published catalogs and media, and generated protobuf decoding. No authenticated
live game/database is required. Recorded browser output has no warnings/errors.

All 12 canonical sample files decoded, including four original 0.16-second mono
WAV footsteps with nonzero sample energy. All four footsteps reached native
playback at the following gain/settings combinations (master `.5`, sfx `1`,
footstep profile volume `.35`, hearing `1`, radius `120`):

| Source | Distance | Distance gain | Actual per-ID volume |
| --- | ---: | ---: | ---: |
| Own | 0 | 1 | .175 |
| Other | 0 | .90 | .1575 |
| Other | 60 | .83173938 | .14555439159649525 |
| Other | 119 | .80041561 | .14007273178526153 |

The same exp sample played simultaneously through two IDs at volumes `.2` and
`.166`, without mutual volume changes. A decoded world chop at distance 800 with
no known source objects played at `.046800000965595244` (`.5 * .9 * .104` with
float32 gain). Fall reached native playback too. Old stream, stale event and a
server local-profile packet were each rejected. The browser exposed Howler's
HTML5 play-lock behavior: immediate per-ID volume can be delayed. Playback now
reapplies volume when native play starts, with expiry/reset/mute rechecks; a
focused regression test covers this case. Two additional tests verify a 250 ms
local native-start deadline and capacity release for expired pending voices, with
the late-start guard retained.

A local-only workload ran 200 known character snapshots × 100 frames = 20,000
contact updates in 17.9 ms in the final recorded run (8.5–17.9 ms across
manual browser runs). Twelve footstep voices were active
(the profile cap); 3,788 requests were dropped by the voice limit. There were
zero invalid/stale/unloaded/failed requests, zero WebSocket construction and zero
`WebSocket.send` calls. This is controller/playback admission CPU work, not a
full renderer or hardware audio-output performance benchmark. The corresponding
server test rejects 10,000 local-mode emissions with zero world events, cell
queries or packets.

Published `chop_l` and `chop_r` share the `.6` target cue. Walk/carry-walk contacts
use manifest stride `1.677975879375` and phases `22/48`, `46/48`. Contact tests
cover stops/restarts, snaps, loading, late visibility/range entry, clip changes,
long pauses and world reset without catch-up/double contacts. No new movement
messages are requested. The recorded gains verify the intended small distinction
between own and other footsteps; subjective speaker/headphone balancing remains
human judgment, not something inferred from native-play flags.

Actual producer-to-browser replay also passed after uploading the exported
fixture through the harness file input. The browser decoded the exact server
WebSocket bytes (rather than constructing substitute sound packets):

| Server tick | Anchor ms | Sound | Present gain | Native per-ID volume |
| --- | ---: | --- | ---: | ---: |
| 12 | 11200 | chop | .10400000214576721 | .046800000965595244 |
| 32 | 13200 | chop | .10400000214576721 | .046800000965595244 |
| 40 | 14000 | tree_fall | .39358600974082947 | .186953354626894 |

All three native voices reported playing. The fixture verifies two gameplay
effects, two chops, one fall, no visible source and a despawned target. The
browser runs against the captured tick anchors to test wire replay without
pretending an old recording is a fresh live-world event.

With the rendered-preview option enabled, real `ActorRenderer` instances ran
`chop_r`, `chop_l`, `walk` and `carry_walk` from the published GLB clips. Across two
preview cycles there were two authored impact playbacks, eight walk contacts and
eight carry-walk contacts: 18 admitted sounds, zero world cues emitted by local
animation processing and zero GL errors. All eight sampled character outputs
contained more than 2,000 opaque pixels. The final browser run reported no
failures, warnings or errors. Exact output is retained in
[browser-validation.json](browser-validation.json). The fixture replay, rendered
variants and native playback were verified; no subjective human listening claim
is made.

## Release checks

The production shard registers 25 systems before and after the change (verified
against HEAD). There is no hearing component, persistence field or migration.
One active route exists for each sound: server animation marker → world chop;
server successful completion → world fall; client gait contacts → steps; existing
owner FX → discovery. See [the revised PRD](../../../docs/prd/sound_event_spatial_audio_refactor.md)
and [ADR 0006](../../../docs/adr/0006_world_and_local_sound.md) for matching-version
deployment and rollback as a single server/client/definition/catalog unit.
