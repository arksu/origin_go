package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type corpseLootChunks struct {
	*destructionChunksFixture
	replacements []repository.Object
	replaceErr   error
}

func (c *corpseLootChunks) ReplaceCommittedSource(raw *repository.Object) error {
	if c.replaceErr != nil {
		return c.replaceErr
	}
	c.replacements = append(c.replacements, *raw)
	return nil
}

type corpseLootPersister struct {
	*destructionPersisterFixture
	replacements []repository.Object
}

func (p *corpseLootPersister) TransformObjectWithDroppedItems(ctx context.Context, raw *repository.Object, maxID types.EntityID, next func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error)) error {
	p.replacements = append(p.replacements, *raw)
	return p.ReplaceObjectWithDroppedItems(ctx, raw.Region, types.EntityID(raw.ID), maxID, next)
}

func installCorpseLootDefinitions(t *testing.T) *objectdefs.ObjectDef {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	registry := objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 11, Key: "player"},
		{DefID: 99, Key: "player_dead", HP: 100, Resource: "player", IsStatic: true, Indestructible: true, BehaviorOrder: []string{"container", "player_dead"}},
		{DefID: 17, Key: "player_skeleton", HP: 100, Resource: "skeleton/with_skull", IsStatic: true, Indestructible: true, BehaviorOrder: []string{"lift"}, Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 9, H: 9}}},
	})
	objectdefs.SetGlobalForTesting(registry)
	destination, _ := registry.GetByKey("player_skeleton")
	return destination
}

func newCorpseLootFixture(t *testing.T) (*destructionServiceFixture, *corpseLootChunks, *corpseLootPersister, *objectdefs.ObjectDef) {
	t.Helper()
	f := newDestructionServiceFixture(t)
	destination := installCorpseLootDefinitions(t)
	chunks := &corpseLootChunks{destructionChunksFixture: f.chunks}
	persister := &corpseLootPersister{destructionPersisterFixture: f.persister}
	f.service.deps.Chunks, f.service.deps.Persister = chunks, persister
	shard := &Shard{world: f.w, logger: zap.NewNop()}
	f.service.deps.TransformCommitted = shard.completeCorpseDecay
	ecs.WithComponent(f.w, f.target, func(info *components.EntityInfo) {
		info.Indestructible = true
		info.Quality = 23
		info.Behaviors = []string{"container", "player_dead"}
	})
	name := "Remembered Player"
	ecs.AddComponent(f.w, f.target, components.Appearance{Resource: "player", Name: &name})
	ecs.AddComponent(f.w, f.target, components.InventoryOwner{})
	ecs.AddComponent(f.w, f.target, components.CorpseVisualState{LyingRevision: 7})
	ecs.WithComponent(f.w, f.target, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, "player_dead", &components.CorpseDecayBehaviorState{DecayAtRuntimeSeconds: 21610})
		state.HP = .49
		state.IsDirty = false
	})
	return f, chunks, persister, destination
}

