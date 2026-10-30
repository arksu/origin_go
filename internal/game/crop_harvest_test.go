package game

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/entitystats"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type cropHarvestPersisterFixture struct {
	records []inventory.CropHarvestPersistenceRecord
	err     error
}

func (p *cropHarvestPersisterFixture) PersistCropHarvest(_ context.Context, record inventory.CropHarvestPersistenceRecord) error {
	p.records = append(p.records, record)
	err := p.err
	p.err = nil
	return err
}

type cropHarvestFixture struct {
	*destructionServiceFixture
	harvest         *cropHarvestService
	persist         *cropHarvestPersisterFixture
	player, root    types.Handle
	grants, rejects int
}

func newCropHarvestFixture(t *testing.T) *cropHarvestFixture {
	t.Helper()
	f := &cropHarvestFixture{destructionServiceFixture: newDestructionServiceFixture(t), persist: &cropHarvestPersisterFixture{}}
	oldItems, oldObjects := itemdefs.Global(), objectdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(oldItems); objectdefs.SetGlobalForTesting(oldObjects) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 201, Key: "carrot", Size: itemdefs.Size{W: 1, H: 1}, Resource: "carrot", DiscoveryLP: 9},
		{DefID: 202, Key: "seed_carrot", Size: itemdefs.Size{W: 1, H: 1}, Resource: "seed"},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 42, Key: "crop_carrot", HP: 100, IsStatic: true, BehaviorOrder: []string{"crop"}, Behaviors: map[string]json.RawMessage{"crop": json.RawMessage(`{}`)}, CropConfig: &objectdefs.CropBehaviorConfig{
		HarvestCycleDurationTicks: 10, HarvestStaminaCost: 10,
		Stages: []contracts.CropStageConfig{{DurationSeconds: 4}, {DurationSeconds: 4}, {DurationSeconds: 8, Harvest: []contracts.CropHarvestProduct{{ItemKey: "carrot", Min: 1, Max: 3}}},
			{Harvest: []contracts.CropHarvestProduct{{ItemKey: "carrot", Min: 1, Max: 3}, {ItemKey: "seed_carrot", Min: 1, Max: 2}}}},
	}}}))
	ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) {
		info.TypeID = 42
		info.Quality = 4000000000
		info.Behaviors = []string{"crop"}
	})
	ecs.WithComponent(f.w, f.target, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, "crop", &components.CropBehaviorState{Stage: 2, NextGrowthRuntimeSeconds: 20})
	})
	f.player = f.w.Spawn(500, nil)
	ecs.AddComponent(f.w, f.player, components.EntityStats{Stamina: 10})
	ecs.AddComponent(f.w, f.player, components.CharacterProfile{})
	f.root = f.addContainer(500, 0)
	ecs.AddComponent(f.w, f.player, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: 500, Handle: f.root}}})
	ecs.AddComponent(f.w, f.player, components.ActiveCyclicAction{BehaviorKey: "crop", ActionID: "harvest", MutatesItems: true, TargetKind: components.CyclicActionTargetObject, TargetID: 1, TargetHandle: f.target, CycleDurationTicks: 10, CycleElapsedTicks: 10})
	ecs.GetResource[ecs.LinkState](f.w).SetLink(ecs.PlayerLink{PlayerID: 500, PlayerHandle: f.player, TargetID: 1, TargetHandle: f.target})
	var err error
	f.harvest, err = newCropHarvestService(f.w, cropHarvestDependencies{chunks: f.chunks, persister: f.persist, ids: f.ids, executor: inventory.NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil),
		withWorldRead: func(fn func(*ecs.World)) { fn(f.w) }, quarantine: func(h types.Handle) { ecs.RemoveComponent[components.Collider](f.w, h) },
		granted: func(types.EntityID, types.Handle, *inventory.GiveItemResult) { f.grants++ }, rejected: func(types.EntityID) { f.rejects++ }, roll: func(n int) int { return n - 1 }})
	require.NoError(t, err)
	return f
}

func (f *cropHarvestFixture) admit(t *testing.T) {
	t.Helper()
	result := f.harvest.harvest(500, f.player, 1, f.target)
	require.True(t, result.OK, result.ReasonCode)
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Zero(t, stats.Stamina)
}

func TestCropHarvestDurableAcceptanceAndFinalTickStageBoundary(t *testing.T) {
	for _, boundary := range []bool{false, true} {
		t.Run(map[bool]string{false: "roots_only", true: "roots_and_seeds"}[boundary], func(t *testing.T) {
			f := newCropHarvestFixture(t)
			if boundary {
				ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 20
			}
			f.admit(t)
			require.True(t, f.w.Alive(f.target))
			require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
			require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
			require.False(t, f.harvest.harvest(500, f.player, 1, f.target).OK)
			// A canceled cycle and broken/disconnected link cannot revoke an
			// operation whose payment and immutable identity were accepted.
			ecs.RemoveComponent[components.ActiveCyclicAction](f.w, f.player)
			_, _, _ = ecs.BreakLinkForPlayer(f.w, 500, ecs.LinkBreakDespawn)
			f.chunks.runOne(t)
			before, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
			require.Empty(t, before.Items)
			f.harvest.Update()
			require.False(t, f.w.Alive(f.target))
			require.False(t, ecs.InventoryOwnerReserved(f.w, 500))
			require.Zero(t, f.harvest.count)
			after, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
			expected := 3
			if boundary {
				expected = 5
			}
			require.Len(t, after.Items, expected)
			for i, item := range after.Items {
				require.Equal(t, uint32(4000000000), item.Quality)
				require.Equal(t, types.EntityID(100000+i), item.ItemID)
			}
			require.Equal(t, 1, f.ids.calls)
			require.Equal(t, 1, f.grants)
			require.Zero(t, f.rejects)
		})
	}
}

