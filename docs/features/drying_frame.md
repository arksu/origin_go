# Drying Frame

Drying Frame automatically converts Wild Windsown Weed (WWW) into seeds. It uses
an existing station, the build-site flow and the ordinary container window. Its
2×2 root inventory can dry four items independently; no start action is needed.

## Construction and interaction

- Object key: `drying_frame`; display name: `Drying Frame`.
- Build: one `branch`, 20 ticks, 8 stamina, no skill or discovery requirements.
- Root inventory: grid key 0, 2×2; collider: 24×8 world coordinates.
- Top-level station metadata: `capabilities: ["drying"]`, `states: ["ready"]`,
  `initialState: "ready"`. Ready means available after construction, independently
  of item completion.
- Action: `Open`. The frame has no `lift` behavior.
- Placement and destruction follow the existing building rules.

Definitions are in `data/objects/drying.jsonc` and `data/builds/basic.jsonc`.

## Drying process contract

The `drying` behavior accepts a `processes` list. Each process declares
`inputItemKey`, a nonempty `outputItemKeys` array, a positive integer
`durationSeconds`, and optionally `visualCategory: "herbs"` or `"hide"`.
A singleton output array is a fixed recipe; multiple outputs form an equally
weighted pool. An empty processes list remains valid. Item definitions must
exist; inputs must be unique and each output pool must have no duplicates.
Items must be nonstackable grid items without nested inventories; every output
must fit inside the input's occupied rectangle. The behavior requires a
`container` with one 2×2 root grid.

If recipes share an output type, their visual categories must agree, including
omitted categories. For appearance, a type that is also a recipe input uses
that input recipe's category and input role before considering output roles.

The live recipe consumes one `wild_windsown_weed` after **57,600 runtime seconds
(16 hours)** and produces exactly one of `seed_wheat`, `seed_carrot`, `seed_beet`,
or `seed_potato`, each with probability 1/4. The seed pool belongs to this drying
recipe; the plant behavior has no `seedOutcomes` configuration. The result is
selected only when drying completes. Neither the plant nor the harvested WWW
item stores a hidden seed choice.

The server permits only declared inputs and outputs, in quantities of one,
through the shared inventory insertion validation, including reverse swaps.
Unsupported items produce an English error. Completed outputs may be placed
back in the frame; they do not acquire a timer unless also declared as an input.
Normal transfers use the hand or backpack. Direct ground pickup into world
containers and direct ground drops from them retain the existing durable
inventory-transfer restrictions.

Each input gets its own deadline. One input becomes one output in the same
position, with a new ItemID and exactly the input quality. Frame quality affects
neither the duration nor output quality. Moving an input to another cell inside
the same frame restarts its full deadline. A swap restarts both moved inputs.
Removal cancels progress; reinsertion starts a full deadline, including removal
and return in the same tick. Failed operations, no-op moves to the same cell and
changes to unrelated items do not reset an input's timer.

## Runtime and persistence

InventoryExecutor synchronously dispatches root inventory mutations to behavior
listeners after every successful committed operation, before returning its
updated snapshots or processing the next command. Failed operations do not
change timers.

Committed root inventory changes mark the frame for persistence even when its
drying timers, flags and appearance stay unchanged, including moves of finished
seeds.

The existing object-state envelope stores at most four entries containing
`input_item_id`, `input_type_id`, `input_x`, `input_y`, and
`completion_runtime_seconds`. Matching includes item identity, type and cell;
a changed position starts a full deadline. Deadlines use
`TimeState.RuntimeSecondsTotal`, so online time in unloaded chunks counts and
offline time pauses. Restoring an object reconciles the saved entries with its
inventory and schedules the nearest deadline. Overdue work runs through the
runtime scheduler after activation, with the normal per-tick budget.

The indexed runtime heap stores one physical task per generational handle and
behavior. Rescheduling replaces that task; despawn and in-place transforms
cancel it. `BehaviorTickSystem` (priority 313) processes existing tick tasks
first, then runtime tasks with the remaining shared budget (200 by default).
There is no periodic scan of frames.

Completion revalidates the live handle, external identity, object definition,
destruction state, root ownership, input identity and cell. It validates and
prepares all due conversions, including fresh ItemIDs and random outputs, before
mutating the inventory. A preparation failure leaves all due inputs unchanged.
Completion increments the inventory version, marks the object for persistence,
and sends inventory updates to all players with that container open. Completed
seed items persist through the ordinary inventory save path; restoring a
completed inventory does not select the result again. Existing object/inventory
persistence is used; there are no new protocol fields, SQL migrations, progress
windows or client timer state.

## Appearance

Appearance is derived on the server from categories present in the root inventory,
not item count or progress. A recipe's optional `visualCategory` applies to its
input and outputs. WWW and its four finished seed types use `herbs`, so seeds left
in the frame keep the herbs visible. For `hide`, inputs use `raw_hide` and outputs
use `dried_hide`; raw hides take precedence when both are present. Herbs overlay
either hide state.

| Inventory categories | Resource |
| --- | --- |
| No categorized items | `dframe/empty` |
| Herbs only | `dframe/herbs` |
| Raw hide, with or without dried hide | `dframe/raw_hide` |
| Dried hide only | `dframe/dried_hide` |
| Raw hide and herbs, with or without dried hide | `dframe/raw_hide_herbs` |
| Dried hide and herbs | `dframe/dried_hide_herbs` |

All six variants use the existing frame PNGs and one shadow, with anchor `(29,60)`
and layer offsets `(0,0)`. Combined variants place the hide at `z: 0` and herbs at
`z: 1`, preserving their order while textures load asynchronously. No art export,
mask or renderer change is needed. Hide categories are supported by the contract,
but the live catalog has no hide drying recipes.

The [WWW plan](wild_windsown_plan.md) describes spawning, harvesting, regrowth and
spot balance. Those rules are unchanged by drying.

## Checks

Server coverage exercises recipe validation, inventory eligibility, independent
deadlines, movement and swap restarts, same-tick removal and return, completion,
quality, persistence and appearance selection. Client asset coverage resolves both
combined variants through the real resource loader and object renderer, checks
the published PNGs and shared anchor, and verifies one shadow and herbs above a
hide that finishes loading later.

Run `make test`; in `web_new`, run `npm run type-check`, `npm run lint -- --no-fix`,
`npm run test:inventory-item`, `npm run test:inventory-transfer`,
`npm run test:inventory-drag`, `npm run test:character-visual`,
`npm run test:asset-pipeline`, and `npm run validate-objects`.
