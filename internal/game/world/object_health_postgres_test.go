package world

import (
	"context"
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"
	"origin/internal/types"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newObjectHealthPostgres(t *testing.T) *persistence.Postgres {
	t.Helper()
	return testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", filepath.Join("..", "..", "..", "migrations", "schema.sql"))
}

func objectHealthUpsertParams(id int64, hp sql.NullFloat64) repository.UpsertObjectParams {
	return repository.UpsertObjectParams{ID: id, TypeID: objectHealthLifecycleTypeID, Region: 1, X: 10, Y: 10, Quality: 10, Hp: hp}
}

func TestObjectHealthPostgresRoundtripCheckAndMigration(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	for index, hp := range []float64{.6, .49, float64(81) / 13, 0, float64(math.MaxInt32) + .5} {
		id := int64(9910 + index)
		require.NoError(t, db.Queries().UpsertObject(ctx, objectHealthUpsertParams(id, sql.NullFloat64{Float64: hp, Valid: true})))
		loaded, err := db.Queries().GetObjectByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, sql.NullFloat64{Float64: hp, Valid: true}, loaded.Hp)
		w := ecs.NewWorldForTesting()
		h, err := NewObjectFactory(nil).Build(w, &loaded, nil)
		require.NoError(t, err)
		health, ok := ecs.GetComponent[components.ObjectInternalState](w, h)
		require.True(t, ok)
		require.True(t, health.HasHP)
		require.Equal(t, hp, health.HP)
	}
	for _, hp := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		err := db.Queries().UpsertObject(ctx, objectHealthUpsertParams(9920, sql.NullFloat64{Float64: hp, Valid: true}))
		var postgresError *pgconn.PgError
		require.ErrorAs(t, err, &postgresError)
		require.Equal(t, "23514", postgresError.Code)
	}
	nullParams := objectHealthUpsertParams(9921, sql.NullFloat64{})
	nullParams.TypeID = 1000 // Special dropped items intentionally have no object health.
	require.NoError(t, db.Queries().UpsertObject(ctx, nullParams))
	dropped, err := db.Queries().GetObjectByID(ctx, 9921)
	require.NoError(t, err)
	require.False(t, dropped.Hp.Valid)
	nullParams.ID, nullParams.TypeID = 9922, objectHealthLifecycleTypeID
	require.NoError(t, db.Queries().UpsertObject(ctx, nullParams))
	missing, err := db.Queries().GetObjectByID(ctx, 9922)
	require.NoError(t, err)
	w := ecs.NewWorldForTesting()
	h, err := NewObjectFactory(nil).Build(w, &missing, nil)
	require.NoError(t, err)
	state, ok := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.True(t, ok)
	require.True(t, state.HasHP)
	require.Equal(t, 100.0, state.HP)
	missing, err = db.Queries().GetObjectByID(ctx, missing.ID)
	require.NoError(t, err)
	require.False(t, missing.Hp.Valid, "loading default HP must not backfill the database")

	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "20261007_fractional_object_health.sql"))
	require.NoError(t, err)
	migrationSQL := strings.ReplaceAll(string(migration), "origin.object", "object")
	for range 2 {
		_, err = db.Pool().Exec(ctx, migrationSQL)
		require.NoError(t, err)
	}
	loaded, err := db.Queries().GetObjectByID(ctx, 9910)
	require.NoError(t, err)
	require.Equal(t, .6, loaded.Hp.Float64, "reapplying migration must retain fractional values")
	dropped, err = db.Queries().GetObjectByID(ctx, 9921)
	require.NoError(t, err)
	require.False(t, dropped.Hp.Valid)
}