func TestCropHarvestFullInventoryDropsEveryUnitAtSourceCenter(t *testing.T) {
	f := newCropHarvestFixture(t)
	ecs.WithComponent(f.w, f.root, func(c *components.InventoryContainer) {
		c.Width = 1
		c.Height = 1
		c.Items = []components.InvItem{{ItemID: 7000, TypeID: 201, Quality: 1, Quantity: 1, W: 1, H: 1}}
	})
	f.admit(t)
	f.chunks.runOne(t)
	require.Empty(t, f.persist.records[0].PlayerInventories)
	f.harvest.Update()
	require.Len(t, f.chunks.inserted, 3)
	for _, drop := range f.chunks.inserted {
		require.Equal(t, 50, drop.object.X)
		require.Equal(t, 50, drop.object.Y)
		var data map[string]any
		require.NoError(t, json.Unmarshal(drop.object.Data.RawMessage, &data))
		require.Equal(t, float64(500), data["dropper_id"])
		require.Equal(t, float64(10), data["drop_time"])
		var items inventory.InventoryDataV1
		require.NoError(t, json.Unmarshal(drop.inventory.Data, &items))
		require.Len(t, items.Items, 1)
		require.Equal(t, uint32(1), items.Items[0].Quantity)
		require.Equal(t, uint32(4000000000), items.Items[0].Quality)
	}
}

func TestCropHarvestRejectedAdmissionNeverCharges(t *testing.T) {
	for _, scenario := range []string{"queue", "pin", "low_stamina", "unfinished", "wrong_cycle", "bad_link", "stale_target", "malformed_state", "ids", "shutdown"} {
		t.Run(scenario, func(t *testing.T) {
			f := newCropHarvestFixture(t)
			target := f.target
			switch scenario {
			case "queue":
				f.chunks.accept = false
			case "pin":
				f.chunks.pinErr = errors.New("busy")
			case "low_stamina":
				ecs.WithComponent(f.w, f.player, func(s *components.EntityStats) { s.Stamina = 9 })
			case "unfinished":
				ecs.WithComponent(f.w, f.player, func(c *components.ActiveCyclicAction) { c.CycleElapsedTicks = 9 })
			case "wrong_cycle":
				ecs.WithComponent(f.w, f.player, func(c *components.ActiveCyclicAction) { c.CycleDurationTicks = 1 })
			case "bad_link":
				_, _, _ = ecs.BreakLinkForPlayer(f.w, 500, ecs.LinkBreakDespawn)
			case "stale_target":
				f.w.Despawn(target)
				f.w.Spawn(1, nil)
			case "malformed_state":
				ecs.WithComponent(f.w, f.target, func(state *components.ObjectInternalState) {
					components.SetBehaviorState(state, "crop", &components.CropBehaviorState{Stage: 9})
				})
			case "ids":
				f.ids.invalid = true
			case "shutdown":
				f.harvest.StopAdmission()
			}
			before, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
			require.False(t, f.harvest.harvest(500, f.player, 1, target).OK)
			after, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
			require.Equal(t, before.Stamina, after.Stamina)
			require.Zero(t, f.harvest.count)
			require.False(t, ecs.InventoryOwnerReserved(f.w, 500))
			for _, pins := range f.chunks.pins {
				require.Zero(t, pins)
			}
		})
	}
}

func TestCropHarvestTwoContendersAndDetachedExpiry(t *testing.T) {
	f := newCropHarvestFixture(t)
	other := f.w.Spawn(600, nil)
	root := f.addContainer(600, 0)
	ecs.AddComponent(f.w, other, components.EntityStats{Stamina: 10})
	ecs.AddComponent(f.w, other, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryGrid, OwnerID: 600, Handle: root}}})
	cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](f.w, f.player)
	ecs.AddComponent(f.w, other, cycle)
	ecs.GetResource[ecs.LinkState](f.w).SetLink(ecs.PlayerLink{PlayerID: 600, PlayerHandle: other, TargetID: 1, TargetHandle: f.target})
	f.admit(t)
	require.False(t, f.harvest.harvest(600, other, 1, f.target).OK)
	require.False(t, ecs.InventoryOwnerReserved(f.w, 600))
	clock := ecs.GetResource[ecs.TimeState](f.w)
	ecs.GetResource[ecs.DetachedEntities](f.w).AddDetachedEntity(500, f.player, clock.Now, clock.Now)
	systems.NewExpireDetachedSystem(zap.NewNop(), nil, nil, nil).Update(f.w, 0)
	require.True(t, f.w.Alive(f.player), "an accepted recipient survives disconnect expiry while its inventory is reserved")
	f.chunks.runOne(t)
	f.harvest.Update()
	require.Zero(t, f.harvest.count)
	container, _ := ecs.GetComponent[components.InventoryContainer](f.w, root)
	require.Empty(t, container.Items)
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, other)
	require.Equal(t, float64(10), stats.Stamina)
}

