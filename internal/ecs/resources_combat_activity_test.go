package ecs

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"origin/internal/types"
)

func TestCombatActivityRegistrationAndSnapshot(t *testing.T) {
	s := NewCombatActivityState()
	h := types.Handle(123)
	require.False(t, s.Prepare(types.InvalidHandle, 1))
	require.False(t, s.Prepare(h, 0))
	require.True(t, s.Prepare(h, 1))
	require.True(t, s.Prepare(h, 1))
	require.False(t, s.Prepare(h, 2))
	require.ErrorIs(t, s.ValidateEvent(h, 2, 0), ErrCombatActivityUnprepared)
	require.NoError(t, s.ValidateEvent(h, 1, 0))
	s.RecordPreparedEvent(h, 0)
	state, exists := s.Capture(h)
	require.True(t, exists)
	require.Equal(t, CombatState{HasEvent: true}, state)
	s.RecordPreparedEvent(h, 200)
	s.RecordPreparedEvent(h, 100)
	s.RecordPreparedEvent(h, 200)
	state, _ = s.Capture(h)
	require.Equal(t, int64(200), state.LastCombatEventAtUnixMs)
	require.True(t, s.Prepare(h, 1), "preparing again preserves activity")
	copied, _ := s.Capture(h)
	s.RecordPreparedEvent(h, 300)
	require.Equal(t, int64(200), copied.LastCombatEventAtUnixMs)
	require.NoError(t, s.Restore(h, copied))
	state, _ = s.Capture(h)
	require.Equal(t, copied, state)
	s.Release(h)
	require.False(t, s.IsPrepared(h, 1))
	require.ErrorIs(t, s.Restore(h, copied), ErrCombatActivityUnprepared)
	require.Zero(t, s.PreparedCount())
}

func TestCombatActivityRejectsInvalidStateBeforeMutation(t *testing.T) {
	s := NewCombatActivityState()
	h := types.Handle(123)
	require.True(t, s.Prepare(h, 1))
	for _, invalid := range []CombatState{
		{LastCombatEventAtUnixMs: 1},
		{HasEvent: true, LastCombatEventAtUnixMs: -1},
		{HasEvent: true, LastCombatEventAtUnixMs: math.MaxInt64 - CombatLogoutHoldMs + 1},
	} {
		require.ErrorIs(t, s.Restore(h, invalid), ErrInvalidCombatActivity)
		state, _ := s.Capture(h)
		require.Zero(t, state)
	}
	for _, now := range []int64{-1, math.MaxInt64 - CombatLogoutHoldMs + 1} {
		require.ErrorIs(t, s.ValidateEvent(h, 1, now), ErrInvalidCombatActivity)
	}
	require.NoError(t, s.ValidateEvent(h, 1, math.MaxInt64-CombatLogoutHoldMs))
	s.records[h] = combatActivityRecord{identity: 1, state: CombatState{HasEvent: true, LastCombatEventAtUnixMs: -1}}
	require.ErrorIs(t, s.ValidateEvent(h, 1, 1), ErrInvalidCombatActivity)
}

func TestCombatActivityPreparedOperationsAllocateNothing(t *testing.T) {
	s := NewCombatActivityState()
	h := types.Handle(123)
	require.True(t, s.Prepare(h, 1))
	var err error
	var state CombatState
	require.Zero(t, testing.AllocsPerRun(100, func() {
		err = s.ValidateEvent(h, 1, 1)
		s.RecordPreparedEvent(h, 1)
		state, _ = s.Capture(h)
	}))
	require.NoError(t, err)
	require.True(t, state.HasEvent)
	for _, now := range []int64{-1, math.MaxInt64} {
		require.Zero(t, testing.AllocsPerRun(100, func() { err = s.ValidateEvent(h, 1, now) }))
		require.ErrorIs(t, err, ErrInvalidCombatActivity)
	}
	s.Release(h)
	require.Zero(t, testing.AllocsPerRun(100, func() { err = s.ValidateEvent(h, 1, 1) }))
	require.ErrorIs(t, err, ErrCombatActivityUnprepared)
}

func BenchmarkCombatActivityPrepared(b *testing.B) {
	s := NewCombatActivityState()
	h := types.Handle(123)
	s.Prepare(h, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.ValidateEvent(h, 1, int64(i)) != nil {
			b.Fatal("event rejected")
		}
		s.RecordPreparedEvent(h, int64(i))
	}
}
