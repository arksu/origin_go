package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWildWeedRefillCommittedStemActivatesInOccupiedChunkAndGrows(t *testing.T) {
	db := testutil.NewPostgres(t, "ORIGIN_SPOTS_TEST_DSN", "../../migrations/schema.sql")
	ctx := context.Background()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	def := objectdefs.ObjectDef{
		DefID: 41, Key: "wild_windsown_weed", HP: 100, IsStatic: true, Resource: "wild_windsown_weed/3",
		Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 4, H: 4, Layer: 1, Mask: 1}},
		Behaviors:  map[string]json.RawMessage{"wild_weed": json.RawMessage(`{}`)}, BehaviorOrder: []string{"wild_weed"},
		WildWeedConfig: &objectdefs.WildWeedBehaviorConfig{},
	}
	for stage := range 4 {
		def.Appearance = append(def.Appearance, objectdefs.Appearance{
			ID: fmt.Sprintf("stage%d", stage), Resource: fmt.Sprintf("wild_windsown_weed/%d", stage),
			When: &objectdefs.AppearanceWhen{Flags: []string{fmt.Sprintf("wild_weed.stage%d", stage)}},
		})
	}
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{def}))
	registry, err := behaviors.DefaultRegistry()
	require.NoError(t, err)
	tiles := make([]byte, constt.ChunkSize*constt.ChunkSize)
	for index := range tiles {
		tiles[index] = 20
	}
	_, err = db.Queries().UpsertChunk(ctx, repository.UpsertChunkParams{Region: 1, TilesData: tiles})
	require.NoError(t, err)
	w := ecs.NewWorldForTesting()
	const preparedAt int64 = 100 * 24 * 60 * 60
	ts := ecs.GetResource[ecs.TimeState](w)
	*ts = ecs.TimeState{Now: time.Unix(preparedAt, 0), WallNow: time.Unix(1_800_000_000, 0), RuntimeSecondsTotal: preparedAt}
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
	cfg := &config.Config{Game: config.GameConfig{
		Region: 1, WorldWidthChunks: 1, WorldHeightChunks: 1,
		ChunkLRUCapacity: 4, ChunkLRUTTL: 60, LoadWorkers: 1, SaveWorkers: 1,
	}}
	cm := gameworld.NewChunkManager(cfg, db, w, nil, 0, 1, gameworld.NewObjectFactory(nil), registry, bus, zap.NewNop())
	t.Cleanup(cm.Stop)
	cm.RegisterEntity(9000, 300, 300, false)
	loadCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	require.NoError(t, cm.WaitPreloaded(loadCtx, types.ChunkCoord{}))
	cm.Update(0)
	chunk := cm.GetChunkFast(types.ChunkCoord{})
	require.NotNil(t, chunk)
	require.Equal(t, types.ChunkStateActive, chunk.GetState())
	observer := w.Spawn(9000, nil)
	ecs.AddComponent(w, observer, components.Transform{X: 300, Y: 300})
	ecs.AddComponent(w, observer, components.Vision{Radius: 100, Power: 100})
	ecs.AddComponent(w, observer, components.ChunkRef{})
	ecs.AddComponent(w, observer, components.EntityInfo{})
	chunk.Spatial().AddDynamic(observer, 300, 300)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.VisibleByObserver[observer] = ecs.ObserverVisibility{Known: map[types.Handle]types.EntityID{observer: 9000}}
	visibility.ObserversByVisibleTarget[observer] = map[types.Handle]struct{}{observer: {}}
	vision := systems.NewVisionSystem(w, cm, bus, false, zap.NewNop())
	vision.Update(w, 0)
	require.NotEmpty(t, visibility.VisibleByObserver[observer].LastChunkGens, "establish the stationary observer's cached spatial generations")
	ts.Now = visibility.VisibleByObserver[observer].NextUpdateTime.Add(time.Second)
	vision.Update(w, 0)
	generation := chunk.Spatial().Generation()
	batches := make(chan *ecs.EntitySpawnBatchEvent, 4)
	bus.SubscribeAsync(ecs.TopicGameplayEntitySpawnBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		batches <- event.(*ecs.EntitySpawnBatchEvent)
		return nil
	})
	state, err := json.Marshal(components.ObjectStateEnvelope{
		Version: 1, Behaviors: map[string]json.RawMessage{"wild_weed": json.RawMessage(fmt.Sprintf(
			`{"spot_id":123,"stage":0,"next_growth_runtime_seconds":%d}`, preparedAt+contracts.WildWeedGrowthSeconds))},
	})
	require.NoError(t, err)
	raw := &repository.Object{
		ID: 9010, TypeID: 41, Region: 1, X: 330, Y: 300, Quality: 16,
		Hp: sql.NullFloat64{Float64: 100, Valid: true}, Data: pqtype.NullRawMessage{RawMessage: state, Valid: true},
	}
	require.NoError(t, db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
		ID: raw.ID, TypeID: raw.TypeID, Region: raw.Region, X: raw.X, Y: raw.Y, Quality: raw.Quality, Hp: raw.Hp, Data: raw.Data,
	}))
	require.NoError(t, cm.PinPersistence(types.ChunkCoord{}))
	require.NoError(t, cm.ReplaceCommittedSource(raw))
	require.False(t, w.Alive(w.GetHandleByEntityID(9010)), "publication first installs committed raw data")
	cm.Update(0)
	cm.UnpinPersistence(types.ChunkCoord{})
	plant := w.GetHandleByEntityID(9010)
	require.True(t, w.Alive(plant), "the normal update activates inserted raw data in an already active chunk")
	require.Empty(t, chunk.GetRawObjects())
	require.Greater(t, chunk.Spatial().Generation(), generation)
	assertRefillWeedStage(t, w, plant, 0, preparedAt+contracts.WildWeedGrowthSeconds)
	ts.Now = visibility.VisibleByObserver[observer].NextUpdateTime.Add(time.Second)
	vision.Update(w, 0)
	require.Equal(t, types.EntityID(9010), visibility.VisibleByObserver[observer].Known[plant])
	select {
	case batch := <-batches:
		require.Equal(t, types.EntityID(9000), batch.ObserverID)
		require.Equal(t, []ecs.SpawnBatchEntry{{EntityID: 9010, Handle: plant}}, batch.Entries)
	case <-time.After(time.Second):
		t.Fatal("normal vision did not publish the new stem to the stationary player")
	}
	position, _ := ecs.GetComponent[components.Transform](w, observer)
	require.Equal(t, float64(300), position.X)
	require.Equal(t, float64(300), position.Y)
	ticks := systems.NewBehaviorTickSystem(zap.NewNop(), systems.BehaviorTickSystemConfig{BehaviorRegistry: registry})
	appearance := systems.NewObjectBehaviorSystem(bus, zap.NewNop(), systems.ObjectBehaviorConfig{BehaviorRegistry: registry})
	// An arbitrary wall-time jump, including time while the server is stopped,
	// does not age a newly prepared stem or consume its runtime deadline.
	ts.WallNow = ts.WallNow.AddDate(10, 0, 0)
	ts.UnixMs = ts.WallNow.UnixMilli()
	ts.Tick = 100_000_000
	ticks.Update(w, 0)
	assertRefillWeedStage(t, w, plant, 0, preparedAt+contracts.WildWeedGrowthSeconds)
	for stage := 1; stage <= 3; stage++ {
		ts.RuntimeSecondsTotal = preparedAt + int64(stage)*contracts.WildWeedGrowthSeconds - 1
		ticks.Update(w, 0)
		assertRefillWeedStage(t, w, plant, stage-1, preparedAt+int64(stage)*contracts.WildWeedGrowthSeconds)
		ts.RuntimeSecondsTotal++
		ticks.Update(w, 0)
		appearance.Update(w, 0)
		next := int64(0)
		if stage < 3 {
			next = preparedAt + int64(stage+1)*contracts.WildWeedGrowthSeconds
		}
		assertRefillWeedStage(t, w, plant, stage, next)
	}
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
}

