package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type destructionChunksFixture struct {
	jobs      []gameworld.PersistenceJob
	accept    bool
	pins      map[types.ChunkCoord]int
	inserted  []destructionDroppedRecord
	removed   []types.EntityID
	insertErr error
}

func (c *destructionChunksFixture) SubmitPersistenceJob(job gameworld.PersistenceJob) bool {
	if !c.accept {
		return false
	}
	c.jobs = append(c.jobs, job)
	return true
}
func (c *destructionChunksFixture) PinPersistence(coord types.ChunkCoord) error {
	c.pins[coord]++
	return nil
}
func (c *destructionChunksFixture) UnpinPersistence(coord types.ChunkCoord) { c.pins[coord]-- }
func (c *destructionChunksFixture) WithPersistence(_ []types.ChunkCoord, fn func() error) error {
	return fn()
}
func (c *destructionChunksFixture) InsertCommittedDropped(raw *repository.Object, root *repository.Inventory) error {
	if c.insertErr != nil {
		return c.insertErr
	}
	c.inserted = append(c.inserted, destructionDroppedRecord{object: raw, inventory: root})
	return nil
}
func (c *destructionChunksFixture) RemoveCommittedSource(_ types.ChunkCoord, id types.EntityID) {
	c.removed = append(c.removed, id)
}
func (c *destructionChunksFixture) runOne(t testing.TB) {
	t.Helper()
	require.NotEmpty(t, c.jobs)
	job := c.jobs[0]
	c.jobs = c.jobs[1:]
	job.Complete(job.Run())
}

type destructionPersisterFixture struct {
	calls [][]inventory.DroppedItemPersistenceRecord
	err   error
	maxID types.EntityID
}

func (p *destructionPersisterFixture) ReplaceObjectWithDroppedItems(_ context.Context, _ int, _ types.EntityID, maxID types.EntityID, next func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error)) error {
	var all, dst []inventory.DroppedItemPersistenceRecord
	for {
		var err error
		dst, err = next(dst)
		if err != nil {
			return err
		}
		if len(dst) == 0 {
			break
		}
		if len(dst) > 100 {
			panic("unbounded persistence batch")
		}
		all = append(all, dst...)
	}
	p.calls = append(p.calls, all)
	p.maxID = maxID
	err := p.err
	p.err = nil
	return err
}

type destructionIDsFixture struct {
	next    types.EntityID
	calls   int
	invalid bool
}

func (a *destructionIDsFixture) ReserveIDs(count uint64) (types.EntityID, types.EntityID, error) {
	a.calls++
	if a.invalid {
		return 0, 0, nil
	}
	first := a.next
	a.next += types.EntityID(count)
	return first, a.next - 1, nil
}

type destructionServiceFixture struct {
	w         *ecs.World
	chunks    *destructionChunksFixture
	persister *destructionPersisterFixture
	ids       *destructionIDsFixture
	service   *ObjectDestructionService
	damage    *ObjectDamageService
	target    types.Handle
}

func newDestructionServiceFixture(t testing.TB) *destructionServiceFixture {
	return newDestructionServiceFixtureWithCapacity(t, 4096)
}

func newDestructionServiceFixtureWithCapacity(t testing.TB, capacity uint32) *destructionServiceFixture {
	t.Helper()
	f := &destructionServiceFixture{w: ecs.NewWorldWithCapacity(capacity, nil, 0), chunks: &destructionChunksFixture{accept: true, pins: make(map[types.ChunkCoord]int)}, persister: &destructionPersisterFixture{}, ids: &destructionIDsFixture{next: 100000}}
	registry := itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1, Key: "ore", Resource: "ore", Size: itemdefs.Size{W: 1, H: 1}, Stack: &itemdefs.Stack{Mode: itemdefs.StackModeStack, Max: 1000}},
		{DefID: 2, Key: "bag", Resource: "bag", Size: itemdefs.Size{W: 1, H: 1}, Container: &itemdefs.ContainerDef{Size: itemdefs.Size{W: 4, H: 4}}},
	})
	var err error
	f.service, err = NewObjectDestructionService(f.w, ObjectDestructionDependencies{
		Chunks: f.chunks, Persister: f.persister, IDs: f.ids, Items: registry, WithWorldRead: func(fn func(*ecs.World)) { fn(f.w) },
		Quarantine: func(h types.Handle) { ecs.RemoveComponent[components.Collider](f.w, h) }, Region: 1, MinX: -1000, MinY: -1000, MaxX: 5000, MaxY: 5000,
	})
	require.NoError(t, err)
	f.damage, err = NewObjectDamageService(f.w, f.service)
	require.NoError(t, err)
	f.target = f.targetWithID(t, 1)
	*ecs.GetResource[ecs.TimeState](f.w) = ecs.TimeState{Now: time.Unix(100, 0), RuntimeSecondsTotal: 10, UnixMs: 100000}
	return f
}

