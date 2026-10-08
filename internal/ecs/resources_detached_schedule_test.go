package ecs

import (
	"runtime"
	"testing"
	"time"

	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestDetachedScheduleStableBoundedPopAndPreparedReuse(t *testing.T) {
	var state DetachedEntities
	now := time.Unix(100, 0)
	for id := types.EntityID(600); id > 0; id-- {
		handle := types.MakeHandle(uint32(id), 1)
		require.NoError(t, state.PreparePlayer(id, handle))
		state.AddDetachedEntity(id, handle, now, now.Add(-time.Minute))
	}
	var buffer [256]DetachedCandidate
	due := state.PopDueInto(now, buffer[:0])
	require.Len(t, due, 256)
	require.Equal(t, 344, state.PendingCheckCount())
	for index, candidate := range due {
		require.Equal(t, types.EntityID(index+1), candidate.EntityID)
		require.True(t, state.IsPrepared(candidate.EntityID, candidate.Handle))
		state.ScheduleNext(candidate.EntityID, candidate.Handle, now, 0, 100*time.Millisecond)
	}
	require.Equal(t, 600, state.PendingCheckCount())
	due = state.PopDueInto(now, buffer[:0])
	require.Equal(t, types.EntityID(257), due[0].EntityID)
	require.Equal(t, types.EntityID(512), due[len(due)-1].EntityID)
}

func TestDetachedScheduleRecheckHonorsBaseAndCaptureRetry(t *testing.T) {
	var state DetachedEntities
	now := time.Unix(100, 0)
	handle := types.MakeHandle(1, 1)
	require.NoError(t, state.PreparePlayer(10, handle))
	state.RequestRecheck(handle, now)
	require.Zero(t, state.PendingCheckCount(), "connected record is not scheduled")
	expiration := now.Add(3 * time.Second)
	state.AddDetachedEntity(10, handle, expiration, now)
	state.AddDetachedEntity(10, handle, now.Add(time.Hour), now.Add(time.Second))
	require.Equal(t, expiration, state.Map[10].ExpirationTime, "duplicate detach preserves original delay")
	state.ScheduleNext(10, handle, now, 10*time.Second, time.Millisecond)
	state.RequestRecheck(handle, now)
	var buffer [1]DetachedCandidate
	require.Empty(t, state.PopDueInto(expiration.Add(-time.Nanosecond), buffer[:0]))
	require.Len(t, state.PopDueInto(expiration, buffer[:0]), 1)
	retryAt := now.Add(5 * time.Second)
	require.True(t, state.SetSaveRetryAt(10, handle, retryAt))
	state.RequestRecheck(handle, now)
	require.Empty(t, state.PopDueInto(retryAt.Add(-time.Nanosecond), buffer[:0]))
	require.Len(t, state.PopDueInto(retryAt, buffer[:0]), 1)
	require.Equal(t, expiration, state.Map[10].ExpirationTime)
	require.Equal(t, now, state.Map[10].DetachedAt)
	require.Equal(t, retryAt, state.Map[10].SaveRetryAt)
	state.ScheduleNext(10, handle, retryAt, time.Nanosecond, 100*time.Millisecond)
	require.Empty(t, state.PopDueInto(retryAt.Add(99*time.Millisecond), buffer[:0]))
	require.Len(t, state.PopDueInto(retryAt.Add(100*time.Millisecond), buffer[:0]), 1)
}

func TestDetachedScheduleRemoveReleaseAndGeneration(t *testing.T) {
	var state DetachedEntities
	now := time.Unix(100, 0)
	old := types.MakeHandle(1, 1)
	next := types.MakeHandle(1, 2)
	require.ErrorIs(t, state.PreparePlayer(0, old), ErrInvalidLogoutIdentity)
	require.ErrorIs(t, state.PreparePlayer(10, 0), ErrInvalidLogoutIdentity)
	require.NoError(t, state.PreparePlayer(10, old))
	require.NoError(t, state.PreparePlayer(10, old))
	require.ErrorIs(t, state.PreparePlayer(20, old), ErrLogoutIdentityConflict)
	require.ErrorIs(t, state.PreparePlayer(10, next), ErrLogoutIdentityConflict)
	state.AddDetachedEntity(10, old, now, now)
	state.RemoveDetachedEntity(10)
	require.True(t, state.IsPrepared(10, old), "reattach preserves the prepared generation")
	require.Zero(t, state.PendingCheckCount())
	require.Empty(t, state.Map)
	state.AddDetachedEntity(10, old, now, now)
	require.True(t, state.Release(old))
	require.Zero(t, state.PendingCheckCount())
	require.Empty(t, state.Map)
	require.False(t, state.IsPrepared(10, old))
	require.NoError(t, state.PreparePlayer(10, next))
	state.AddDetachedEntity(10, next, now, now)
	require.False(t, state.Release(old))
	require.True(t, state.IsPrepared(10, next))
	require.Equal(t, next, state.Map[10].Handle)
	require.False(t, state.SetSaveRetryAt(10, old, now.Add(time.Hour)))
	state.RequestRecheck(old, now)
	require.Equal(t, 1, state.PendingCheckCount())
}

func TestDetachedScheduleHeapRepairsRemovalAndBothDeadlineDirections(t *testing.T) {
	var state DetachedEntities
	now := time.Unix(100, 0)
	for id := types.EntityID(1); id <= 32; id++ {
		state.AddDetachedEntity(id, types.MakeHandle(uint32(id), 1), now, now)
		state.ScheduleNext(id, state.Map[id].Handle, now, time.Duration(id)*time.Second, time.Millisecond)
	}
	state.RemoveDetachedEntity(2)
	state.RemoveDetachedEntity(12)
	state.RemoveDetachedEntity(32)
	state.SetSaveRetryAt(1, state.Map[1].Handle, now.Add(20*time.Second))
	state.RequestRecheck(state.Map[31].Handle, now)
	var buffer [64]DetachedCandidate
	due := state.PopDueInto(now.Add(40*time.Second), buffer[:0])
	expected := []types.EntityID{31, 3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 14, 15, 16, 17, 18, 19, 1, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30}
	actual := make([]types.EntityID, len(due))
	for i := range due {
		actual[i] = due[i].EntityID
	}
	require.Equal(t, expected, actual)
	for _, record := range state.records {
		require.Equal(t, -1, record.HeapIndex)
	}
}

func TestDetachedPreparedSchedulingAllocations(t *testing.T) {
	var state DetachedEntities
	now := time.Unix(100, 0)
	handle := types.MakeHandle(1, 1)
	require.NoError(t, state.PreparePlayer(10, handle))
	state.AddDetachedEntity(10, handle, now, now)
	var buffer [1]DetachedCandidate
	allocations := testing.AllocsPerRun(1000, func() {
		state.RequestRecheck(handle, now)
		state.PopDueInto(now, buffer[:0])
		state.ScheduleNext(10, handle, now, time.Second, time.Millisecond)
		state.SetSaveRetryAt(10, handle, now)
		state.IsPrepared(10, handle)
		state.RequestRecheck(types.MakeHandle(1, 2), now)
		state.SetSaveRetryAt(10, types.MakeHandle(1, 2), now)
	})
	require.Zero(t, allocations)
}

func TestDetachedFirstPreparedEnqueueAllocations(t *testing.T) {
	var state DetachedEntities
	now := time.Unix(100, 0)
	for id := types.EntityID(1); id <= 1100; id++ {
		require.NoError(t, state.PreparePlayer(id, types.MakeHandle(uint32(id), 1)))
	}
	var nextID types.EntityID
	allocations := testing.AllocsPerRun(1000, func() {
		nextID++
		state.AddDetachedEntity(nextID, types.MakeHandle(uint32(nextID), 1), now, now)
	})
	require.Zero(t, allocations, "first detach uses reserved maps and heap for each prepared handle")
	require.Equal(t, 1001, state.PendingCheckCount())
	require.Zero(t, testing.AllocsPerRun(1000, func() {
		_ = state.PreparePlayer(0, types.InvalidHandle)
		_ = state.PreparePlayer(2, types.MakeHandle(1, 1))
		state.RemoveDetachedEntity(2000)
		state.Release(types.MakeHandle(1, 2))
	}))
}

func BenchmarkDetachedSchedule(b *testing.B) {
	for _, count := range []int{1000, 30000} {
		b.Run(benchmarkDetachedCount(count), func(b *testing.B) {
			var state DetachedEntities
			now := time.Unix(100, 0)
			for id := types.EntityID(1); id <= types.EntityID(count); id++ {
				state.AddDetachedEntity(id, types.MakeHandle(uint32(id), 1), now.Add(time.Hour), now)
			}
			var buffer [256]DetachedCandidate
			b.Run("idle", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					state.PopDueInto(now, buffer[:0])
				}
			})
			b.Run("reschedule", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					state.ScheduleNext(1, types.MakeHandle(1, 1), now, 2*time.Hour, time.Millisecond)
					state.RequestRecheck(types.MakeHandle(1, 1), now)
				}
			})
			b.Run("due_blocked", func(b *testing.B) {
				// Each repeated benchmark starts with fresh deadlines. Reusing
				// the previous run's increasing query time would first measure
				// an empty queue, then suddenly begin processing due bodies.
				var state DetachedEntities
				for id := types.EntityID(1); id <= types.EntityID(count); id++ {
					state.AddDetachedEntity(id, types.MakeHandle(uint32(id), 1), now, now)
				}
				queryTime := now.Add(3 * time.Hour)
				b.ReportAllocs()
				for b.Loop() {
					due := state.PopDueInto(queryTime, buffer[:0])
					for _, candidate := range due {
						state.ScheduleNext(candidate.EntityID, candidate.Handle, queryTime, time.Second, time.Millisecond)
					}
					queryTime = queryTime.Add(time.Second)
				}
				b.ReportMetric(256, "checks/op")
			})
		})
	}
}

func BenchmarkDetachedPreparation(b *testing.B) {
	for _, count := range []int{1000, 30000} {
		b.Run(benchmarkDetachedCount(count), func(b *testing.B) {
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)
			var last *DetachedEntities
			b.ReportAllocs()
			for b.Loop() {
				state := &DetachedEntities{}
				for id := types.EntityID(1); id <= types.EntityID(count); id++ {
					if err := state.PreparePlayer(id, types.MakeHandle(uint32(id), 1)); err != nil {
						b.Fatal(err)
					}
				}
				last = state
			}
			b.StopTimer()
			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/float64(count), "retained_B/player")
			runtime.KeepAlive(last)
			b.ReportMetric(float64(count), "players/op")
		})
	}
}

func benchmarkDetachedCount(count int) string {
	if count == 1000 {
		return "1000"
	}
	return "30000"
}
