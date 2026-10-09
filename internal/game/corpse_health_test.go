package game

import (
	"context"
	"testing"
	"time"

	"origin/internal/charactervisual"
	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	gameworld "origin/internal/game/world"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCorpseConversionInitializesAndPersistsObjectHealth(t *testing.T) {
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 1, Key: "player"},
		{DefID: 901, Key: "player_dead", HP: 125, Resource: "corpse", IsStatic: true, Indestructible: true},
	}))
	w := ecs.NewWorldForTesting()
	player := w.Spawn(10, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1, Quality: 20, Region: 1})
		ecs.AddComponent(w, h, components.EntityHealth{})
		ecs.AddComponent(w, h, components.Transform{X: 10, Y: 20})
		ecs.AddComponent(w, h, components.Appearance{Resource: "player"})
		ecs.AddComponent(w, h, components.Movement{})
	})
	shard := &Shard{world: w, cfg: &config.Config{Game: config.GameConfig{Region: 1}}, logger: zap.NewNop()}
	shard.convertPlayerEntityToCorpse(w, 10, player)
	require.True(t, w.Alive(player))
	require.False(t, ecs.HasComponent[components.EntityHealth](w, player))
	require.False(t, ecs.HasComponent[components.Movement](w, player))
	state, exists := ecs.GetComponent[components.ObjectInternalState](w, player)
	require.True(t, exists)
	require.True(t, state.HasHP)
	require.Equal(t, 125.0, state.HP)
	require.True(t, state.IsDirty)
	info, exists := ecs.GetComponent[components.EntityInfo](w, player)
	require.True(t, exists)
	require.Equal(t, uint32(901), info.TypeID)
	require.Equal(t, uint32(20), info.Quality)
	require.True(t, info.Indestructible)
	ecs.AddComponent(w, player, components.ChunkRef{})
	require.NoError(t, gameworld.SetObjectHP(w, player, .49))
	factory := gameworld.NewObjectFactory(nil)
	raw, err := factory.Serialize(w, player)
	require.NoError(t, err)
	require.True(t, raw.Hp.Valid)
	require.Equal(t, .49, raw.Hp.Float64)
	restoredWorld := ecs.NewWorldForTesting()
	restored, err := factory.Build(restoredWorld, raw, nil)
	require.NoError(t, err)
	state, exists = ecs.GetComponent[components.ObjectInternalState](restoredWorld, restored)
	require.True(t, exists)
	require.True(t, state.HasHP)
	require.Equal(t, .49, state.HP)
	require.False(t, ecs.HasComponent[components.EntityHealth](restoredWorld, restored))
	restoredInfo, exists := ecs.GetComponent[components.EntityInfo](restoredWorld, restored)
	require.True(t, exists)
	require.True(t, restoredInfo.Indestructible)
}

func TestCorpseConversionUsesDefinitionIndestructibility(t *testing.T) {
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	for _, indestructible := range []bool{false, true} {
		objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
			{DefID: 901, Key: "player_dead", HP: 125, Resource: "corpse", IsStatic: true, Indestructible: indestructible},
		}))
		w := ecs.NewWorldForTesting()
		player := w.Spawn(10, nil)
		ecs.AddComponent(w, player, components.EntityInfo{TypeID: 1, Indestructible: !indestructible})
		ecs.AddComponent(w, player, components.Appearance{Resource: "player"})
		ecs.AddComponent(w, player, components.EntityHealth{})
		shard := &Shard{world: w, cfg: &config.Config{}, logger: zap.NewNop()}
		shard.convertPlayerEntityToCorpse(w, 10, player)
		info, exists := ecs.GetComponent[components.EntityInfo](w, player)
		require.True(t, exists)
		require.Equal(t, uint32(901), info.TypeID)
		require.Equal(t, indestructible, info.Indestructible)
	}
}