func (f *destructionServiceFixture) targetWithID(t testing.TB, id types.EntityID) types.Handle {
	t.Helper()
	h := f.w.Spawn(id, nil)
	ecs.AddComponent(f.w, h, components.EntityInfo{TypeID: 99, Region: 1, IsStatic: true})
	ecs.AddComponent(f.w, h, components.Transform{X: 50, Y: 50})
	ecs.AddComponent(f.w, h, components.ChunkRef{})
	ecs.AddComponent(f.w, h, components.Collider{HalfWidth: 3, HalfHeight: 3})
	ecs.AddComponent(f.w, h, components.ObjectInternalState{HP: 100, HasHP: true})
	require.NoError(t, f.damage.PrepareTarget(h))
	return h
}

func (f *destructionServiceFixture) addContainer(owner types.EntityID, key uint32, items ...components.InvItem) types.Handle {
	h := f.w.SpawnWithoutExternalID()
	ecs.AddComponent(f.w, h, components.InventoryContainer{OwnerID: owner, Kind: constt.InventoryGrid, Key: key, Version: 1, Width: 4, Height: 4, Items: items})
	ecs.GetResource[ecs.InventoryRefIndex](f.w).Add(constt.InventoryGrid, owner, key, h)
	return h
}

func lootItem(id types.EntityID, typeID uint32, qty uint32) components.InvItem {
	return components.InvItem{ItemID: id, TypeID: typeID, Quality: 17, Quantity: qty, W: 1, H: 1}
}

func TestObjectDestructionAllStationRootsAndNestedBagAfterCommit(t *testing.T) {
	f := newDestructionServiceFixture(t)
	a := f.addContainer(1, 3, lootItem(20, 1, 3))
	b := f.addContainer(1, 0, lootItem(30, 2, 1))
	nested := f.addContainer(30, 0, lootItem(900, 1, 2))
	result, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	require.True(t, result.EnteredDestruction)
	require.Empty(t, f.chunks.inserted)
	require.True(t, f.w.Alive(a))
	require.True(t, f.w.Alive(nested))
	f.service.Update()
	require.Empty(t, f.chunks.inserted)
	f.chunks.runOne(t)
	require.True(t, f.w.Alive(f.target))
	require.Empty(t, f.chunks.inserted)
	f.service.Update()
	require.False(t, f.w.Alive(f.target))
	require.False(t, f.w.Alive(a))
	require.False(t, f.w.Alive(b))
	require.False(t, f.w.Alive(nested))
	require.Len(t, f.chunks.inserted, 4)
	require.Len(t, f.persister.calls, 1)
	require.Equal(t, 1, f.ids.calls)
	require.Equal(t, []types.EntityID{1}, f.chunks.removed)
	require.Zero(t, f.service.PendingCount())
	seen := map[int64]bool{}
	for _, record := range f.chunks.inserted {
		require.False(t, seen[record.object.ID])
		seen[record.object.ID] = true
		require.InDelta(t, 50, record.object.X, 10)
		require.InDelta(t, 50, record.object.Y, 10)
		var data inventory.InventoryDataV1
		require.NoError(t, json.Unmarshal(record.inventory.Data, &data))
		require.Len(t, data.Items, 1)
		require.Equal(t, uint32(1), data.Items[0].Quantity)
		require.Equal(t, uint32(17), data.Items[0].Quality)
		if record.object.ID == 30 {
			require.NotNil(t, data.Items[0].NestedInventory)
			require.Equal(t, uint64(900), data.Items[0].NestedInventory.Items[0].ItemID)
			require.Equal(t, uint32(2), data.Items[0].NestedInventory.Items[0].Quantity)
		}
	}
	require.True(t, seen[20])
	require.True(t, seen[30])
	require.True(t, seen[100000])
	require.True(t, seen[100001])
	for _, count := range f.chunks.pins {
		require.Zero(t, count)
	}
}