func TestObjectHealthPostgresChunkCaptureErrorPreservesAuthoritativeState(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	db := newObjectHealthPostgres(t)
	w := ecs.NewWorldForTesting()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	valid := spawnObjectHealthLifecycleFixture(t, w, chunk, 9931, objectHealthLifecycleTypeID, .6)
	invalid := spawnObjectHealthLifecycleFixture(t, w, chunk, 9932, objectHealthLifecycleTypeID, .49)
	require.NoError(t, db.Queries().UpsertObject(context.Background(), objectHealthUpsertParams(9932, sql.NullFloat64{Float64: 50, Valid: true})))
	stale := &repository.Object{ID: 9932, TypeID: objectHealthLifecycleTypeID, Region: 1, Quality: 10, Hp: sql.NullFloat64{Float64: 99, Valid: true}}
	chunk.SetRawObjects([]*repository.Object{stale})
	chunk.SetRawDirtyObjectIDs(map[types.EntityID]struct{}{9932: {}})
	chunk.MarkRawDataDirty()
	ecs.WithComponent(w, invalid, func(health *components.ObjectInternalState) { health.HP = math.NaN() })
	factory := NewObjectFactory(nil)
	err := chunk.SaveToDB(db, w, factory, zap.NewNop())
	require.ErrorIs(t, err, ErrInvalidObjectHP)
	loaded, err := db.Queries().GetObjectByID(context.Background(), 9931)
	require.NoError(t, err)
	require.Equal(t, .6, loaded.Hp.Float64, "other valid entities must still be saved")
	loaded, err = db.Queries().GetObjectByID(context.Background(), 9932)
	require.NoError(t, err)
	require.Equal(t, 50.0, loaded.Hp.Float64, "invalid active entity must never fall back to stale raw cache")
	for _, h := range []types.Handle{valid, invalid} {
		state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
		require.True(t, state.IsDirty)
	}
	require.True(t, chunk.IsDirty(w))
	require.Contains(t, chunk.GetRawDirtyObjectIDs(), types.EntityID(9932))
	require.NoError(t, SetObjectHP(w, invalid, .49))
	require.NoError(t, chunk.SaveToDB(db, w, factory, zap.NewNop()))
	loaded, err = db.Queries().GetObjectByID(context.Background(), 9932)
	require.NoError(t, err)
	require.Equal(t, .49, loaded.Hp.Float64)
	require.False(t, chunk.IsDirty(w))
	for _, h := range []types.Handle{valid, invalid} {
		state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
		require.False(t, state.IsDirty)
	}
}

func TestObjectHealthPostgresChunkWriteAndTileFailuresRetry(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	db := newObjectHealthPostgres(t)
	w := ecs.NewWorldForTesting()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	h := spawnObjectHealthLifecycleFixture(t, w, chunk, 9941, objectHealthLifecycleTypeID, .6)
	factory := NewObjectFactory(nil)
	ctx := context.Background()
	_, err := db.Pool().Exec(ctx, `ALTER TABLE object ADD CONSTRAINT fixture_hp_failure CHECK (hp <> 0.6)`)
	require.NoError(t, err)
	require.Error(t, chunk.SaveToDB(db, w, factory, zap.NewNop()))
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.True(t, state.IsDirty)
	_, err = db.Pool().Exec(ctx, `ALTER TABLE object DROP CONSTRAINT fixture_hp_failure`)
	require.NoError(t, err)
	require.NoError(t, chunk.SaveToDB(db, w, factory, zap.NewNop()))
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	require.False(t, state.IsDirty)

	chunk.SetTile(0, 0, types.TileGrass)
	require.NoError(t, SetObjectHP(w, h, .49))
	_, err = db.Pool().Exec(ctx, `ALTER TABLE chunk ADD CONSTRAINT fixture_tile_failure CHECK (entity_count < 0)`)
	require.NoError(t, err)
	require.Error(t, chunk.SaveToDB(db, w, factory, zap.NewNop()))
	require.True(t, chunk.TilesDirty())
	loaded, err := db.Queries().GetObjectByID(ctx, 9941)
	require.NoError(t, err)
	require.Equal(t, .49, loaded.Hp.Float64, "tile failure must not skip other valid writes")
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	require.True(t, state.IsDirty)
	_, err = db.Pool().Exec(ctx, `ALTER TABLE chunk DROP CONSTRAINT fixture_tile_failure`)
	require.NoError(t, err)
	require.NoError(t, chunk.SaveToDB(db, w, factory, zap.NewNop()))
	require.False(t, chunk.TilesDirty())
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	require.False(t, state.IsDirty)
}

func TestObjectHealthPostgresSaveWorkerRetainsChunkUntilRetry(t *testing.T) {
	db := newObjectHealthPostgres(t)
	cfg := newTestConfig()
	cfg.Game.LoadWorkers, cfg.Game.SaveWorkers = 0, 0
	cm := NewChunkManager(cfg, db, ecs.NewWorldForTesting(), nil, 0, 1, NewObjectFactory(nil), nil, nil, zap.NewNop())
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateInactive)
	cm.chunks[chunk.Coord] = chunk
	raw := &repository.Object{ID: 9951, TypeID: objectHealthLifecycleTypeID, Region: 1, Quality: 10, Hp: sql.NullFloat64{Float64: -1, Valid: true}}
	chunk.SetRawObjects([]*repository.Object{raw})
	chunk.SetRawDirtyObjectIDs(map[types.EntityID]struct{}{9951: {}})
	chunk.MarkRawDataDirty()
	cm.safeSaveAndRemove(chunk.Coord, chunk)
	require.Same(t, chunk, cm.GetChunkFast(chunk.Coord))
	require.True(t, chunk.IsDirty(cm.world))
	require.Contains(t, cm.saveRetries, chunk.Coord)
	raw.Hp.Float64 = .6
	cm.safeSaveAndRemove(chunk.Coord, chunk)
	require.Nil(t, cm.GetChunkFast(chunk.Coord))
	loaded, err := db.Queries().GetObjectByID(context.Background(), 9951)
	require.NoError(t, err)
	require.Equal(t, .6, loaded.Hp.Float64)
}

