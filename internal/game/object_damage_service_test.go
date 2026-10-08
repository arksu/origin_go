package game

import (
	"context"
	"errors"
	"math"
	"testing"

	"origin/internal/combat"
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

type damageTestChunks struct{ pinErr error }

func (*damageTestChunks) SubmitPersistenceJob(gameworld.PersistenceJob) bool          { return false }
func (c *damageTestChunks) PinPersistence(types.ChunkCoord) error                     { return c.pinErr }
func (*damageTestChunks) UnpinPersistence(types.ChunkCoord)                           {}
func (*damageTestChunks) WithPersistence(_ []types.ChunkCoord, fn func() error) error { return fn() }
func (*damageTestChunks) InsertCommittedDropped(*repository.Object, *repository.Inventory) error {
	return nil
}
func (*damageTestChunks) RemoveCommittedSource(types.ChunkCoord, types.EntityID) {}

type damageTestPersister struct{}

func (*damageTestPersister) ReplaceObjectWithDroppedItems(context.Context, int, types.EntityID, types.EntityID, func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error)) error {
	return nil
}

type damageTestIDs struct{}

func (*damageTestIDs) ReserveIDs(uint64) (types.EntityID, types.EntityID, error) {
	return 1000, 1000, nil
}

type objectDamageFixture struct {
	w           *ecs.World
	target      types.Handle
	service     *ObjectDamageService
	destruction *ObjectDestructionService
	chunks      *damageTestChunks
	quarantines int
}

func newObjectDamageFixture(t testing.TB, w *ecs.World) *objectDamageFixture {
	t.Helper()
	if w == nil {
		w = ecs.NewWorldForTesting()
	}
	f := &objectDamageFixture{w: w, chunks: &damageTestChunks{}}
	f.target = w.Spawn(1, nil)
	ecs.AddComponent(w, f.target, components.EntityInfo{TypeID: 99, Region: 1})
	ecs.AddComponent(w, f.target, components.Transform{X: 50, Y: 50})
	ecs.AddComponent(w, f.target, components.ChunkRef{})
	ecs.AddComponent(w, f.target, components.ObjectInternalState{HP: 100, HasHP: true})
	var err error
	f.destruction, err = NewObjectDestructionService(w, ObjectDestructionDependencies{
		Chunks: f.chunks, Persister: &damageTestPersister{}, IDs: &damageTestIDs{}, Items: itemdefs.NewRegistry(nil),
		WithWorldRead: func(fn func(*ecs.World)) { fn(w) }, Quarantine: func(types.Handle) { f.quarantines++ }, Region: 1, MaxX: 1000, MaxY: 1000,
	})
	require.NoError(t, err)
	f.service, err = NewObjectDamageService(w, f.destruction)
	require.NoError(t, err)
	require.NoError(t, f.service.PrepareTarget(f.target))
	return f
}

func (f *objectDamageFixture) hp() components.ObjectInternalState {
	h, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	return h
}

func TestObjectDamageServiceFractionsAndZeroNoop(t *testing.T) {
	f := newObjectDamageFixture(t, nil)
	writes := 0
	f.w.AddComponentObserver(components.ObjectInternalStateComponentID, func(types.Handle) { writes++ })
	for _, draw := range []float64{0.6, 0.49, 81.0 / 13, 3.6} {
		before := f.hp()
		result, err := f.service.Apply(f.target, draw)
		require.NoError(t, err)
		require.Equal(t, 0.0, result.Armor)
		require.Equal(t, draw, result.Damage)
		require.Equal(t, before.HP, result.BeforeHP)
		require.Equal(t, before.HP-draw, result.AfterHP)
		require.False(t, result.EnteredDestruction)
		require.Equal(t, result.AfterHP, f.hp().HP)
		require.True(t, f.hp().IsDirty)
	}
	require.Equal(t, 4, writes)
	before := f.hp()
	result, err := f.service.Apply(f.target, 0)
	require.NoError(t, err)
	require.Equal(t, before.HP, result.BeforeHP)
	require.Equal(t, before.HP, result.AfterHP)
	require.Equal(t, before, f.hp())
	require.Equal(t, 4, writes)
}

func TestObjectDamageServiceFatalAdmissionIsAtomicAndOnce(t *testing.T) {
	f := newObjectDamageFixture(t, nil)
	blocked := errors.New("pin rejected")
	f.chunks.pinErr = blocked
	before := f.hp()
	result, err := f.service.Apply(f.target, math.MaxFloat64)
	require.ErrorIs(t, err, blocked)
	require.Zero(t, result)
	require.Equal(t, before, f.hp())
	require.Zero(t, f.destruction.PendingCount())
	require.Zero(t, f.quarantines)
	f.chunks.pinErr = nil
	result, err = f.service.Apply(f.target, 100)
	require.NoError(t, err)
	require.True(t, result.EnteredDestruction)
	require.Equal(t, 100.0, result.BeforeHP)
	require.Zero(t, result.AfterHP)
	require.Zero(t, f.hp().HP)
	require.True(t, f.hp().IsDirty)
	require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
	require.Equal(t, 1, f.destruction.PendingCount())
	require.Equal(t, 1, f.quarantines)
	result, err = f.service.Apply(f.target, 1)
	require.ErrorIs(t, err, ErrObjectDamageTargetDead)
	require.Zero(t, result)
	require.Equal(t, 1, f.destruction.PendingCount())
}