func TestObjectDestructionCleanupIsBoundedAcrossTicks(t *testing.T) {
	f := newDestructionServiceFixture(t)
	roots := make([]types.Handle, 201)
	for i := range roots {
		roots[i] = f.addContainer(1, uint32(i))
	}
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	f.service.Update()
	f.chunks.runOne(t)
	for tick, expectedAlive := range []int{101, 1, 0} {
		f.service.Update()
		alive := 0
		for _, root := range roots {
			if f.w.Alive(root) {
				alive++
			}
		}
		require.Equal(t, expectedAlive, alive)
		require.Equal(t, tick < 2, f.w.Alive(f.target))
	}
	require.Zero(t, f.service.PendingCount())
}

func TestObjectDestructionRejectsInconsistentChunkBeforeAdmission(t *testing.T) {
	f := newDestructionServiceFixture(t)
	ecs.WithComponent(f.w, f.target, func(ref *components.ChunkRef) { ref.CurrentChunkX = 1 })
	before, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	_, err := f.damage.Apply(f.target, 100)
	require.ErrorIs(t, err, ErrObjectDestructionCapture)
	after, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	require.Equal(t, before, after)
	require.Zero(t, f.service.PendingCount())
	require.Empty(t, f.chunks.pins)
}

func TestObjectDestructionRetryUsesSameImmutableReplacement(t *testing.T) {
	f := newDestructionServiceFixture(t)
	root := f.addContainer(1, 0, lootItem(20, 1, 12))
	f.persister.err = errors.New("ambiguous commit")
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.True(t, f.w.Alive(root))
	require.True(t, f.w.Alive(f.target))
	require.Empty(t, f.chunks.inserted)
	require.Empty(t, f.chunks.jobs)
	// Even an accidental mutation of retained runtime state cannot change an
	// already owned replacement after a failed/ambiguous commit.
	ecs.WithComponent(f.w, root, func(c *components.InventoryContainer) {
		c.Items[0].Quality = 99
		c.Version++
	})
	ecs.GetResource[ecs.TimeState](f.w).Now = ecs.GetResource[ecs.TimeState](f.w).Now.Add(time.Second)
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.Equal(t, f.persister.calls[0], f.persister.calls[1])
	require.Equal(t, 1, f.ids.calls)
	require.Len(t, f.chunks.inserted, 12)
}

func TestObjectDestructionBackpressureAndMaterializationBudget(t *testing.T) {
	f := newDestructionServiceFixture(t)
	f.addContainer(1, 0, lootItem(20, 1, 201))
	f.chunks.accept = false
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	f.service.Update()
	require.Empty(t, f.chunks.jobs)
	require.Equal(t, 1, f.service.PendingCount())
	require.True(t, f.w.Alive(f.target))
	f.chunks.accept = true
	for f.service.PendingCount() != 0 {
		f.service.Update()
		if len(f.chunks.jobs) > 0 {
			f.chunks.runOne(t)
		}
		before := len(f.chunks.inserted)
		f.service.Update()
		require.LessOrEqual(t, len(f.chunks.inserted)-before, ObjectDestructionDropBudget)
	}
	require.Len(t, f.persister.calls, 1)
	require.Len(t, f.persister.calls[0], 201)
	require.Len(t, f.chunks.inserted, 201)
}

func TestObjectDestructionCaptureFailureRetainsAllAndCanRetry(t *testing.T) {
	f := newDestructionServiceFixture(t)
	item := lootItem(20, 1, 1)
	item.Quality = 0
	root := f.addContainer(1, 0, item)
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.True(t, f.w.Alive(f.target))
	require.True(t, f.w.Alive(root))
	require.Empty(t, f.persister.calls)
	require.Empty(t, f.chunks.inserted)
	ecs.WithComponent(f.w, root, func(c *components.InventoryContainer) { c.Items[0].Quality = 17; c.Version++ })
	ecs.GetResource[ecs.TimeState](f.w).Now = ecs.GetResource[ecs.TimeState](f.w).Now.Add(time.Second)
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.Zero(t, f.service.PendingCount())
	require.Len(t, f.chunks.inserted, 1)
}

