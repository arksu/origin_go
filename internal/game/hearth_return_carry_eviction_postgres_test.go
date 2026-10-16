package game

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	gameworld "origin/internal/game/world"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ordinary transfer fixture uses nil-DB loaders. Give its source a real
// loader and a one-entry cache so returning AOI can expose an old SQL row.
func newHearthCarryEvictionFixture(t *testing.T) *hearthReturnTransferFixture {
	t.Helper()
	return newHearthReturnTransferFixture(t, 1, true, false, func(f *hearthReturnTransferFixture) {
		source := f.source
		for x := range 3 {
			_, err := f.db.Pool().Exec(t.Context(), `INSERT INTO chunk (region,x,y,layer,last_tick,tiles_data) VALUES (1,$1,0,1,1,$2)`, x, bytes.Repeat([]byte{types.TileGrass}, constt.ChunkSize*constt.ChunkSize))
			require.NoError(t, err)
		}
		source.chunkManager.Stop()
		source.cfg.Game.WorldWidthChunks = 3
		source.cfg.Game.ChunkLRUCapacity = 1
		source.cfg.Game.SaveWorkers = 1
		source.chunkManager = gameworld.NewChunkManager(source.cfg, f.db, source.world, source, 1, 1,
			gameworld.NewObjectFactory(nil), behaviors.MustDefaultRegistry(), source.eventBus, source.logger)
		source.chunkManager.SetPositionObserver(source.soundEvents)
	})
}

type hearthCarryEvictionBarrier struct{ done chan struct{} }

func (*hearthCarryEvictionBarrier) Run() error       { return nil }
func (b *hearthCarryEvictionBarrier) Complete(error) { close(b.done) }

func waitHearthCarryEvictions(t *testing.T, source *Shard) {
	t.Helper()
	// One save worker makes completion a barrier after all preceding evictions.
	barrier := &hearthCarryEvictionBarrier{done: make(chan struct{})}
	require.True(t, source.chunkManager.SubmitPersistenceJob(barrier))
	select {
	case <-barrier.done:
	case <-time.After(5 * time.Second):
		t.Fatal("chunk eviction worker did not drain")
	}
}

func pressureHearthCarrySourceCache(t *testing.T, source *Shard) {
	t.Helper()
	for x := 1; x <= 2; x++ {
		coord, observerID := types.ChunkCoord{X: x}, types.EntityID(9000+x)
		func() {
			source.mu.Lock()
			defer source.mu.Unlock()
			source.chunkManager.RegisterEntity(observerID, x*constt.ChunkWorldSize+200, 200, false)
		}()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		err := source.chunkManager.WaitPreloaded(ctx, coord)
		cancel()
		require.NoError(t, err)
		func() {
			source.mu.Lock()
			defer source.mu.Unlock()
			source.chunkManager.Update(0)
			source.chunkManager.UnregisterEntity(observerID)
			source.chunkManager.Update(0)
		}()
		waitHearthCarryEvictions(t, source)
	}
	require.Nil(t, source.chunkManager.GetChunkFast(types.ChunkCoord{X: 1}), "capacity pressure must actually evict an unpinned chunk")
}

func hearthCarryWorldCounts(f *hearthReturnTransferFixture) map[types.EntityID]int {
	counts := make(map[types.EntityID]int)
	for _, shard := range f.shards {
		shard.mu.Lock()
		// Iterate every entity: the EntityID lookup can hide an older duplicate.
		shard.world.Query().ForEach(func(handle types.Handle) {
			if id, exists := shard.world.GetExternalID(handle); exists {
				counts[id]++
			}
		})
		shard.mu.Unlock()
	}
	return counts
}

type hearthCarryFallbackGate struct {
	*LiftCarryTransferParticipant
	entered chan struct{}
	release <-chan struct{}
}

func (p *hearthCarryFallbackGate) OnTargetRestoreFailure(g *Game, shard *Shard, req PlayerTransferRequest, player types.Handle, state any, err error) {
	close(p.entered)
	select {
	case <-p.release:
	case <-time.After(10 * time.Second):
	}
	p.LiftCarryTransferParticipant.OnTargetRestoreFailure(g, shard, req, player, state, err)
}

