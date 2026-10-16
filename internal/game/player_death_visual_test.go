package game

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	"origin/internal/network"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestPlayerDeathFxIncludesSelfExactlyOnce(t *testing.T) {
	for _, selfVisible := range []bool{false, true} {
		name := "before_first_visibility"
		if selfVisible {
			name = "already_visible_to_self"
		}
		t.Run(name, func(t *testing.T) {
			f := newAttackResultFixture(t, 32)
			w := f.shard.world
			body, observer := w.Spawn(1, nil), w.Spawn(2, nil)
			ecs.AddComponent(w, body, components.Transform{X: -123, Y: 456})
			oldClient, oldConn := f.connect(t, 1, 4)
			client, conn := f.connect(t, 1, 7)
			viewer, viewerConn := f.connect(t, 2, 9)
			outside, outsideConn := f.connect(t, 3, 7)
			observers := map[types.Handle]struct{}{observer: {}}
			if selfVisible {
				observers[body] = struct{}{}
			}
			ecs.GetResource[ecs.VisibilityState](w).ObserversByVisibleTarget[body] = observers
			send := func() {
				f.shard.mu.Lock()
				defer f.shard.mu.Unlock()
				f.shard.sendPlayerDeathFx(1, body)
			}
			send()
			effects := drainObjectFx(t, client, conn)
			require.Len(t, effects, 1, "self visibility must not duplicate the direct send")
			require.Equal(t, "skull_smoke", effects[0].FxKey)
			require.EqualValues(t, -123, effects[0].Position.X)
			require.EqualValues(t, 456, effects[0].Position.Y)
			require.EqualValues(t, 7, effects[0].StreamEpoch)
			seen := drainObjectFx(t, viewer, viewerConn)
			require.Len(t, seen, 1)
			require.EqualValues(t, -123, seen[0].Position.X)
			require.EqualValues(t, 456, seen[0].Position.Y)
			require.EqualValues(t, 9, seen[0].StreamEpoch)
			require.Empty(t, drainObjectFx(t, oldClient, oldConn))
			require.Empty(t, drainObjectFx(t, outside, outsideConn))
			for _, invalidate := range []func(){
				func() { client.InWorld.Store(false) },
				func() { client.StreamEpoch.Store(0) },
				func() { client.Layer++ },
				func() { client.CharacterID = 99 },
			} {
				invalidate()
				send()
				require.Empty(t, drainObjectFx(t, client, conn))
				require.Len(t, drainObjectFx(t, viewer, viewerConn), 1)
				client.InWorld.Store(true)
				client.StreamEpoch.Store(7)
				client.Layer, client.CharacterID = f.shard.layer, 1
			}
		})
	}
}

func TestPlayerDeathFxAndHearthFxKeepSeparatePositions(t *testing.T) {
	f := newHearthActivationFixture(t)
	f.chunk.UpsertRawObject(f.rawHearth())
	require.NoError(t, f.runtime.Index.LoadIfUnknown(1, &hearth.Home{
		Object: hearth.ObjectKey{Region: 1, ID: 1000}, X: 200, Y: 200,
	}))
	f.activate(t)
	f.firstVisible(t)
	index := f.runtime.Index
	*f.runtime = hearth.NewRuntime(f.shard.world, f.shard.chunkManager, f.reader,
		f.shard.chunkManager.ObjectFactory(), objectFxTerminalSaver{hearthActivationSaver: f.saver},
		behaviors.MustDefaultRegistry(), f.shard.eventBus, f.shard.logger)
	f.runtime.Index = index
	connections := newAttackResultFixture(t, 32)
	f.shard.Clients = make(map[types.EntityID]*network.Client)
	connections.shard = f.shard
	viewer, conn := connections.connect(t, 9000, 7)
	body := f.shard.world.Spawn(1, nil)
	ecs.AddComponent(f.shard.world, body, components.Transform{X: 250, Y: 300})
	ecs.GetResource[ecs.VisibilityState](f.shard.world).ObserversByVisibleTarget[body] = map[types.Handle]struct{}{f.observer: {}}
	f.shard.mu.Lock()
	f.shard.sendPlayerDeathFx(1, body)
	f.shard.mu.Unlock()
	require.NoError(t, f.runtime.NotifyTerminal(1))
	require.NoError(t, f.shard.completeHearthDeath(1))
	effects := drainObjectFx(t, viewer, conn)
	require.Len(t, effects, 2)
	for _, effect := range effects {
		require.Equal(t, "skull_smoke", effect.FxKey)
		require.EqualValues(t, 7, effect.StreamEpoch)
	}
	require.EqualValues(t, 250, effects[0].Position.X)
	require.EqualValues(t, 300, effects[0].Position.Y)
	require.EqualValues(t, 200, effects[1].Position.X)
	require.EqualValues(t, 200, effects[1].Position.Y)
	require.NoError(t, f.shard.completeHearthDeath(1))
	require.Empty(t, drainObjectFx(t, viewer, conn))
}