func TestObjectDestructionRetainsCommittedPageWhileDestinationIsBusy(t *testing.T) {
	f := newDestructionServiceFixture(t)
	f.addContainer(1, 0, lootItem(20, 1, 2))
	f.chunks.insertErr = gameworld.ErrChunkPersistenceBusy
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.False(t, f.w.Alive(f.target))
	require.Empty(t, f.chunks.inserted)
	require.Equal(t, 1, f.service.PendingCount())
	require.Len(t, f.persister.calls, 1)
	f.chunks.insertErr = nil
	f.service.Update()
	require.Len(t, f.chunks.inserted, 2)
	require.Zero(t, f.service.PendingCount())
	require.Equal(t, []types.EntityID{1}, f.chunks.removed)
	require.Len(t, f.persister.calls, 1)
}

func TestObjectDestructionAdmissionQueueCapacityLeavesRejectedTargetUntouched(t *testing.T) {
	f := newDestructionServiceFixture(t)
	for i := 0; i < ObjectDestructionQueueCapacity; i++ {
		target := f.target
		if i != 0 {
			target = f.targetWithID(t, types.EntityID(i+1))
		}
		_, err := f.damage.Apply(target, 100)
		require.NoError(t, err)
	}
	target := f.targetWithID(t, 999)
	result, err := f.damage.Apply(target, 100)
	require.ErrorIs(t, err, ErrObjectDestructionQueueFull)
	require.Zero(t, result)
	hp, _ := ecs.GetComponent[components.ObjectInternalState](f.w, target)
	require.Equal(t, 100.0, hp.HP)
	require.False(t, hp.IsDirty)
	require.False(t, ecs.ObjectDestructionPending(f.w, target))
}

func TestObjectDestructionEmptyObjectsIgnoreBehaviorResources(t *testing.T) {
	f := newDestructionServiceFixture(t)
	ecs.WithComponent(f.w, f.target, func(s *components.ObjectInternalState) {
		s.State = &components.RuntimeObjectState{Behaviors: map[string]any{"tree": "ignored"}}
	})
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.Empty(t, f.chunks.inserted)
	require.Len(t, f.persister.calls, 1)
	require.Empty(t, f.persister.calls[0])
	require.False(t, f.w.Alive(f.target))
}

func TestObjectDestructionInvalidIDReservationCannotPersist(t *testing.T) {
	f := newDestructionServiceFixture(t)
	f.addContainer(1, 0, lootItem(20, 1, 2))
	f.ids.invalid = true
	_, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.Empty(t, f.persister.calls)
	require.True(t, f.w.Alive(f.target))
	require.Equal(t, 1, f.service.PendingCount())
}

func TestDestroyedObjectScatterIsStableAndWithinWorldEdges(t *testing.T) {
	f := newDestructionServiceFixture(t)
	f.addContainer(1, 0, lootItem(20, 1, 100))
	ecs.WithComponent(f.w, f.target, func(p *components.Transform) { p.X = -1000; p.Y = 4999 })
	coord := types.WorldToChunkCoord(-1000, 4999, constt.ChunkSize, constt.CoordPerTile)
	ecs.WithComponent(f.w, f.target, func(ref *components.ChunkRef) { ref.CurrentChunkX, ref.CurrentChunkY = coord.X, coord.Y })
	_, err := f.damage.Apply(f.target, math.MaxFloat64)
	require.NoError(t, err)
	for f.service.PendingCount() > 0 {
		f.service.Update()
		if len(f.chunks.jobs) > 0 {
			f.chunks.runOne(t)
		}
	}
	for _, r := range f.chunks.inserted {
		require.GreaterOrEqual(t, r.object.X, -1000)
		require.LessOrEqual(t, r.object.X, -990)
		require.GreaterOrEqual(t, r.object.Y, 4989)
		require.Less(t, r.object.Y, 5000)
		require.Equal(t, types.WorldToChunkCoord(r.object.X, r.object.Y, constt.ChunkSize, constt.CoordPerTile), types.ChunkCoord{X: r.object.ChunkX, Y: r.object.ChunkY})
	}
}