func TestCropHarvestShutdownDrainsAcceptedWorkerWithoutHoldingShardLock(t *testing.T) {
	f := newCropHarvestFixture(t)
	shard := &Shard{world: f.w, cropHarvest: f.harvest, logger: zap.NewNop()}
	f.harvest.deps.withWorldRead = shard.WithWorldRead
	f.admit(t)
	job := f.chunks.jobs[0]
	f.chunks.jobs = nil
	done := make(chan struct{})
	go func() { job.Complete(job.Run()); close(done) }()
	shard.drainCropHarvest()
	<-done
	require.True(t, f.harvest.closed)
	require.False(t, shard.pendingCropHarvest())
	require.False(t, f.w.Alive(f.target))
	require.False(t, ecs.InventoryOwnerReserved(f.w, 500))
	for _, pins := range f.chunks.pins {
		require.Zero(t, pins)
	}
}

func TestCropHarvestUncertainCommitRetriesExactPayloadWithoutRecharge(t *testing.T) {
	f := newCropHarvestFixture(t)
	f.admit(t)
	f.persist.err = errors.New("commit acknowledgement lost")
	f.chunks.runOne(t)
	f.harvest.Update()
	require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
	require.True(t, f.w.Alive(f.target))
	require.Zero(t, f.grants)
	ecs.GetResource[ecs.TimeState](f.w).Now = ecs.GetResource[ecs.TimeState](f.w).Now.Add(time.Second)
	f.harvest.Update()
	f.chunks.runOne(t)
	f.harvest.Update()
	require.Zero(t, f.harvest.count)
	require.Equal(t, 1, f.ids.calls)
	require.Len(t, f.persist.records, 2)
	require.Equal(t, f.persist.records[0], f.persist.records[1])
	require.Equal(t, 1, f.grants)
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Zero(t, stats.Stamina)
}

func TestCropHarvestDefiniteRollbackRefundsAndRearmsGrowth(t *testing.T) {
	for _, failure := range []error{inventory.ErrCropHarvestInventoryConflict, &pgconn.PgError{Code: "23514"}, inventory.ErrInvalidObjectLootCapture} {
		f := newCropHarvestFixture(t)
		f.admit(t)
		f.persist.err = failure
		f.chunks.runOne(t)
		f.harvest.Update()
		require.Zero(t, f.harvest.count)
		require.True(t, f.w.Alive(f.target))
		require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
		require.False(t, ecs.InventoryOwnerReserved(f.w, 500))
		stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
		require.Equal(t, float64(10), stats.Stamina)
		require.Equal(t, 1, f.rejects)
		require.Zero(t, f.grants)
		require.Equal(t, 2, behaviors.CropState(f.w, f.target).Stage)
		require.Positive(t, ecs.EnsureBehaviorRuntimeSchedule(f.w).PendingCount())
		for _, pins := range f.chunks.pins {
			require.Zero(t, pins)
		}
	}
}

func TestCropHarvestExactCostSchedulesRegenAndRefundClampsAtCurrentMax(t *testing.T) {
	f := newCropHarvestFixture(t)
	maximum := cropHarvestMaxStamina(f.w, f.player)
	ecs.WithComponent(f.w, f.player, func(stats *components.EntityStats) { stats.Stamina, stats.Energy = maximum, 1000 })
	updates := ecs.GetResource[ecs.EntityStatsUpdateState](f.w)
	require.Zero(t, updates.PendingRegenCount())
	require.True(t, f.harvest.harvest(500, f.player, 1, f.target).OK)
	require.Equal(t, 1, updates.PendingRegenCount(), "harvesting from full stamina must start regeneration")
	clock := ecs.GetResource[ecs.TimeState](f.w)
	clock.Tick += ecs.ResolveStaminaRegenIntervalTicks(f.w)
	systems.NewEntityStatsRegenSystem().Update(f.w, 0)
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Greater(t, stats.Stamina, maximum-10)
	// Model additional regeneration while the durable operation is unresolved.
	ecs.WithComponent(f.w, f.player, func(stats *components.EntityStats) { stats.Stamina = maximum })
	f.persist.err = inventory.ErrCropHarvestInventoryConflict
	f.chunks.runOne(t)
	f.harvest.Update()
	stats, _ = ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Equal(t, entitystats.ClampStamina(maximum+10, maximum), stats.Stamina)
	require.Zero(t, updates.PendingRegenCount(), "a refunded full balance must retire its old regen entry")
}