func TestObjectDamageServiceRejectsInvalidInputsAndTargets(t *testing.T) {
	f := newObjectDamageFixture(t, nil)
	for _, draw := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		before := f.hp()
		result, err := f.service.Apply(f.target, draw)
		require.ErrorIs(t, err, combat.ErrInvalidInput)
		require.Zero(t, result)
		require.Equal(t, before, f.hp())
	}
	for _, hp := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1), 0} {
		ecs.WithComponent(f.w, f.target, func(s *components.ObjectInternalState) { s.HP = hp; s.IsDirty = false })
		result, err := f.service.Apply(f.target, 1)
		if hp == 0 {
			require.ErrorIs(t, err, ErrObjectDamageTargetDead)
		} else {
			require.ErrorIs(t, err, ErrInvalidObjectDamageHealth)
		}
		require.Zero(t, result)
		require.Equal(t, math.Float64bits(hp), math.Float64bits(f.hp().HP))
		require.False(t, f.hp().IsDirty)
	}
	ecs.AddComponent(f.w, f.target, components.ObjectInternalState{HP: 100, HasHP: true})
	delete(ecs.GetResource[ecs.ObjectDestructionState](f.w).Prepared, f.target)
	result, err := f.service.Apply(f.target, 0)
	require.ErrorIs(t, err, ErrObjectDamageTargetUnprepared)
	require.Zero(t, result)
	require.NoError(t, f.service.PrepareTarget(f.target))
	ecs.AddComponent(f.w, f.target, components.EntityHealth{SHP: 10, HHP: 10})
	_, err = f.service.Apply(f.target, 1)
	require.ErrorIs(t, err, ErrInvalidObjectDamageTarget)
	ecs.RemoveComponent[components.EntityHealth](f.w, f.target)
	ecs.AddComponent(f.w, f.target, components.DroppedItem{})
	_, err = f.service.Apply(f.target, 1)
	require.ErrorIs(t, err, ErrInvalidObjectDamageTarget)
	ecs.RemoveComponent[components.DroppedItem](f.w, f.target)
	ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) { info.TypeID = constt.DroppedItemTypeID })
	_, err = f.service.Apply(f.target, 1)
	require.ErrorIs(t, err, ErrInvalidObjectDamageTarget)
	ecs.RemoveComponent[components.ObjectInternalState](f.w, f.target)
	_, err = f.service.Apply(f.target, 1)
	require.ErrorIs(t, err, ErrInvalidObjectDamageTarget)
	stale := f.target
	require.True(t, f.w.Despawn(stale))
	replacement := f.w.Spawn(2, nil)
	require.NotEqual(t, stale, replacement)
	_, err = f.service.Apply(stale, 1)
	require.ErrorIs(t, err, ErrInvalidObjectDamageTarget)
	require.False(t, ecs.GetResource[ecs.ObjectDestructionState](f.w).Prepared[stale])
}

func TestObjectDamageServiceAllocations(t *testing.T) {
	f := newObjectDamageFixture(t, nil)
	var result ObjectDamageResult
	var err error
	for _, draw := range []float64{0, 0.49, -1, math.NaN()} {
		allocs := testing.AllocsPerRun(1000, func() {
			ecs.WithComponent(f.w, f.target, func(hp *components.ObjectInternalState) { hp.HP = 100 })
			result, err = f.service.Apply(f.target, draw)
		})
		require.Zero(t, allocs)
		if draw >= 0 {
			require.NoError(t, err)
			require.Equal(t, draw, result.Damage)
		} else {
			require.Error(t, err)
			require.Zero(t, result)
		}
	}
	_, err = NewObjectDamageService(nil, f.destruction)
	require.ErrorIs(t, err, ErrInvalidObjectDamageService)
	_, err = NewObjectDamageService(ecs.NewWorldForTesting(), f.destruction)
	require.ErrorIs(t, err, ErrInvalidObjectDamageService)
}

func TestObjectDamageRejectsForeignRegionAndLayerWithoutMutation(t *testing.T) {
	for _, field := range []string{"region", "layer"} {
		t.Run(field, func(t *testing.T) {
			f := newObjectDamageFixture(t, nil)
			ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) {
				if field == "region" {
					info.Region++
				} else {
					info.Layer++
				}
			})
			before := f.hp()
			for _, draw := range []float64{0, 1, 100} {
				result, err := f.service.Apply(f.target, draw)
				require.ErrorIs(t, err, ErrInvalidObjectDamageTarget)
				require.Zero(t, result)
				require.Equal(t, before, f.hp())
				require.Zero(t, f.destruction.PendingCount())
				require.Zero(t, f.quarantines)
			}
		})
	}
}

func TestObjectDamageRejectsIdentityOutsidePersistenceRange(t *testing.T) {
	f := newObjectDamageFixture(t, nil)
	target := f.w.Spawn(types.EntityID(math.MaxInt64)+1, nil)
	ecs.AddComponent(f.w, target, components.EntityInfo{TypeID: 99, Region: 1})
	ecs.AddComponent(f.w, target, components.ObjectInternalState{HP: 100, HasHP: true})
	require.ErrorIs(t, f.service.PrepareTarget(target), ErrInvalidObjectDamageTarget)
	result, err := f.service.Apply(target, 100)
	require.ErrorIs(t, err, ErrInvalidObjectDamageTarget)
	require.Zero(t, result)
	health, _ := ecs.GetComponent[components.ObjectInternalState](f.w, target)
	require.Equal(t, float64(100), health.HP)
	require.Zero(t, f.destruction.PendingCount())
}
