package game

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	gameworld "origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type hearthActivationReader struct {
	rows   []repository.GetCharacterHearthOwnersRow
	calls  []repository.GetCharacterHearthOwnersParams
	onRead func()
}

func (*hearthActivationReader) GetCharacterHearth(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
	panic("activation must batch ownership reads")
}

func (r *hearthActivationReader) GetCharacterHearthOwners(_ context.Context, params repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
	r.calls = append(r.calls, params)
	if r.onRead != nil {
		r.onRead()
	}
	return r.rows, nil
}

type hearthActivationSaver struct {
	saved  []repository.Object
	calls  int
	onSave func([]*repository.Object)
}

func (*hearthActivationSaver) SaveConstruction(context.Context, types.EntityID, *repository.Object, *repository.Object) error {
	panic("activation must not save construction")
}

func (*hearthActivationSaver) SaveTerminal(context.Context, types.EntityID, hearth.ObjectKey, *repository.Object) error {
	panic("unexpected terminal save")
}

func (*hearthActivationSaver) RepairInvalidHome(context.Context, types.EntityID, hearth.ObjectKey, int) error {
	panic("unexpected hearth repair")
}

func (s *hearthActivationSaver) SaveOrphans(_ context.Context, objects []*repository.Object) error {
	s.calls++
	if s.onSave != nil {
		s.onSave(objects)
	}
	for _, object := range objects {
		s.saved = append(s.saved, *object)
	}
	return nil
}

type hearthActivationFixture struct {
	shard       *Shard
	chunk       *core.Chunk
	runtime     *hearth.Runtime
	reader      *hearthActivationReader
	saver       *hearthActivationSaver
	hearthDef   *objectdefs.ObjectDef
	campfireDef *objectdefs.ObjectDef
	observer    types.Handle
	spawns      <-chan *ecs.EntitySpawnBatchEvent
}

func newHearthActivationFixture(t *testing.T) *hearthActivationFixture {
	t.Helper()
	_, hearthDef := loadHearthBuildRecipeForTest(t)
	campfireDef, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	cfg := &config.Config{Game: config.GameConfig{
		ChunkLRUCapacity: 4, ChunkLRUTTL: 60, LoadWorkers: 1,
		WorldWidthChunks: 1, WorldHeightChunks: 1,
	}}
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	w := ecs.NewWorldWithCapacity(16, bus, hearth.SurfaceLayer)
	ecs.GetResource[ecs.TimeState](w).Tick = 123
	shard := &Shard{world: w, cfg: cfg, eventBus: bus, logger: zap.NewNop()}
	registry := behaviors.MustDefaultRegistry()
	factory := gameworld.NewObjectFactory(nil)
	shard.chunkManager = gameworld.NewChunkManager(cfg, nil, w, shard, hearth.SurfaceLayer, 1, factory, registry, bus, shard.logger)
	t.Cleanup(func() {
		shard.chunkManager.Stop()
		require.NoError(t, bus.Shutdown(context.Background()))
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, shard.chunkManager.WaitPreloaded(ctx, types.ChunkCoord{}))
	reader, saver := &hearthActivationReader{}, &hearthActivationSaver{}
	runtime := ecs.InitResource(w, hearth.NewRuntime(w, shard.chunkManager, reader, factory, saver, registry, bus, shard.logger))
	shard.chunkManager.SetActivationPreparer(runtime.PrepareChunkActivation)
	shard.objectCollisionFilter = runtime.IgnoreObjectCollision
	observer := w.Spawn(9000, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.Transform{X: 200, Y: 200})
		ecs.AddComponent(w, handle, components.ChunkRef{})
		ecs.AddComponent(w, handle, components.Vision{Radius: 100, Power: 100})
	})
	ecs.GetResource[ecs.VisibilityState](w).VisibleByObserver[observer] = ecs.ObserverVisibility{Known: map[types.Handle]types.EntityID{}}
	spawns := make(chan *ecs.EntitySpawnBatchEvent, 8)
	bus.SubscribeAsync(ecs.TopicGameplayEntitySpawnBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		spawns <- event.(*ecs.EntitySpawnBatchEvent)
		return nil
	})
	return &hearthActivationFixture{
		shard: shard, chunk: shard.chunkManager.GetChunk(types.ChunkCoord{}), runtime: runtime,
		reader: reader, saver: saver, hearthDef: hearthDef, campfireDef: campfireDef,
		observer: observer, spawns: spawns,
	}
}

func (f *hearthActivationFixture) rawHearth() *repository.Object {
	return &repository.Object{ID: 1000, TypeID: f.hearthDef.DefID, Region: 1, X: 200, Y: 200,
		Quality: 17, Hp: sql.NullFloat64{Float64: 7, Valid: true}, CreateTick: 12, LastTick: 19}
}

