package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type skullPostgresClaimPersister struct {
	*gameworld.DroppedItemPersisterDB
	metadataCalls atomic.Int32
	claimCalls    atomic.Int32
	entered       chan struct{}
	release       chan struct{}
	enteredOnce   sync.Once
	ambiguous     bool
}

func (p *skullPostgresClaimPersister) LoadDeadCharacter(ctx context.Context, id types.EntityID) (inventory.DeadCharacterInfo, error) {
	p.metadataCalls.Add(1)
	return p.DroppedItemPersisterDB.LoadDeadCharacter(ctx, id)
}

func (p *skullPostgresClaimPersister) PersistSkullClaim(ctx context.Context, record inventory.SkullClaimPersistenceRecord) error {
	p.enteredOnce.Do(func() { close(p.entered) })
	select {
	case <-p.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	err := p.DroppedItemPersisterDB.PersistSkullClaim(ctx, record)
	if err == nil && p.claimCalls.Add(1) == 1 && p.ambiguous {
		return errors.New("simulate lost skull claim commit acknowledgement")
	}
	return err
}

func skullPostgresStartWorker(t *testing.T, f *skullClaimFixture) <-chan struct{} {
	t.Helper()
	f.shard.mu.Lock()
	f.service.Update()
	require.NotEmpty(t, f.chunks.jobs)
	job := f.chunks.jobs[0]
	f.chunks.jobs = f.chunks.jobs[1:]
	f.shard.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		job.Complete(job.Run())
	}()
	return done
}

func skullPostgresWait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("skull claim persistence worker did not complete")
	}
}

