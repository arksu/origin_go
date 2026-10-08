package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var errMeleePostgresBatch = errors.New("test: reject second source after its first SQL batch")

type meleePostgresRetryPersister struct {
	base     ObjectDestructionPersister
	fail     atomic.Bool
	mu       sync.Mutex
	attempts map[types.EntityID][][]inventory.DroppedItemPersistenceRecord
}

func (p *meleePostgresRetryPersister) ReplaceObjectWithDroppedItems(ctx context.Context, region int, source, maxID types.EntityID,
	next func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error),
) error {
	var records []inventory.DroppedItemPersistenceRecord
	started := false
	err := p.base.ReplaceObjectWithDroppedItems(ctx, region, source, maxID, func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
		if started && source == 200 && p.fail.Load() {
			return nil, errMeleePostgresBatch
		}
		batch, err := next(dst)
		for _, record := range batch {
			record.ObjectData = append(json.RawMessage(nil), record.ObjectData...)
			record.InventoryData = append(json.RawMessage(nil), record.InventoryData...)
			records = append(records, record)
		}
		started = true
		return batch, err
	})
	p.mu.Lock()
	p.attempts[source] = append(p.attempts[source], records)
	p.mu.Unlock()
	return err
}

func (p *meleePostgresRetryPersister) attemptsFor(source types.EntityID) [][]inventory.DroppedItemPersistenceRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Attempts and their copied payloads are immutable once appended by the worker.
	return append([][]inventory.DroppedItemPersistenceRecord(nil), p.attempts[source]...)
}

