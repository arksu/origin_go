# Farming

Four annual crops complete the existing WWW → Drying Frame → seed loop. All
rules, random outcomes, growth and item creation are server-authoritative.

## Planting and interaction

Take one seed into the hand and right-click plowed ground. The server approaches
the tile center and performs one 10-tick action costing exactly 10 stamina, with
no percentage reserve threshold or additional cooldown. The ordinary Plant seed
menu action uses the same validation and execution. Left-click hand drops retain
their existing behavior. One click never processes neighboring tiles.

An attempt captures the seed's identity and hand revision before approaching.
Completion rechecks that seed, stamina, tile center, loaded active chunk, plowed
terrain and bounded spatial occupancy. A crop occupies one tile; other static
collider footprints also prevent planting at its center. Players and loose drops
do not occupy planting space. Invalid, canceled or unavailable attempts consume
neither seed nor action stamina. Ordinary movement can still cost stamina.

Each crop definition configures `plantingSuccessChance` in [0, 1], default 1.
A valid random miss consumes the seed and stamina but creates no crop; it is a
completed paid attempt and sends an informational alert. Successful planting
creates the crop before committing seed consumption, and preparation failure
leaves the seed intact. Hand offsets are cleared and its revision advances.

Crops are ordinary damageable static objects with 100 HP and a 4×4 collider.
Collider layer/mask remain zero at every stage: walking through crops is harmless.
An existing crop still prevents duplicate planting, including while harvest is
pending. Terrain overgrowth can turn plowed soil into grass without changing the
crop. The next planting still requires plowed soil.

## Growth and harvest

Growth uses accumulated server runtime. Unloaded chunks continue to age, stopped
servers do not. Four visual stages use configurable durations: vegetables
4/4/8 hours, wheat 6/6/12 hours; the last stage has no deadline and never rots.

| Crop | Harvest window from planting | Uniform inclusive outputs |
| --- | --- | --- |
| Carrot | 8 to less than 16 runtime hours | 1–3 carrots |
| Carrot | 16 runtime hours onward | 1–3 carrots + 1–2 carrot seeds |
| Beet | 16 runtime hours onward | 1–3 beets + 1–2 beet seeds |
| Potato | 16 runtime hours onward | 2–4 potatoes + 1–2 potato seeds |
| Wheat | 24 runtime hours onward | 2–3 wheat seeds/grains + 1–2 straw |

Right-clicking an eligible crop starts one 10-tick Harvest cycle costing exactly
10 stamina. Growth is reconciled before interaction and at final-tick acceptance;
a carrot that crosses 16 hours during the action gives the later-stage harvest.
One shared bounded helper advances from the previous persisted deadline, with at
most three transitions; it never starts missed stages from the current time.
The terminal stage cancels its scheduler entry. No crop scan is added per tick.

Harvest removes the plant. Outputs follow existing placement order: player root
grids, eligible nested bags, then a free hand. Remaining individual items drop at
the crop center, using normal drop lifetime and ownership metadata. Each output
is nonstackable and 1×1. `seed_wheat` is both planting seed and grain.

## Quality and definitions

`game.farming_quality_variance` is an integer N in [0, 4294967295], default 2.
A successful planting draws one uniform integer delta in [-N, +N] and stores
`seed quality + delta` as canonical object quality. Every product and seed from
that plant has this same Q. No rerolls occur during growth, harvest or retries.
There is no gameplay cap; arithmetic saturates only at the existing uint32
representation boundaries, 0 and 4294967295. N=0 exactly preserves seed quality.

Object definitions use the `crop` behavior with `seedItemKey`, optional
`plantingSuccessChance`, four `stages` containing `durationSeconds` and optional
`harvest: [{itemKey, min, max}]`, `harvestCycleDurationTicks` and
`harvestStaminaCost`. Only stages 2 and 3 may declare harvest; final-stage harvest
is required. Seed mappings are unique. Configuration fails startup for unknown
items, invalid chances/durations/counts, incompatible items or a configured
maximum above 32 output units per harvest.

The existing behavior envelope stores only `stage` and
`next_growth_runtime_seconds`. Appearance derives from `crop.stage0` through
`crop.stage3`; temporary visuals reuse the existing WWW stage images. Carrot and
straw use existing icons; beet and potato use their seed icons temporarily.
Vegetables are ordinary items without food/FEP behavior. Final artwork, cooking,
skills, seasons, watering, fertilizer, hives, trellises and batch work are deferred.

World-object quality uses checked BIGINT storage so uint32 Q is not truncated to
32767 on save. Apply `20261030_object_quality_uint32.sql` before the new server.
Runtime/protobuf types remain uint32; old portable snapshot version 1 remains
readable. Spot peak-quality storage is unchanged.

## Persistence and failure handling

Planting keeps the ordinary independent object/player checkpoint contract.
Clean saves and restarts retain the consumed seed and crop state; cross-entity
crash-atomic planting is not added.

Harvest uses bounded operation slots and the existing chunk persistence workers.
Admission reserves the crop, recipient inventory, source chunk and operation
capacity before accepting the action and charging once. The crop remains a tile
occupant while pending. Later movement, cancellation or disconnect cannot revoke
an accepted operation. Its stage, counts, quality and allocated identities are
retained across retries. Inventory trees are captured in bounded read batches;
placement is computed on owned scratch data, without changing live inventories.

One transaction commits post-grant player inventories, overflow objects and their
inventories, the ID checkpoint, and the crop's soft deletion. The retained source
row stores a harvest receipt. Replay checks it before inventory writes and never
uses an upsert that can resurrect the crop. Confirmed results apply through the
owning shard, normal lifecycle cleanup, raw-cache installation and visibility.
Ground activation retains the existing bounded work and ECS-capacity retry path.

A proven permanent failure before commit preserves the crop, releases its
reservations, rearms growth, refunds action stamina and alerts the player.
Uncertain commit results retain the same payload and retry with backoff. Shutdown
stops admission and drains operations before stopping persistence workers.

## Verification

Focused Go coverage includes planting/revalidation/paid misses, stage boundaries,
checkpoint restoration, output placement, bounded admission, replay/rollback and
quality boundaries. PostgreSQL tests use isolated schemas for transactions and
migration checks. Client crop tests resolve all temporary stage and item assets.

Run `CGO_ENABLED=0 go test ./...`, and in `web_new` run `npm run type-check`,
`node scripts/test-character-visual.mjs tests/crops.test.ts`,
`npm run validate-objects`, `npm run test:map-click`, `npm run test:actions`,
`npm run test:inventory-transfer`, `npm run test:trees`, and
`npm run test:asset-pipeline`. Run PostgreSQL tests with their documented local
DSN environment variables; a skipped integration test is not a database pass.
