package events

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/actionanimationdefs"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"os"
	"sync"
	"testing"
	"time"
)

func TestSpawnAnimationTracksLateEntryAndOrdersCancellation(t *testing.T) {
	contents, err := os.ReadFile("../../../tests/fixtures/action_animations/bindings.json")
	require.NoError(t, err)
	bindings, err := actionanimationdefs.Parse(contents, "fixture.json")
	require.NoError(t, err)
	registry, err := actionanimationdefs.NewRegistry(bindings)
	require.NoError(t, err)
	previous := actionanimationdefs.Global()
	actionanimationdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { actionanimationdefs.SetGlobalForTesting(previous) })
	w := ecs.NewWorldForTesting()
	handle := w.Spawn(101, nil)
	ecs.AddComponent(w, handle, components.Appearance{Resource: "player"})
	ecs.AddComponent(w, handle, components.EntityInfo{TypeID: 1})
	ecs.AddComponent(w, handle, components.Transform{})
	timing := ecs.GetResource[ecs.TimeState](w)
	timing.TickPeriod = 100 * time.Millisecond
	timing.UnixMs = 11000
	cyclicaction.Start(w, handle, components.ActiveCyclicAction{CycleDurationTicks: 20, CycleElapsedTicks: 10}, bindings[1].Source)
	dispatcher := &NetworkVisibilityDispatcher{logger: zap.NewNop()}
	var lock sync.RWMutex
	lock.RLock()
	spawn := dispatcher.buildObjectSpawn(w, 101, handle)
	require.Equal(t, uint32(10), spawn.ActionAnimation.ElapsedTicks)
	stopped := make(chan struct{})
	started := make(chan struct{})
	go func() { close(started); lock.Lock(); cyclicaction.Clear(w, handle); lock.Unlock(); close(stopped) }()
	<-started
	select {
	case <-stopped:
		t.Fatal("transition interleaved with snapshot capture/enqueue")
	default:
	}
	lock.RUnlock()
	<-stopped
	refresh := dispatcher.buildObjectSpawn(w, 101, handle)
	require.Empty(t, refresh.ActionAnimation.AnimationKey)
	require.Greater(t, refresh.ActionAnimation.Revision, spawn.ActionAnimation.Revision)
	require.Equal(t, spawn.CharacterVisual.Revision, refresh.CharacterVisual.Revision)
	require.Equal(t, spawn.CharacterVisual.Generation, refresh.ActionAnimation.Generation)
}
