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

func hydrate(t *testing.T, index *Index, character types.EntityID, home *Home) Snapshot {
	t.Helper()
	lookup, started := index.BeginLookup(character)
	require.True(t, started)
	require.NoError(t, index.CompleteLookup(lookup, home))
	return index.Snapshot(character)
}

func TestIndexUnknownAndKnownAbsence(t *testing.T) {
	index := NewIndex()
	unknown := index.Snapshot(1)
	require.Equal(t, Unknown, unknown.State)
	require.False(t, unknown.CanAcquire())
	require.False(t, unknown.CanReturn())
	require.Empty(t, index.characters)
	_, err := index.ReplaceHome(1, unknown.Version, testHome(10))
	require.ErrorIs(t, err, ErrUnknownHome)

	lookup, started := index.BeginLookup(1)
	require.True(t, started)
	_, started = index.BeginLookup(1)
	require.False(t, started)
	require.False(t, index.Evict(1))
	require.NoError(t, index.CompleteLookup(lookup, nil))
	none := index.Snapshot(1)
	require.Equal(t, NoHome, none.State)
	require.NotZero(t, none.Version.Generation)
	require.False(t, none.Dirty)
	require.True(t, none.CanAcquire())
	require.False(t, none.CanReturn())
	_, started = index.BeginLookup(1)
	require.False(t, started)
	require.ErrorIs(t, index.CompleteLookup(lookup, nil), ErrStaleLookup)
}

func TestIndexAssignmentReplacementAndRemoval(t *testing.T) {
	index := NewIndex()
	initial := hydrate(t, index, 1, nil)
	first := testHome(10)
	assigned, err := index.ReplaceHome(1, initial.Version, first)
	require.NoError(t, err)
	require.True(t, assigned.CanReturn())
	require.True(t, assigned.Dirty)
	require.Equal(t, uint64(1), assigned.Version.Revision)
	owner, owned := index.Owner(first.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)

	unchanged, err := index.ReplaceHome(1, assigned.Version, first)
	require.NoError(t, err)
	require.Equal(t, assigned, unchanged)
	require.True(t, index.AcknowledgeSaved(1, assigned.Version))

	second := testHome(20)
	replaced, err := index.ReplaceHome(1, assigned.Version, second)
	require.NoError(t, err)
	require.Equal(t, uint64(2), replaced.Version.Revision)
	_, owned = index.Owner(first.Object)
	require.False(t, owned)
	owner, owned = index.Owner(second.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)

	revoked, err := index.RevokeHome(1)
	require.NoError(t, err)
	require.Equal(t, NoHome, revoked.State)
	require.Equal(t, Home{}, revoked.Home)
	require.True(t, revoked.Dirty)
	_, owned = index.Owner(second.Object)
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
	_, err := index.ReplaceHome(2, two.Version, first)
	require.ErrorIs(t, err, ErrObjectOwned)
	_, err = index.ReplaceHome(2, Version{}, testHome(30))
	require.ErrorIs(t, err, ErrVersionChanged)
	require.Equal(t, one, index.Snapshot(1))
	require.Equal(t, two, index.Snapshot(2))
	require.Len(t, index.owners, 2)
	owner, owned := index.Owner(second.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(2), owner)
}

func TestIndexLookupCannotOverwriteRuntimeRevocationOrNewHome(t *testing.T) {
	for _, returnAbsent := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale-home", true: "stale-absence"}[returnAbsent], func(t *testing.T) {
			index := NewIndex()
			lookup, started := index.BeginLookup(1)
			require.True(t, started)
			revoked, err := index.RevokeHome(1)
			require.NoError(t, err)
			if returnAbsent {
				_, err = index.ReplaceHome(1, revoked.Version, testHome(20))
				require.NoError(t, err)
			}
			current := index.Snapshot(1)
			require.True(t, index.AcknowledgeSaved(1, current.Version))
			require.False(t, index.Evict(1), "unfinished read must retain the entry")
			var old *Home
			if !returnAbsent {
				home := testHome(10)
				old = &home
			}
			require.ErrorIs(t, index.CompleteLookup(lookup, old), ErrStaleLookup)
			expected := current
			expected.Dirty = false
			require.Equal(t, expected, index.Snapshot(1))
			require.True(t, index.Evict(1))
		})
	}
}

