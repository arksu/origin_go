package game

import (
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"net"
	"origin/internal/characterattrs"
	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"testing"
	"time"
)

func readKOStats(t *testing.T, connection net.Conn) *netproto.S2C_PlayerStats {
	t.Helper()
	require.NoError(t, connection.SetReadDeadline(time.Now().Add(time.Second)))
	encoded, _, err := wsutil.ReadServerData(connection)
	require.NoError(t, err)
	packet := &netproto.ServerMessage{}
	require.NoError(t, proto.Unmarshal(encoded, packet))
	require.NotNil(t, packet.GetPlayerStats())
	return packet.GetPlayerStats()
}

func TestStandUpAppliedAfterHealthAndAlwaysAcknowledged(t *testing.T) {
	for _, condition := range []string{"boundary", "active", "stun", "standing", "stale_epoch", "duplicate"} {
		t.Run(condition, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityHealth{HHP: 20, SHP: 0, KOUntilUnixMs: 61000, IsLying: true, LyingRevision: 1})
				ecs.AddComponent(w, h, components.EntityStats{Stamina: 50, Energy: 900})
				ecs.AddComponent(w, h, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, TargetX: 42})
			})
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID = 1
			client.InWorld.Store(true)
			client.StreamEpoch.Store(7)
			shard := &Shard{world: world, cfg: &config.Config{}, logger: zap.NewNop(), Clients: map[types.EntityID]*network.Client{1: client}}
			ecs.GetResource[ecs.CharacterEntities](world).Add(1, player, time.Now())
			*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 1, UnixMs: 61000}
			epoch := uint32(7)
			switch condition {
			case "active":
				ecs.GetResource[ecs.TimeState](world).UnixMs = 60999
			case "stun":
				ecs.WithComponent(world, player, func(m *components.Movement) { m.State = constt.StateStunned })
			case "standing":
				ecs.WithComponent(world, player, func(h *components.EntityHealth) { h.SHP = 3; h.KOUntilUnixMs = 0; h.IsLying = false })
			case "stale_epoch":
				epoch = 6
			}
			command := &network.PlayerCommand{ClientID: client.ID, CharacterID: 1, Payload: &netproto.StandUp{StreamEpoch: epoch}}
			shard.queueStandUp(world, player, command)
			before, _ := ecs.GetComponent[components.EntityHealth](world, player)
			if condition != "stale_epoch" {
				require.True(t, before.IsLying || condition == "standing", "queue changed pose")
			}
			NewPlayerDeathSystem(shard, PlayerDeathSystemConfig{}).Update(world, 0)
			stats := readKOStats(t, connection)
			require.Equal(t, uint32(7), stats.StreamEpoch)
			after, _ := ecs.GetComponent[components.EntityHealth](world, player)
			allowed := condition == "boundary" || condition == "duplicate"
			require.Equal(t, !(allowed || condition == "standing"), after.IsLying)
			require.Equal(t, after.IsLying, stats.IsLying)
			require.Equal(t, after.KOUntilUnixMs != 0, stats.IsKnockedOut)
			movement, _ := ecs.GetComponent[components.Movement](world, player)
			require.Equal(t, constt.TargetPoint, movement.TargetType)
			if condition == "stun" {
				require.Equal(t, constt.StateStunned, movement.State)
			}
			if condition == "duplicate" {
				shard.queueStandUp(world, player, command)
				NewPlayerDeathSystem(shard, PlayerDeathSystemConfig{}).Update(world, 0)
				require.False(t, readKOStats(t, connection).IsLying)
				next, _ := ecs.GetComponent[components.EntityHealth](world, player)
				require.Equal(t, after, next, "duplicate gave health or incremented pose revision")
			}
		})
	}
}