// A sweep commits both zero-HP intents once. The existing persistence workers
// can finish one replacement while another transaction rolls back and retries.
func TestMeleePostgresSweepDestructionRetryAndRestart(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 128, 50)
	ctx := context.Background()
	secondRaw := repository.Object{ID: 200, TypeID: 99, Region: 1, X: 58, Y: 50, Quality: 31, Hp: sql.NullFloat64{Float64: 100, Valid: true}}
	require.NoError(t, f.db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
		ID: secondRaw.ID, TypeID: secondRaw.TypeID, Region: secondRaw.Region, X: secondRaw.X, Y: secondRaw.Y,
		Quality: secondRaw.Quality, Hp: secondRaw.Hp,
	}))
	secondData, err := json.Marshal(inventory.InventoryDataV1{
		Kind: uint8(constt.InventoryGrid), Width: 4, Height: 4, Version: 1,
		Items: []inventory.InventoryItemV1{{ItemID: 220, TypeID: 1, Quality: 31, Quantity: 2}},
	})
	require.NoError(t, err)
	secondRoot, err := f.db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{
		OwnerID: 200, Kind: int16(constt.InventoryGrid), Data: secondData, Version: 1,
	})
	require.NoError(t, err)
	persister := &meleePostgresRetryPersister{base: f.shard.objectDestruction.deps.Persister, attempts: make(map[types.EntityID][][]inventory.DroppedItemPersistenceRecord)}
	persister.fail.Store(true)
	sender := &meleeTestSender{}
	var actor, second types.Handle
	var secondOwned []ecs.InventoryRefEntry
	var cyclic *CyclicActionSystem
	func() {
		f.shard.mu.Lock()
		defer f.shard.mu.Unlock()
		w := f.shard.world
		definitions := make([]itemdefs.ItemDef, 0, 3)
		for _, definition := range itemdefs.Global().All() {
			definitions = append(definitions, *definition)
		}
		definitions = append(definitions, itemdefs.ItemDef{
			DefID: 3, Key: "melee_test_weapon", Tags: []string{"axe"}, Size: itemdefs.Size{W: 1, H: 1},
			Allowed: itemdefs.Allowed{EquipmentSlots: []string{"right_hand", "left_hand"}}, Melee: &itemdefs.MeleeDef{BaseDamage: 200},
		})
		items := itemdefs.NewRegistry(definitions)
		itemdefs.SetGlobalForTesting(items) // The integration fixture restores the original registry.
		f.shard.objectDestruction.deps.Items, f.shard.objectDestruction.deps.Persister = items, persister
		core.AttachColliderSpatial(w)
		second, err = f.cm.ObjectFactory().Build(w, &secondRaw, []repository.Inventory{secondRoot})
		require.NoError(t, err)
		ecs.AddComponent(w, second, components.ChunkRef{})
		chunk := f.cm.GetChunkFast(types.ChunkCoord{})
		chunk.Spatial().AddStatic(second, 58, 50)
		chunk.UpsertRawObject(&secondRaw)
		chunk.SetRawInventoriesForOwner(200, []repository.Inventory{secondRoot})
		require.NoError(t, f.shard.objectDamage.PrepareTarget(second))
		index := ecs.GetResource[ecs.InventoryRefIndex](w)
		secondOwned = index.EntriesByOwnerInto(200, nil)
		actor = w.Spawn(500, nil)
		ecs.AddComponent(w, actor, components.Transform{X: 40, Y: 50})
		ecs.AddComponent(w, actor, components.Collider{HalfWidth: 1, HalfHeight: 1})
		ecs.AddComponent(w, actor, components.EntityHealth{SHP: 25, HHP: 25})
		ecs.AddComponent(w, actor, components.EntityStats{Stamina: 1000, Energy: 900})
		ecs.AddComponent(w, actor, components.CharacterProfile{Attributes: characterattrs.Default()})
		equipment := w.SpawnWithoutExternalID()
		ecs.AddComponent(w, equipment, components.InventoryContainer{OwnerID: 500, Kind: constt.InventoryEquipment,
			Items: []components.InvItem{combatTestItem(5000, 3, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND)},
		})
		index.Add(constt.InventoryEquipment, 500, 0, equipment)
		actions, loadErr := actiondefs.LoadFromDirectory("../../data/actions", zap.NewNop())
		require.NoError(t, loadErr)
		combat, prepareErr := NewCombatDefinitions(items, actions)
		require.NoError(t, prepareErr)
		resolver, resolverErr := NewCombatEquipmentResolver(w, combat.catalog)
		require.NoError(t, resolverErr)
		f.shard.creatureDamage, err = NewCreatureDamageService(w, resolver)
		require.NoError(t, err)
		require.NoError(t, f.shard.creatureDamage.PrepareTarget(actor))
		sectors, sectorErr := NewSectorResolver(w)
		require.NoError(t, sectorErr)
		f.shard.meleeExecution, err = NewMeleeExecutionService(w, resolver, sectors, f.shard.creatureDamage, f.shard.objectDamage, &AttackEventSequence{}, sender)
		require.NoError(t, err)
		definition, found := actions.Get("axe_sweep")
		require.True(t, found)
		handler, handlerErr := NewMeleeActionHandler(f.shard.meleeExecution, definition, combat.actions[definition.ID])
		require.NoError(t, handlerErr)
		f.shard.actionService, err = NewActionService(w, actions, map[string]ActionHandler{"axe_sweep": handler}, sender)
		require.NoError(t, err)
		cyclic = NewCyclicActionSystem(nil, sender, zap.NewNop())
		cyclic.SetActionService(f.shard.actionService)
		aim := float32(0)
		f.shard.actionService.ActivateRequest(w, 500, actor, &netproto.C2S_ActivateAction{ActionId: "axe_sweep", AimAngle: &aim, StreamEpoch: 1})
		for tick := 0; tick < 6; tick++ {
			clock := ecs.GetResource[ecs.TimeState](w)
			clock.Tick++
			clock.UnixMs += 100
			cyclic.Update(w, .1)
		}
		require.Len(t, sender.attacks, 1)
		require.Equal(t, []meleeHitSample{{id: 1, damage: 200}, {id: 200, damage: 200}}, sender.attacks[0].hits)
		require.Equal(t, 2, f.shard.objectDestruction.PendingCount())
		for _, target := range []types.Handle{f.target, second} {
			health, ok := ecs.GetComponent[components.ObjectInternalState](w, target)
			require.True(t, ok)
			require.Zero(t, health.HP)
			require.True(t, ecs.ObjectDestructionPending(w, target))
			require.False(t, ecs.GetOrCreateStorage[components.Collider](w).Has(target))
		}
		stats, _ := ecs.GetComponent[components.EntityStats](w, actor)
		require.Equal(t, 940.0, stats.Stamina)
		require.Len(t, f.shard.actionService.State(w, actor).Cooldowns, 1)
		cyclic.Update(w, .1)
		require.Len(t, sender.attacks, 1, "repeated completion must not replay the sweep")
	}()

	f.eventually(t, func() bool {
		return f.shard.objectDestruction.PendingCount() == 1 && len(persister.attemptsFor(200)) > 0
	})
	_, err = f.db.Queries().GetObjectByID(ctx, 1)
	require.ErrorIs(t, err, sql.ErrNoRows)
	retained, err := f.db.Queries().GetObjectByID(ctx, 200)
	require.NoError(t, err)
	require.Equal(t, 100.0, retained.Hp.Float64, "failed replacement must retain its previous durable source")
	require.NotEmpty(t, f.logs.FilterMessage("Object destruction persistence failed").All())
	firstAttempts, failedAttempts := persister.attemptsFor(1), persister.attemptsFor(200)
	require.Len(t, firstAttempts, 1)
	require.Len(t, firstAttempts[0], 4)
	require.Len(t, failedAttempts[0], 2)
	var firstDrops []repository.Object
	for _, record := range firstAttempts[0] {
		drop, readErr := f.db.Queries().GetObjectByID(ctx, int64(record.EntityID))
		require.NoError(t, readErr)
		firstDrops = append(firstDrops, drop)
	}
	f.assertMaterialized(t, firstDrops)
	for _, record := range failedAttempts[0] {
		_, err = f.db.Queries().GetObjectByID(ctx, int64(record.EntityID))
		require.ErrorIs(t, err, sql.ErrNoRows, "first SQL batch must roll back completely")
		rows, readErr := f.db.Queries().GetInventoriesByOwner(ctx, int64(record.EntityID))
		require.NoError(t, readErr)
		require.Empty(t, rows)
	}
	f.shard.WithWorldRead(func(w *ecs.World) {
		require.True(t, w.Alive(second))
		require.True(t, ecs.ObjectDestructionPending(w, second))
		for _, ref := range secondOwned {
			require.True(t, w.Alive(ref.Handle), "failed replacement must retain all of its owned inventories")
		}
		for _, record := range failedAttempts[0] {
			require.Equal(t, types.InvalidHandle, w.GetHandleByEntityID(record.EntityID), "failed drops must not materialize")
		}
	})
	persister.fail.Store(false)
	f.eventually(t, func() bool { return f.shard.objectDestruction.PendingCount() == 0 })
	attempts := persister.attemptsFor(200)
	require.GreaterOrEqual(t, len(attempts), 2)
	for _, attempt := range attempts[1:] {
		require.Equal(t, attempts[0], attempt, "retry must preserve IDs, coordinates, drop time and every payload")
	}
	var drops []repository.Object
	for chunkX := 0; chunkX < 2; chunkX++ {
		rows, readErr := f.db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{Region: 1, ChunkX: chunkX})
		require.NoError(t, readErr)
		drops = append(drops, rows...)
	}
	require.Len(t, drops, 6)
	byID := make(map[types.EntityID]repository.Object)
	for _, drop := range drops {
		require.Equal(t, constt.DroppedItemTypeID, drop.TypeID)
		byID[types.EntityID(drop.ID)] = drop
	}
	require.Len(t, byID, 6)
	mark, err := f.db.Queries().GetGlobalVar(ctx, constt.LAST_USED_ID)
	require.NoError(t, err)
	require.True(t, mark.ValueLong.Valid)
	for _, drop := range drops {
		require.LessOrEqual(t, drop.ID, mark.ValueLong.Int64, "durable ID watermark must cover every published drop")
	}
	for _, originalID := range []types.EntityID{20, 30, 220} {
		require.Contains(t, byID, originalID, "the first unit must preserve its original item ID")
	}
	for source, sourceAttempts := range map[types.EntityID][][]inventory.DroppedItemPersistenceRecord{1: firstAttempts, 200: attempts} {
		for _, record := range sourceAttempts[0] {
			drop, found := byID[record.EntityID]
			require.True(t, found)
			require.Equal(t, record.X, drop.X)
			require.Equal(t, record.Y, drop.Y)
			require.JSONEq(t, string(record.ObjectData), string(drop.Data.RawMessage))
			var metadata gameworld.DroppedItemData
			require.NoError(t, json.Unmarshal(drop.Data.RawMessage, &metadata))
			require.EqualValues(t, 10, metadata.DropTime)
			originX := 50
			if source == 200 {
				originX = 58
			}
			require.InDelta(t, originX, drop.X, float64(constt.DestroyedObjectDropSpread))
			require.InDelta(t, 50, drop.Y, float64(constt.DestroyedObjectDropSpread))
			rows, readErr := f.db.Queries().GetInventoriesByOwner(ctx, drop.ID)
			require.NoError(t, readErr)
			require.Len(t, rows, 1)
			require.JSONEq(t, string(record.InventoryData), string(rows[0].Data))
			var root inventory.InventoryDataV1
			require.NoError(t, json.Unmarshal(rows[0].Data, &root))
			require.Len(t, root.Items, 1)
			item := root.Items[0]
			require.EqualValues(t, record.EntityID, item.ItemID)
			require.EqualValues(t, 1, item.Quantity)
			quality := uint32(17)
			if source == 200 {
				quality = 31
			}
			require.Equal(t, quality, item.Quality)
			if record.EntityID == 30 {
				require.EqualValues(t, 2, item.TypeID)
				require.NotNil(t, item.NestedInventory)
				require.Len(t, item.NestedInventory.Items, 1)
				require.EqualValues(t, 900, item.NestedInventory.Items[0].ItemID)
				require.EqualValues(t, 2, item.NestedInventory.Items[0].Quantity)
				require.EqualValues(t, 23, item.NestedInventory.Items[0].Quality)
			} else {
				require.EqualValues(t, 1, item.TypeID)
				require.Nil(t, item.NestedInventory)
			}
		}
		_, readErr := f.db.Queries().GetObjectByID(ctx, int64(source))
		require.ErrorIs(t, readErr, sql.ErrNoRows)
		rows, readErr := f.db.Queries().GetInventoriesByOwner(ctx, int64(source))
		require.NoError(t, readErr)
		require.Empty(t, rows)
	}
	f.assertMaterialized(t, drops)
	f.shard.WithWorldRead(func(w *ecs.World) {
		require.False(t, w.Alive(second))
		for _, ref := range secondOwned {
			require.False(t, w.Alive(ref.Handle))
		}
		stats, _ := ecs.GetComponent[components.EntityStats](w, actor)
		require.Equal(t, 940.0, stats.Stamina, "persistence retries must not charge the action again")
		require.Len(t, sender.attacks, 1)
	})
	f.stop()

	w := ecs.NewWorldWithCapacity(128, nil, 0)
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Now: time.Now(), RuntimeSecondsTotal: 10}
	shard := &Shard{world: w, layer: 0, logger: f.shard.logger}
	cm := gameworld.NewChunkManager(f.cfg, f.db, w, shard, 0, 1, gameworld.NewObjectFactory(nil), nil, nil, shard.logger)
	shard.chunkManager = cm
	restarted := &objectDestructionIntegrationFixture{shard: shard, db: f.db, cm: cm, cfg: f.cfg}
	t.Cleanup(restarted.stop)
	cm.RegisterEntity(9000, 50, 50, false)
	loadCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	require.NoError(t, cm.WaitPreloaded(loadCtx, types.ChunkCoord{}))
	restarted.assertMaterialized(t, drops)
	require.Equal(t, types.InvalidHandle, w.GetHandleByEntityID(1))
	require.Equal(t, types.InvalidHandle, w.GetHandleByEntityID(200))
	require.Equal(t, 13, w.EntityCount(), "six drops, six roots and one retained bag inventory")
	for i := 0; i < 3; i++ {
		restarted.tick()
	}
	require.Equal(t, 13, w.EntityCount(), "restart and activation retries must not duplicate loot")
}