func (f *hearthActivationFixture) activate(t *testing.T) {
	t.Helper()
	require.NoError(t, f.shard.PrepareEntityAOI(context.Background(), 9000, 200, 200))
	require.Equal(t, types.ChunkStateActive, f.chunk.GetState())
}

func (f *hearthActivationFixture) firstVisible(t *testing.T) types.Handle {
	t.Helper()
	f.shard.mu.Lock()
	systems.NewVisionSystem(f.shard.world, f.shard.chunkManager, f.shard.eventBus, false, f.shard.logger).ForceUpdateForObserver(f.shard.world, f.observer)
	f.shard.mu.Unlock()
	select {
	case event := <-f.spawns:
		require.Equal(t, types.EntityID(9000), event.ObserverID)
		require.Len(t, event.Entries, 1)
		require.Equal(t, types.EntityID(1000), event.Entries[0].EntityID)
		return event.Entries[0].Handle
	case <-time.After(time.Second):
		t.Fatal("restored object did not become visible")
		return types.InvalidHandle
	}
}

func (f *hearthActivationFixture) requireCampfire(t *testing.T, handle types.Handle) {
	t.Helper()
	w := f.shard.world
	info, ok := ecs.GetComponent[components.EntityInfo](w, handle)
	require.True(t, ok)
	require.Equal(t, uint32(f.campfireDef.DefID), info.TypeID)
	require.False(t, info.Indestructible)
	appearance, ok := ecs.GetComponent[components.Appearance](w, handle)
	require.True(t, ok)
	require.Equal(t, "campfire/unlit", appearance.Resource)
	collider, ok := ecs.GetComponent[components.Collider](w, handle)
	require.True(t, ok)
	require.Equal(t, float64(f.campfireDef.Components.Collider.W)/2, collider.HalfWidth)
	require.Equal(t, float64(f.campfireDef.Components.Collider.H)/2, collider.HalfHeight)
	internal, ok := ecs.GetComponent[components.ObjectInternalState](w, handle)
	require.True(t, ok)
	require.True(t, internal.HasHP)
	require.Equal(t, float64(f.campfireDef.HP), internal.HP)
	station, ok := ecs.GetComponent[components.StationState](w, handle)
	require.True(t, ok)
	require.Equal(t, "unlit", station.CurrentState)
	burner, ok := components.GetBehaviorState[components.BurnerBehaviorState](internal, "burner")
	require.True(t, ok)
	require.Equal(t, f.campfireDef.BurnerConfig.InitialFuel, burner.Fuel)
	require.Zero(t, burner.NextFuelBurnAtTick)
}

func TestHearthRestorationOfflineOwnerIsKnownBeforeFirstVisibility(t *testing.T) {
	f := newHearthActivationFixture(t)
	raw := f.rawHearth()
	f.chunk.SetRawObjects([]*repository.Object{raw})
	f.reader.rows = []repository.GetCharacterHearthOwnersRow{{CharacterID: 71, ObjectRegion: 1, ObjectID: raw.ID, X: raw.X, Y: raw.Y}}
	f.reader.onRead = func() {
		require.False(t, f.shard.world.Alive(f.shard.world.GetHandleByEntityID(types.EntityID(raw.ID))))
		require.Empty(t, f.chunk.GetHandles())
		if f.shard.mu.TryLock() {
			f.shard.mu.Unlock()
			t.Fatal("ownership lookup must hold the surface shard lock")
		}
	}
	built := 0
	f.shard.world.AddComponentObserver(components.EntityInfoComponentID, func(handle types.Handle) {
		built++
		owner, owned := f.runtime.Index.Owner(hearth.ObjectKey{Region: 1, ID: types.EntityID(raw.ID)})
		require.True(t, owned, "ownership must be ready before entity creation")
		require.Equal(t, types.EntityID(71), owner)
		require.Empty(t, f.chunk.GetHandles(), "ownership must precede spatial publication")
	})
	f.activate(t)
	require.Equal(t, 1, built)
	require.Equal(t, []repository.GetCharacterHearthOwnersParams{{ObjectRegions: []int{1}, ObjectIds: []int64{1000}, HearthTypeID: f.hearthDef.DefID}}, f.reader.calls)
	require.Zero(t, f.saver.calls)
	require.False(t, f.shard.world.Alive(f.shard.world.GetHandleByEntityID(71)), "the saved owner remains offline")
	handle := f.firstVisible(t)
	info, found := ecs.GetComponent[components.EntityInfo](f.shard.world, handle)
	require.True(t, found)
	require.Equal(t, uint32(f.hearthDef.DefID), info.TypeID)
	require.True(t, info.Indestructible)
	require.True(t, f.runtime.IgnoreObjectCollision(71, handle))
	require.False(t, f.runtime.IgnoreObjectCollision(72, handle))
	require.Equal(t, []types.Handle{handle}, f.chunk.GetHandles())
}

