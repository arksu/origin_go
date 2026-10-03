# Phase 1 public combat test range

This range exercises `axe_aoe` (Axe sweep) and `axe_single` through the shared
server combat path. Any authenticated player can use `/combat-range` while the
server test flag is enabled. No administrator role is required during public
testing; roles are deferred.

## Enable and prepare

Start the server from the repository root with its usual database/world settings
and `GAME_COMBAT_TEST_ENABLED=true`. Alternatively add this to `config.yaml`:

```yaml
game:
  combat_test_enabled: true
```

The default is false: range commands reject and enter-world does not advertise
combat support. Use a disposable test world. Rebuild the paired protocol/client
and publish animation definitions when installing this change:

```sh
make proto
npm --prefix web_new run proto
./tools/assets publish-action-animations
npm --prefix web_new run build
```

Log in normally. Stand on clear ground with a loaded, active 96 × 96 world-unit
patch centered on the player. Creation rejects overlap with existing colliders;
it never deletes ordinary objects. Move the observer outside this patch during
creation, then bring them within visibility. Each player may own one range, with
at most eight per shard. Commands affect the caller's own range and actor only.

Send these chat messages separately (normal chat throttling still applies):

```text
/combat-range create small
/combat-range strength 1
/combat-range stamina 500
```

Creation grants one Q=10 `stone_axe` through normal inventory placement if one is
not already present. Leave inventory space, then equip it in the right or left
hand through the usual inventory UI. Right hand wins if both hands contain
compatible weapons, so remove a different-quality right-hand axe for the Q=10
baseline. `/combat-range axe` retries the same setup grant without duplicating an
existing Q=10 axe. A failed grant reports the inventory error; the created range
can still be removed normally.

`strength` is a temporary effective-STR override (0.01–1000), not a saved attribute
change. `stamina` accepts 0 through the character's current maximum and uses the
ordinary stat update/save path. Neither command changes real-character health.

## Input and expected presentation

1. Select Axe sweep or the single-target axe action in Actions, or drag it to the
   hotbar and activate there. Arming is free and waits for the server's selection.
2. Point toward the targets, then primary-click/tap once to commit the direction.
   The green preview is local; no continuous aim packets are sent. The initial
   fixtures lie along world +X (down-right in the isometric view).
3. The sector turns gold for the 600 ms windup and blue for the 400 ms recovery.
   The public label shows the phase/countdown. A sweep costs 60 stamina once at
   commitment; the single-target action has the same cost and timings.
4. At 600 ms the server samples current positions and applies hits. At STR=1,
   Q=10 and armor=0, sweep damage is 6.0 per contact and single-hit damage is 9.0.
   Target HP and hit/miss labels come from server packets. Positive values below
   0.1 display as `<0.1`; other HP/damage uses one decimal place.
5. The full chop clip fits 1000 ms; its art frame does not decide the strike time.
   Right/left hand bindings reuse `chop_r`/`chop_l` without tree sound cues.
6. Move with a fresh WASD press or primary coordinate click after commitment.
   Windup movement is capped at Crawl; recovery uses ordinary movement rules.
   The sector follows the moving actor while its direction stays locked. Pointer
   motion and turning the actor cannot redirect it.
7. Cooldown starts at commitment and ends at 2000 ms, independently for each
   action. The icon countdown is informational; icons remain selectable and the
   server supplies rejection reasons. Another action can start after recovery at
   1000 ms even while the first action's cooldown remains.

Escape cancels selection. Once committed, Escape, secondary clicks/long-press,
craft/build/context/lift requests and equipment swaps cannot cancel or queue an
action. Primary clicks route only to coordinate movement during commitment;
held items and clicked drops do not start an inventory operation. Input handoff
releases an old WASD hold; a fresh press can move during combat.

Nearby observers see public phases, locked aim, animation, visible target HP and
results. Cooldowns and stamina belong to the owner. Entering visibility midway
seeks current progress and HP; it does not replay the swing or old hits. Invisible
target identities/damage are omitted from observer results. A connection whose
critical queue fills closes rather than silently losing authoritative state.

