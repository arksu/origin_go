package game

import (
	"math"
	"testing"
	"time"

	"origin/internal/characterattrs"
	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func installLogoutLifecycleService(t *testing.T, shard *Shard) {
	t.Helper()
	combat, err := NewCombatLogoutPolicy(shard.world)
	require.NoError(t, err)
	shard.playerLogout, err = NewPlayerLogoutService(shard.world, NewDisconnectDelayLogoutPolicy(), combat)
	require.NoError(t, err)
}

func TestLogoutRuntimeStateSourcePriorityAndInvalidCache(t *testing.T) {
	w := ecs.NewWorldForTesting()
	shard := &Shard{world: w}
	g := &Game{cfg: &config.Config{}, shardManager: &ShardManager{shards: map[int]*Shard{0: shard}}}
	character := repository.Character{ID: 10, Shp: 22, Hhp: 24}
	cached := playerRuntimeState{Health: components.EntityHealth{SHP: 21.4, HHP: 24.28}, CombatState: ecs.CombatState{HasEvent: true}}
	shard.offlineHealth.Store(types.EntityID(10), cached)
	state, err := g.resolveLoginRuntimeState(w, character, characterattrs.Default(), nil)
	require.NoError(t, err)
	require.Equal(t, cached, state, "event at UnixMs zero is distinct from no event")
	explicit := playerRuntimeState{Health: components.EntityHealth{SHP: .25, HHP: .49}, CombatState: ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 1234}}
	state, err = g.resolveLoginRuntimeState(w, character, characterattrs.Default(), &explicit)
	require.NoError(t, err)
	require.Equal(t, explicit, state)
	for _, invalid := range []ecs.CombatState{{LastCombatEventAtUnixMs: 1}, {HasEvent: true, LastCombatEventAtUnixMs: -1}, {HasEvent: true, LastCombatEventAtUnixMs: math.MaxInt64}} {
		cached.CombatState = invalid
		shard.offlineHealth.Store(types.EntityID(10), cached)
		state, err = g.resolveLoginRuntimeState(w, character, characterattrs.Default(), nil)
		require.ErrorIs(t, err, ecs.ErrInvalidCombatActivity)
		require.Zero(t, state)
		actual, exists := shard.offlineHealth.Load(types.EntityID(10))
		require.True(t, exists)
		require.Equal(t, cached, actual, "failed restoration must retain the cache")
	}
	shard.offlineHealth.Delete(types.EntityID(10))
	state, err = g.resolveLoginRuntimeState(w, character, characterattrs.Default(), nil)
	require.NoError(t, err)
	require.Zero(t, state.CombatState, "a DB-only login starts with no runtime combat event")
}

func TestZeroDelayLogoutRetainsBodyForCombatAndIncomingHit(t *testing.T) {
	shard, _ := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
	installMeleeLifecycleReceiver(t, shard)
	installLogoutLifecycleService(t, shard)
	before := components.EntityHealth{SHP: 21.4, HHP: 24.28}
	player := spawnMeleeLifecyclePlayer(t, shard, 10, 200, 100, before)
	clock := ecs.GetResource[ecs.TimeState](shard.world)
	clock.Now, clock.UnixMs = time.Unix(100, 0), 10_000
	_, err := shard.creatureDamage.Apply(player, 0)
	require.NoError(t, err)
	client := &network.Client{ID: 1, CharacterID: 10}
	client.InWorld.Store(true)
	shard.Clients[10] = client
	require.True(t, shard.detachClientForLogout(client, clock.Now, 0))
	deadline := ecs.GetResource[ecs.DetachedEntities](shard.world).Map[10].ExpirationTime
	deadlineSystem := systems.NewExpireDetachedSystem(zap.NewNop(), nil, shard.onDetachedEntityExpired, nil, shard.playerLogout.Check)
	deadlineSystem.Update(shard.world, 0)
	require.True(t, shard.world.Alive(player))
	clock.Now = clock.Now.Add(29 * time.Second)
	clock.UnixMs = 39_999
	_, err = shard.creatureDamage.Apply(player, 0)
	require.NoError(t, err, "a detached body remains a prepared combat target")
	deadlineSystem.Update(shard.world, 0)
	require.True(t, shard.world.Alive(player))
	require.Equal(t, deadline, ecs.GetResource[ecs.DetachedEntities](shard.world).Map[10].ExpirationTime)
	clock.Now = clock.Now.Add(time.Second)
	clock.UnixMs = 40_000
	deadlineSystem.Update(shard.world, 0)
	require.True(t, shard.world.Alive(player), "the latest received hit extends the combat gate")
	clock.Now = clock.Now.Add(30 * time.Second)
	clock.UnixMs = 69_999
	deadlineSystem.Update(shard.world, 0)
	require.False(t, shard.world.Alive(player))
	cached, exists := shard.offlineHealth.Load(types.EntityID(10))
	require.True(t, exists)
	require.Equal(t, playerRuntimeState{Health: before, CombatState: ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 39_999}}, cached)
	require.False(t, ecs.GetResource[ecs.DetachedEntities](shard.world).IsPrepared(10, player))
	_, prepared := ecs.GetResource[ecs.CombatActivityState](shard.world).Capture(player)
	require.False(t, prepared)
}

