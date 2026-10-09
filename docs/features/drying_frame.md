# Drying Frame

Drying Frame is the first construction step before Wild Windsown Weed (WWW).
The frame uses the existing build-site flow and container window. The live
definition intentionally has no drying processes yet: it can be built and
opened, but rejects all items until recipes are added in a separate feature.

## Construction and interaction

- Object key: `drying_frame`; display name: `Drying Frame`.
- Build: one `branch`, 20 ticks, 8 stamina, no skill or discovery requirements.
- Root inventory: grid key 0, 2×2; collider: 24×8 world coordinates.
- Action: `Open`. The frame has no `lift` behavior.
- Appearance: existing `dframe/empty` resource and its client shadow.
- Placement and destruction follow the existing building rules.

Definitions are in `data/objects/drying.jsonc` and `data/builds/basic.jsonc`.

## Drying process contract

The `drying` behavior accepts a `processes` list. Each process declares
`inputItemKey`, `outputItemKey`, and a positive integer `durationSeconds`.
An empty list is valid. Item definitions must exist; inputs must be unique;
items must be nonstackable grid items without nested inventories; the output
must fit inside the input's occupied rectangle. The behavior requires a
`container` with one 2×2 root grid.

For example, a future WWW process will declare its corresponding raw item and
seed item with `durationSeconds: 300`. No WWW or hide processes are part of the
current live catalog.

The server permits only declared inputs and outputs, in quantities of one,
through the shared inventory insertion validation, including reverse swaps.
Unsupported items produce an English error. Completed outputs may be placed
back in the frame; they do not acquire a timer unless also declared as an input.
Normal transfers use the hand or backpack. Direct ground pickup into world
containers and direct ground drops from them retain the existing durable
inventory-transfer restrictions.

Each input gets its own deadline. One input becomes one output in the same
position, with a new ItemID and the input quality. Frame quality affects neither
the duration nor output quality. Moving an item within the frame preserves its
deadline. Removal cancels it; reinsertion starts a new deadline, including
removal and return in the same tick.

## Runtime and persistence

InventoryExecutor synchronously dispatches root inventory mutations to behavior
listeners after every successful committed operation, before returning its
updated snapshots or processing the next command. Failed operations do not
change timers.

The existing object-state envelope stores at most four entries containing
`input_item_id`, `input_type_id`, and `completion_runtime_seconds`. Deadlines use
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
destruction state, root ownership and input identity. It increments the inventory
version, marks the object for persistence, and sends inventory updates to all
players with that container open. Existing object/inventory persistence is used;
there are no new protocol fields, SQL migrations or station windows.

The [WWW plan](wild_windsown_plan.md) retains one WWW → one seed and the fixed
five-minute test duration. WWW spawning, harvesting and regrowth remain outside
this stage.
