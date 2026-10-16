package game

import (
	"fmt"
	"sync"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entityhealth"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type hearthReturnDeathBoundary struct {
	handle  types.Handle
	living  bool
	bound   bool
	carried bool
}

func newHearthReturnDeathFixture(t *testing.T, sourceLayer int) *hearthReturnTransferFixture {
	t.Helper()
	f := newHearthReturnTransferFixture(t, sourceLayer, true, false)
	// A cross-layer rollback never activates the surface AOI, so the existing
	// fixture's collision-only Hearth must also be serializable on death.
	home := f.runtime.Index.Snapshot(10).Home
	homeHandle := f.shards[0].world.GetHandleByEntityID(home.Object.ID)
	ecs.AddComponent(f.shards[0].world, homeHandle, components.ChunkRef{})
	return f
}

func captureHearthReturnDeathBoundary(shard *Shard, client *network.Client, handle types.Handle) hearthReturnDeathBoundary {
	health, hasHealth := ecs.GetComponent[components.EntityHealth](shard.world, handle)
	shard.ClientsMu.RLock()
	bound := shard.Clients[10] == client
	shard.ClientsMu.RUnlock()
	return hearthReturnDeathBoundary{
		handle: handle, living: shard.world.Alive(handle) && hasHealth && health.HHP > 0,
		bound: bound, carried: ecs.HasComponent[components.LiftCarryState](shard.world, handle),
	}
}

// Rollback restores participants in reverse order. Registering this after carry
// pauses the owning finalizer before carry is restored or the client attached.
type hearthReturnRollbackDeathGate struct {
	pause  func(*Shard, types.Handle)
	target func(*Shard, types.Handle)
}

func (*hearthReturnRollbackDeathGate) Key() string { return "rollback_death_boundary" }
func (*hearthReturnRollbackDeathGate) CaptureSource(*Game, *Shard, PlayerTransferRequest, types.Handle) (any, error) {
	return nil, nil
}
func (p *hearthReturnRollbackDeathGate) RestoreTarget(_ *Game, shard *Shard, _ PlayerTransferRequest, handle types.Handle, _ any) error {
	if p.target != nil {
		p.target(shard, handle)
	}
	return nil
}
func (p *hearthReturnRollbackDeathGate) RestoreSourceRollback(_ *Game, shard *Shard, _ PlayerTransferRequest, handle types.Handle, _ any) error {
	p.pause(shard, handle)
	return nil
}
func (*hearthReturnRollbackDeathGate) OnTargetRestoreFailure(*Game, *Shard, PlayerTransferRequest, types.Handle, any, error) {
}

type hearthReturnLethalUpdate struct {
	ecs.BaseSystem
	shard    *Shard
	client   *network.Client
	observed hearthReturnDeathBoundary
}

func (s *hearthReturnLethalUpdate) Update(w *ecs.World, _ float64) {
	handle := w.GetHandleByEntityID(10)
	s.observed = captureHearthReturnDeathBoundary(s.shard, s.client, handle)
	ecs.WithComponent(w, handle, func(health *components.EntityHealth) {
		health.SHP, health.HHP, _, _ = entityhealth.ApplyDamage(health.SHP, health.HHP,
			resolveMaxHHPForHandle(w, handle, 1), 0, health.HHP+1)
	})
}