func TestIndexLookupFailureAndConflictingOwner(t *testing.T) {
	index := NewIndex()
	first := testHome(10)
	hydrate(t, index, 1, &first)
	lookup, started := index.BeginLookup(2)
	require.True(t, started)
	require.ErrorIs(t, index.CompleteLookup(lookup, &first), ErrObjectOwned)
	require.Equal(t, Unknown, index.Snapshot(2).State)
	owner, owned := index.Owner(first.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)

	lookup, started = index.BeginLookup(2)
	require.True(t, started)
	require.True(t, index.CancelLookup(lookup))
	require.False(t, index.CancelLookup(lookup))
	require.Equal(t, Unknown, index.Snapshot(2).State)
	retry, started := index.BeginLookup(2)
	require.True(t, started)
	require.ErrorIs(t, index.CompleteLookup(lookup, nil), ErrStaleLookup)
	require.False(t, index.CancelLookup(lookup))
	require.False(t, index.Evict(2), "old callback must not finish the newer read")
	require.NoError(t, index.CompleteLookup(retry, nil))
}

func TestIndexEvictionAndRecreationRejectOldTokens(t *testing.T) {
	index := NewIndex()
	lookup, started := index.BeginLookup(1)
	require.True(t, started)
	require.True(t, index.CancelLookup(lookup))
	old := index.Snapshot(1)
	require.True(t, index.Evict(1))
	newLookup, started := index.BeginLookup(1)
	require.True(t, started)
	require.NotEqual(t, old.Version.Generation, index.Snapshot(1).Version.Generation)
	require.ErrorIs(t, index.CompleteLookup(lookup, nil), ErrStaleLookup)
	require.False(t, index.CancelLookup(lookup))
	require.NoError(t, index.CompleteLookup(newLookup, nil))
	current := index.Snapshot(1)
	assigned, err := index.ReplaceHome(1, current.Version, testHome(10))
	require.NoError(t, err)
	require.False(t, index.AcknowledgeSaved(1, old.Version))
	require.True(t, assigned.Dirty)
}

func TestIndexSaveAcknowledgmentOnlyClearsMatchingRevision(t *testing.T) {
	index := NewIndex()
	none := hydrate(t, index, 1, nil)
	first, err := index.ReplaceHome(1, none.Version, testHome(10))
	require.NoError(t, err)
	require.False(t, index.Evict(1), "unsaved data must not be evicted")
	second, err := index.ReplaceHome(1, first.Version, testHome(20))
	require.NoError(t, err)
	require.False(t, index.AcknowledgeSaved(1, first.Version))
	require.True(t, index.Snapshot(1).Dirty)
	require.True(t, index.AcknowledgeSaved(1, second.Version))
	require.False(t, index.AcknowledgeSaved(1, second.Version))
	require.False(t, index.Snapshot(1).Dirty)
	revoked, err := index.RevokeHome(1)
	require.NoError(t, err)
	require.False(t, index.AcknowledgeSaved(1, second.Version))
	require.Equal(t, revoked, index.Snapshot(1))
	// Failed submission/writing simply leaves dirty intent without an acknowledgment.
	require.True(t, index.Snapshot(1).Dirty)
}

func TestIndexReferencesAreReleaseOnceAndProtectEviction(t *testing.T) {
	index := NewIndex()
	hydrate(t, index, 1, nil)
	first, err := index.Retain(1)
	require.NoError(t, err)
	second, err := index.Retain(1)
	require.NoError(t, err)
	require.False(t, index.Evict(1))
	alias := first
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(alias.Release)
	}
	workers.Wait()
	require.False(t, index.Evict(1), "duplicate release must not consume another reference")
	second.Release()
	require.True(t, index.Evict(1))
	hydrate(t, index, 1, nil)
	newReference, err := index.Retain(1)
	require.NoError(t, err)
	first.Release()
	second.Release()
	require.False(t, index.Evict(1))
	newReference.Release()
	require.True(t, index.Evict(1))
	var empty *Reference
	empty.Release()
}

func TestIndexWriteReferenceSurvivesAcknowledgment(t *testing.T) {
	index := NewIndex()
	none := hydrate(t, index, 1, nil)
	assigned, err := index.ReplaceHome(1, none.Version, testHome(10))
	require.NoError(t, err)
	write, err := index.Retain(1)
	require.NoError(t, err)
	require.True(t, index.AcknowledgeSaved(1, assigned.Version))
	require.False(t, index.Evict(1))
	write.Release()
	require.True(t, index.Evict(1))
}