func TestHearthRestorationOrphanPersistsBeforePublicationAndStaysCampfire(t *testing.T) {
	f := newHearthActivationFixture(t)
	raw := f.rawHearth()
	raw.OwnerID = sql.NullInt64{Int64: 71, Valid: true}
	f.chunk.SetRawObjects([]*repository.Object{raw})
	committed := false
	f.saver.onSave = func(objects []*repository.Object) {
		require.Len(t, objects, 1)
		require.Equal(t, f.campfireDef.DefID, objects[0].TypeID)
		require.Equal(t, sql.NullFloat64{Float64: float64(f.campfireDef.HP), Valid: true}, objects[0].Hp)
		require.False(t, objects[0].Data.Valid)
		require.Equal(t, int64(123), objects[0].LastTick)
		require.Equal(t, raw.CreateTick, objects[0].CreateTick)
		require.Equal(t, raw.Quality, objects[0].Quality)
		require.Same(t, raw, f.chunk.GetRawObjectByID(1000), "cache publication must follow the save")
		require.Equal(t, f.hearthDef.DefID, raw.TypeID)
		require.False(t, f.shard.world.Alive(f.shard.world.GetHandleByEntityID(1000)))
		require.Empty(t, f.chunk.GetHandles())
		require.Empty(t, f.spawns)
		committed = true
	}
	f.shard.world.AddComponentObserver(components.EntityInfoComponentID, func(types.Handle) {
		require.True(t, committed, "entity creation must follow the synchronous save")
	})
	f.activate(t)
	require.Equal(t, 1, f.saver.calls)
	require.Len(t, f.reader.calls, 1)
	handle := f.firstVisible(t)
	f.requireCampfire(t, handle)
	require.False(t, f.runtime.IgnoreObjectCollision(71, handle))
	_, owned := f.runtime.Index.Owner(hearth.ObjectKey{Region: 1, ID: 1000})
	require.False(t, owned, "object.owner_id must not confer Hearth ownership")

	f.shard.mu.Lock()
	f.shard.chunkManager.UnregisterEntity(9000)
	f.shard.mu.Unlock()
	require.Equal(t, types.ChunkStateInactive, f.chunk.GetState())
	require.False(t, f.shard.world.Alive(handle))
	require.Equal(t, f.campfireDef.DefID, f.chunk.GetRawObjectByID(1000).TypeID)
	f.activate(t)
	reactivated := f.shard.world.GetHandleByEntityID(1000)
	require.NotEqual(t, handle, reactivated)
	f.requireCampfire(t, reactivated)
	require.Len(t, f.reader.calls, 1)
	require.Equal(t, 1, f.saver.calls)

	reloaded := newHearthActivationFixture(t)
	saved := f.saver.saved[0]
	reloaded.chunk.SetRawObjects([]*repository.Object{&saved})
	reloaded.activate(t)
	reloaded.requireCampfire(t, reloaded.firstVisible(t))
	require.Empty(t, reloaded.reader.calls, "persisted campfires need no Hearth ownership lookup")
	require.Zero(t, reloaded.saver.calls)
}

func TestHearthRestorationSkipsRawDuplicateOfLiveCampfire(t *testing.T) {
	f := newHearthActivationFixture(t)
	raw := f.rawHearth()
	campfire := *raw
	campfire.TypeID = f.campfireDef.DefID
	campfire.Hp = sql.NullFloat64{Float64: float64(f.campfireDef.HP), Valid: true}
	f.chunk.SetRawObjects([]*repository.Object{&campfire})
	f.activate(t)
	handle := f.shard.world.GetHandleByEntityID(1000)
	f.requireCampfire(t, handle)
	f.shard.mu.Lock()
	f.chunk.UpsertRawObject(raw)
	f.shard.chunkManager.RegisterEntity(9001, 200, 200, false)
	f.shard.mu.Unlock()
	require.Empty(t, f.reader.calls, "live ECS identity supersedes stale raw Hearth metadata")
	require.Zero(t, f.saver.calls)
	require.Empty(t, f.chunk.GetRawObjects())
	require.Equal(t, handle, f.shard.world.GetHandleByEntityID(1000))
	require.Equal(t, []types.Handle{handle}, f.chunk.GetHandles())
	f.requireCampfire(t, f.firstVisible(t))
}

func (*hearthActivationReader) GetSavedCharacterHearth(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
	panic("unexpected saved hearth query")
}