func TestHearthReturnPostgresSpawnFinalizationExcludesDeathUpdate(t *testing.T) {
	for _, sourceLayer := range []int{0, 1} {
		for _, rollback := range []bool{false, true} {
			t.Run(fmt.Sprintf("source_layer_%d_rollback_%t", sourceLayer, rollback), func(t *testing.T) {
				f := newHearthReturnDeathFixture(t, sourceLayer)
				readSoundLifecycleEntry(t, f.connection)
				shard := f.shards[0]
				if rollback {
					shard = f.source
				}
				entered, release := make(chan hearthReturnDeathBoundary, 1), make(chan struct{})
				var releaseOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(release) }) }
				t.Cleanup(unblock)
				pause := func(owner *Shard, handle types.Handle) {
					entered <- captureHearthReturnDeathBoundary(owner, f.client, handle)
					<-release
				}
				req := f.request(t)
				if rollback {
					f.game.transferService.RegisterParticipant(&hearthReturnRollbackDeathGate{pause: pause})
					req.SpawnTarget = func(_ *Shard, character repository.Character, _ playerRuntimeState, _ playerSpawnFinalizer) (types.Handle, repository.Character, error) {
						return types.InvalidHandle, character, fmt.Errorf("reject target to exercise rollback death")
					}
				} else {
					spawn := req.SpawnTarget
					req.SpawnTarget = func(owner *Shard, character repository.Character, runtime playerRuntimeState, finalize playerSpawnFinalizer) (types.Handle, repository.Character, error) {
						return spawn(owner, character, runtime, func(handle types.Handle, actual repository.Character) {
							pause(owner, handle)
							finalize(handle, actual)
						})
					}
				}
				lethal := &hearthReturnLethalUpdate{BaseSystem: ecs.NewBaseSystem("transfer lethal update", 1), shard: shard, client: f.client}
				shard.world.AddSystem(lethal)
				shard.world.AddSystem(NewPlayerDeathSystem(shard, PlayerDeathSystemConfig{}))
				require.NoError(t, f.game.transferService.RequestTransfer(req))
				before := waitHearthTerminal(t, entered)
				require.True(t, before.living)
				require.False(t, before.bound)
				require.False(t, before.carried)
				lockAvailable := shard.mu.TryLock()
				if lockAvailable {
					shard.mu.Unlock()
				}
				require.False(t, lockAvailable, "newly spawned bodies must remain under the owning shard lock through finalization")
				started, done := make(chan struct{}), make(chan struct{})
				go func() {
					close(started)
					shard.Update(ecs.TimeState{Tick: 1, UnixMs: 10000})
					close(done)
				}()
				waitHearthTerminal(t, started)
				select {
				case <-done:
					t.Fatal("death update entered while transfer finalization was blocked")
				default:
				}
				unblock()
				waitHearthTerminal(t, done)
				f.game.wg.Wait()
				assert.Equal(t, hearthReturnDeathBoundary{handle: before.handle, living: true, bound: true, carried: true}, lethal.observed,
					"the first competing update must see the complete living attachment and restored carry")
				requireHearthReturnDeathState(t, f, shard, before.handle)
				assert.NotContains(t, ecs.GetResource[ecs.CharacterEntities](shard.world).Map, types.EntityID(10))
				requireHearthReturnDeathPackets(t, f, 1)
			})
		}
	}
}

func TestHearthReturnPostgresAttachmentDeathCompletesAfterUnlock(t *testing.T) {
	for _, sourceLayer := range []int{0, 1} {
		for _, rollback := range []bool{false, true} {
			t.Run(fmt.Sprintf("source_layer_%d_rollback_%t", sourceLayer, rollback), func(t *testing.T) {
				f := newHearthReturnDeathFixture(t, sourceLayer)
				readSoundLifecycleEntry(t, f.connection)
				var corpse types.Handle
				deplete := func(shard *Shard, handle types.Handle) {
					corpse = handle
					ecs.WithComponent(shard.world, handle, func(health *components.EntityHealth) {
						health.SHP, health.HHP, _, _ = entityhealth.ApplyDamage(health.SHP, health.HHP,
							resolveMaxHHPForHandle(shard.world, handle, 1), 0, health.HHP+1)
					})
				}
				// Target restoration follows carry; rollback restoration precedes
				// carry. Both must reach attachment with carry present and HHP zero.
				f.game.transferService.RegisterParticipant(&hearthReturnRollbackDeathGate{pause: deplete, target: deplete})
				req := f.request(t)
				shard := f.shards[0]
				if rollback {
					shard = f.source
					req.SpawnTarget = func(_ *Shard, character repository.Character, _ playerRuntimeState, _ playerSpawnFinalizer) (types.Handle, repository.Character, error) {
						return types.InvalidHandle, character, fmt.Errorf("reject target to exercise rollback attachment death")
					}
				}
				originalComplete := shard.permanentDeathComplete
				completedUnlocked := false
				shard.permanentDeathComplete = func(id types.EntityID) error {
					completedUnlocked = shard.mu.TryLock()
					if completedUnlocked {
						shard.mu.Unlock()
					} else {
						return fmt.Errorf("death completion reacquired the still-held owning shard")
					}
					return originalComplete(id)
				}
				f.run(t, req)
				require.True(t, completedUnlocked, "synchronous Hearth death cleanup must run after the owning shard unlocks")
				requireHearthReturnDeathState(t, f, shard, corpse)
				requireHearthReturnDeathPackets(t, f, 1)
			})
		}
	}
}