func TestIndexTerminalPendingPreservesAdmittedCompletion(t *testing.T) {
	index := NewIndex()
	first := testHome(10)
	admitted := hydrate(t, index, 1, &first)
	require.NoError(t, index.MarkTerminalPending(1))
	pending := index.Snapshot(1)
	require.Equal(t, admitted.Version, pending.Version)
	require.Equal(t, first, pending.Home)
	require.False(t, pending.CanAcquire())
	require.False(t, pending.CanReturn())
	_, owned := index.Owner(first.Object)
	require.False(t, owned)
	require.False(t, index.Evict(1))

	second := testHome(20)
	completed, err := index.ReplaceHome(1, admitted.Version, second)
	require.NoError(t, err, "already admitted owning completion precedes terminal apply")
	require.True(t, completed.PendingTerminal)
	require.False(t, completed.CanReturn())
	terminal, err := index.ApplyTerminal(1)
	require.NoError(t, err)
	require.True(t, terminal.Terminal)
	require.False(t, terminal.PendingTerminal)
	require.False(t, terminal.CanAcquire())
	require.Equal(t, NoHome, terminal.State)
	require.Empty(t, index.owners)
	_, err = index.ReplaceHome(1, terminal.Version, testHome(30))
	require.ErrorIs(t, err, ErrTerminal)
	again, err := index.ApplyTerminal(1)
	require.NoError(t, err)
	require.Equal(t, terminal, again)
}

func TestIndexTerminalBlocksLateHydrationAndRetainsLiveBody(t *testing.T) {
	index := NewIndex()
	body, err := index.Retain(1)
	require.NoError(t, err)
	lookup, started := index.BeginLookup(1)
	require.True(t, started)
	terminal, err := index.ApplyTerminal(1)
	require.NoError(t, err)
	old := testHome(10)
	require.ErrorIs(t, index.CompleteLookup(lookup, &old), ErrStaleLookup)
	require.Equal(t, terminal, index.Snapshot(1))
	require.True(t, index.AcknowledgeSaved(1, terminal.Version))
	require.False(t, index.Evict(1))
	body.Release()
	require.True(t, index.Evict(1))
	require.Empty(t, index.characters, "no permanent terminal registration")
}

func TestIndexPendingTerminalCanPrepareCleanupMetadata(t *testing.T) {
	index := NewIndex()
	require.NoError(t, index.MarkTerminalPending(1))
	lookup, started := index.BeginLookup(1)
	require.True(t, started)
	old := testHome(10)
	require.NoError(t, index.CompleteLookup(lookup, &old))
	require.Equal(t, old, index.Snapshot(1).Home)
	require.False(t, index.Snapshot(1).CanReturn())
	require.False(t, index.Snapshot(1).CanAcquire())
	_, owned := index.Owner(old.Object)
	require.False(t, owned)
}

func TestIndexInvalidInputDoesNotMutateOwnership(t *testing.T) {
	index := NewIndex()
	for _, character := range []types.EntityID{0, math.MaxUint64} {
		_, started := index.BeginLookup(character)
		require.False(t, started)
		_, err := index.Retain(character)
		require.ErrorIs(t, err, ErrInvalidCharacter)
		_, err = index.RevokeHome(character)
		require.ErrorIs(t, err, ErrInvalidCharacter)
		_, err = index.ApplyTerminal(character)
		require.ErrorIs(t, err, ErrInvalidCharacter)
		require.ErrorIs(t, index.MarkTerminalPending(character), ErrInvalidCharacter)
		_, err = index.ReplaceHome(character, Version{}, testHome(10))
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
	} {
		_, err := index.ReplaceHome(1, none.Version, home)
		require.ErrorIs(t, err, ErrInvalidHome)
		require.Equal(t, none, index.Snapshot(1))
	}
	lookup, started := index.BeginLookup(2)
	require.True(t, started)
	bad := Home{}
	require.ErrorIs(t, index.CompleteLookup(lookup, &bad), ErrInvalidHome)
	_, started = index.BeginLookup(2)
	require.True(t, started, "invalid completion must not strand the read")
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
				snapshot := index.Snapshot(character)
				home := testHome(character*1000 + types.EntityID(iteration))
				assigned, err := index.ReplaceHome(character, snapshot.Version, home)
				if err != nil {
					t.Errorf("replace: %v", err)
					return
				}
				index.AcknowledgeSaved(character, assigned.Version)
				if _, err = index.RevokeHome(character); err != nil {
					t.Errorf("revoke: %v", err)
					return
				}
			}
		})
		workers.Go(func() {
			for range 400 {
				index.Snapshot(character)
				index.Owner(testHome(character * 1000).Object)
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
	})
	require.Zero(t, allocations)
}

func BenchmarkIndexReads(b *testing.B) {
	index := NewIndex()
	home := testHome(10)
	lookup, _ := index.BeginLookup(1)
	if err := index.CompleteLookup(lookup, &home); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		index.Snapshot(1)
		index.Owner(home.Object)
	}
}
