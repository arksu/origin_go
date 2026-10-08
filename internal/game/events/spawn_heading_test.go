package events

import (
	"context"
	"math"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func TestBuildObjectSpawnPreservesRuntimeHeadingRadians(t *testing.T) {
	for _, tc := range []struct {
		name    string
		heading float64
	}{
		{name: "positive", heading: math.Pi / 4},
		{name: "negative", heading: -3 * math.Pi / 4},
		{name: "zero", heading: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			transform := components.Transform{X: 12.75, Y: -9.25, Direction: tc.heading}
			handle := w.Spawn(101, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, transform)
				ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1001})
				ecs.AddComponent(w, h, components.Appearance{Resource: "tree"})
			})
			dispatcher := &NetworkVisibilityDispatcher{logger: zap.NewNop()}
			spawn := dispatcher.buildObjectSpawn(w, 101, handle)
			require.NotNil(t, spawn)
			require.Equal(t, float32(tc.heading), spawn.Position.Position.Heading)
			current, ok := ecs.GetComponent[components.Transform](w, handle)
			require.True(t, ok)
			require.Equal(t, transform, current, "spawn snapshots must not mutate runtime heading")

			encoded, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectSpawn{ObjectSpawn: spawn}})
			require.NoError(t, err)
			var received netproto.ServerMessage
			require.NoError(t, proto.Unmarshal(encoded, &received))
			position := received.GetObjectSpawn().GetPosition().GetPosition()
			require.Equal(t, float32(tc.heading), position.Heading, "wire heading stays in radians without integer rounding")
			require.EqualValues(t, 12, position.X)
			require.EqualValues(t, -9, position.Y)
		})
	}
}

func TestAppearanceRefreshSpawnUsesLatestRuntimeHeading(t *testing.T) {
	f := newBatchFixture(t)
	w := f.shard.World()
	observer := w.Spawn(1, nil)
	client, conn := f.connect(t, 1)
	target := spawnBatchTarget(w, 10, "tree/mature")
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.ObserversByVisibleTarget[target] = map[types.Handle]struct{}{observer: {}}
	initialHeading := math.Pi / 4
	ecs.WithComponent(w, target, func(transform *components.Transform) { transform.Direction = initialHeading })
	require.NoError(t, f.dispatcher.handleEntitySpawn(context.Background(), ecs.NewEntitySpawnEvent(1, 10, target, 0)))
	messages := f.drain(t, client, conn)
	require.Len(t, messages, 1)
	require.Equal(t, float32(initialHeading), messages[0].GetObjectSpawn().GetPosition().GetPosition().Heading)

	refresh := ecs.NewEntityAppearanceChangedEvent(0, 10, target)
	latestHeading := -math.Pi / 3
	ecs.WithComponent(w, target, func(transform *components.Transform) { transform.Direction = latestHeading })
	ecs.WithComponent(w, target, func(appearance *components.Appearance) { appearance.Resource = "tree/cut" })
	require.NoError(t, f.dispatcher.handleEntityAppearanceChanged(context.Background(), refresh))
	messages = f.drain(t, client, conn)
	require.Len(t, messages, 1)
	spawn := messages[0].GetObjectSpawn()
	require.NotNil(t, spawn)
	require.Equal(t, "tree/cut", spawn.ResourcePath)
	require.Equal(t, float32(latestHeading), spawn.GetPosition().GetPosition().Heading)
	current, ok := ecs.GetComponent[components.Transform](w, target)
	require.True(t, ok)
	require.Equal(t, latestHeading, current.Direction)
}
