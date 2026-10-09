# Taking a skull

The context menu for `player_skeleton` offers **Take Skull** through the unified
`player_skeleton` behavior. The server approaches the target through the normal
context-action path, then delegates immediately to `ExecutionDeps.TakeSkull`.
There is no timed action or stamina cost. Execution rechecks the exact target
handle, external entity ID, current full-skeleton definition and the player's
item-mutation eligibility. Headless skeletons do not expose this action.

Successful admission reserves the source and the recipient's inventory tree
until the durable operation resolves. The existing bounded transformation queue
and persistence pins protect the source and recipient from concurrent writes,
transfer or deletion. A worker obtains the soft-deleted `character` record by
the skeleton's retained entity ID. Its `deleted_at` is the death timestamp.
Missing death metadata rejects the operation without removing the skull.
The reserved skeleton stays visible with its collider and observer membership.
After commit the existing appearance update replaces it immediately; taking a
skull never sends a temporary despawn or waits for a periodic vision pass.

The worker atomically persists the recipient inventory with one `skull`, the
same-ID `player_skeleton_without_skull` replacement, the allocated-ID watermark
and a receipt in the skeleton's existing behavior state. A retry checks that
receipt before writing: an ambiguous successful commit cannot duplicate the
skull or overwrite a later recipient inventory with the old snapshot. ECS and
client updates are applied only by the owning shard after commit confirmation.
An admitted grant follows the same recipient if KO or death happens meanwhile.

The item definition is `skull` (3014), **Skull**, 1×1, nonstackable, allowed in
grid inventories and the hand, with no equipment slot. Quality comes from the
skeleton. Placement follows the ordinary grant order: main inventory, eligible
nested containers, then the empty hand. If no placement is available, the
server warns the player and leaves the original skeleton intact. Discovery and
other grant rules use their standard behavior.

Each memorial skull stores typed instance metadata: dead character ID, nickname
snapshot and calendar death date. Existing JSON inventory storage carries this
metadata through moves, swaps, bags, ground drops, pickup, destruction loot and
restart. An administratively created skull without memorial metadata remains a
normal skull. No new ECS component, database schema or protocol field is needed.

Both inventory snapshot paths populate the existing `ItemInstance.hint_ext`
with a line such as **Alice died on January 10, 2026**. Calendar dates use the
server's calendar and English month names without a time or timezone suffix.
The inventory and nested-inventory tooltip appends this line to the item's name
and quality. It inserts the content as plain text, so HTML-like nicknames render
literally. Ground-object and hand-item tooltips are outside this feature.

The generated pixel-art source and prompt live in `art_source/items/skull/`.
`python3 tools/export_skull_icon.py` deterministically publishes its transparent
32×32 inventory icon; `--check` verifies the source and final bytes.
