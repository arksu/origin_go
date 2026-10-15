package hearth

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func newCollisionRuntime(t testing.TB) (*Runtime, types.Handle) {
	t.Helper()
	w := ecs.NewWorldForTesting()
	target := spawnCollisionHearth(w, 100)
	runtime := NewRuntime(w, nil, nil, nil, nil, nil, nil, nil)
	version, started := runtime.Index.BeginLookup(1)
	require.True(t, started)
	home := testHome(100)
	require.NoError(t, runtime.Index.CompleteLookup(1, version, &home))
	return &runtime, target
}

func spawnCollisionHearth(w *ecs.World, id types.EntityID) types.Handle {
	return w.Spawn(id, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 109, Region: 1, Layer: SurfaceLayer, Behaviors: []string{"hearth"}})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})
}

func TestIgnoreObjectCollisionRequiresCurrentOwner(t *testing.T) {
	runtime, target := newCollisionRuntime(t)
	require.True(t, runtime.IgnoreObjectCollision(1, target))
	for _, actor := range []types.EntityID{0, 2} {
		require.False(t, runtime.IgnoreObjectCollision(actor, target))
	}
	for _, state := range []string{"unknown", "no_home", "pending_terminal", "terminal"} {
		t.Run(state, func(t *testing.T) {
			runtime, target := newCollisionRuntime(t)
			switch state {
			case "unknown":
				runtime.Index = NewIndex()
			case "no_home":
				_, err := runtime.Index.RevokeHome(1)
				require.NoError(t, err)
			case "pending_terminal":
				require.NoError(t, runtime.Index.MarkTerminalPending(1))
			case "terminal":
				_, err := runtime.Index.ApplyTerminal(1)
				require.NoError(t, err)
			}
			require.False(t, runtime.IgnoreObjectCollision(1, target))
		})
	}
}

func TestIgnoreObjectCollisionRequiresLiveSurfaceHearth(t *testing.T) {
	for _, scenario := range []string{"nil_runtime", "nil_world", "nil_index", "invalid_handle", "stale_handle", "missing_info", "missing_id", "other_region", "other_layer", "other_world_layer", "other_behavior"} {
		t.Run(scenario, func(t *testing.T) {
			runtime, target := newCollisionRuntime(t)
			switch scenario {
			case "nil_runtime":
				runtime = nil
			case "nil_world":
				runtime.w = nil
			case "nil_index":
				runtime.Index = nil
			case "invalid_handle":
				target = types.InvalidHandle
			case "stale_handle":
				require.True(t, runtime.w.Despawn(target))
				replacement := spawnCollisionHearth(runtime.w, 101)
				require.True(t, runtime.w.Alive(replacement))
				require.NotEqual(t, target, replacement)
			case "missing_info":
				ecs.RemoveComponent[components.EntityInfo](runtime.w, target)
			case "missing_id":
				ecs.RemoveComponent[ecs.ExternalID](runtime.w, target)
			case "other_region":
				ecs.WithComponent(runtime.w, target, func(info *components.EntityInfo) { info.Region++ })
			case "other_layer":
				ecs.WithComponent(runtime.w, target, func(info *components.EntityInfo) { info.Layer++ })
			case "other_world_layer":
				runtime.w.Layer++
			case "other_behavior":
				ecs.WithComponent(runtime.w, target, func(info *components.EntityInfo) { info.Behaviors = []string{"burner"} })
			}
			require.False(t, runtime.IgnoreObjectCollision(1, target))
		})
	}
}

func TestIgnoreObjectCollisionFollowsReplacementAndTransformation(t *testing.T) {
	runtime, previous := newCollisionRuntime(t)
	next := spawnCollisionHearth(runtime.w, 200)
	current := runtime.Index.Snapshot(1)
	_, err := runtime.Index.ReplaceHome(1, current.Version, testHome(200))
	require.NoError(t, err)
	require.False(t, runtime.IgnoreObjectCollision(1, previous))
	require.True(t, runtime.IgnoreObjectCollision(1, next))

	campfire := &objectdefs.ObjectDef{DefID: 110, Key: "campfire", HP: 100, BehaviorOrder: []string{"burner"}}
	require.True(t, world.TransformObjectToDefInPlace(runtime.w, 200, next, campfire, world.TransformObjectInPlaceOptions{}))
	require.True(t, runtime.w.Alive(next), "conversion retains the same live handle")
	require.False(t, runtime.IgnoreObjectCollision(1, next), "the current object behavior must also match, even before metadata cleanup")
}

func TestIgnoreObjectCollisionDoesNotAllocate(t *testing.T) {
	runtime, target := newCollisionRuntime(t)
	require.Zero(t, testing.AllocsPerRun(1000, func() {
		if !runtime.IgnoreObjectCollision(1, target) {
			panic("owner collision permission changed")
		}
	}))
}

func BenchmarkIgnoreObjectCollision(b *testing.B) {
	runtime, target := newCollisionRuntime(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !runtime.IgnoreObjectCollision(1, target) {
			b.Fatal("owner collision permission changed")
		}
	}
}
