package game

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/playerstate"
	"origin/internal/types"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type skullClaimPersisterFixture struct {
	*destructionPersisterFixture
	metadataErr error
	commitErr   error
	metadata    inventory.DeadCharacterInfo
	records     []inventory.SkullClaimPersistenceRecord
}

func (p *skullClaimPersisterFixture) LoadDeadCharacter(context.Context, types.EntityID) (inventory.DeadCharacterInfo, error) {
	return p.metadata, p.metadataErr
}

func (p *skullClaimPersisterFixture) PersistSkullClaim(_ context.Context, record inventory.SkullClaimPersistenceRecord) error {
	p.records = append(p.records, record)
	err := p.commitErr
	p.commitErr = nil
	return err
}

type skullClaimFixture struct {
	*destructionServiceFixture
	shard     *Shard
	chunks    *corpseLootChunks
	persister *skullClaimPersisterFixture
	recipient types.Handle
	root      types.Handle
	grants    int
	rejects   int
}

func newSkullClaimFixture(t *testing.T) *skullClaimFixture {
	t.Helper()
	f := &skullClaimFixture{destructionServiceFixture: newDestructionServiceFixture(t)}
	previousObjects, previousItems := objectdefs.Global(), itemdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previousObjects); itemdefs.SetGlobalForTesting(previousItems) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 11, Key: "player"},
		{DefID: 99, Key: "player_dead", HP: 100, Resource: "player", IsStatic: true, Indestructible: true, BehaviorOrder: []string{"container", "player_dead"}},
		{DefID: 17, Key: "player_skeleton", HP: 100, Resource: "skeleton/with_skull", IsStatic: true, Indestructible: true, BehaviorOrder: []string{"lift", "player_skeleton"}},
		{DefID: 18, Key: "player_skeleton_without_skull", HP: 100, Resource: "skeleton/without_skull", IsStatic: true, Indestructible: true, BehaviorOrder: []string{"lift"}},
	}))
	items := itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 3014, Key: "skull", Resource: "items/skull.png", Name: "Skull", Size: itemdefs.Size{W: 1, H: 1}, DiscoveryLP: 50}})
	itemdefs.SetGlobalForTesting(items)
	f.service.deps.Items = items
	f.chunks = &corpseLootChunks{destructionChunksFixture: f.destructionServiceFixture.chunks}
	f.persister = &skullClaimPersisterFixture{destructionPersisterFixture: f.destructionServiceFixture.persister, metadata: inventory.DeadCharacterInfo{Nickname: "Alice <b>literal</b>", DeathDate: "2026-01-10"}}
	f.service.deps.Chunks, f.service.deps.Persister = f.chunks, f.persister
	executor := inventory.NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	f.shard = &Shard{world: f.w, objectDestruction: f.service, inventoryExecutor: executor, logger: zap.NewNop(), cfg: &config.Config{Game: config.GameConfig{Region: 1}}}
	f.service.deps.TransformCommitted = f.shard.completeCorpseDecay
	f.service.deps.SkullGranted = func(types.EntityID, types.Handle, *inventory.GiveItemResult) { f.grants++ }
	f.service.deps.SkullRejected = func(types.EntityID) { f.rejects++ }
	ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) {
		info.TypeID = 17
		info.Quality = 23
		info.Indestructible = true
		info.Behaviors = []string{"lift", "player_skeleton"}
	})
	ecs.AddComponent(f.w, f.target, components.Appearance{Resource: "skeleton/with_skull"})
	f.recipient = f.w.Spawn(500, nil)
	ecs.AddComponent(f.w, f.recipient, components.EntityInfo{TypeID: 11, Region: 1, Quality: 10})
	ecs.AddComponent(f.w, f.recipient, components.CharacterProfile{})
	ecs.AddComponent(f.w, f.recipient, components.EntityHealth{HHP: 20, SHP: 20})
	f.root = f.addContainer(500, 0)
	ecs.AddComponent(f.w, f.recipient, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: 500, Key: 0, Handle: f.root}}})
	ecs.GetResource[ecs.LinkState](f.w).SetLink(ecs.PlayerLink{PlayerID: 500, PlayerHandle: f.recipient, TargetID: 1, TargetHandle: f.target})
	return f
}

func (f *skullClaimFixture) admit(t *testing.T) {
	t.Helper()
	require.True(t, f.shard.takeSkull(f.w, 500, f.recipient, 1, f.target).OK)
	require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
	require.True(t, playerstate.ItemsLocked(f.w, f.recipient))
}

func (f *skullClaimFixture) persist(t *testing.T) {
	t.Helper()
	f.service.Update()
	f.chunks.runOne(t)
}

