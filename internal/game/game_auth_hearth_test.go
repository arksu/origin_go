package game

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/game/hearth"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type hearthLoginReader func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error)

func (r hearthLoginReader) GetCharacterHearth(ctx context.Context, params repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
	return r(ctx, params)
}

func installHearthLoginRuntime(t *testing.T, shard *Shard, reader hearth.HomeReader) *hearth.Runtime {
	t.Helper()
	previous := objectdefs.Global()
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 123, Key: "hearth_fire", Name: "Hearth Fire"}}))
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	ecs.InitResource(shard.world, hearth.NewRuntime(shard.world, nil, reader, nil, nil, nil, nil, nil))
	return ecs.GetResource[hearth.Runtime](shard.world)
}

func TestLoadPlayerHearthMetadata(t *testing.T) {
	for _, hasHome := range []bool{true, false} {
		name := "no_home"
		if hasHome {
			name = "current_home"
		}
		t.Run(name, func(t *testing.T) {
			surface := &Shard{world: ecs.NewWorldForTesting()}
			game := &Game{shardManager: &ShardManager{shards: map[int]*Shard{0: surface}}}
			reads := 0
			row := repository.GetCharacterHearthRow{CharacterID: 10, ObjectRegion: 3, ObjectID: 200, X: 5000, Y: 200, Layer: 0}
			runtime := installHearthLoginRuntime(t, surface, hearthLoginReader(func(ctx context.Context, params repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
				reads++
				require.Equal(t, repository.GetCharacterHearthParams{CharacterID: 10, HearthTypeID: 123, SurfaceLayer: 0}, params)
				deadline, exists := ctx.Deadline()
				require.True(t, exists)
				require.InDelta(t, 5, time.Until(deadline).Seconds(), .1)
				require.False(t, surface.mu.TryLock(), "lookup must hold the surface shard lock")
				if !hasHome {
					return repository.GetCharacterHearthRow{}, sql.ErrNoRows
				}
				return row, nil
			}))
			require.NoError(t, game.loadPlayerHearth(10))
			snapshot := runtime.Index.Snapshot(10)
			if hasHome {
				require.Equal(t, hearth.CurrentHome, snapshot.State)
				require.Equal(t, hearth.Home{Object: hearth.ObjectKey{Region: 3, ID: 200}, X: 5000, Y: 200, Layer: 0}, snapshot.Home)
				owner, exists := runtime.Index.Owner(snapshot.Home.Object)
				require.True(t, exists)
				require.Equal(t, types.EntityID(10), owner)
			} else {
				require.Equal(t, hearth.NoHome, snapshot.State)
			}
			require.Zero(t, surface.world.EntityCount(), "metadata lookup must not create objects")
			require.NoError(t, game.loadPlayerHearth(10))
			require.Equal(t, snapshot, runtime.Index.Snapshot(10))
			require.Equal(t, 1, reads, "known runtime metadata must avoid another database lookup")
		})
	}
}

func TestSpawnAndLoginLoadsSurfaceHearthBeforeReattach(t *testing.T) {
	for _, layer := range []int{0, 1} {
		name := "surface"
		if layer != 0 {
			name = "underground"
		}
		t.Run(name, func(t *testing.T) {
			surface, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
			target := surface
			if layer != 0 {
				target, _ = newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
				target.layer, target.world.Layer = layer, layer
				game.shardManager.shards[layer] = target
			}
			player := spawnSoundLifecycleEntity(t, target, 10, 200, 200)
			detached := ecs.GetResource[ecs.DetachedEntities](target.world)
			detached.AddDetachedEntity(10, player, game.clock.GameNow().Add(time.Minute), game.clock.GameNow())
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID, client.Layer = 10, layer
			started, release := make(chan struct{}), make(chan struct{})
			reads := 0
			runtime := installHearthLoginRuntime(t, surface, hearthLoginReader(func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
				reads++
				close(started)
				<-release
				return repository.GetCharacterHearthRow{CharacterID: 10, ObjectRegion: 4, ObjectID: 200, X: 5000, Y: 200}, nil
			}))
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				game.spawnAndLogin(client, repository.Character{ID: 10, Layer: layer})
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("hearth lookup did not start")
			}
			require.False(t, client.InWorld.Load())
			require.Empty(t, target.Clients)
			require.True(t, detached.IsDetached(10), "lookup must finish before reattach")
			require.Equal(t, hearth.Unknown, runtime.Index.Snapshot(10).State)
			if layer != 0 {
				require.True(t, target.mu.TryLock(), "lookup must not also lock the player's underground shard")
				target.mu.Unlock()
				_, exists := ecs.TryGetResource[hearth.Runtime](target.world)
				require.False(t, exists, "underground shard has no hearth runtime")
			}
			close(release)
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("login did not finish after lookup")
			}
			readSoundLifecycleEntry(t, connection)
			require.Equal(t, layer, client.Layer)
			require.True(t, client.InWorld.Load())
			require.Equal(t, player, target.world.GetHandleByEntityID(10))
			require.False(t, detached.IsDetached(10))
			require.Equal(t, hearth.CurrentHome, runtime.Index.Snapshot(10).State)
			owner, exists := runtime.Index.Owner(hearth.ObjectKey{Region: 4, ID: 200})
			require.True(t, exists)
			require.Equal(t, types.EntityID(10), owner)

			// A later connection must preserve runtime metadata and the existing body.
			client.InWorld.Store(false)
			delete(target.Clients, 10)
			detached.AddDetachedEntity(10, player, game.clock.GameNow().Add(time.Minute), game.clock.GameNow())
			next, nextConnection := connectPlayerSpawnTestClient(t)
			next.CharacterID, next.Layer = 10, layer
			game.spawnAndLogin(next, repository.Character{ID: 10, Layer: layer})
			readSoundLifecycleEntry(t, nextConnection)
			require.True(t, next.InWorld.Load())
			require.Equal(t, 1, reads)
		})
	}
}

