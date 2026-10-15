# Inventory quick transfer

Shift + left click transfers a grid item or one deposited construction material
to another open inventory without using the hand. The feature applies to the
player inventory, open containers, station grids, nested bags and construction
sites. Equipment retains its existing interaction behavior. An ordinary build
row click still deposits the held item or takes one deposited unit into an empty
hand; Shift takes precedence even when the hand already holds an item.

## Target selection

Targets follow window opening order, not window focus or stacking order. Start
after the source and wrap around. Exclude the source and the item's own nested
inventory. Snapshot refreshes keep the existing position; closing removes the
target, and reopening appends it. Cached inventories whose windows are closed
are not targets. Future linked objects exposing an open grid use the same path.

The client includes the ordered open-window snapshot with each Shift-click. The
server checks source eligibility and tries each distinct destination at most
once, stopping at the first success. Inaccessible, missing, rejecting or full
destinations are skipped. The list is limited to 64 references, and the entire
choice and transfer run in one shard command.

A grid receives a whole grid item or stack in its first free fitting area;
existing stacks are not merged. A build destination accepts up to the first
matching incomplete requirement's remaining count, leaving excess in the source
with the same identity and position. A build source returns exactly one unit
from the selected row's last nonempty deposited stack, preserving its quality.
It can target only grids. Failed take-back leaves material in the build site.

## Commands and lifecycle

Quick transfer uses `InventoryOp.move` with these additive fields:

- `modifiers = 1` (`SHIFT`), with no explicit `dst`, placement, quantity or hand
  offsets, and `allowSwapOrMerge: false`.
- `open_inventories`: source and visible GRID/BUILD references in opening order.
- Optional `build_slot`: required for a BUILD source, including valid slot zero;
  build sources omit `item_id`. Grid sources send their existing item ID.

Grid sources include only their expected source revision. Build sources are
aggregated rows without inventory revisions. Destination selection uses current
server state; the server validates access, linked build target, content rules
and capacity. No new packet or result message is added. Normal moves and the
legacy `C2S_BuildTakeBack` hand flow retain their existing behavior. The new client
requires a server supporting the new Shift fields; existing clients continue to
use their normal commands.

Only one quick transfer is active at a time; additional Shift-clicks are ignored
until its correlated `InventoryOpResult` arrives. The client applies authoritative
snapshots before completing the operation and never retries against another
destination. The server returns one result, so failed candidate admissions cannot
produce intermediate alerts. With no fitting destination, nothing moves and the
existing mini-alert flow reports the final failure. With no other open window,
Shift-click does nothing and never falls back to pickup.

Closing the source suppresses a late failure alert. In-flight actions can still
complete on the server; their authoritative snapshots are applied. World/session
changes clear pending state. Ordinary clicks and right-click container opening
remain available; quick transfer itself never changes the hand.

The server's existing build deposit handler accepts both `HAND` and `GRID`
sources. Deposits and direct grid take-back commit atomically on the owning shard
execution path, retaining the existing BuildState, inventory, nested-parent and
observer update flows. Selected build IDs travel through internal result metadata
so BuildState refresh works even though the quick request has no explicit `dst`.

## Verification

- At the repository root: `make test`.
- In `web_new`: `npm run type-check`, `npm run lint`,
  `npm run test:inventory-item`, and `npm run test:inventory-transfer`.
- Review the lint diff and run `git diff --check`.