func (f *skullClaimFixture) requireCompleted(t *testing.T) {
	t.Helper()
	require.Zero(t, f.service.PendingCount())
	require.False(t, ecs.InventoryOwnerReserved(f.w, 500))
	require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.True(t, f.w.Alive(f.target))
	require.Equal(t, f.target, f.w.GetHandleByEntityID(1))
	info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.target)
	require.Equal(t, uint32(18), info.TypeID)
	require.Equal(t, uint32(23), info.Quality)
	require.Equal(t, []string{"lift"}, info.Behaviors)
	position, _ := ecs.GetComponent[components.Transform](f.w, f.target)
	require.Equal(t, float64(50), position.X)
	require.Equal(t, float64(50), position.Y)
	root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
	require.Len(t, root.Items, 1)
	require.Equal(t, types.EntityID(100000), root.Items[0].ItemID)
	require.Equal(t, uint32(23), root.Items[0].Quality)
	require.Equal(t, types.EntityID(1), root.Items[0].Skull.CharacterID)
	require.Equal(t, "Alice <b>literal</b> died on January 10, 2026", root.Items[0].Skull.HintExt())
	require.Equal(t, 1, f.grants)
	require.Zero(t, f.rejects)
	for _, pins := range f.chunks.pins {
		require.Zero(t, pins)
	}
}

func TestSkullClaimDurableTransitionAndRepeatedCommands(t *testing.T) {
	f := newSkullClaimFixture(t)
	f.admit(t)
	require.False(t, f.shard.takeSkull(f.w, 500, f.recipient, 1, f.target).OK)
	f.persist(t)
	root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
	require.Empty(t, root.Items, "worker must not mutate ECS before owning shard applies commit")
	require.Len(t, f.persister.records, 1)
	record := f.persister.records[0]
	var saved inventory.InventoryDataV1
	require.NoError(t, json.Unmarshal(record.PlayerInventories[0].Data, &saved))
	require.Len(t, saved.Items, 1)
	require.Equal(t, uint64(record.SkullItemID), saved.Items[0].ItemID)
	require.Equal(t, uint32(23), saved.Items[0].Quality)
	f.service.Update()
	f.requireCompleted(t)
	require.False(t, f.shard.takeSkull(f.w, 500, f.recipient, 1, f.target).OK)
	require.Equal(t, 1, f.ids.calls)
	state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	receipt, ok := components.GetBehaviorState[inventory.SkullClaimReceipt](state, "player_skeleton")
	require.True(t, ok)
	require.Equal(t, record.SkullItemID, receipt.SkullItemID)
}

func TestSkullClaimAcceptedGrantSurvivesKnockoutAndDeath(t *testing.T) {
	for _, afterCapture := range []bool{false, true} {
		for _, death := range []bool{false, true} {
			t.Run(map[bool]string{false: "KO", true: "death"}[death]+map[bool]string{false: "_before_capture", true: "_after_commit"}[afterCapture], func(t *testing.T) {
				f := newSkullClaimFixture(t)
				f.admit(t)
				if afterCapture {
					f.persist(t)
				}
				ecs.WithComponent(f.w, f.recipient, func(h *components.EntityHealth) { h.SHP = 0; h.IsLying = true })
				if death {
					f.shard.convertPlayerEntityToCorpse(f.w, 500, f.recipient)
				}
				require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
				if !afterCapture {
					f.persist(t)
				}
				f.service.Update()
				f.requireCompleted(t)
				info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.recipient)
				require.Equal(t, map[bool]uint32{false: 11, true: 99}[death], info.TypeID)
			})
		}
	}
}

func TestSkullClaimUnknownCommitReusesExactPayloadAndReservation(t *testing.T) {
	f := newSkullClaimFixture(t)
	f.admit(t)
	f.persister.commitErr = errors.New("connection lost after commit")
	f.persist(t)
	f.service.Update()
	require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
	root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
	require.Empty(t, root.Items)
	require.Equal(t, 1, f.ids.calls)
	ecs.GetResource[ecs.TimeState](f.w).Now = ecs.GetResource[ecs.TimeState](f.w).Now.Add(time.Second)
	f.persist(t)
	f.service.Update()
	f.requireCompleted(t)
	require.Len(t, f.persister.records, 2)
	require.Equal(t, f.persister.records[0], f.persister.records[1])
	require.Equal(t, 1, f.ids.calls)
}