func TestCorpseDecayLootAtomicRetryPreservesIdentity(t *testing.T) {
	for _, withLoot := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "nested_and_stacked"}[withLoot], func(t *testing.T) {
			f, chunks, persister, destination := newCorpseLootFixture(t)
			var containers []types.Handle
			if withLoot {
				containers = []types.Handle{
					f.addContainer(1, 0, lootItem(20, 1, 3)),
					f.addContainer(1, 3, lootItem(30, 2, 1)),
					f.addContainer(30, 0, lootItem(900, 1, 2)),
				}
			}
			before, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
			_, damageErr := f.damage.Apply(f.target, 100)
			require.ErrorIs(t, damageErr, ErrObjectDamageTargetIndestructible)
			require.NoError(t, f.service.TransformWithLoot(f.target, destination))
			require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
			after, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
			require.Equal(t, before, after, "decay admission must not simulate fatal damage")
			require.ErrorIs(t, f.service.TransformWithLoot(f.target, destination), ErrObjectDestructionPending)
			persister.err = errors.New("ambiguous commit")
			f.service.Update()
			chunks.runOne(t)
			require.Len(t, persister.calls, 1, "first persistence operation: %v", f.service.operations[0].err)
			f.service.Update()
			info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.target)
			require.Equal(t, uint32(99), info.TypeID)
			for _, handle := range containers {
				require.True(t, f.w.Alive(handle))
			}
			require.Empty(t, chunks.inserted)
			ecs.GetResource[ecs.TimeState](f.w).Now = ecs.GetResource[ecs.TimeState](f.w).Now.Add(time.Second)
			f.service.Update()
			chunks.runOne(t)
			f.service.Update()
			require.Len(t, persister.calls, 2)
			require.Equal(t, persister.calls[0], persister.calls[1], "retry must reuse identities, coordinates and payloads")
			require.Equal(t, persister.replacements[0], persister.replacements[1])
			require.Len(t, chunks.replacements, 1)
			require.Empty(t, chunks.removed)
			require.True(t, f.w.Alive(f.target), "preserve the exact live handle as well as EntityID")
			require.Equal(t, f.target, f.w.GetHandleByEntityID(1))
			info, _ = ecs.GetComponent[components.EntityInfo](f.w, f.target)
			require.Equal(t, uint32(17), info.TypeID)
			require.Equal(t, uint32(23), info.Quality)
			require.Equal(t, []string{"lift"}, info.Behaviors)
			require.True(t, info.Indestructible)
			appearance, _ := ecs.GetComponent[components.Appearance](f.w, f.target)
			require.Equal(t, "skeleton/with_skull", appearance.Resource)
			require.Equal(t, "Remembered Player", *appearance.Name)
			position, _ := ecs.GetComponent[components.Transform](f.w, f.target)
			require.Equal(t, components.Transform{X: 50, Y: 50}, position)
			state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
			require.Equal(t, float64(100), state.HP)
			_, oldState := components.GetBehaviorState[components.CorpseDecayBehaviorState](state, "player_dead")
			require.False(t, oldState)
			require.False(t, ecs.HasComponent[components.InventoryOwner](f.w, f.target))
			require.False(t, ecs.HasComponent[components.CorpseVisualState](f.w, f.target))
			require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
			for _, handle := range containers {
				require.False(t, f.w.Alive(handle))
			}
			if withLoot {
				require.Len(t, chunks.inserted, 4)
				require.Equal(t, 1, f.ids.calls)
			} else {
				require.Empty(t, chunks.inserted)
				require.Zero(t, f.ids.calls)
			}
			require.Zero(t, f.service.PendingCount())
			for _, pins := range chunks.pins {
				require.Zero(t, pins)
			}
			f.service.Update()
			require.Len(t, chunks.replacements, 1)
		})
	}
}

func TestCorpseDecayCacheBackpressureKeepsSourceQuarantined(t *testing.T) {
	f, chunks, _, destination := newCorpseLootFixture(t)
	f.addContainer(1, 0, lootItem(20, 1, 1))
	require.NoError(t, f.service.TransformWithLoot(f.target, destination))
	f.service.Update()
	chunks.runOne(t)
	require.NoError(t, f.service.operations[0].err)
	chunks.replaceErr = errors.New("cache gate busy")
	f.service.Update()
	require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
	info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.target)
	require.Equal(t, uint32(99), info.TypeID)
	require.Empty(t, chunks.inserted)
	require.Equal(t, 1, f.service.PendingCount())
	chunks.replaceErr = nil
	f.service.Update()
	require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
	info, _ = ecs.GetComponent[components.EntityInfo](f.w, f.target)
	require.Equal(t, uint32(17), info.TypeID)
	require.Len(t, chunks.inserted, 1)
	require.Zero(t, f.service.PendingCount())
}

func TestCorpseDecayAdmissionBackpressureDoesNotMutateSource(t *testing.T) {
	f, _, _, destination := newCorpseLootFixture(t)
	state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	f.service.freeCount = 0
	require.ErrorIs(t, f.service.TransformWithLoot(f.target, destination), ErrObjectDestructionQueueFull)
	after, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
	require.Equal(t, state, after)
	require.True(t, ecs.HasComponent[components.Collider](f.w, f.target))
	require.False(t, ecs.ObjectDestructionPending(f.w, f.target))
	f.service.freeCount = ObjectDestructionQueueCapacity
	require.NoError(t, f.service.TransformWithLoot(f.target, destination))
}