func requireHearthReturnDeathState(t *testing.T, f *hearthReturnTransferFixture, shard *Shard, corpse types.Handle) {
	t.Helper()
	require.True(t, shard.world.Alive(corpse))
	require.Equal(t, corpse, shard.world.GetHandleByEntityID(10))
	corpseDef, found := objectdefs.Global().GetByKey("player_dead")
	require.True(t, found)
	info, found := ecs.GetComponent[components.EntityInfo](shard.world, corpse)
	require.True(t, found)
	assert.Equal(t, uint32(corpseDef.DefID), info.TypeID)
	assert.False(t, ecs.HasComponent[components.EntityHealth](shard.world, corpse))
	assert.False(t, ecs.HasComponent[components.LiftCarryState](shard.world, corpse), "transfer must not bind carry to the corpse")
	assert.True(t, f.client.IsDeadObserverMode(), "the live connection must receive the ordinary permanent-death observer flow")
	assert.Same(t, f.client, shard.Clients[10])
	assert.Equal(t, shard.layer, f.client.Layer)

	object := shard.world.GetHandleByEntityID(3000)
	require.True(t, shard.world.Alive(object), "the transferred carry must remain at the death position")
	assert.False(t, ecs.HasComponent[components.LiftedObjectState](shard.world, object), "permanent death must drop the carried object")
	position, found := ecs.GetComponent[components.Transform](shard.world, corpse)
	require.True(t, found)
	droppedPosition, found := ecs.GetComponent[components.Transform](shard.world, object)
	require.True(t, found)
	assert.Equal(t, position.X, droppedPosition.X)
	assert.Equal(t, position.Y, droppedPosition.Y)
	for _, id := range []types.EntityID{10, 3000} {
		count := 0
		for _, candidate := range f.shards {
			if candidate.world.Alive(candidate.world.GetHandleByEntityID(id)) {
				count++
			}
		}
		assert.Equal(t, 1, count, "entity %d must exist in exactly one shard", id)
	}
	f.requireDurableDeath(t, 10)
}

func requireHearthReturnDeathPackets(t *testing.T, f *hearthReturnTransferFixture, expectedEntries int) {
	t.Helper()
	// A same-channel marker bounds the read without a timing-based assertion
	// that no further EnterWorld packet arrived.
	const markerTime = 8675309
	marker, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_Pong{
		Pong: &netproto.S2C_Pong{ClientTimeMs: markerTime},
	}})
	require.NoError(t, err)
	f.client.Send(marker)
	entries, dialogs := 0, 0
	readSoundLifecyclePacket(t, f.connection, func(packet *netproto.ServerMessage) bool {
		if packet.GetPlayerEnterWorld() != nil {
			entries++
			assert.Zero(t, dialogs, "transfer must never send an ordinary EnterWorld after death")
		}
		if dialog := packet.GetDeathDialog(); dialog != nil {
			dialogs++
			assert.Equal(t, "Death", dialog.Title)
			assert.Equal(t, "You are dead", dialog.Message)
		}
		return packet.GetPong().GetClientTimeMs() == markerTime
	})
	assert.Equal(t, expectedEntries, entries, "only a completed living attachment may send EnterWorld")
	assert.Equal(t, 1, dialogs, "permanent death must be shown exactly once")
}