func TestSkullClaimConfirmedRefusalRestoresSourceAndUnblocksInventory(t *testing.T) {
	for _, refusal := range []string{"inventory_conflict", "missing_character", "constraint", "invalid_capture"} {
		t.Run(refusal, func(t *testing.T) {
			f := newSkullClaimFixture(t)
			switch refusal {
			case "missing_character":
				f.persister.metadataErr = inventory.ErrSkullClaimCharacterMissing
			case "inventory_conflict":
				f.persister.commitErr = inventory.ErrSkullClaimInventoryConflict
			case "constraint":
				f.persister.commitErr = &pgconn.PgError{Code: "23514", Message: "check rejected"}
			case "invalid_capture":
				ecs.WithComponent(f.w, f.root, func(c *components.InventoryContainer) {
					c.Items = []components.InvItem{{ItemID: 10, TypeID: 999, Quality: 1, Quantity: 1, W: 1, H: 1}}
				})
			}
			before, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
			f.admit(t)
			f.persist(t)
			f.service.Update()
			require.Zero(t, f.service.PendingCount())
			require.False(t, ecs.InventoryOwnerReserved(f.w, 500))
			require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
			info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.target)
			require.Equal(t, uint32(17), info.TypeID)
			require.Equal(t, []string{"lift", "player_skeleton"}, info.Behaviors)
			root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
			require.Equal(t, before.Items, root.Items)
			require.Equal(t, 1, f.rejects)
			require.Zero(t, f.grants)
			for _, pins := range f.chunks.pins {
				require.Zero(t, pins)
			}
		})
	}
}

func TestSkullClaimBusyWorkerAndCacheKeepReservations(t *testing.T) {
	f := newSkullClaimFixture(t)
	f.chunks.accept = false
	f.admit(t)
	f.service.Update()
	require.Empty(t, f.chunks.jobs)
	require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
	f.chunks.accept = true
	f.persist(t)
	f.chunks.replaceErr = errors.New("cache busy")
	f.service.Update()
	require.Equal(t, 1, f.service.PendingCount())
	require.Zero(t, f.grants)
	f.chunks.replaceErr = nil
	f.service.Update()
	f.requireCompleted(t)
}

func TestSkullClaimFinalizationRetryDoesNotApplyInventoryTwice(t *testing.T) {
	f := newSkullClaimFixture(t)
	f.admit(t)
	f.persist(t)
	complete := f.service.deps.TransformCommitted
	f.service.deps.TransformCommitted = func(types.Handle, *objectdefs.ObjectDef) bool { return false }
	f.service.Update()
	require.Equal(t, 1, f.grants)
	require.Equal(t, 1, f.service.PendingCount())
	require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
	f.service.deps.TransformCommitted = complete
	f.service.Update()
	f.requireCompleted(t)
}

func TestSkullClaimTwoContendersAndDisconnectedRecipient(t *testing.T) {
	f := newSkullClaimFixture(t)
	other := f.w.Spawn(600, nil)
	otherRoot := f.addContainer(600, 0)
	ecs.AddComponent(f.w, other, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: 600, Handle: otherRoot}}})
	ecs.GetResource[ecs.LinkState](f.w).SetLink(ecs.PlayerLink{PlayerID: 600, PlayerHandle: other, TargetID: 1, TargetHandle: f.target})
	f.admit(t)
	require.False(t, f.shard.takeSkull(f.w, 600, other, 1, f.target).OK)
	require.False(t, ecs.InventoryOwnerReserved(f.w, 600))
	clock := ecs.GetResource[ecs.TimeState](f.w)
	detached := ecs.GetResource[ecs.DetachedEntities](f.w)
	detached.AddDetachedEntity(500, f.recipient, clock.Now, clock.Now)
	expiry := systems.NewExpireDetachedSystem(zap.NewNop(), nil, nil, nil)
	expiry.Update(f.w, 0)
	require.True(t, f.w.Alive(f.recipient), "accepted owner must survive detached expiry until completion")
	f.persist(t)
	f.service.Update()
	f.requireCompleted(t)
	root, _ := ecs.GetComponent[components.InventoryContainer](f.w, otherRoot)
	require.Empty(t, root.Items)
	require.False(t, f.shard.takeSkull(f.w, 600, other, 1, f.target).OK)
}

func TestSkullClaimRejectsFullQueueNoSpaceAndStaleLink(t *testing.T) {
	for _, reason := range []string{"queue", "space", "stale_link"} {
		t.Run(reason, func(t *testing.T) {
			f := newSkullClaimFixture(t)
			switch reason {
			case "queue":
				f.service.freeCount = 0
			case "space":
				ecs.WithComponent(f.w, f.root, func(c *components.InventoryContainer) { c.Width = 0; c.Height = 0 })
			case "stale_link":
				ecs.GetResource[ecs.LinkState](f.w).RemoveLink(500)
			}
			result := f.shard.takeSkull(f.w, 500, f.recipient, 1, f.target)
			require.False(t, result.OK)
			require.True(t, result.UserVisible)
			require.False(t, ecs.InventoryOwnerReserved(f.w, 500))
			require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
			require.Zero(t, f.ids.calls)
			require.Empty(t, f.persister.records)
		})
	}
}