func TestStandUpRejectsConnectionChangedWhileQueued(t *testing.T) {
	for _, change := range []string{"disconnect", "replacement", "epoch", "detached"} {
		world := ecs.NewWorldForTesting()
		player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.EntityHealth{SHP: 3, HHP: 20, IsLying: true})
		})
		client := &network.Client{ID: 1, CharacterID: 1}
		client.InWorld.Store(true)
		client.StreamEpoch.Store(1)
		shard := &Shard{world: world, Clients: map[types.EntityID]*network.Client{1: client}}
		shard.queueStandUp(world, player, &network.PlayerCommand{ClientID: 1, CharacterID: 1, Payload: &netproto.StandUp{StreamEpoch: 1}})
		switch change {
		case "disconnect":
			client.InWorld.Store(false)
		case "replacement":
			shard.Clients[1] = &network.Client{ID: 2, CharacterID: 1}
		case "epoch":
			client.StreamEpoch.Store(2)
		case "detached":
			ecs.GetResource[ecs.DetachedEntities](world).AddDetachedEntity(1, player, time.Time{}, time.Time{})
		}
		shard.ApplyPendingStandUp(world, 1, player)
		health, _ := ecs.GetComponent[components.EntityHealth](world, player)
		require.True(t, health.IsLying, change)
	}
}

func TestLoginRuntimeHealthPrecedesDatabaseAndSurvivesFailedSpawn(t *testing.T) {
	world := ecs.NewWorldForTesting()
	clock := ecs.GetResource[ecs.TimeState](world)
	clock.UnixMs = 60999
	shard := &Shard{world: world}
	game := &Game{cfg: &config.Config{}, shardManager: &ShardManager{shards: map[int]*Shard{0: shard}}}
	character := repository.Character{ID: 1, Shp: 20, Hhp: 20}
	runtime := components.EntityHealth{SHP: .25, HHP: 19.5, KOUntilUnixMs: 61000, IsLying: true, LyingRevision: 3}
	shard.offlineHealth.Store(types.EntityID(1), runtime)
	require.Equal(t, runtime, game.resolveLoginHealth(world, character, characterattrs.Default(), nil))
	clock.UnixMs = 61000
	restored := game.resolveLoginHealth(world, character, characterattrs.Default(), nil)
	require.Equal(t, 1.0, restored.SHP)
	require.Equal(t, 19.5, restored.HHP)
	require.True(t, restored.IsLying)
	require.Zero(t, restored.KOUntilUnixMs)
	require.Equal(t, restored, game.resolveLoginHealth(world, character, characterattrs.Default(), nil), "failed spawn lost entry")
	// A transfer or rollback supplies its exact state before any attachment.
	transfer := components.EntityHealth{SHP: 7.125, HHP: 10.75, IsLying: true, LyingRevision: 5}
	require.Equal(t, transfer, game.resolveLoginHealth(world, character, characterattrs.Default(), []components.EntityHealth{transfer}))
	// A restarted process has no runtime deadline; persisted pose is independent.
	shard.offlineHealth.Delete(types.EntityID(1))
	character.Shp = 0
	character.IsLying = true
	require.Equal(t, int64(121000), game.resolveLoginHealth(world, character, characterattrs.Default(), nil).KOUntilUnixMs)
	character.Shp = 2
	restarted := game.resolveLoginHealth(world, character, characterattrs.Default(), nil)
	require.True(t, restarted.IsLying)
	require.Zero(t, restarted.KOUntilUnixMs)
}