func TestLogoutStaleDisconnectCannotDetachReplacementSession(t *testing.T) {
	shard, _ := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
	player := spawnSoundLifecycleEntity(t, shard, 10, 200, 100)
	old := &network.Client{ID: 1, CharacterID: 10}
	replacement := &network.Client{ID: 2, CharacterID: 10}
	replacement.InWorld.Store(true)
	replacement.StreamEpoch.Store(7)
	shard.Clients[10] = replacement
	require.False(t, shard.detachClientForLogout(old, time.Unix(100, 0), 0))
	require.Same(t, replacement, shard.Clients[10])
	require.True(t, replacement.InWorld.Load())
	require.Equal(t, uint32(7), replacement.StreamEpoch.Load())
	require.True(t, shard.world.Alive(player))
	require.False(t, ecs.GetResource[ecs.DetachedEntities](shard.world).IsDetached(10))
}

func TestLogoutClosedReattachKeepsPendingAndRuntimeCache(t *testing.T) {
	shard, g := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
	installMeleeLifecycleReceiver(t, shard)
	installLogoutLifecycleService(t, shard)
	health := components.EntityHealth{SHP: 21.4, HHP: 24.28}
	player := spawnMeleeLifecyclePlayer(t, shard, 10, 200, 100, health)
	state := playerRuntimeState{Health: health, CombatState: ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 10_000}}
	require.NoError(t, ecs.GetResource[ecs.CombatActivityState](shard.world).Restore(player, state.CombatState))
	detached := ecs.GetResource[ecs.DetachedEntities](shard.world)
	now := g.clock.GameNow()
	detached.AddDetachedEntity(10, player, now.Add(time.Minute), now)
	entry := detached.Map[10]
	shard.offlineHealth.Store(types.EntityID(10), state)
	client, _ := connectPlayerSpawnTestClient(t)
	client.CharacterID = 10
	client.Close()
	require.True(t, g.tryReattachPlayer(client, shard, 10, repository.Character{ID: 10}))
	require.Equal(t, entry, detached.Map[10])
	require.True(t, shard.world.Alive(player))
	require.Empty(t, shard.Clients)
	actual, exists := shard.offlineHealth.Load(types.EntityID(10))
	require.True(t, exists)
	require.Equal(t, state, actual)
	combat, prepared := ecs.GetResource[ecs.CombatActivityState](shard.world).Capture(player)
	require.True(t, prepared)
	require.Equal(t, state.CombatState, combat)
	open, connection := connectPlayerSpawnTestClient(t)
	open.CharacterID = 10
	require.True(t, g.tryReattachPlayer(open, shard, 10, repository.Character{ID: 10}))
	readSoundLifecycleEntry(t, connection)
	require.False(t, detached.IsDetached(10))
	_, exists = shard.offlineHealth.Load(types.EntityID(10))
	require.False(t, exists)
	combat, _ = ecs.GetResource[ecs.CombatActivityState](shard.world).Capture(player)
	require.Equal(t, state.CombatState, combat)
}