func TestCorpseConversionRejectsInvalidDefinitionBeforeMutation(t *testing.T) {
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 901, Key: "player_dead"}}))
	w := ecs.NewWorldForTesting()
	player := w.Spawn(10, nil)
	info := components.EntityInfo{TypeID: 1, Region: 1}
	health := components.EntityHealth{SHP: 5, HHP: 10}
	ecs.AddComponent(w, player, info)
	ecs.AddComponent(w, player, health)
	shard := &Shard{world: w, logger: zap.NewNop()}
	shard.convertPlayerEntityToCorpse(w, 10, player)
	afterInfo, _ := ecs.GetComponent[components.EntityInfo](w, player)
	afterHealth, _ := ecs.GetComponent[components.EntityHealth](w, player)
	require.Equal(t, info, afterInfo)
	require.Equal(t, health, afterHealth)
	state, exists := ecs.GetComponent[components.ObjectInternalState](w, player)
	require.False(t, exists && state.HasHP)
}

type corpseVisualDelivery struct {
	observerID types.EntityID
	state      *netproto.CharacterVisualState
}

type corpseVisualRecorder struct {
	deliveries []corpseVisualDelivery
}

func (r *corpseVisualRecorder) SendCharacterVisual(observerID, _ types.EntityID, state *netproto.CharacterVisualState) {
	r.deliveries = append(r.deliveries, corpseVisualDelivery{observerID: observerID, state: state})
}

func TestCorpseConversionPublishesLyingWithoutLosingPoseRevision(t *testing.T) {
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 1, Key: "player"},
		{DefID: 901, Key: "player_dead", HP: 125, Resource: "player", IsStatic: true},
	}))
	for _, wasLying := range []bool{false, true} {
		name := "standing"
		if wasLying {
			name = "knocked_out"
		}
		t.Run(name, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			player := w.Spawn(10, nil)
			ecs.AddComponent(w, player, components.EntityInfo{TypeID: 1})
			ecs.AddComponent(w, player, components.Appearance{Resource: "player"})
			ecs.AddComponent(w, player, components.EntityHealth{IsLying: wasLying, LyingRevision: 6})
			equipment := w.SpawnWithoutExternalID()
			ecs.AddComponent(w, equipment, components.InventoryContainer{OwnerID: 10, Kind: constt.InventoryEquipment, Version: 40})
			ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryEquipment, 10, 0, equipment)
			observer := w.Spawn(11, nil)
			ecs.GetResource[ecs.VisibilityState](w).ObserversByVisibleTarget[player] = map[types.Handle]struct{}{
				player: {}, observer: {},
			}

			before, err := charactervisual.Snapshot(w, player)
			require.NoError(t, err)
			require.Equal(t, wasLying, before.IsLying)
			shard := &Shard{world: w, cfg: &config.Config{}, logger: zap.NewNop()}
			shard.convertPlayerEntityToCorpse(w, 10, player)
			require.False(t, ecs.HasComponent[components.EntityHealth](w, player))
			require.Equal(t, 1, ecs.GetResource[ecs.CharacterVisualDirtyQueue](w).PendingCount())

			expectedRevision := before.Revision
			if !wasLying {
				expectedRevision++
			}
			recorder := &corpseVisualRecorder{}
			systems.NewCharacterVisualSystem(recorder, zap.NewNop()).Update(w, 0)
			require.Len(t, recorder.deliveries, 2)
			require.ElementsMatch(t, []types.EntityID{10, 11}, []types.EntityID{
				recorder.deliveries[0].observerID, recorder.deliveries[1].observerID,
			})
			for _, delivery := range recorder.deliveries {
				require.True(t, delivery.state.IsLying)
				require.Equal(t, before.Generation, delivery.state.Generation)
				require.Equal(t, expectedRevision, delivery.state.Revision)
			}

			// Visibility entry uses this same snapshot, without a health component.
			late, err := charactervisual.Snapshot(w, player)
			require.NoError(t, err)
			require.True(t, late.IsLying)
			require.Equal(t, expectedRevision, late.Revision)
			// Corpse equipment changes must keep increasing the combined revision.
			ecs.WithComponent(w, equipment, func(container *components.InventoryContainer) { container.Version++ })
			looted, err := charactervisual.Snapshot(w, player)
			require.NoError(t, err)
			require.True(t, looted.IsLying)
			require.Equal(t, expectedRevision+1, looted.Revision)
		})
	}
}