func TestHearthReturnPostgresCarrySourceSurvivesEvictionUntilTransferFinishes(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(fmt.Sprintf("rollback_%t", rollback), func(t *testing.T) {
			f := newHearthCarryEvictionFixture(t)
			addHearthTransferNestedInventory(t, f, 10, 7001)
			addHearthTransferNestedInventory(t, f, 3000, 7101)
			wantInventories := hearthTransferInventoryData(t, f.source)
			func() {
				f.source.mu.Lock()
				defer f.source.mu.Unlock()
				carry, exists := ecs.GetComponent[components.LiftCarryState](f.source.world, f.player)
				require.True(t, exists)
				require.NoError(t, f.game.objectFactory.PersistWorldObjectNow(f.db, f.source.world, carry.ObjectHandle))
				NewLiftCarryTransferParticipant(f.game.logger).patchChunkCachesAfterTransfer(f.game, f.source,
					&liftCarryTransferState{ObjectEntityID: 3000}, carry.ObjectHandle)
			}()
			oldRow, err := f.db.Queries().GetObjectByID(t.Context(), 3000)
			require.NoError(t, err)
			require.Equal(t, 1, oldRow.Layer)
			require.Zero(t, oldRow.ChunkX)
			require.Zero(t, oldRow.ChunkY)
			sourceChunk := f.source.chunkManager.GetChunkFast(types.ChunkCoord{})
			require.NotNil(t, sourceChunk)

			req := f.request(t)
			spawnTarget := req.SpawnTarget
			captured, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			req.SpawnTarget = func(target *Shard, character repository.Character, runtime playerRuntimeState, finalize playerSpawnFinalizer) (types.Handle, repository.Character, error) {
				close(captured)
				select {
				case <-release:
				case <-time.After(10 * time.Second):
					return types.InvalidHandle, character, fmt.Errorf("test transfer gate timed out")
				}
				if rollback {
					return types.InvalidHandle, character, fmt.Errorf("injected target spawn failure")
				}
				return spawnTarget(target, character, runtime, finalize)
			}
			var resultErr error
			onResult := req.OnResult
			req.OnResult = func(shard *Shard, handle types.Handle, err error) {
				resultErr = err
				onResult(shard, handle, err)
			}
			require.NoError(t, f.game.transferService.RequestTransfer(req))
			go func() { f.game.wg.Wait(); close(finished) }()
			t.Cleanup(func() {
				unblock()
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Error("transfer did not stop before fixture cleanup")
				}
			})
			select {
			case <-captured:
			case <-time.After(5 * time.Second):
				t.Fatal("transfer did not reach post-capture gate")
			}
			pressureHearthCarrySourceCache(t, f.source)
			assert.Same(t, sourceChunk, f.source.chunkManager.GetChunkFast(types.ChunkCoord{}), "capture must retain the source cache while SQL still owns the old row")
			stillOld, err := f.db.Queries().GetObjectByID(t.Context(), 3000)
			require.NoError(t, err)
			require.Equal(t, oldRow, stillOld, "no target persistence has run while the spawn gate is blocked")

			func() {
				f.source.mu.Lock()
				defer f.source.mu.Unlock()
				f.source.chunkManager.RegisterEntity(9000, 200, 200, false)
			}()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			err = f.source.chunkManager.WaitPreloaded(ctx, types.ChunkCoord{})
			cancel()
			require.NoError(t, err)
			func() {
				f.source.mu.Lock()
				defer f.source.mu.Unlock()
				f.source.chunkManager.Update(0)
				chunk := f.source.chunkManager.GetChunkFast(types.ChunkCoord{})
				assert.Nil(t, chunk.GetRawObjectByID(3000))
				assert.Empty(t, chunk.GetRawInventoriesByOwner()[3000])
				refs := ecs.GetResource[ecs.InventoryRefIndex](f.source.world)
				for key := range wantInventories {
					_, exists := refs.Lookup(key.Kind, key.OwnerID, key.Key)
					assert.False(t, exists, "source AOI must not restore an old inventory: %+v", key)
				}
				f.source.world.Query().ForEach(func(handle types.Handle) {
					if container, exists := ecs.GetComponent[components.InventoryContainer](f.source.world, handle); exists {
						key := ecs.InventoryRefKey{Kind: container.Kind, OwnerID: container.OwnerID, Key: container.Key}
						_, captured := wantInventories[key]
						assert.False(t, captured, "source retained a captured inventory entity: %+v", key)
					}
				})
			}()
			counts := hearthCarryWorldCounts(f)
			assert.Zero(t, counts[10], "player remains detached until the gate opens")
			assert.Zero(t, counts[3000], "SQL reload must not revive the captured object")
			unblock()
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				t.Fatal("transfer did not finish")
			}
			resultShard := f.shards[0]
			if rollback {
				require.ErrorContains(t, resultErr, "injected target spawn failure")
				resultShard = f.source
			} else {
				require.NoError(t, resultErr)
			}
			counts = hearthCarryWorldCounts(f)
			assert.Equal(t, 1, counts[10], "all worlds must contain exactly one player")
			assert.Equal(t, 1, counts[3000], "all worlds must contain exactly one carried object")
			assert.Equal(t, wantInventories, hearthTransferInventoryData(t, resultShard))
			func() {
				resultShard.mu.Lock()
				defer resultShard.mu.Unlock()
				player := resultShard.world.GetHandleByEntityID(10)
				carry, exists := ecs.GetComponent[components.LiftCarryState](resultShard.world, player)
				require.True(t, exists)
				assert.Equal(t, types.EntityID(3000), carry.ObjectEntityID)
				assert.True(t, resultShard.world.Alive(carry.ObjectHandle))
			}()
			row, err := f.db.Queries().GetObjectByID(t.Context(), 3000)
			require.NoError(t, err)
			assert.Equal(t, resultShard.layer, row.Layer)

			func() {
				f.source.mu.Lock()
				defer f.source.mu.Unlock()
				f.source.chunkManager.UnregisterEntities([]types.EntityID{9000, 10})
				f.source.chunkManager.Update(0)
			}()
			pressureHearthCarrySourceCache(t, f.source)
			assert.Nil(t, f.source.chunkManager.GetChunkFast(types.ChunkCoord{}), "completed transfer must release its source pin on success and rollback")
		})
	}
}

