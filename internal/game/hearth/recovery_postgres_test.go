package hearth_test

import (
	"context"
	"database/sql"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/hearth"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type recoveryReader struct {
	*repository.Queries
	homeReads, savedReads int
}

func (r *recoveryReader) GetCharacterHearth(ctx context.Context, params repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
	r.homeReads++
	return r.Queries.GetCharacterHearth(ctx, params)
}

func (r *recoveryReader) GetSavedCharacterHearth(ctx context.Context, character int64) (repository.GetSavedCharacterHearthRow, error) {
	r.savedReads++
	return r.Queries.GetSavedCharacterHearth(ctx, character)
}

type recoveryRepairSaver struct {
	hearth.ConstructionSaver
	repairs int
	keys    []hearth.ObjectKey
}

func (s *recoveryRepairSaver) RepairInvalidHome(ctx context.Context, character types.EntityID, key hearth.ObjectKey, hearthType int) error {
	s.repairs++
	s.keys = append(s.keys, key)
	return s.ConstructionSaver.RepairInvalidHome(ctx, character, key, hearthType)
}

type recoveryInterruptedSaver struct {
	hearth.ConstructionSaver
}

const recoveryProcessLoss = "process lost before hearth persistence"

func (recoveryInterruptedSaver) SaveConstruction(context.Context, types.EntityID, *repository.Object, *repository.Object) error {
	panic(recoveryProcessLoss)
}

func (recoveryInterruptedSaver) SaveTerminal(context.Context, types.EntityID, hearth.ObjectKey, *repository.Object) error {
	panic(recoveryProcessLoss)
}

func TestHearthRecoveryPostgresRepairsOnlyRequestedInvalidAssociation(t *testing.T) {
	for _, source := range []string{"missing", "deleted", "unsupported_type", "campfire", "wrong_layer", "deleted_owner"} {
		t.Run(source, func(t *testing.T) {
			db := newHearthPostgres(t, false)
			ctx := context.Background()
			q := db.Queries()
			for _, character := range []int64{1, 2, 3} {
				insertHearthCharacter(t, db, character)
			}
			insertHearthObject(t, db, 1, 10)
			insertHearthObject(t, db, 2, 10)
			insertHearthObject(t, db, 1, 11)
			for _, params := range []repository.SetCharacterHearthParams{
				hearthSetParams(1, 1, 10), hearthSetParams(2, 2, 10), hearthSetParams(3, 1, 11),
			} {
				rows, err := q.SetCharacterHearth(ctx, params)
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
			}
			_, err := db.Pool().Exec(ctx, "UPDATE object SET type_id = 16 WHERE region = 1 AND id = 11")
			require.NoError(t, err)
			switch source {
			case "missing":
				// The production FK prevents this state; relax only this isolated
				// fixture to verify recovery never borrows another region's row.
				_, err = db.Pool().Exec(ctx, "ALTER TABLE character_hearth DROP CONSTRAINT character_hearth_object_fk")
				require.NoError(t, err)
				_, err = db.Pool().Exec(ctx, "DELETE FROM object WHERE region = 1 AND id = 10")
			case "deleted":
				_, err = db.Pool().Exec(ctx, "UPDATE object SET deleted_at = now() WHERE region = 1 AND id = 10")
			case "unsupported_type":
				_, err = db.Pool().Exec(ctx, "UPDATE object SET type_id = 1 WHERE region = 1 AND id = 10")
			case "campfire":
				_, err = db.Pool().Exec(ctx, "UPDATE object SET type_id = 16 WHERE region = 1 AND id = 10")
			case "wrong_layer":
				_, err = db.Pool().Exec(ctx, "UPDATE object SET layer = 1 WHERE region = 1 AND id = 10")
			case "deleted_owner":
				_, err = db.Pool().Exec(ctx, "UPDATE character SET deleted_at = now() WHERE id = 1")
			}
			require.NoError(t, err)
			var objectsBefore string
			require.NoError(t, db.Pool().QueryRow(ctx, "SELECT jsonb_agg(to_jsonb(o) ORDER BY region, id)::text FROM object o").Scan(&objectsBefore))
			reader := &recoveryReader{Queries: q}
			saver := &recoveryRepairSaver{ConstructionSaver: hearth.NewConstructionSaverDB(db)}
			runtime := hearth.NewRuntime(nil, nil, reader, nil, saver, nil, nil,
				zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic)))
			for range 2 {
				require.NoError(t, runtime.LoadHome(1, testHearthTypeID))
				require.Equal(t, hearth.Snapshot{State: hearth.NoHome}, runtime.Index.Snapshot(1))
				require.False(t, runtime.Index.Snapshot(1).CanReturn())
			}
			require.Equal(t, 1, reader.homeReads)
			require.Equal(t, 1, reader.savedReads)
			require.Equal(t, 1, saver.repairs, "cached NoHome must not repeat any repair I/O")
			require.Equal(t, []hearth.ObjectKey{{Region: 1, ID: 10}}, saver.keys)
			_, err = q.GetSavedCharacterHearth(ctx, 1)
			require.ErrorIs(t, err, sql.ErrNoRows)
			valid, err := q.GetCharacterHearth(ctx, hearthGetParams(2))
			require.NoError(t, err)
			require.Equal(t, 2, valid.ObjectRegion)
			require.Equal(t, int64(10), valid.ObjectID)
			unvisited, err := q.GetSavedCharacterHearth(ctx, 3)
			require.NoError(t, err, "lazy repair must not scan or repair other characters")
			require.Equal(t, int64(11), unvisited.ObjectID)
			var objectsAfter string
			require.NoError(t, db.Pool().QueryRow(ctx, "SELECT jsonb_agg(to_jsonb(o) ORDER BY region, id)::text FROM object o").Scan(&objectsAfter))
			require.Equal(t, objectsBefore, objectsAfter, "association repair must never recreate, upgrade or mutate a source object")
		})
	}
}