func TestRestoredCorpseHasLyingPoseWithoutCharacterHealth(t *testing.T) {
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 901, Key: "player_dead", HP: 125, Resource: "player", IsStatic: true},
	}))
	w := ecs.NewWorldForTesting()
	definition, found := objectdefs.Global().GetByID(901)
	require.True(t, found)
	corpse := gameworld.SpawnEntityFromDef(w, definition, gameworld.DefSpawnParams{
		EntityID: 10, X: 20, Y: 30,
	})
	require.NotEqual(t, types.InvalidHandle, corpse)
	ecs.AddComponent(w, corpse, components.ChunkRef{})
	ecs.WithComponent(w, corpse, func(visual *components.CorpseVisualState) { visual.LyingRevision = 7 })
	before, err := charactervisual.Snapshot(w, corpse)
	require.NoError(t, err)
	factory := gameworld.NewObjectFactory(nil)
	raw, err := factory.Serialize(w, corpse)
	require.NoError(t, err)
	w.Despawn(corpse)
	restored, err := factory.Build(w, raw, nil)
	require.NoError(t, err)
	require.False(t, ecs.HasComponent[components.EntityHealth](w, restored))
	visual, err := charactervisual.Snapshot(w, restored)
	require.NoError(t, err)
	require.True(t, visual.IsLying)
	require.Zero(t, visual.Revision, "runtime revision is reset only with a new generation")
	require.NotEqual(t, before.Generation, visual.Generation)
	stale, err := charactervisual.Snapshot(w, corpse)
	require.NoError(t, err)
	require.Nil(t, stale)
}

func TestMovingPlayerDeathPublishesOneFinalStopAtCurrentTransform(t *testing.T) {
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 901, Key: "player_dead", HP: 125, Resource: "player", IsStatic: true},
	}))
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
	events := make(chan *ecs.ObjectMoveBatchEvent, 2)
	bus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		events <- event.(*ecs.ObjectMoveBatchEvent)
		return nil
	})
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).UnixMs = 123456
	player := w.Spawn(10, nil)
	transform := components.Transform{X: 12.5, Y: 23.75, Direction: 1.25}
	ecs.AddComponent(w, player, transform)
	ecs.AddComponent(w, player, components.Appearance{Resource: "player"})
	ecs.AddComponent(w, player, components.EntityHealth{})
	ecs.AddComponent(w, player, components.Movement{
		State: constt.StateMoving, Mode: constt.Run, TargetType: constt.TargetPoint,
		VelocityX: 20, VelocityY: 30, TargetX: 100, TargetY: 200, MoveSeq: 34,
	})
	movementSystem := systems.NewMovementSystem(w, nil, zap.NewNop())
	shard := &Shard{world: w, cfg: &config.Config{}, eventBus: bus, logger: zap.NewNop()}
	shard.convertPlayerEntityToCorpse(w, 10, player)
	// A cached movement query and another stop attempt cannot restore movement.
	movementSystem.Update(w, .1)
	shard.publishDeathMovementStop(w, 10, player, transform)
	require.False(t, ecs.HasComponent[components.Movement](w, player))
	require.Zero(t, ecs.GetResource[ecs.MovedEntities](w).Count)
	after, exists := ecs.GetComponent[components.Transform](w, player)
	require.True(t, exists)
	require.Equal(t, transform, after)

	// The same-priority marker waits for the only worker's previous handler.
	bus.PublishAsync(ecs.NewObjectMoveBatchEvent(-1, nil), eventbus.PriorityMedium)
	var stops []ecs.MoveBatchEntry
	for {
		select {
		case event := <-events:
			if event.Layer == -1 {
				require.Len(t, stops, 1)
				require.Equal(t, ecs.MoveBatchEntry{
					EntityID: 10, Handle: player, X: 12, Y: 23, Heading: 1.25,
					MoveMode: constt.Run, ServerTimeMs: 123456, MoveSeq: 34,
				}, stops[0])
				return
			}
			stops = append(stops, event.Entries...)
		case <-time.After(time.Second):
			t.Fatal("missing final corpse movement stop")
		}
	}
}