func TestDetachedExpiryRestoresHealthBeforeSpawnAndFirstOwnerSnapshot(t *testing.T) {
	for _, nowMs := range []int64{60999, 61000, 90000} {
		t.Run(time.UnixMilli(nowMs).String(), func(t *testing.T) {
			shard, entered := newPlayerSpawnTestShard(t, 16)
			shard.playerInbox = network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 16})
			shard.serverInbox = network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 16})
			world := shard.world
			runtime := components.EntityHealth{SHP: .25, HHP: 19.5, KOUntilUnixMs: 61000, IsLying: true, LyingRevision: 3}
			oldPlayer := world.Spawn(10, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, runtime) })
			clock := ecs.GetResource[ecs.TimeState](world)
			clock.Now = time.UnixMilli(2000)
			clock.UnixMs = 2000
			ecs.GetResource[ecs.DetachedEntities](world).AddDetachedEntity(10, oldPlayer, time.UnixMilli(1000), time.UnixMilli(1500))
			systems.NewExpireDetachedSystem(zap.NewNop(), nil, shard.onDetachedEntityExpired, nil).Update(world, .1)
			require.False(t, world.Alive(oldPlayer))
			cached, exists := shard.offlineHealth.Load(types.EntityID(10))
			require.True(t, exists)
			require.Equal(t, runtime, cached)
			clock.UnixMs = nowMs
			character := repository.Character{ID: 10, Name: "KO restore", Shp: 20, Hhp: 20}
			game := &Game{cfg: shard.cfg, logger: zap.NewNop(), shardManager: &ShardManager{shards: map[int]*Shard{0: shard}}}
			require.NoError(t, shard.PrepareEntityAOI(t.Context(), 10, 200, 200))
			ok, player := shard.TrySpawnPlayer(200, 200, character, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, game.resolveLoginHealth(w, character, characterattrs.Default(), nil))
				ecs.AddComponent(w, h, components.EntityStats{Stamina: 100, Energy: 900})
				ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
			})
			require.True(t, ok)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("spawn publication missing")
			}
			health, _ := ecs.GetComponent[components.EntityHealth](world, player)
			require.True(t, health.IsLying)
			require.Equal(t, runtime.HHP, health.HHP)
			if nowMs < runtime.KOUntilUnixMs {
				require.Equal(t, runtime, health)
			} else {
				require.Equal(t, 1.0, health.SHP)
				require.Zero(t, health.KOUntilUnixMs)
			}
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID = 10
			game.attachClientToWorld(shard, client, 10, character, player)
			_, exists = shard.offlineHealth.Load(types.EntityID(10))
			require.False(t, exists)
			encoded, _, err := wsutil.ReadServerData(connection)
			require.NoError(t, err)
			packet := &netproto.ServerMessage{}
			require.NoError(t, proto.Unmarshal(encoded, packet))
			require.NotNil(t, packet.GetPlayerEnterWorld())
			shard.sendPlayerStats(world, 10, player, true)
			stats := readKOStats(t, connection)
			require.Equal(t, health.KOUntilUnixMs, stats.KoUntilMs)
			require.Equal(t, health.IsLying, stats.IsLying)
			require.Equal(t, nowMs >= runtime.KOUntilUnixMs, stats.CanStandUp)
		})
	}
}

func TestKnockoutRetiresOnlyItemIntentsAndKeepsMovement(t *testing.T) {
	for _, itemAction := range []bool{false, true} {
		w := ecs.NewWorldForTesting()
		movement := components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, TargetX: 42}
		player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.EntityHealth{SHP: 0, HHP: 20})
			ecs.AddComponent(w, h, movement)
			ecs.AddComponent(w, h, components.EntityStats{Stamina: 150, Energy: 900})
			ecs.AddComponent(w, h, components.ActiveCyclicAction{ActionID: "test", TargetKind: components.CyclicActionTargetSelf, MutatesItems: itemAction})
			ecs.AddComponent(w, h, components.PendingContextAction{ActionID: "test", MutatesItems: itemAction})
			ecs.AddComponent(w, h, components.PendingInteraction{TargetEntityID: 2})
			ecs.AddComponent(w, h, components.PendingLiftTransition{})
		})
		ecs.GetResource[ecs.CharacterEntities](w).Add(1, player, time.Time{})
		*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Tick: 1, UnixMs: 1000}
		contextActions := &ContextActionService{world: w}
		shard := &Shard{world: w, contextActions: contextActions}
		NewPlayerDeathSystem(shard, PlayerDeathSystemConfig{}).Update(w, 0)
		currentMovement, _ := ecs.GetComponent[components.Movement](w, player)
		require.Equal(t, movement, currentMovement)
		_, cyclic := ecs.GetComponent[components.ActiveCyclicAction](w, player)
		_, pending := ecs.GetComponent[components.PendingContextAction](w, player)
		require.Equal(t, !itemAction, cyclic)
		require.Equal(t, !itemAction, pending)
		_, pickup := ecs.GetComponent[components.PendingInteraction](w, player)
		require.False(t, pickup)
		_, lift := ecs.GetComponent[components.PendingLiftTransition](w, player)
		require.True(t, lift)
		stats, _ := ecs.GetComponent[components.EntityStats](w, player)
		require.Equal(t, 150.0, stats.Stamina)
	}
}