func TestHearthReturnPostgresCarrySourcePinReleasedAfterTransferFailures(t *testing.T) {
	for _, failure := range []string{"later_capture", "rollback_spawn", "persist_once", "persist_fallback"} {
		t.Run(failure, func(t *testing.T) {
			f := newHearthCarryEvictionFixture(t)
			addHearthTransferNestedInventory(t, f, 10, 7001)
			addHearthTransferNestedInventory(t, f, 3000, 7101)
			wantInventories := hearthTransferInventoryData(t, f.source)
			func() {
				f.source.mu.Lock()
				defer f.source.mu.Unlock()
				carry, exists := ecs.GetComponent[components.LiftCarryState](f.source.world, f.player)
				require.True(t, exists)
				require.NoError(t, f.game.objectFactory.PersistWorldObjectNow(f.db, f.source.world, carry.ObjectHandle))
			}()
			req := f.request(t)
			lateParticipant := &transferSaveGuardParticipant{}
			var fallbackGate *hearthCarryFallbackGate
			var releaseFallback chan struct{}
			switch failure {
			case "later_capture":
				// The carry participant has already captured and pinned its source
				// when this later participant rejects the overall detach.
				f.game.transferService.RegisterParticipant(lateParticipant)
			case "rollback_spawn":
				req.SpawnTarget = func(_ *Shard, character repository.Character, _ playerRuntimeState, _ playerSpawnFinalizer) (types.Handle, repository.Character, error) {
					f.source.mu.Lock()
					defer f.source.mu.Unlock()
					f.source.chunkManager.GetChunkFast(types.ChunkCoord{}).SetTile(200/constt.CoordPerTile, 200/constt.CoordPerTile, types.TileDeepWater)
					return types.InvalidHandle, character, fmt.Errorf("injected target spawn failure")
				}
			case "persist_once", "persist_fallback":
				releaseFallback = make(chan struct{})
				fallbackGate = &hearthCarryFallbackGate{
					LiftCarryTransferParticipant: NewLiftCarryTransferParticipant(f.game.logger),
					entered:                      make(chan struct{}), release: releaseFallback,
				}
				f.game.transferService.participants[0] = fallbackGate
				// A sequence survives the failed statement, allowing a real first
				// write failure followed by either successful or failed fallback.
				reject := "nextval('carry_persist_attempt') = 1"
				if failure == "persist_fallback" {
					reject = "nextval('carry_persist_attempt') > 0"
				}
				_, err := f.db.Pool().Exec(t.Context(), `
					CREATE SEQUENCE carry_persist_attempt;
					CREATE FUNCTION reject_carry_persist() RETURNS trigger LANGUAGE plpgsql AS $$
					BEGIN
						IF NEW.id = 3000 AND NEW.layer = 0 AND `+reject+` THEN
							RAISE EXCEPTION 'injected carry persistence failure';
						END IF;
						RETURN NEW;
					END $$;
					CREATE TRIGGER reject_carry_persist BEFORE UPDATE ON object
					FOR EACH ROW EXECUTE FUNCTION reject_carry_persist()`)
				require.NoError(t, err)
			}
			var resultErr error
			onResult := req.OnResult
			req.OnResult = func(shard *Shard, handle types.Handle, err error) {
				resultErr = err
				onResult(shard, handle, err)
			}
			if fallbackGate == nil {
				f.run(t, req)
			} else {
				sourceChunk := f.source.chunkManager.GetChunkFast(types.ChunkCoord{})
				var releaseOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(releaseFallback) }) }
				finished := make(chan struct{})
				require.NoError(t, f.game.transferService.RequestTransfer(req))
				go func() { f.game.wg.Wait(); close(finished) }()
				t.Cleanup(func() {
					unblock()
					select {
					case <-finished:
					case <-time.After(5 * time.Second):
						t.Error("fallback did not finish before fixture cleanup")
					}
				})
				select {
				case <-fallbackGate.entered:
				case <-time.After(5 * time.Second):
					t.Fatal("transfer did not reach the persistence failure fallback")
				}
				// RestoreTarget has failed but its real force-drop/persist fallback
				// has not run. SQL still points at the captured source object.
				pressureHearthCarrySourceCache(t, f.source)
				assert.Same(t, sourceChunk, f.source.chunkManager.GetChunkFast(types.ChunkCoord{}), "source pin must survive target restore failure until fallback completes")
				row, err := f.db.Queries().GetObjectByID(t.Context(), 3000)
				require.NoError(t, err)
				require.Equal(t, 1, row.Layer)
				unblock()
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Fatal("transfer fallback did not finish")
				}
			}
			counts := hearthCarryWorldCounts(f)
			switch failure {
			case "later_capture":
				require.True(t, lateParticipant.captured)
				require.ErrorContains(t, resultErr, "participant save_guard capture failed")
				require.Equal(t, 1, counts[10], "capture failure leaves the original player attached")
				require.Zero(t, counts[3000], "the earlier carry capture must have run")
			case "rollback_spawn":
				require.ErrorContains(t, resultErr, "injected target spawn failure")
				require.Zero(t, counts[10], "blocked source terrain must reject rollback spawn too")
				require.Zero(t, counts[3000])
			default:
				// Participant failure keeps the successful player transfer and
				// force-drops the object. Check that both persistence attempts ran.
				require.NoError(t, resultErr)
				var attempts int
				require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT last_value FROM carry_persist_attempt`).Scan(&attempts))
				require.Equal(t, 2, attempts)
				require.Equal(t, 1, counts[10])
				require.Equal(t, 1, counts[3000])
				require.Equal(t, wantInventories, hearthTransferInventoryData(t, f.shards[0]))
				func() {
					target := f.shards[0]
					target.mu.Lock()
					defer target.mu.Unlock()
					_, carrying := ecs.GetComponent[components.LiftCarryState](target.world, target.world.GetHandleByEntityID(10))
					require.False(t, carrying, "failed carry persistence must run force-drop fallback")
				}()
				row, err := f.db.Queries().GetObjectByID(t.Context(), 3000)
				require.NoError(t, err)
				if failure == "persist_once" {
					require.Equal(t, 0, row.Layer)
				} else {
					// Existing fallback is best effort; this case asserts cleanup,
					// without claiming a failed write moved durable ownership.
					require.Equal(t, 1, row.Layer)
				}
			}
			func() {
				f.source.mu.Lock()
				defer f.source.mu.Unlock()
				f.source.chunkManager.UnregisterEntity(10)
				f.source.chunkManager.Update(0)
			}()
			pressureHearthCarrySourceCache(t, f.source)
			require.Nil(t, f.source.chunkManager.GetChunkFast(types.ChunkCoord{}), "failed transfer must release every captured source pin")
		})
	}
}
