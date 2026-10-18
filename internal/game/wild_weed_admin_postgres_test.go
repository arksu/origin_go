package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type committedWildWeedAdminError struct {
	*gameworld.DroppedItemPersisterDB
	mu        sync.Mutex
	deletions []gameworld.WildWeedObjectDeletion
}

func (p *committedWildWeedAdminError) DeleteWildWeedObject(ctx context.Context, region int, deletion gameworld.WildWeedObjectDeletion) (gameworld.SpotStateCommit, error) {
	commit, err := p.DroppedItemPersisterDB.DeleteWildWeedObject(ctx, region, deletion)
	if err != nil {
		return commit, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deletions = append(p.deletions, deletion)
	if len(p.deletions) == 1 {
		// The transaction really committed; only its acknowledgement was lost.
		return gameworld.SpotStateCommit{}, errors.New("commit acknowledgement lost")
	}
	return commit, nil
}

func TestWildWeedAdminPostgresAmbiguousCommitCannotResurrect(t *testing.T) {
	for _, shutdownBeforeRetry := range []bool{false, true} {
		t.Run(fmt.Sprintf("shutdown_before_retry=%t", shutdownBeforeRetry), func(t *testing.T) {
			db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", "../../migrations/schema.sql")
			previousItems, previousObjects := itemdefs.Global(), objectdefs.Global()
			t.Cleanup(func() {
				itemdefs.SetGlobalForTesting(previousItems)
				objectdefs.SetGlobalForTesting(previousObjects)
			})
			itemdefs.SetGlobalForTesting(itemdefs.NewRegistry(nil))
			objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
				DefID: 41, Key: "wild_windsown_weed", HP: 100, Resource: "wild_windsown_weed/3", IsStatic: true,
				Behaviors: map[string]json.RawMessage{"wild_weed": json.RawMessage(`{}`)}, BehaviorOrder: []string{"wild_weed"},
				Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 4, H: 4, Layer: 1, Mask: 1}},
			}}))
			ctx := t.Context()
			_, err := db.Queries().UpsertChunk(ctx, repository.UpsertChunkParams{Region: 1, TilesData: make([]byte, constt.ChunkSize*constt.ChunkSize)})
			require.NoError(t, err)
			require.NoError(t, db.Queries().InsertSpots(ctx, repository.InsertSpotsParams{
				Region: 1, SpotTypes: []string{"www"}, DistrictXs: []int{0}, DistrictYs: []int{0},
				CenterXs: []int{50}, CenterYs: []int{50}, Radii: []int{64}, PeakQualities: []int16{37},
			}))
			spots, err := db.Queries().LoadSpotsByRegion(ctx, 1)
			require.NoError(t, err)
			require.Len(t, spots, 1)
			spotID := spots[0].ID
			_, err = db.Queries().SaveSpotState(ctx, repository.SaveSpotStateParams{
				ID: spotID, Region: 1, ExpectedRevision: 1,
				State: json.RawMessage(`{"www":{"objects":{"1":{"x":50,"y":50}}}}`),
			})
			require.NoError(t, err)
			require.NoError(t, db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
				ID: 1, TypeID: 41, Region: 1, X: 50, Y: 50, Quality: 17, Hp: sql.NullFloat64{Float64: 75, Valid: true},
				Data: pqtype.NullRawMessage{Valid: true, RawMessage: json.RawMessage(fmt.Sprintf(
					`{"v":1,"behaviors":{"wild_weed":{"spot_id":%d,"stage":3}}}`, spotID))},
			}))

			w := ecs.NewWorldWithCapacity(64, nil, 0)
			logCore, logs := observer.New(zap.InfoLevel)
			shard := &Shard{world: w, layer: 0, logger: zap.New(logCore)}
			cfg := &config.Config{Game: config.GameConfig{
				ChunkLRUCapacity: 10, ChunkLRUTTL: 60, LoadWorkers: 1, SaveWorkers: 1,
				WorldWidthChunks: 1, WorldHeightChunks: 1, Region: 1,
			}}
			cm := gameworld.NewChunkManager(cfg, db, w, shard, 0, 1, gameworld.NewObjectFactory(nil), nil, nil, shard.logger)
			shard.chunkManager = cm
			f := &objectDestructionIntegrationFixture{shard: shard, db: db, cfg: cfg, cm: cm, logs: logs}
			t.Cleanup(f.stop)
			shard.spotManager, err = gameworld.NewSpotManager(ctx, cfg, gameworld.NewSpotStoreDB(db), cm.SubmitPersistenceJob, shard.logger)
			require.NoError(t, err)
			persister := &committedWildWeedAdminError{DroppedItemPersisterDB: gameworld.NewDroppedItemPersisterDB(db, shard.logger)}
			minX, minY, maxX, maxY := cm.WorldBounds()
			shard.objectDestruction, err = NewObjectDestructionService(w, ObjectDestructionDependencies{
				Chunks: cm, Persister: persister, IDs: &destructionIDsFixture{next: 100000}, Items: itemdefs.Global(),
				WithWorldRead: shard.WithWorldRead, Quarantine: shard.quarantineDestroyedObject,
				SpotStateCommitted: shard.applyCommittedSpotState,
				MinX:               minX, MinY: minY, MaxX: maxX, MaxY: maxY, Region: 1, Logger: shard.logger,
			})
			require.NoError(t, err)
			*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Now: time.Now(), RuntimeSecondsTotal: 10}
			cm.RegisterEntity(9000, 50, 50, false)
			loadCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			require.NoError(t, cm.WaitPreloaded(loadCtx, types.ChunkCoord{}))
			f.eventually(t, func() bool { return w.Alive(w.GetHandleByEntityID(1)) })
			f.target = w.GetHandleByEntityID(1)

			chat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
			handler := NewChatAdminCommandHandler(nil, nil, chat, nil, nil, cm, nil, nil, nil, shard.logger)
			handler.SetObjectDeleter(wildWeedAdminDeleter{destruction: shard.objectDestruction, fallback: persister})
			chunk := cm.GetChunkFast(types.ChunkCoord{})
			func() {
				shard.mu.Lock()
				defer shard.mu.Unlock()
				require.NoError(t, shard.objectDestruction.PrepareTarget(f.target))
				ecs.MutateComponent(w, f.target, func(state *components.ObjectInternalState) bool {
					components.SetBehaviorState(state, "wild_weed", &components.WildWeedBehaviorState{
						SpotID: spotID, Stage: 0, NextGrowthRuntimeSeconds: 8 * 60 * 60,
					})
					return true
				})
				state, _ := ecs.GetComponent[components.ObjectInternalState](w, f.target)
				require.True(t, state.IsDirty, "a collected plant must be able to exercise the old save-resurrection path")
				ecs.GetResource[ecs.PendingAdminDestroy](w).Set(9000)
				handler.ExecutePendingDestroy(w, 9000, 1)
				require.Equal(t, "Object destruction failed.", chat.messages[9000])
				require.True(t, w.Alive(f.target))
				require.True(t, ecs.ObjectDestructionPending(w, f.target))
				require.Equal(t, 1, shard.objectDestruction.PendingCount())
				cm.UnregisterEntity(9000)
				require.Equal(t, types.ChunkStateActive, chunk.GetState(), "the source pin must prevent deactivation after its last observer leaves")
				require.Empty(t, chunk.GetRawObjects(), "dirty source must not become an ordinary save snapshot")
				snapshot, found := shard.spotManager.GetState(spotID)
				require.True(t, found)
				require.EqualValues(t, 2, snapshot.Revision, "uncertain SQL result must not publish a guessed spot snapshot")
			}()

			assertDeleted := func() repository.Spot {
				_, err := db.Queries().GetObjectByID(ctx, 1)
				require.ErrorIs(t, err, sql.ErrNoRows)
				rows, err := db.Queries().LoadSpotsByRegion(ctx, 1)
				require.NoError(t, err)
				require.Len(t, rows, 1)
				require.EqualValues(t, 3, rows[0].Revision)
				require.EqualValues(t, 10, rows[0].LastRuntimeSeconds)
				require.JSONEq(t, `{"www":{"objects":{}}}`, string(rows[0].State))
				return rows[0]
			}
			assertDeleted()
			if shutdownBeforeRetry {
				f.stop()
				require.NotEmpty(t, logs.FilterMessage("chunk still has pending persistence during shutdown").All(), "ordinary shutdown saving must refuse the pinned dirty source")
				require.True(t, w.Alive(f.target))
				assertDeleted()
			} else {
				shard.mu.Lock()
				ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 999
				shard.mu.Unlock()
				f.eventually(t, func() bool { return shard.objectDestruction.PendingCount() == 0 })
				require.False(t, w.Alive(f.target))
				persister.mu.Lock()
				deletions := append([]gameworld.WildWeedObjectDeletion(nil), persister.deletions...)
				persister.mu.Unlock()
				require.Equal(t, []gameworld.WildWeedObjectDeletion{
					{SpotID: spotID, EntityID: 1, RuntimeSeconds: 10},
					{SpotID: spotID, EntityID: 1, RuntimeSeconds: 10},
				}, deletions)
				durable := assertDeleted()
				func() {
					shard.mu.Lock()
					defer shard.mu.Unlock()
					snapshot, found := shard.spotManager.GetState(spotID)
					require.True(t, found)
					require.Equal(t, durable.Revision, snapshot.Revision)
					require.JSONEq(t, string(durable.State), string(snapshot.State))
					require.Equal(t, types.ChunkStateInactive, chunk.GetState())
					require.Empty(t, chunk.GetRawObjects())
					require.True(t, chunk.SetTile(0, 0, 20), "force a real ordinary chunk write after completion")
				}()
				f.stop()
				assertDeleted()
			}

			// A fresh chunk manager reads durable rows, rather than reusing the old
			// entity lookup or cache, after either retry completion or shutdown.
			reloadedWorld := ecs.NewWorldWithCapacity(64, nil, 0)
			reloadedShard := &Shard{world: reloadedWorld, layer: 0, logger: shard.logger}
			reloadedCM := gameworld.NewChunkManager(cfg, db, reloadedWorld, reloadedShard, 0, 1, gameworld.NewObjectFactory(nil), nil, nil, shard.logger)
			reloadedShard.chunkManager = reloadedCM
			reloaded := &objectDestructionIntegrationFixture{shard: reloadedShard, db: db, cfg: cfg, cm: reloadedCM}
			t.Cleanup(reloaded.stop)
			reloadedCM.RegisterEntity(9000, 50, 50, false)
			require.NoError(t, reloadedCM.WaitPreloaded(loadCtx, types.ChunkCoord{}))
			reloaded.eventually(t, func() bool { return reloadedCM.GetChunkFast(types.ChunkCoord{}).GetState() == types.ChunkStateActive })
			require.Equal(t, types.InvalidHandle, reloadedWorld.GetHandleByEntityID(1))
			require.Empty(t, reloadedCM.GetChunkFast(types.ChunkCoord{}).GetHandles())
			assertDeleted()
		})
	}
}
