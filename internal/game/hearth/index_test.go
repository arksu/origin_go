package hearth

import (
	"math"
	"sync"
	"testing"

	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func testHome(id types.EntityID) Home {
	return Home{Object: ObjectKey{Region: 1, ID: id}, X: -12, Y: 24, Layer: SurfaceLayer}
}

func TestIndexLastPingSurvivesOwnershipChanges(t *testing.T) {
	index := NewIndex()
	index.RecordPing(1, 1000)
	require.Equal(t, Unknown, index.Snapshot(1).State)
	require.NoError(t, index.LoadIfUnknown(1, nil))
	for _, id := range []types.EntityID{10, 20} {
		_, err := index.ReplaceHome(1, testHome(id))
		require.NoError(t, err)
		require.Equal(t, int64(1000), index.Snapshot(1).LastPingUnixMs)
	}
	_, err := index.RevokeHome(1)
	require.NoError(t, err)
	require.Equal(t, int64(1000), index.Snapshot(1).LastPingUnixMs)
	index.RecordPing(1, 999)
	require.Equal(t, int64(1000), index.Snapshot(1).LastPingUnixMs)
	index.RecordPing(0, 2000)
	index.RecordPing(2, 0)
	require.Equal(t, Unknown, index.Snapshot(2).State)
	require.Len(t, index.characters, 1)
}

func TestIndexConcurrentPingUpdates(t *testing.T) {
	index := NewIndex()
	home := testHome(10)
	require.NoError(t, index.LoadIfUnknown(1, &home))
	var workers sync.WaitGroup
	for n := range 8 {
		workers.Go(func() {
			for v := int64(1); v <= 100; v++ {
				index.RecordPing(1, int64(n)*100+v)
				index.SnapshotByObject(home.Object)
			}
		})
	}
	workers.Wait()
	require.Equal(t, int64(800), index.Snapshot(1).LastPingUnixMs)
	require.Equal(t, home, index.Snapshot(1).Home)
}

func hydrate(t *testing.T, index *Index, character types.EntityID, home *Home) Snapshot {
	t.Helper()
	require.NoError(t, index.LoadIfUnknown(character, home))
	return index.Snapshot(character)
}

func TestIndexUnknownAndKnownAbsence(t *testing.T) {
	index := NewIndex()
	unknown := index.Snapshot(1)
	require.Equal(t, Unknown, unknown.State)
	require.False(t, unknown.CanAcquire())
	require.False(t, unknown.CanReturn())
	require.Empty(t, index.characters)
	_, err := index.ReplaceHome(1, testHome(10))
	require.ErrorIs(t, err, ErrUnknownHome)

	none := hydrate(t, index, 1, nil)
	require.Equal(t, NoHome, none.State)
	require.Equal(t, Home{}, none.Home)
	require.True(t, none.CanAcquire())
	require.False(t, none.CanReturn())
	require.NoError(t, index.LoadIfUnknown(1, nil))
	require.Equal(t, none, index.Snapshot(1))
}

func TestIndexAssignmentReplacementAndRemoval(t *testing.T) {
	index := NewIndex()
	hydrate(t, index, 1, nil)
	first := testHome(10)
	assigned, err := index.ReplaceHome(1, first)
	require.NoError(t, err)
	require.True(t, assigned.CanReturn())
	owner, owned := index.Owner(first.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)

	unchanged, err := index.ReplaceHome(1, first)
	require.NoError(t, err)
	require.Equal(t, assigned, unchanged)

	second := testHome(20)
	replaced, err := index.ReplaceHome(1, second)
	require.NoError(t, err)
	require.Equal(t, second, replaced.Home)
	_, _, owned = index.SnapshotByObject(first.Object)
	require.False(t, owned)
	owner, snapshot, owned := index.SnapshotByObject(second.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
	require.Equal(t, replaced, snapshot)

	revoked, err := index.RevokeHome(1)
	require.NoError(t, err)
	require.Equal(t, NoHome, revoked.State)
	require.Equal(t, Home{}, revoked.Home)
	_, _, owned = index.SnapshotByObject(second.Object)
	require.False(t, owned)
	again, err := index.RevokeHome(1)
	require.NoError(t, err)
	require.Equal(t, revoked, again)
}

func TestIndexCopiesMetadataAndUsesCompositeObjectIdentity(t *testing.T) {
	index := NewIndex()
	first := testHome(10)
	hydrate(t, index, 1, &first)
	first.X = 999
	copy := index.Snapshot(1)
	copy.Home.Y = 999
	_, reverseCopy, exists := index.SnapshotByObject(testHome(10).Object)
	require.True(t, exists)
	reverseCopy.Home.Y = 888
	require.Equal(t, testHome(10), index.Snapshot(1).Home)

	second := testHome(10)
	second.Object.Region = 2
	hydrate(t, index, 2, &second)
	firstOwner, firstOwned := index.Owner(testHome(10).Object)
	secondOwner, secondOwned := index.Owner(second.Object)
	require.True(t, firstOwned)
	require.True(t, secondOwned)
	require.Equal(t, types.EntityID(1), firstOwner)
	require.Equal(t, types.EntityID(2), secondOwner)
}

func TestIndexRejectedAssignmentPreservesBothIndexes(t *testing.T) {
	index := NewIndex()
	first, second := testHome(10), testHome(20)
	one := hydrate(t, index, 1, &first)
	two := hydrate(t, index, 2, &second)
	_, err := index.ReplaceHome(2, first)
	require.ErrorIs(t, err, ErrObjectOwned)
	require.Equal(t, one, index.Snapshot(1))
	require.Equal(t, two, index.Snapshot(2))
	require.Len(t, index.owners, 2)
	owner, snapshot, owned := index.SnapshotByObject(second.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(2), owner)
	require.Equal(t, two, snapshot)
}

func TestIndexKnownRuntimeStateWinsOverHydration(t *testing.T) {
	for _, state := range []string{"no_home", "current_home", "terminal"} {
		t.Run(state, func(t *testing.T) {
			index := NewIndex()
			_, err := index.RevokeHome(1)
			require.NoError(t, err)
			switch state {
			case "current_home":
				_, err = index.ReplaceHome(1, testHome(20))
			case "terminal":
				_, err = index.ApplyTerminal(1)
			}
			require.NoError(t, err)
			known := index.Snapshot(1)
			old := testHome(10)
			for _, home := range []*Home{nil, &old, {}} {
				require.NoError(t, index.LoadIfUnknown(1, home))
				require.Equal(t, known, index.Snapshot(1))
			}
			_, _, owned := index.SnapshotByObject(old.Object)
			require.False(t, owned)
		})
	}
}

func TestIndexHydrationRejectsConflictingOwnerAtomically(t *testing.T) {
	index := NewIndex()
	first := testHome(10)
	known := hydrate(t, index, 1, &first)
	require.NoError(t, index.MarkTerminalPending(2))
	pending := index.Snapshot(2)
	require.ErrorIs(t, index.LoadIfUnknown(2, &first), ErrObjectOwned)
	require.Equal(t, pending, index.Snapshot(2))
	owner, snapshot, owned := index.SnapshotByObject(first.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
	require.Equal(t, known, snapshot)
	require.Len(t, index.owners, 1)
}

func TestIndexTerminalPendingPreservesAdmittedCompletion(t *testing.T) {
	index := NewIndex()
	first := testHome(10)
	hydrate(t, index, 1, &first)
	require.NoError(t, index.MarkTerminalPending(1))
	pending := index.Snapshot(1)
	require.Equal(t, first, pending.Home)
	require.False(t, pending.CanAcquire())
	require.False(t, pending.CanReturn())
	_, owned := index.Owner(first.Object)
	require.False(t, owned)
	owner, snapshot, owned := index.SnapshotByObject(first.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
	require.Equal(t, pending, snapshot)

	second := testHome(20)
	completed, err := index.ReplaceHome(1, second)
	require.NoError(t, err, "already admitted owning completion precedes terminal apply")
	require.True(t, completed.PendingTerminal)
	require.False(t, completed.CanReturn())
	_, _, owned = index.SnapshotByObject(first.Object)
	require.False(t, owned)
	owner, snapshot, owned = index.SnapshotByObject(second.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
	require.Equal(t, completed, snapshot)

	terminal, err := index.ApplyTerminal(1)
	require.NoError(t, err)
	require.True(t, terminal.Terminal)
	require.False(t, terminal.PendingTerminal)
	require.False(t, terminal.CanAcquire())
	require.Equal(t, NoHome, terminal.State)
	require.Empty(t, index.owners)
	_, err = index.ReplaceHome(1, testHome(30))
	require.ErrorIs(t, err, ErrTerminal)
	again, err := index.ApplyTerminal(1)
	require.NoError(t, err)
	require.Equal(t, terminal, again)
	require.NoError(t, index.MarkTerminalPending(1))
	require.Equal(t, terminal, index.Snapshot(1))
}

func TestIndexPendingTerminalCanPrepareCleanupMetadata(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(map[bool]string{false: "home", true: "absence"}[absent], func(t *testing.T) {
			index := NewIndex()
			require.NoError(t, index.MarkTerminalPending(1))
			old := testHome(10)
			home := &old
			if absent {
				home = nil
			}
			require.NoError(t, index.LoadIfUnknown(1, home))
			pending := index.Snapshot(1)
			require.True(t, pending.PendingTerminal)
			require.False(t, pending.CanReturn())
			require.False(t, pending.CanAcquire())
			_, owned := index.Owner(old.Object)
			require.False(t, owned)
			owner, snapshot, owned := index.SnapshotByObject(old.Object)
			if absent {
				require.Equal(t, NoHome, pending.State)
				require.False(t, owned)
			} else {
				require.Equal(t, old, pending.Home)
				require.True(t, owned)
				require.Equal(t, types.EntityID(1), owner)
				require.Equal(t, pending, snapshot)
			}
		})
	}
}

func TestIndexInvalidInputDoesNotMutateOwnership(t *testing.T) {
	index := NewIndex()
	for _, character := range []types.EntityID{0, math.MaxUint64} {
		require.ErrorIs(t, index.LoadIfUnknown(character, nil), ErrInvalidCharacter)
		_, err := index.RevokeHome(character)
		require.ErrorIs(t, err, ErrInvalidCharacter)
		_, err = index.ApplyTerminal(character)
		require.ErrorIs(t, err, ErrInvalidCharacter)
		require.ErrorIs(t, index.MarkTerminalPending(character), ErrInvalidCharacter)
		_, err = index.ReplaceHome(character, testHome(10))
		require.ErrorIs(t, err, ErrInvalidCharacter)
	}
	require.Empty(t, index.characters)
	none := hydrate(t, index, 1, nil)
	for _, home := range []Home{
		{},
		{Object: ObjectKey{Region: 1, ID: math.MaxUint64}},
		{Object: ObjectKey{Region: 1, ID: 10}, Layer: -1},
		{Object: ObjectKey{Region: math.MaxInt64, ID: 10}},
		{Object: ObjectKey{Region: 1, ID: 10}, X: math.MaxInt64},
		{Object: ObjectKey{Region: 1, ID: 10}, Y: math.MinInt64},
	} {
		_, err := index.ReplaceHome(1, home)
		require.ErrorIs(t, err, ErrInvalidHome)
		require.Equal(t, none, index.Snapshot(1))
		require.ErrorIs(t, index.LoadIfUnknown(2, &home), ErrInvalidHome)
		require.Equal(t, Snapshot{State: Unknown}, index.Snapshot(2))
		require.Len(t, index.characters, 1)
		require.Empty(t, index.owners)
	}
}

func TestIndexConcurrentAccess(t *testing.T) {
	index := NewIndex()
	for character := types.EntityID(1); character <= 4; character++ {
		hydrate(t, index, character, nil)
	}
	var workers sync.WaitGroup
	for character := types.EntityID(1); character <= 4; character++ {
		workers.Go(func() {
			for iteration := range 200 {
				home := testHome(character*1000 + types.EntityID(iteration))
				if _, err := index.ReplaceHome(character, home); err != nil {
					t.Errorf("replace: %v", err)
					return
				}
				if _, err := index.RevokeHome(character); err != nil {
					t.Errorf("revoke: %v", err)
					return
				}
			}
		})
		workers.Go(func() {
			for range 400 {
				index.Snapshot(character)
				index.Owner(testHome(character * 1000).Object)
				index.SnapshotByObject(testHome(character * 1000).Object)
			}
		})
	}
	workers.Wait()
	require.Empty(t, index.owners)
}

func TestIndexReadsDoNotAllocate(t *testing.T) {
	index := NewIndex()
	home := testHome(10)
	hydrate(t, index, 1, &home)
	allocations := testing.AllocsPerRun(1000, func() {
		index.Snapshot(1)
		index.Owner(home.Object)
		index.SnapshotByObject(home.Object)
	})
	require.Zero(t, allocations)
}

func BenchmarkIndexReads(b *testing.B) {
	index := NewIndex()
	home := testHome(10)
	if err := index.LoadIfUnknown(1, &home); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		index.Snapshot(1)
		index.Owner(home.Object)
		index.SnapshotByObject(home.Object)
	}
}
