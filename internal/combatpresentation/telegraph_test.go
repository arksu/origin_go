package combatpresentation

import (
	"fmt"
	"math"
	"testing"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type transitionCapture struct{ count int }

func (s *transitionCapture) SendCombatTelegraph(types.Handle) { s.count++ }

func telegraphWorld() (*ecs.World, types.Handle, *transitionCapture) {
	w := ecs.NewWorldWithCapacity(16, nil, 2)
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Tick: 10, TickPeriod: 100 * time.Millisecond, UnixMs: 1000}
	sender := &transitionCapture{}
	Attach(w, sender)
	h := w.Spawn(100, nil)
	ecs.AddComponent(w, h, components.NPCBrain{})
	return w, h, sender
}

func TestTelegraphSnapshotAndExecutionIdentity(t *testing.T) {
	w, h, sender := telegraphWorld()
	initial := Snapshot(w, h)
	require.NotEmpty(t, initial.Generation)
	require.False(t, initial.Active)
	require.Zero(t, initial.Revision)
	sector := actiondefs.Sector{Range: 12, Angle: math.Pi / 3}
	require.True(t, Start(w, h, 7, 0, sector, 4))
	require.False(t, Start(w, h, 7, 0, sector, 4))
	first := Snapshot(w, h)
	require.EqualValues(t, 1, first.Revision)
	require.EqualValues(t, 7, first.ExecutionGeneration)
	require.True(t, first.Active)
	clock := ecs.GetResource[ecs.TimeState](w)
	clock.Tick += 2
	late := Snapshot(w, h)
	require.EqualValues(t, 2, late.ElapsedTicks)
	require.EqualValues(t, 100, late.TickDurationMs)
	Stop(w, h, 6)
	require.True(t, Snapshot(w, h).Active)
	Stop(w, h, 7)
	ended := Snapshot(w, h)
	require.False(t, ended.Active)
	require.EqualValues(t, 2, ended.Revision)
	require.Equal(t, first.Generation, ended.Generation)
	require.True(t, Start(w, h, 8, 1, sector, 4))
	Stop(w, h, 7)
	require.True(t, Snapshot(w, h).Active, "old cancellation must not clear a new execution")
	require.Equal(t, 3, sender.count)
	clock.Tick += 4
	require.False(t, Snapshot(w, h).Active, "a late snapshot must not revive an expired warning")
}

func TestTelegraphRetainsWatermarkAcrossInPlaceTransform(t *testing.T) {
	w, h, _ := telegraphWorld()
	sector := actiondefs.Sector{Range: 12, Angle: 1}
	require.True(t, Start(w, h, 1, 0, sector, 4))
	Stop(w, h, 1)
	ecs.RemoveComponent[components.NPCBrain](w, h)
	require.Nil(t, Snapshot(w, h))
	ecs.AddComponent(w, h, components.NPCBrain{})
	require.EqualValues(t, 2, Snapshot(w, h).Revision)
	require.True(t, Start(w, h, 2, 0, sector, 4))
	require.EqualValues(t, 3, Snapshot(w, h).Revision)
	w.Despawn(h)
	require.Empty(t, (*ecs.GetResource[*State](w)).records)
	newHandle := w.Spawn(101, nil)
	ecs.AddComponent(w, newHandle, components.NPCBrain{})
	require.NotEqual(t, h, newHandle)
	require.NotEqual(t, Snapshot(w, newHandle).Generation, fmtGeneration(w, h))
	require.Zero(t, Snapshot(w, newHandle).Revision)
}

func fmtGeneration(w *ecs.World, h types.Handle) string {
	// The old incarnation is retained here only for comparison with a recycled slot.
	return fmt.Sprintf("%d:%d", w.Layer, h)
}

func TestTelegraphWorldIsolationRevisionExhaustionAndHotUpdates(t *testing.T) {
	w, h, _ := telegraphWorld()
	other, otherHandle, _ := telegraphWorld()
	sector := actiondefs.Sector{Range: 12, Angle: 1}
	require.True(t, Start(w, h, 1, 0, sector, 4))
	require.False(t, Snapshot(other, otherHandle).Active)
	Stop(w, h, 1)
	state := *ecs.GetResource[*State](w)
	state.sender = nil
	var generation uint64 = 2
	require.Zero(t, testing.AllocsPerRun(100, func() { Start(w, h, generation, 0, sector, 4); Stop(w, h, generation); generation++ }))
	r := state.records[h]
	r.revision = math.MaxUint64 - 2
	state.records[h] = r
	require.True(t, Start(w, h, generation, 0, sector, 4))
	Stop(w, h, generation)
	require.Equal(t, uint64(math.MaxUint64), Snapshot(w, h).Revision)
	require.False(t, Start(w, h, generation+1, 0, sector, 4))
}