func TestSkullClaimPostgresServiceGrantRestartAndAmbiguousCommit(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		for _, recipientDies := range []bool{false, true} {
			t.Run(fmt.Sprintf("ambiguous_%t_recipient_dies_%t", ambiguous, recipientDies), func(t *testing.T) {
				f := newSkullClaimFixture(t)
				db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
				ctx := context.Background()
				_, err := db.Pool().Exec(ctx, `INSERT INTO account (id,login,password_hash) VALUES (1,'skull_service_test','unused')`)
				require.NoError(t, err)
				_, err = db.Pool().Exec(ctx, `INSERT INTO character
					(id,account_id,name,region,x,y,layer,heading,stamina,energy,shp,hhp,attributes,exp,skills,discovery,deleted_at)
					VALUES (1,1,$1,1,50,50,0,0,0,0,0,0,'{}','{}','[]','[]',$2)`,
					"Alice <b>literal</b>", time.Date(2026, time.January, 10, 12, 0, 0, 0, time.Local))
				require.NoError(t, err)
				persister := &skullPostgresClaimPersister{
					DroppedItemPersisterDB: gameworld.NewDroppedItemPersisterDB(db, zap.NewNop()),
					entered:                make(chan struct{}), release: make(chan struct{}), ambiguous: ambiguous,
				}
				f.service.deps.Persister = persister
				f.service.deps.WithWorldRead = f.shard.WithWorldRead
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(persister.release) }) }
				t.Cleanup(release)

				f.shard.mu.Lock()
				f.admit(t)
				f.shard.mu.Unlock()
				require.Zero(t, persister.metadataCalls.Load(), "admission must not read PostgreSQL on the shard tick")
				require.Zero(t, persister.claimCalls.Load())
				done := skullPostgresStartWorker(t, f)
				skullPostgresWait(t, persister.entered)
				// The database phase may block; another shard update still runs and
				// retains both reservations without exposing a tentative item.
				f.shard.mu.Lock()
				f.service.Update()
				root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
				require.Empty(t, root.Items)
				require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
				require.True(t, ecs.ObjectDestructionPending(f.w, f.target))
				f.shard.mu.Unlock()
				release()
				skullPostgresWait(t, done)

				// Inspect restart-visible state before the original shard applies
				// the completion, including the ambiguous-acknowledgement case.
				skeleton, err := db.Queries().GetObjectByID(ctx, 1)
				require.NoError(t, err)
				require.Equal(t, 18, skeleton.TypeID)
				require.Equal(t, int16(23), skeleton.Quality)
				var envelope components.ObjectStateEnvelope
				require.NoError(t, json.Unmarshal(skeleton.Data.RawMessage, &envelope))
				var receipt inventory.SkullClaimReceipt
				require.NoError(t, json.Unmarshal(envelope.Behaviors["player_skeleton"], &receipt))
				require.Equal(t, inventory.SkullClaimReceipt{SkullItemID: 100000, RecipientID: 500}, receipt)
				rows, err := db.Queries().GetInventoriesByOwner(ctx, 500)
				require.NoError(t, err)
				require.Len(t, rows, 1)
				var durable inventory.InventoryDataV1
				require.NoError(t, json.Unmarshal(rows[0].Data, &durable))
				require.Len(t, durable.Items, 1)
				require.Equal(t, uint64(100000), durable.Items[0].ItemID)
				require.Equal(t, &components.SkullMetadata{CharacterID: 1, Nickname: "Alice <b>literal</b>", DeathDate: "2026-01-10"}, durable.Items[0].Skull)
				require.Equal(t, int64(100000), db.GetGlobalVarLong(ctx, constt.LAST_USED_ID))

				restarted := ecs.NewWorldWithCapacity(64, nil, 0)
				factory := gameworld.NewObjectFactory(nil)
				restoredSkeleton, err := factory.BuildForChunk(restarted, &skeleton, nil)
				require.NoError(t, err)
				ecs.AddComponent(restarted, restoredSkeleton, components.ChunkRef{CurrentChunkX: skeleton.ChunkX, CurrentChunkY: skeleton.ChunkY})
				restoredState, err := factory.DeserializeObjectState(&skeleton)
				require.NoError(t, err)
				ecs.WithComponent(restarted, restoredSkeleton, func(state *components.ObjectInternalState) { state.State = restoredState })
				reserialized, err := factory.Serialize(restarted, restoredSkeleton)
				require.NoError(t, err)
				require.JSONEq(t, string(skeleton.Data.RawMessage), string(reserialized.Data.RawMessage))
				info, _ := ecs.GetComponent[components.EntityInfo](restarted, restoredSkeleton)
				require.Equal(t, uint32(18), info.TypeID)
				require.True(t, info.Indestructible)
				require.Equal(t, []string{"lift"}, info.Behaviors)
				require.False(t, ecs.HasComponent[components.InventoryOwner](restarted, restoredSkeleton))
				loader := inventory.NewInventoryLoader(zap.NewNop())
				parsed, warnings := loader.ParseInventoriesFromDB(rows)
				require.Empty(t, warnings)
				loaded, err := loader.LoadPlayerInventories(restarted, 500, parsed)
				require.NoError(t, err)
				require.Empty(t, loaded.Warnings)
				require.Len(t, loaded.ContainerHandles, 1)
				restoredInventory, exists := ecs.GetComponent[components.InventoryContainer](restarted, loaded.ContainerHandles[0])
				require.True(t, exists)
				require.Len(t, restoredInventory.Items, 1)
				require.Equal(t, types.EntityID(100000), restoredInventory.Items[0].ItemID)
				require.Equal(t, "Alice <b>literal</b> died on January 10, 2026", restoredInventory.Items[0].Skull.HintExt())
				require.Equal(t, 2, restarted.EntityCount(), "restart must contain exactly one skeleton and one root with one skull")

				f.shard.mu.Lock()
				if recipientDies {
					f.shard.convertPlayerEntityToCorpse(f.w, 500, f.recipient)
					info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.recipient)
					require.Equal(t, uint32(99), info.TypeID)
					require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
				}
				f.service.Update()
				if ambiguous {
					require.Equal(t, 1, f.service.PendingCount())
					require.True(t, ecs.InventoryOwnerReserved(f.w, 500))
					root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
					require.Empty(t, root.Items)
					ecs.GetResource[ecs.TimeState](f.w).Now = ecs.GetResource[ecs.TimeState](f.w).Now.Add(time.Second)
				}
				f.shard.mu.Unlock()
				if ambiguous {
					done = skullPostgresStartWorker(t, f)
					skullPostgresWait(t, done)
					f.shard.mu.Lock()
					f.service.Update()
					f.shard.mu.Unlock()
				}
				f.requireCompleted(t)
				require.Equal(t, int32(1), persister.metadataCalls.Load())
				require.Equal(t, map[bool]int32{false: 1, true: 2}[ambiguous], persister.claimCalls.Load())
				require.Equal(t, 1, f.ids.calls)
				currentRows, err := db.Queries().GetInventoriesByOwner(ctx, 500)
				require.NoError(t, err)
				require.Len(t, currentRows, 1)
				require.Equal(t, rows[0].Version, currentRows[0].Version)
				require.JSONEq(t, string(rows[0].Data), string(currentRows[0].Data))
				objects, err := db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{Region: 1})
				require.NoError(t, err)
				require.Len(t, objects, 1)
				require.Equal(t, int64(1), objects[0].ID)
				require.Equal(t, 18, objects[0].TypeID)
			})
		}
	}
}
