# Corpse decay

`player_dead` becomes `player_skeleton` after `CorpseDecaySeconds = 21600`
seconds of server runtime. `RuntimeSecondsTotal` advances while the server runs,
including while the corpse's chunk is unloaded, and pauses while the process is
offline. The deadline is stored as `decay_at_runtime_seconds` in the existing
`player_dead` behavior state inside `ObjectInternalState`; no ECS component,
database schema or protocol field is added.

Spawn, restore and direct player-to-corpse conversion initialize the behavior.
Repeated initialization preserves a recorded deadline. An indexed minimum heap
in the shard's ECS resources schedules loaded corpses. Its entries contain the
full generational handle and entity ID; removal or unload cancels the runtime
entry. Loading an overdue corpse schedules it immediately. The shard processes
at most 100 due attempts per update, and rejected admission retries after one
server-runtime second.

Decay uses the existing object destruction capture, bounded workers and
quarantine to release every inventory root, including equipment and the hand.
Stacks split into quantity-one ground items; bags retain their nested contents.
The worker atomically persists the drops, removes old inventory rows and upserts
the skeleton under the original region and entity ID. Capture, reserved IDs and
drop positions remain unchanged across retries. The existing drop lifetime is
measured from the operation's original runtime timestamp.

After commit, bounded cleanup removes the captured runtime inventories. The
source's raw cache is replaced with the durable inventory-free snapshot, and the
owning shard applies the ordinary in-place definition transform. The skeleton
retains the character's `EntityID`, position and quality while the corpse's
inventory ownership, decay state and character corpse visuals are removed.
Persistence pins and the raw-cache revision fence prevent old chunk saves or
loads from restoring the body and its inventories. A restart after commit loads
the skeleton and ground items directly from PostgreSQL.

The skeleton definitions are `player_skeleton` (17) and
`player_skeleton_without_skull` (18). Both have 100 HP, are indestructible and
liftable, and have no inventories. Decay creates only the first. Removing the
skull and granting a skull inventory item are future work.

The same ID is retained from player to corpse to skeleton. The soft-deleted
`character` row therefore remains the future authoritative source for the dead
player's name. The ordinary active-character query filters deleted rows and is
not an appropriate lookup for that future interaction. No name is duplicated in
the skeleton state, and this feature does not expose a new name lookup API.

Validation covers persisted runtime deadlines, overdue restoration, bounded
scheduling, admission retries, inventory release, preserved identity and
appearance replacement. PostgreSQL integration tests use isolated schemas via
`ORIGIN_OBJECT_HEALTH_TEST_DSN`; their transaction coverage includes multi-batch
loot, retry after commit, rollback after iterator/replacement failures and an
unsaved source with no items. No existing-corpse migration is needed for the
current empty database.
