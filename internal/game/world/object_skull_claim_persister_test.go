package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func installSkullClaimDefinitions(t *testing.T) {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 17, Key: "player_skeleton", HP: 100, Indestructible: true, IsStatic: true},
		{DefID: 18, Key: "player_skeleton_without_skull", HP: 100, Indestructible: true, IsStatic: true},
	}))
}

func skullClaimRecord() inventory.SkullClaimPersistenceRecord {
	source := &repository.Object{
		ID: 41, TypeID: 17, Region: 1, X: 23, Y: 29, Layer: 2, ChunkX: 3, ChunkY: 4,
		Heading: sql.NullInt16{Int16: 90, Valid: true}, Quality: 27,
		Hp: sql.NullFloat64{Float64: 100, Valid: true}, CreateTick: 7, LastTick: 21600,
	}
	replacement := *source
	replacement.TypeID = 18
	replacement.Data = pqtype.NullRawMessage{Valid: true, RawMessage: []byte(`{"v":1,"behaviors":{"player_skeleton":{"skull_item_id":10001,"recipient_id":73}}}`)}
	return inventory.SkullClaimPersistenceRecord{
		Source: source, Replacement: &replacement, RecipientID: 73, SkullItemID: 10001,
		PlayerInventories: []systems.InventorySnapshot{
			{CharacterID: 73, Kind: int16(constt.InventoryGrid), InventoryKey: 0, Version: 2,
				Data: []byte(`{"kind":0,"key":0,"v":2,"width":4,"height":4,"items":[{"item_id":10001,"type_id":3014,"quality":27,"quantity":1,"skull":{"character_id":41,"nickname":"Alice","death_date":"2026-01-10"}}]}`)},
			{CharacterID: 73, Kind: int16(constt.InventoryHand), InventoryKey: 0, Version: 2, Data: []byte(`{"kind":1,"key":0,"v":2,"items":[]}`)},
			{CharacterID: 73, Kind: int16(constt.InventoryEquipment), InventoryKey: 0, Version: 2, Data: []byte(`{"kind":2,"key":0,"v":2,"items":[]}`)},
		},
	}
}

func persistSkullTestSource(t *testing.T, p *DroppedItemPersisterDB, source *repository.Object) {
	t.Helper()
	require.NoError(t, p.db.Queries().UpsertObject(context.Background(), repository.UpsertObjectParams{
		ID: source.ID, TypeID: source.TypeID, Region: source.Region, X: source.X, Y: source.Y,
		Layer: source.Layer, ChunkX: source.ChunkX, ChunkY: source.ChunkY, Heading: source.Heading,
		Quality: source.Quality, Hp: source.Hp, OwnerID: source.OwnerID, Data: source.Data,
		CreateTick: source.CreateTick, LastTick: source.LastTick,
	}))
}