func TestSpawnAndLoginCancelledHearthLookupCompletesWithoutReattach(t *testing.T) {
	for _, disconnect := range []bool{true, false} {
		name := "timeout"
		if disconnect {
			name = "disconnect"
		}
		t.Run(name, func(t *testing.T) {
			shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
			if !disconnect {
				game.cfg.Game.SpawnTimeout = 20 * time.Millisecond
			}
			player := spawnSoundLifecycleEntity(t, shard, 10, 200, 200)
			detached := ecs.GetResource[ecs.DetachedEntities](shard.world)
			detached.AddDetachedEntity(10, player, game.clock.GameNow().Add(time.Minute), game.clock.GameNow())
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID = 10
			started, release := make(chan context.Context, 1), make(chan struct{})
			runtime := installHearthLoginRuntime(t, shard, hearthLoginReader(func(ctx context.Context, _ repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
				started <- ctx
				<-release
				return repository.GetCharacterHearthRow{CharacterID: 10, ObjectID: 200, X: 5000, Y: 200}, nil
			}))
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				game.spawnAndLogin(client, repository.Character{ID: 10})
			}()
			var lookupContext context.Context
			select {
			case lookupContext = <-started:
			case <-time.After(time.Second):
				t.Fatal("hearth lookup did not start")
			}
			if disconnect {
				client.Close()
			} else {
				<-time.After(2 * game.cfg.Game.SpawnTimeout)
			}
			require.NoError(t, lookupContext.Err(), "accepted metadata reads must survive login cancellation")
			close(release)
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("cancelled login did not finish")
			}
			require.Equal(t, hearth.CurrentHome, runtime.Index.Snapshot(10).State)
			owner, exists := runtime.Index.Owner(hearth.ObjectKey{ID: 200})
			require.True(t, exists)
			require.Equal(t, types.EntityID(10), owner)
			require.False(t, client.InWorld.Load())
			require.Zero(t, client.StreamEpoch.Load())
			require.Empty(t, shard.Clients)
			require.True(t, detached.IsDetached(10))
			if !disconnect {
				message := readSoundLifecyclePacket(t, connection, func(message *netproto.ServerMessage) bool { return message.GetError() != nil })
				require.Equal(t, netproto.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED, message.GetError().Code)
			}
		})
	}
}

func TestLoadPlayerHearthRejectsUnavailableMetadata(t *testing.T) {
	for _, dependency := range []string{"surface", "runtime", "index", "definitions", "definition", "reader"} {
		t.Run(dependency, func(t *testing.T) {
			surface := &Shard{world: ecs.NewWorldForTesting()}
			game := &Game{shardManager: &ShardManager{shards: map[int]*Shard{0: surface}}}
			runtime := installHearthLoginRuntime(t, surface, nil)
			switch dependency {
			case "surface":
				delete(game.shardManager.shards, 0)
			case "runtime":
				surface.world = ecs.NewWorldForTesting()
			case "index":
				runtime.Index = nil
			case "definitions":
				objectdefs.SetGlobalForTesting(nil)
			case "definition":
				objectdefs.SetGlobalForTesting(objectdefs.NewRegistry(nil))
			}
			err := game.loadPlayerHearth(10)
			require.Error(t, err)
		})
	}
}

func TestSpawnAndLoginUnavailableHearthDoesNotReattach(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	player := spawnSoundLifecycleEntity(t, shard, 10, 200, 200)
	detached := ecs.GetResource[ecs.DetachedEntities](shard.world)
	detached.AddDetachedEntity(10, player, game.clock.GameNow().Add(time.Minute), game.clock.GameNow())
	client, connection := connectPlayerSpawnTestClient(t)
	client.CharacterID = 10
	game.spawnAndLogin(client, repository.Character{ID: 10})
	message := readSoundLifecyclePacket(t, connection, func(message *netproto.ServerMessage) bool { return message.GetError() != nil })
	require.Equal(t, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, message.GetError().Code)
	require.Contains(t, message.GetError().Message, "hearth data unavailable")
	require.False(t, client.InWorld.Load())
	require.Zero(t, client.StreamEpoch.Load())
	require.Empty(t, shard.Clients)
	require.True(t, detached.IsDetached(10))
	require.Empty(t, ecs.GetResource[ecs.CharacterEntities](shard.world).GetAll())
}

func (r hearthLoginReader) GetCharacterHearthOwners(context.Context, repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
	panic("unexpected GetCharacterHearthOwners")
}

func (hearthLoginReader) GetSavedCharacterHearth(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
	panic("unexpected saved hearth query")
}