func TestLogoutFreshClosedAttachmentQueuesBodyWithoutRetiringCache(t *testing.T) {
	shard, g := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
	installMeleeLifecycleReceiver(t, shard)
	installLogoutLifecycleService(t, shard)
	shard.cfg.Game.DisconnectDelay = 0
	player := spawnMeleeLifecyclePlayer(t, shard, 10, 200, 100, components.EntityHealth{SHP: 21.4, HHP: 24.28})
	state, exists := shard.capturePlayerRuntimeState(player)
	require.True(t, exists)
	shard.offlineHealth.Store(types.EntityID(10), state)
	client, _ := connectPlayerSpawnTestClient(t)
	client.Close()
	require.False(t, g.attachClientToWorld(shard, client, 10, repository.Character{ID: 10}, player))
	require.True(t, shard.world.Alive(player))
	require.True(t, ecs.GetResource[ecs.DetachedEntities](shard.world).IsDetached(10))
	require.Empty(t, shard.Clients)
	actual, exists := shard.offlineHealth.Load(types.EntityID(10))
	require.True(t, exists)
	require.Equal(t, state, actual)
}

type logoutTransferBarrierParticipant struct {
	entered, release chan struct{}
	container        types.Handle
}

func (*logoutTransferBarrierParticipant) Key() string { return "logout_restore_barrier" }
func (*logoutTransferBarrierParticipant) CaptureSource(*Game, *Shard, PlayerTransferRequest, types.Handle) (any, error) {
	return nil, nil
}
func (p *logoutTransferBarrierParticipant) RestoreTarget(_ *Game, shard *Shard, req PlayerTransferRequest, player types.Handle, _ any) error {
	close(p.entered)
	<-p.release
	p.container = shard.world.SpawnWithoutExternalID()
	ecs.AddComponent(shard.world, p.container, components.InventoryContainer{OwnerID: req.PlayerID, Kind: constt.InventoryGrid, Width: 2, Height: 2})
	ecs.GetResource[ecs.InventoryRefIndex](shard.world).Add(constt.InventoryGrid, req.PlayerID, 0, p.container)
	ecs.AddComponent(shard.world, player, components.InventoryOwner{Inventories: []components.InventoryLink{{OwnerID: req.PlayerID, Kind: constt.InventoryGrid, Handle: p.container}}})
	return nil
}
func (*logoutTransferBarrierParticipant) RestoreSourceRollback(*Game, *Shard, PlayerTransferRequest, types.Handle, any) error {
	return nil
}
func (*logoutTransferBarrierParticipant) OnTargetRestoreFailure(*Game, *Shard, PlayerTransferRequest, types.Handle, any, error) {
}