func TestSkullClaimPostgresAtomicGrantAndReceiptReplay(t *testing.T) {
	installSkullClaimDefinitions(t)
	for _, sourceSaved := range []bool{true, false} {
		t.Run(fmt.Sprintf("source_saved_%t", sourceSaved), func(t *testing.T) {
			db := newObjectHealthPostgres(t)
			ctx := context.Background()
			persister := NewDroppedItemPersisterDB(db, zap.NewNop())
			record := skullClaimRecord()
			if sourceSaved {
				persistSkullTestSource(t, persister, record.Source)
			}
			require.NoError(t, persister.PersistSkullClaim(ctx, record))
			stored, err := db.Queries().GetObjectByID(ctx, record.Source.ID)
			require.NoError(t, err)
			require.Equal(t, 18, stored.TypeID)
			require.Equal(t, record.Replacement.X, stored.X)
			require.Equal(t, record.Replacement.Y, stored.Y)
			require.Equal(t, record.Replacement.Quality, stored.Quality)
			require.Equal(t, record.Replacement.Hp, stored.Hp)
			require.JSONEq(t, string(record.Replacement.Data.RawMessage), string(stored.Data.RawMessage))
			roots, err := db.Queries().GetInventoriesByOwner(ctx, int64(record.RecipientID))
			require.NoError(t, err)
			require.Len(t, roots, 3)
			for index, root := range roots {
				require.Equal(t, record.PlayerInventories[index].Version, root.Version)
				require.JSONEq(t, string(record.PlayerInventories[index].Data), string(root.Data))
			}
			require.Equal(t, int64(record.SkullItemID), db.GetGlobalVarLong(ctx, constt.LAST_USED_ID))
			// The first acknowledgement was lost. The recipient subsequently moved
			// the skull and the skeleton was moved too. Replay must touch neither.
			newerData := json.RawMessage(`{"items":[],"newer":true}`)
			_, err = db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{
				OwnerID: int64(record.RecipientID), Kind: int16(constt.InventoryGrid), Data: newerData, Version: 9,
			})
			require.NoError(t, err)
			stored.X = 100
			persistSkullTestSource(t, persister, &stored)
			require.NoError(t, persister.PersistSkullClaim(ctx, record))
			roots, err = db.Queries().GetInventoriesByOwner(ctx, int64(record.RecipientID))
			require.NoError(t, err)
			require.Equal(t, 9, roots[0].Version)
			require.JSONEq(t, string(newerData), string(roots[0].Data))
			stored, err = db.Queries().GetObjectByID(ctx, record.Source.ID)
			require.NoError(t, err)
			require.Equal(t, 100, stored.X)
		})
	}
}

func TestSkullClaimPostgresRollsBackOnInventoryConflict(t *testing.T) {
	installSkullClaimDefinitions(t)
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	persister := NewDroppedItemPersisterDB(db, zap.NewNop())
	record := skullClaimRecord()
	// The grid writes first, then a newer hand version rejects the transaction.
	_, err := db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{
		OwnerID: int64(record.RecipientID), Kind: int16(constt.InventoryHand), Data: []byte(`{"must_survive":true}`), Version: 9,
	})
	require.NoError(t, err)
	require.ErrorIs(t, persister.PersistSkullClaim(ctx, record), inventory.ErrSkullClaimInventoryConflict)
	_, err = db.Queries().GetObjectByID(ctx, record.Source.ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "the newly inserted source must roll back too")
	roots, err := db.Queries().GetInventoriesByOwner(ctx, int64(record.RecipientID))
	require.NoError(t, err)
	require.Len(t, roots, 1)
	require.Equal(t, int16(constt.InventoryHand), roots[0].Kind)
	require.Equal(t, 9, roots[0].Version)
	require.JSONEq(t, `{"must_survive":true}`, string(roots[0].Data))
	require.Zero(t, db.GetGlobalVarLong(ctx, constt.LAST_USED_ID))
}

func TestSkullClaimPostgresReplacementFailureRollsBackGrant(t *testing.T) {
	installSkullClaimDefinitions(t)
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	persister := NewDroppedItemPersisterDB(db, zap.NewNop())
	record := skullClaimRecord()
	persistSkullTestSource(t, persister, record.Source)
	record.Replacement.Heading = sql.NullInt16{Int16: 360, Valid: true}
	require.Error(t, persister.PersistSkullClaim(ctx, record))
	roots, err := db.Queries().GetInventoriesByOwner(ctx, int64(record.RecipientID))
	require.NoError(t, err)
	require.Empty(t, roots)
	stored, err := db.Queries().GetObjectByID(ctx, record.Source.ID)
	require.NoError(t, err)
	require.Equal(t, 17, stored.TypeID)
	require.Zero(t, db.GetGlobalVarLong(ctx, constt.LAST_USED_ID))
}