## Presets and controls

Offsets are relative to the player's position at creation; collider sizes below
are half-width × half-height. Presets live in `data/combat_range.json`.

| Preset | Layout | Aim +X from the origin |
|---|---|---|
| `small` | (10,0), (13,-7), (13,7); each 1 × 1, HP 100 | Sweep leaves all three at 94; single leaves the near one at 91. |
| `large` | (24,0); 8 × 4, HP 100 | Nearest edge is 16 away: hit although center is outside range 18. |
| `tied` | (10,-4), (10,4); each 1 × 1, HP 100 | Single hits only the first/lower-ID fixture. |
| `moving` | (12,0); 1 × 1, HP 100; Y sine amplitude 20, period 2400 ms | A commit synchronized to reset misses at 600 ms when the target is at Y+20. |
| `blocker` | Blocker (7,0), 1 × 5; receiver (14,0), 1 × 1, HP 100 | Target behind blocker loses 6 on sweep; blocking movement does not occlude axe hits. |

The existing boulder visual is a placeholder; the red outline shows the actual
target collider. The blocker has collision but no damage receiver/HP label.
Depleted receivers remain in the range with HP zero and stop receiving hits.

```text
/combat-range reset
/combat-range interrupt
/combat-range remove
/combat-range create large
```

`reset` restores only fixture HP, positions and motion phase; it does not refund
stamina, reset cooldowns, change equipment or reset real health. `interrupt`
exercises the external interruption entry point on the caller: no refund and no
cooldown reset, including when recovery is canceled. `remove` unregisters
receivers, removes spatial entries, despawns fixtures and clears temporary STR.
Change presets with remove/create. Owner teardown or unloaded fixture cleanup
also removes its range.

For low-stamina checks, set `/combat-range stamina 59`, then arm/commit: rejection
must leave stamina and cooldown unchanged. Repeat at 60: acceptance leaves zero
and still resolves the hit/miss. Normal regeneration continues, so use the
controlled-time packet harness below for exact boundary assertions; typing or
waiting in the live client may allow regeneration. The moving preset's exact
reset-to-impact timing is likewise deterministic in the harness.

## Repeatable acceptance evidence

Recorded 2026-10-03 using controlled server time, actual WebSocket clients and
client input/playback tests. The two-client demonstration is automated; it is
not a claim that a manual browser playthrough was performed.

```sh
CGO_ENABLED=0 GOCACHE=/tmp/origin-go-build go test ./internal/game -run 'TestCombatTwoClient' -count=1 -v
CGO_ENABLED=0 GOCACHE=/tmp/origin-go-build go test ./internal/combat ./internal/game/... ./internal/ecs/systems ./internal/entitystats
node web_new/scripts/test-character-visual.mjs tests/combat.test.ts
node web_new/scripts/test-character-visual.mjs tests/combat-protocol.test.ts
npm --prefix web_new run test:actions
npm --prefix web_new run test:map-click
npm --prefix web_new run test:movement-input
npm --prefix web_new run test:action-animations
npm --prefix web_new run test:character-visual
```