func TestHearthRecoveryPostgresUnsavedTerminalRestoresEarlierAliveHome(t *testing.T) {
	db := newHearthPostgres(t, false)
	f := newConstructionFixture(t)
	ctx := context.Background()
	q := db.Queries()
	insertHearthCharacter(t, db, 1)
	chunk, original := f.oldRaw(t, types.ChunkStateInactive)
	f.chunks.chunks[chunk.Coord] = chunk
	saver := hearth.NewConstructionSaverDB(db)
	require.NoError(t, saver.SaveConstruction(ctx, 1, original, nil))
	f.runtime = hearth.NewRuntime(f.w, f.chunks, q, f.factory, recoveryInterruptedSaver{ConstructionSaver: saver}, f.registry, nil, nil)
	require.NoError(t, f.runtime.LoadHome(1, f.result.DefID))
	require.NoError(t, f.runtime.NotifyTerminal(1))
	require.PanicsWithValue(t, recoveryProcessLoss, func() { _, _ = f.runtime.CompleteTerminal(1) })
	require.True(t, f.runtime.Index.Snapshot(1).Terminal)
	require.Equal(t, 16, chunk.GetRawObjectByID(types.EntityID(original.ID)).TypeID)

	// A fresh process has neither the terminal index entry nor the unsaved raw
	// campfire. The independently saved living character and home may return.
	character, err := q.GetCharacter(ctx, 1)
	require.NoError(t, err)
	require.False(t, character.DeletedAt.Valid)
	stored, err := q.GetObjectByID(ctx, original.ID)
	require.NoError(t, err)
	require.Equal(t, f.result.DefID, stored.TypeID)
	restarted := hearth.NewRuntime(ecs.NewWorldForTesting(), nil, q, f.factory, saver, f.registry, nil, nil)
	require.NoError(t, restarted.LoadHome(1, f.result.DefID))
	require.Equal(t, hearth.Snapshot{State: hearth.CurrentHome, Home: restoredHome(&stored)}, restarted.Index.Snapshot(1))
	require.True(t, restarted.Index.Snapshot(1).CanReturn())
	owner, owned := restarted.Index.Owner(restoredHome(&stored).Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
}

func TestHearthRecoveryPostgresCommittedHomeKeepsIndependentCharacterAndMaterialSnapshots(t *testing.T) {
	db := newHearthPostgres(t, false)
	f := newConstructionFixture(t)
	ctx := context.Background()
	q := db.Queries()
	insertHearthCharacter(t, db, 1)
	_, err := db.Pool().Exec(ctx, "UPDATE character SET x = 17, y = 23, stamina = 75 WHERE id = 1")
	require.NoError(t, err)
	const materials = `{"kind":0,"key":0,"width":4,"height":4,"v":1,"items":[{"item_id":9000,"type_id":3004,"quality":10,"quantity":1}]}`
	_, err = db.Pool().Exec(ctx, "INSERT INTO inventory (owner_id, kind, inventory_key, data, version) VALUES (1, 0, 0, $1, 1)", materials)
	require.NoError(t, err)
	before, err := q.GetCharacter(ctx, 1)
	require.NoError(t, err)
	ecs.AddComponent(f.w, f.ctx.PlayerHandle, components.EntityStats{Stamina: 50})
	saver := hearth.NewConstructionSaverDB(db)
	f.runtime = hearth.NewRuntime(f.w, f.chunks, q, f.factory, saver, f.registry, nil, nil)
	costCalls := 0
	result := f.runtime.CompleteConstruction(&f.ctx, func() bool {
		costCalls++
		// The ordinary final-cycle callback has already paid its runtime cost;
		// its character/material save need not share the Hearth checkpoint.
		ecs.MutateComponent[components.EntityStats](f.w, f.ctx.PlayerHandle, func(stats *components.EntityStats) bool {
			stats.Stamina -= 8
			return true
		})
		return f.finishSite()
	})
	require.True(t, result.OK)
	require.Equal(t, 1, costCalls)
	stats, present := ecs.GetComponent[components.EntityStats](f.w, f.ctx.PlayerHandle)
	require.True(t, present)
	require.Equal(t, float64(42), stats.Stamina)

	freshWorld := ecs.NewWorldForTesting()
	restarted := hearth.NewRuntime(freshWorld, nil, q, f.factory, saver, f.registry, nil, nil)
	chunk := loadHearthRestorationPostgresChunk(t, q)
	for range 2 {
		require.NoError(t, restarted.LoadHome(1, f.result.DefID))
		require.NoError(t, restarted.PrepareChunkActivation(chunk))
	}
	home := restarted.Index.Snapshot(1)
	require.True(t, home.CanReturn())
	require.Equal(t, f.ctx.TargetID, home.Home.Object.ID)
	stored := chunk.GetRawObjectByID(f.ctx.TargetID)
	require.NotNil(t, stored)
	require.Equal(t, f.result.DefID, stored.TypeID)
	require.False(t, stored.Data.Valid, "committed Hearth has no build state to replay")
	after, err := q.GetCharacter(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, before, after, "restoration must keep the earlier position and stamina checkpoint")
	var savedMaterials string
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT data::text FROM inventory WHERE owner_id = 1 AND kind = 0 AND inventory_key = 0").Scan(&savedMaterials))
	require.JSONEq(t, materials, savedMaterials, "recovery does not charge old saved materials to reconcile the newer Hearth")
	require.Equal(t, 1, costCalls)
}

func TestHearthRecoveryPostgresLostCompletionKeepsIncompleteBuildSite(t *testing.T) {
	db := newHearthPostgres(t, false)
	f := newConstructionFixture(t)
	ctx := context.Background()
	q := db.Queries()
	insertHearthCharacter(t, db, 1)
	progress := &components.BuildBehaviorState{
		BuildKey: "hearth_fire", BuildDefID: 6, ObjectKey: "hearth_fire", ObjectTypeID: uint32(f.result.DefID), TargetX: 50, TargetY: 60,
		Items: []components.BuildRequiredItemState{{ItemKey: "branch", RequiredCount: 2, BuildCount: 1,
			PutItems: []components.BuildPutItemState{{ItemKey: "branch", Quality: 10, Count: 1}}}},
	}
	ecs.MutateComponent[components.ObjectInternalState](f.w, f.ctx.TargetHandle, func(state *components.ObjectInternalState) bool {
		state.HP, state.HasHP = 100, true
		components.SetBehaviorState(state, "build", progress)
		return true
	})
	site, err := f.factory.Serialize(f.w, f.ctx.TargetHandle)
	require.NoError(t, err)
	require.NotNil(t, site)
	require.NoError(t, q.UpsertObject(ctx, repository.UpsertObjectParams{
		ID: site.ID, TypeID: site.TypeID, Region: site.Region, X: site.X, Y: site.Y, Layer: site.Layer,
		ChunkX: site.ChunkX, ChunkY: site.ChunkY, Hp: site.Hp, Data: site.Data,
	}))
	saver := hearth.NewConstructionSaverDB(db)
	f.runtime = hearth.NewRuntime(f.w, f.chunks, q, f.factory, recoveryInterruptedSaver{ConstructionSaver: saver}, f.registry, nil, nil)
	require.PanicsWithValue(t, recoveryProcessLoss, func() { f.runtime.CompleteConstruction(&f.ctx, f.finishSite) })
	require.Equal(t, hearth.CurrentHome, f.runtime.Index.Snapshot(1).State)

	freshWorld := ecs.NewWorldForTesting()
	restarted := hearth.NewRuntime(freshWorld, nil, q, f.factory, saver, f.registry, nil, nil)
	chunk := loadHearthRestorationPostgresChunk(t, q)
	require.NoError(t, restarted.LoadHome(1, f.result.DefID))
	require.NoError(t, restarted.PrepareChunkActivation(chunk))
	raw := chunk.GetRawObjectByID(f.ctx.TargetID)
	require.NotNil(t, raw)
	require.EqualValues(t, constt.BuildObjectTypeID, raw.TypeID)
	require.JSONEq(t, string(site.Data.RawMessage), string(raw.Data.RawMessage))
	handle, err := f.factory.BuildForChunk(freshWorld, raw, nil)
	require.NoError(t, err)
	restored, err := f.factory.DeserializeObjectState(raw)
	require.NoError(t, err)
	state := components.ObjectInternalState{State: restored}
	savedProgress, ok := components.GetBehaviorState[components.BuildBehaviorState](state, "build")
	require.True(t, ok)
	require.Equal(t, progress, savedProgress, "the lost final branch must not be replayed")
	info, ok := ecs.GetComponent[components.EntityInfo](freshWorld, handle)
	require.True(t, ok)
	require.EqualValues(t, constt.BuildObjectTypeID, info.TypeID)
	require.Equal(t, hearth.Snapshot{State: hearth.NoHome}, restarted.Index.Snapshot(1))
	_, owned := restarted.Index.Owner(hearth.ObjectKey{Region: raw.Region, ID: types.EntityID(raw.ID)})
	require.False(t, owned, "recovery must not fabricate the lost finisher")
	_, err = q.GetSavedCharacterHearth(ctx, 1)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