func TestLogoutWaitsForTransferParticipantRestoreBeforeClosedAttachment(t *testing.T) {
	shard, g := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
	installMeleeLifecycleReceiver(t, shard)
	installLogoutLifecycleService(t, shard)
	shard.cfg.Game.DisconnectDelay = 0
	clock := ecs.GetResource[ecs.TimeState](shard.world)
	clock.Now, clock.UnixMs = time.Unix(100, 0), 50_000
	player := spawnMeleeLifecyclePlayer(t, shard, 10, 200, 100, components.EntityHealth{SHP: 21.4, HHP: 24.28})
	client, _ := connectPlayerSpawnTestClient(t)
	client.CharacterID = 10
	participant := &logoutTransferBarrierParticipant{entered: make(chan struct{}), release: make(chan struct{})}
	transfer := NewPlayerTransferService(g, zap.NewNop())
	transfer.RegisterParticipant(participant)
	restored := make(chan struct{})
	go func() {
		transfer.restoreParticipantsOnTarget(PlayerTransferRequest{PlayerID: 10}, shard, player, nil)
		close(restored)
	}()
	<-participant.entered
	client.Close()
	expiry := systems.NewExpireDetachedSystem(zap.NewNop(), nil, shard.onDetachedEntityExpired, nil, shard.playerLogout.Check)
	checked := make(chan struct{})
	go func() {
		shard.mu.Lock()
		expiry.Update(shard.world, 0)
		shard.mu.Unlock()
		close(checked)
	}()
	disconnected := make(chan bool, 1)
	go func() { disconnected <- shard.detachClientForLogout(client, clock.Now, 0) }()
	close(participant.release)
	<-restored
	<-checked
	require.True(t, <-disconnected)
	require.True(t, shard.world.Alive(player), "an unbound transfer body cannot enter expiry during restoration")
	require.True(t, shard.world.Alive(participant.container))
	require.False(t, ecs.GetResource[ecs.DetachedEntities](shard.world).IsDetached(10))
	recorder := &captureRetryInventoryRecorder{disconnectInventoryRecorder: disconnectInventoryRecorder{containerHandles: []types.Handle{participant.container}}}
	saver := systems.NewCharacterSaver(nil, 0, recorder, zap.NewNop())
	shard.characterSaver = saver
	require.False(t, g.attachClientToWorld(shard, client, 10, repository.Character{ID: 10}, player))
	require.True(t, ecs.GetResource[ecs.DetachedEntities](shard.world).IsDetached(10))
	expiry = systems.NewExpireDetachedSystem(zap.NewNop(), saver, shard.onDetachedEntityExpired, nil, shard.playerLogout.Check)
	expiry.Update(shard.world, 0)
	require.Equal(t, 1, recorder.calls)
	require.True(t, recorder.aliveAtSave)
	require.True(t, recorder.containersAliveAtSave)
	require.False(t, shard.world.Alive(player))
	require.False(t, shard.world.Alive(participant.container))
}

func TestLogoutDeferredAOICleanupCannotRemoveNewSpawnPreparation(t *testing.T) {
	shard, _ := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
	installMeleeLifecycleReceiver(t, shard)
	installLogoutLifecycleService(t, shard)
	health := components.EntityHealth{SHP: 21.4, HHP: 24.28}
	old := spawnMeleeLifecyclePlayer(t, shard, 10, 200, 100, health)
	clock := ecs.GetResource[ecs.TimeState](shard.world)
	clock.Now = time.Unix(100, 0)
	ecs.GetResource[ecs.DetachedEntities](shard.world).AddDetachedEntity(10, old, clock.Now, clock.Now)
	// Defer the AOI callback past body removal, as production batches may do.
	expiry := systems.NewExpireDetachedSystem(zap.NewNop(), nil, shard.onDetachedEntityExpired, nil, shard.playerLogout.Check)
	expiry.Update(shard.world, 0)
	require.False(t, shard.world.Alive(old))
	require.Contains(t, shard.pendingLogoutAOI, types.EntityID(10))
	require.NoError(t, shard.PrepareEntityAOI(t.Context(), 10, 250, 150))
	require.NotContains(t, shard.pendingLogoutAOI, types.EntityID(10))
	// The new AOI exists while its body has not been spawned yet.
	require.Equal(t, types.InvalidHandle, shard.world.GetHandleByEntityID(10))
	shard.mu.Lock()
	shard.onDetachedEntitiesExpired([]types.EntityID{10})
	shard.mu.Unlock()
	_, registered := shard.chunkManager.GetEntityChunk(10)
	require.True(t, registered, "an old logout batch must not release the new AOI reservation")
	ok, replacement := shard.TrySpawnPlayer(250, 150, repository.Character{ID: 10}, meleeLifecycleSetup(shard, 250, 150, health))
	require.True(t, ok)
	require.NotEqual(t, old, replacement)
	require.True(t, shard.world.Alive(replacement))
}
