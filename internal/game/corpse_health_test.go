package game

import (
	"testing"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	gameworld "origin/internal/game/world"
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
		{DefID: 901, Key: "player_death", HP: 125, Resource: "corpse", IsStatic: true},
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
}

func TestCorpseConversionRejectsInvalidDefinitionBeforeMutation(t *testing.T) {
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 901, Key: "player_death"}}))
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