func TestSkullClaimPostgresRejectsAlreadyClaimedOrDeletedSource(t *testing.T) {
	installSkullClaimDefinitions(t)
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprintf("deleted_%t", deleted), func(t *testing.T) {
			db := newObjectHealthPostgres(t)
			ctx := context.Background()
			persister := NewDroppedItemPersisterDB(db, zap.NewNop())
			record := skullClaimRecord()
			require.NoError(t, persister.PersistSkullClaim(ctx, record))
			if deleted {
				_, err := db.Queries().SoftDeleteObject(ctx, repository.SoftDeleteObjectParams{Region: 1, ID: record.Source.ID})
				require.NoError(t, err)
			} else {
				record.SkullItemID++
				record.Replacement.Data.RawMessage = []byte(`{"v":1,"behaviors":{"player_skeleton":{"skull_item_id":10002,"recipient_id":73}}}`)
			}
			require.ErrorIs(t, persister.PersistSkullClaim(ctx, record), inventory.ErrSkullClaimSourceConflict)
			require.Equal(t, int64(10001), db.GetGlobalVarLong(ctx, constt.LAST_USED_ID))
		})
	}
}

func TestSkullClaimPostgresConcurrentClaimHasOneWinner(t *testing.T) {
	installSkullClaimDefinitions(t)
	db := newObjectHealthPostgres(t)
	persister := NewDroppedItemPersisterDB(db, zap.NewNop())
	first, second := skullClaimRecord(), skullClaimRecord()
	second.SkullItemID = 10002
	second.RecipientID = 74
	second.Replacement.Data.RawMessage = []byte(`{"v":1,"behaviors":{"player_skeleton":{"skull_item_id":10002,"recipient_id":74}}}`)
	for index := range second.PlayerInventories {
		second.PlayerInventories[index].CharacterID = 74
	}
	second.PlayerInventories[0].Data = []byte(`{"items":[{"item_id":10002,"type_id":3014,"quantity":1}]}`)
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)
	for _, record := range []inventory.SkullClaimPersistenceRecord{first, second} {
		go func() {
			defer wg.Done()
			results <- persister.PersistSkullClaim(context.Background(), record)
		}()
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, inventory.ErrSkullClaimSourceConflict)
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	var rootCount int
	for _, recipient := range []int64{73, 74} {
		roots, err := db.Queries().GetInventoriesByOwner(context.Background(), recipient)
		require.NoError(t, err)
		rootCount += len(roots)
	}
	require.Equal(t, 3, rootCount)
}

func TestSkullClaimPostgresDeadCharacterMetadata(t *testing.T) {
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	persister := NewDroppedItemPersisterDB(db, zap.NewNop())
	oldLocation := time.Local
	time.Local = time.FixedZone("server-calendar", 3*60*60)
	t.Cleanup(func() { time.Local = oldLocation })
	_, err := db.Pool().Exec(ctx, `INSERT INTO account (id,login,password_hash) VALUES (1,'skull_test','unused')`)
	require.NoError(t, err)
	_, err = db.Pool().Exec(ctx, `INSERT INTO character
		(id,account_id,name,region,x,y,layer,heading,stamina,energy,shp,hhp,attributes,exp,skills,discovery,deleted_at)
		VALUES (41,1,'Alice',1,0,0,0,0,0,0,0,0,'{}','{}','[]','[]','2026-01-09 22:30:00+00'),
		       (42,1,'Living',1,0,0,0,0,0,0,100,100,'{}','{}','[]','[]',NULL)`)
	require.NoError(t, err)
	metadata, err := persister.LoadDeadCharacter(ctx, 41)
	require.NoError(t, err)
	require.Equal(t, inventory.DeadCharacterInfo{Nickname: "Alice", DeathDate: "2026-01-10"}, metadata)
	for _, id := range []types.EntityID{0, 42, 43} {
		_, err := persister.LoadDeadCharacter(ctx, id)
		require.ErrorIs(t, err, inventory.ErrSkullClaimCharacterMissing)
	}
	_, err = db.Queries().GetCharacter(ctx, 41)
	require.ErrorIs(t, err, sql.ErrNoRows, "ordinary character lookup must continue to exclude deleted rows")
}
