# Floating damage numbers

Client implementation, 2026-10-08. No server, definition, dependency or wire changes.

## Authority and lifecycle

`AttackResultReceiver` remains the sole validator of epoch, exact uint64 event
ordering and the entire hit list. Only accepted packets are projected to plain
decimal target IDs and damage, then passed through `GameFacade` to `Render`.
This includes observed attacks, object hits and zero damage. Misses create no
label. Numbers show damage after armor, before health distribution/clamping;
they do not predict pools or create unknown entities.

Current ObjectView maps use numeric identities. A target is resolved only when
its canonical ID is exactly representable as a positive safe integer. Larger
uint64 IDs remain valid protocol data but are skipped for this presentation.

The manager captures the current visual bounds-top once. A launched label stays
at that world anchor, inheriting camera pan; text and animation offsets are
counter-scaled so zoom does not change their screen size. Ordinary despawn keeps
launched numbers and copies a last-known anchor into a FIFO cache of at most
1,024 entries for 2 seconds. This supports object quarantine arriving before its
fatal AttackResult. Cache lookup is lazy; expiry does not scan the world or cache.
Live views take precedence, spawn removes old anchors, and unknown/expired targets
are skipped. World/epoch/connection reset clears both numbers and cache. Destroy
releases the manager's views and installed bitmap font.

## Presentation and limits

Arial bold, 22 screen pixels, red `#FF4040`, dark 3-pixel outline. A label lives
900 ms, pops from scale 1.2 to 1 in 120 ms, rises 48 pixels with quadratic ease-out
and fades during the final 300 ms. Three left/up/right paths have initial offsets
of -32/0/+32 pixels and a further 18-pixel drift; a second row 28 pixels higher
separates simultaneous hits 1 and 4. Each hit stays separate.

Values use one decimal with trailing `.0` removed; tiny positives use `<0.1`,
zero uses `0`, and values from one million use scientific notation with one
decimal (e.g. `1.0e+6`). The presentation never changes authoritative values.

The pure presentation model owns the limits: 512 fixed records and four live
labels per target. Overflow replaces the target's oldest label first, otherwise
the global oldest, with insertion order resolving equal timestamps. A single
noninteractive layer sits below nicknames/chat and above ordinary objects.
Pixi reuses 512 BitmapText views and one preinstalled numeric glyph atlas. The
existing render loop supplies `performance.now()`; there are no extra clocks,
timers or ticker callbacks. Returning from a background pause expires old labels.

The model and manager updates create no temporary arrays/objects. Incoming
packet projection, strings and Pixi's bounded glyph-layout cache may allocate;
this is not a zero-allocation contract for the whole Pixi renderer. Work scans
at most 512 records/views, independent of the world entity count. Emission uses
a bounded scan per hit to keep replacement simple; a full 512-hit batch therefore
does at most 512 x 512 record visits before one view sync.

## Verification

Run from `web_new`:

```sh
npm run test:damage-numbers
npm run test:combat-protocol
npm run test:actions
npm run test:direction-aim
npm run test:knockout
npm run test:chunks
npm run type-check
npm run lint
npm run bench:damage-numbers
```

The Node suite covers formatting, identity conversion, fully accepted network
events, reset, cache eviction/TTL, despawn-before-result, fixed anchors, pan/zoom,
absolute-time animation, simultaneous paths, bounds and resource reuse. Existing
render bridge fixtures include the new manager lifecycle.

With the existing Vite server, open `/tests/damage-numbers-preview.html`. This
standalone fixture uses the production receiver and renderer with an existing
tree sprite over a dark green background and never connects to the game server.
Controls exercise actual font rasterization, zoom 0.12/10, renderer resolution
1/2, fatal despawn, four simultaneous hits, fade frames and a 512-target sweep.
`Measure update / reuse` measures CPU update and draw submission separately and
checks 512 retained views after 100 full emission/reset cycles. Frame timing
includes draw submission, not GPU completion.

The benchmark reports five-sample CPU medians and retained JS heap after explicit
GC for 20 model/manager instances. Node does not rasterize text or measure GPU
memory. The browser fixture reports actual font-page dimensions: the installed
Pixi font uses one 1024 x 1024 RGBA page, approximately 4 MiB (excluding driver
overhead and canvas backing).

## Measurements and checks (2026-10-08)

Node CPU medians, five samples; timings are diagnostics, not CI thresholds:

| Labels | Pure update, ns/op | Manager update, ns/op |
| --- | ---: | ---: |
| 0 | 18 | 30 |
| 1 | 740 | 3,194 |
| 10 | 774 | 3,547 |
| 512 | 2,855 | 21,139 |

A full `clear + show(512)` batch took 1,069,657 ns; constructing/destroying one
manager took 1,860,879 ns. Retained JS heap averaged 98.8 KiB for the pure model
and 1,006.8 KiB for the complete manager/pool, excluding font, retired anchors and
GPU resources. Measurements use 20 retained instances with explicit GC and should
be treated as approximate heap attribution.

The browser fixture at resolution 2 / zoom 1 reported median draw-submission
frames of 2/50/44/146 microseconds for 0/1/10/512 labels (five 100-frame samples).
CPU updates measured 0.02/2.81/3.21/18.91 microseconds. These runs used the same
fixture scene, not the complete game, and do not wait for GPU completion. After
100 full emission/reset cycles, the pool still held 512 views and the font one
page. Visual checks covered a real tree sprite over dark green, zoom 0.12/10,
resolution 1/2, separated simultaneous numbers, 512 hits, and fatal damage after
despawn. `Save PNG` exports the current fixture canvas for review.

Functional checks: 18 damage-number tests, 9 protocol tests, 10 direction-aim
tests, 5 knockout tests, the actions suite and 14 chunk tests pass. Type-check and
targeted ESLint of new files/changed runtime pass. Full `npm run lint` reports 479
pre-existing errors, including generated protobuf, the Basis decoder and old
tests; no unrelated automatic-fix diff was produced. Generated SQL/protobuf
outputs are unchanged. Independent code review found no significant defects.