func assertRefillWeedStage(t *testing.T, w *ecs.World, plant types.Handle, stage int, nextGrowth int64) {
	t.Helper()
	internal, exists := ecs.GetComponent[components.ObjectInternalState](w, plant)
	require.True(t, exists)
	state, exists := components.GetBehaviorState[components.WildWeedBehaviorState](internal, "wild_weed")
	require.True(t, exists)
	require.Equal(t, 123, int(state.SpotID))
	require.Equal(t, stage, state.Stage)
	require.Equal(t, nextGrowth, state.NextGrowthRuntimeSeconds)
	require.True(t, internal.HasHP)
	require.Equal(t, float64(100), internal.HP)
	info, exists := ecs.GetComponent[components.EntityInfo](w, plant)
	require.True(t, exists)
	require.Equal(t, uint32(41), info.TypeID)
	require.Equal(t, uint32(16), info.Quality)
	actualID, exists := w.GetExternalID(plant)
	require.True(t, exists)
	require.Equal(t, types.EntityID(9010), actualID)
	collider, exists := ecs.GetComponent[components.Collider](w, plant)
	require.True(t, exists, "passable stages retain the collider used by ordinary object damage")
	require.Equal(t, float64(2), collider.HalfWidth)
	require.Equal(t, float64(2), collider.HalfHeight)
	mask := uint64(0)
	if stage == 3 {
		mask = 1
	}
	require.Equal(t, mask, collider.Layer)
	require.Equal(t, mask, collider.Mask)
	appearance, exists := ecs.GetComponent[components.Appearance](w, plant)
	require.True(t, exists)
	require.Equal(t, fmt.Sprintf("wild_windsown_weed/%d", stage), appearance.Resource)
}
