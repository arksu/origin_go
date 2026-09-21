package events

import (
	"context"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildObjectSpawnCarriesNameAndColor(t *testing.T) {
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
	w := ecs.NewWorld(bus, 0)

	name := "Arik"
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1})
		ecs.AddComponent(w, h, components.Transform{})
		ecs.AddComponent(w, h, components.Appearance{Name: &name, Resource: "player"})
	})
	tree := w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 100})
		ecs.AddComponent(w, h, components.Transform{})
		ecs.AddComponent(w, h, components.Appearance{Resource: "trees/oak"})
	})

	dispatcher := &NetworkVisibilityDispatcher{logger: zap.NewNop()}

	playerSpawn := dispatcher.buildObjectSpawn(w, 1, player)
	require.NotNil(t, playerSpawn)
	require.Equal(t, "Arik", playerSpawn.Name)
	require.Equal(t, netproto.NicknameColor_NICKNAME_COLOR_DEFAULT, playerSpawn.NameColor)

	treeSpawn := dispatcher.buildObjectSpawn(w, 2, tree)
	require.NotNil(t, treeSpawn)
	require.Empty(t, treeSpawn.Name)
	require.Equal(t, netproto.NicknameColor_NICKNAME_COLOR_DEFAULT, treeSpawn.NameColor)
}