| Check | Observed result / automated evidence |
|---|---|
| Owner + observer sweep | `TestCombatTwoClientPublicRange`: windup at 0, three 6-damage hits/HP 94 at 600 ms, idle at 1000 ms; repeated update leaves HP unchanged. Observer with only one visible target receives one hit entry and its own epoch. |
| Single, tie, escape, moving actor | `TestCombatTwoClientSingleEscapeAndMovingAttacker`: both sockets receive one 9-damage nearest/tied hit, zero escape hits, and three 6-damage moving-attacker hits with +X still locked. Owner cooldown deadline is 2000 ms; 500 stamina becomes 440. |
| Late visibility and removal | `TestCombatVisibilityEntryCapturesLatestState`: delayed spawn reads HP 94/revision 2 and recovery elapsed 700 ms. Removed fixture handles cannot respawn. Public-range test verifies despawn and visibility cleanup. |
| Delivery failure | `TestSpawnBatchCriticalDeliveryOnFullQueue`: combat target/execution snapshots close a full connection; ordinary spawn behavior remains unchanged. Existing critical-send tests cover nonblocking failure and healthy-peer isolation. |
| Geometry | `internal/combat`: both sector edges, exact range/angular tangency, just-outside misses, containment, large/cross-chunk targets, nearest intersection rather than center, shuffled ties, self exclusion and duplicate candidates. All range presets use the same receiver path. |
| Payment/replays/timers | Combat service tests: 59 rejects, 60 pays to zero, payment once, no refund, independent cooldowns, alternate tick duration, crossed deadlines, consumed rejected attempts, old epochs/selections and late replay. |
| Bypass attempts | `TestCombatRawWorkAndObjectClicksCannotBypassCommitment`, movement and inventory guard tests: Escape/secondary/craft/build/context/lift, hand transfers/swaps/drop, deferred intents, zero-stamina movement, collision and external interruption. No queued work or duplicate effects. |
| Client input | Actual Render/InputController callbacks: held-item preservation, discrete revision-bearing clicks, touch tap, secondary/long-press, old keyboard hold suppression, fresh WASD and support-absent activation. Escape sends cancellation for server policy. |
| Client convergence | Combat cache and shared animation tests: full uint64 IDs, wrong epochs/incarnations, stale/contradictory revisions, duplicate/late results, fractional HP, milliseconds, late assets, culling resume and interruption. |

Full `go test ./...` passed with both PostgreSQL integration variables set against
a disposable PostgreSQL 15.3 container. The combat save/load test freezes
regeneration, performs the real transactional character/inventory save and loads
it into a fresh world: stamina 120 → 60 remains 60 and axe Quality remains 10.
Fixture serialization returns no persisted object; runtime combat state is absent
on a fresh actor. No new migration was added.

To repeat database coverage, use a disposable database initialized from
`migrations/schema.sql` (the chunk integration test uses the `origin` schema):

```sh
ORIGIN_CHARACTER_SAVE_TEST_DSN='postgres://USER@HOST:PORT/TEST_DB?sslmode=disable' \
ORIGIN_TEST_DATABASE_URL='postgres://USER@HOST:PORT/TEST_DB?sslmode=disable&search_path=origin' \
CGO_ENABLED=0 GOCACHE=/tmp/origin-go-build go test ./...
```

Character/combat persistence tests create and drop their own uniquely named
schemas. The chunk persistence test rolls back its transaction. Without these
environment variables those integration tests skip. On the acceptance machine,
the native PostgreSQL binary had a missing ICU library; the disposable Docker
instance supplied the working prerequisite. `CGO_ENABLED=0` avoids that machine's
unrelated Darwin C linker/SDK mismatch.

Client checks above, client type-check/production build, and all 127 asset-pipeline
definition/publication tests passed. The build reports existing bundle-size,
mixed-import and protobuf `eval` warnings. Required atlas regression, Pixi
semantics and render simulation also passed (1899 frames; zero differing pixels).

## Persistence and later phases

Fixtures, target HP, active execution, cooldowns, temporary STR and
`LastCombatEventAt` do not survive restart. Stamina and inventory Quality use their
existing persistence. Reconnect/transfer timer protection is not provided by this
milestone; this is public testing, not persistent-world combat readiness.

Deferred: ordinary world-object HP/destruction (phase 2); combat player health,
KO/death (3); real equipment armor and broader combat gear (4); status effects,
wounds/stun sources (5); bow/arrows (6); combat logout and session restoration
(7). Monsters follow later. Existing invalid/dead/KO states can interrupt this
runtime, but phase 1 does not add damage to players or ordinary objects.

The implementation and delta specs are ready for review and the normal OpenSpec
sync/archive handoff after acceptance. Disabling the flag gates the prototype;
there is no database migration to roll back.