func TestObjectHealthPostgresRawWriteErrorStillSavesFollowingValidObject(t *testing.T) {
	db := newObjectHealthPostgres(t)
	w := ecs.NewWorldForTesting()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateInactive)
	invalid := &repository.Object{ID: 9952, TypeID: objectHealthLifecycleTypeID, Region: 1, Quality: 10, Hp: sql.NullFloat64{Float64: -1, Valid: true}}
	valid := &repository.Object{ID: 9953, TypeID: objectHealthLifecycleTypeID, Region: 1, Quality: 10, Hp: sql.NullFloat64{Float64: .6, Valid: true}}
	chunk.SetRawObjects([]*repository.Object{invalid, valid})
	chunk.SetRawDirtyObjectIDs(map[types.EntityID]struct{}{9952: {}, 9953: {}})
	chunk.MarkRawDataDirty()
	require.Error(t, chunk.SaveToDB(db, w, NewObjectFactory(nil), zap.NewNop()))
	loaded, err := db.Queries().GetObjectByID(context.Background(), 9953)
	require.NoError(t, err)
	require.Equal(t, .6, loaded.Hp.Float64)
	require.Contains(t, chunk.GetRawDirtyObjectIDs(), types.EntityID(9952))
	require.Contains(t, chunk.GetRawDirtyObjectIDs(), types.EntityID(9953))
}

func TestObjectHealthPostgresChunkLoadDeactivateAndReload(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	_, err := db.Queries().UpsertChunk(ctx, repository.UpsertChunkParams{Region: 1, TilesData: make([]byte, 128*128)})
	require.NoError(t, err)
	for id, hp := range map[int64]float64{9954: .6, 9955: 0} {
		require.NoError(t, db.Queries().UpsertObject(ctx, objectHealthUpsertParams(id, sql.NullFloat64{Float64: hp, Valid: true})))
	}
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	cm.chunks[chunk.Coord] = chunk
	require.NoError(t, chunk.LoadFromDB(db, 1, 0, zap.NewNop()))
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	for id, hp := range map[types.EntityID]float64{9954: .6, 9955: 0} {
		h := cm.world.GetHandleByEntityID(id)
		health, ok := ecs.GetComponent[components.ObjectInternalState](cm.world, h)
		require.True(t, ok)
		require.True(t, health.HasHP)
		require.Equal(t, hp, health.HP)
	}
	require.NoError(t, SetObjectHP(cm.world, cm.world.GetHandleByEntityID(9954), .49))
	require.NoError(t, cm.deactivateChunkInternal(chunk))
	require.NoError(t, chunk.SaveToDB(db, cm.world, cm.objectFactory, zap.NewNop()))
	require.NoError(t, chunk.LoadFromDB(db, 1, 0, zap.NewNop()))
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	for id, hp := range map[types.EntityID]float64{9954: .49, 9955: 0} {
		h := cm.world.GetHandleByEntityID(id)
		health, ok := ecs.GetComponent[components.ObjectInternalState](cm.world, h)
		require.True(t, ok)
		require.True(t, health.HasHP)
		require.Equal(t, hp, health.HP)
	}
}

func TestObjectHealthPostgresShutdownReportsCaptureFailures(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	db := newObjectHealthPostgres(t)
	cfg := newTestConfig()
	cfg.Game.LoadWorkers, cfg.Game.SaveWorkers = 0, 2
	logCore, logs := observer.New(zap.InfoLevel)
	w := ecs.NewWorldForTesting()
	cm := NewChunkManager(cfg, db, w, nil, 0, 1, NewObjectFactory(nil), nil, nil, zap.New(logCore))
	for index, hp := range []float64{.6, .49} {
		coord := types.ChunkCoord{X: index}
		chunk := core.NewChunk(coord, 1, 0, 128)
		chunk.SetState(types.ChunkStateActive)
		cm.chunks[coord] = chunk
		h := spawnObjectHealthLifecycleFixture(t, w, chunk, types.EntityID(9961+index), objectHealthLifecycleTypeID, hp)
		if index == 1 {
			ecs.WithComponent(w, h, func(health *components.ObjectInternalState) { health.HP = math.NaN() })
		}
	}
	cm.Stop()
	report := logs.FilterMessage("chunk manager stopped")
	require.Equal(t, 1, report.Len())
	fields := report.All()[0].ContextMap()
	require.EqualValues(t, 1, fields["chunks_saved"])
	require.EqualValues(t, 1, fields["chunks_failed"])
	loaded, err := db.Queries().GetObjectByID(context.Background(), 9961)
	require.NoError(t, err)
	require.Equal(t, .6, loaded.Hp.Float64)
	_, err = db.Queries().GetObjectByID(context.Background(), 9962)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
