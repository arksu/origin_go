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
	version, started := index.BeginLookup(character)
	require.True(t, started)
	require.NoError(t, index.CompleteLookup(character, version, home))
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

	version, started := index.BeginLookup(1)
	require.True(t, started)
	_, started = index.BeginLookup(1)
	require.False(t, started)
	require.False(t, index.Evict(1))
	require.NoError(t, index.CompleteLookup(1, version, nil))
	none := index.Snapshot(1)
	require.Equal(t, NoHome, none.State)
	require.NotZero(t, none.Version)
	require.False(t, none.Dirty)
	require.True(t, none.CanAcquire())
	require.False(t, none.CanReturn())
	_, started = index.BeginLookup(1)
	require.False(t, started)
	require.ErrorIs(t, index.CompleteLookup(1, version, nil), ErrStaleLookup)
}

func TestIndexAssignmentReplacementAndRemoval(t *testing.T) {
	index := NewIndex()
	initial := hydrate(t, index, 1, nil)
	first := testHome(10)
	assigned, err := index.ReplaceHome(1, initial.Version, first)
	require.NoError(t, err)
	require.True(t, assigned.CanReturn())
	require.True(t, assigned.Dirty)
	require.Greater(t, assigned.Version, initial.Version)
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
	require.Greater(t, replaced.Version, assigned.Version)
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
	_, err = index.ReplaceHome(2, 0, testHome(30))
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
			version, started := index.BeginLookup(1)
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
			require.ErrorIs(t, index.CompleteLookup(1, version, old), ErrStaleLookup)
			expected := current
			expected.Dirty = false
			require.Equal(t, expected, index.Snapshot(1))
			require.True(t, index.Evict(1))
		})
	}
}

func TestIndexLookupRejectsConflictingOwner(t *testing.T) {
	index := NewIndex()
	first := testHome(10)
	hydrate(t, index, 1, &first)
	version, started := index.BeginLookup(2)
	require.True(t, started)
	require.ErrorIs(t, index.CompleteLookup(2, version, &first), ErrObjectOwned)
	require.Equal(t, Unknown, index.Snapshot(2).State)
	owner, owned := index.Owner(first.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
	require.True(t, index.Evict(2), "rejected metadata must release loading")
}

func TestIndexRejectedLookupSubmissionCanRetry(t *testing.T) {
	index := NewIndex()
	version, started := index.BeginLookup(1)
	require.True(t, started)
	before := index.Snapshot(1)
	require.True(t, index.RejectLookup(1))
	require.Equal(t, before, index.Snapshot(1))

	retryVersion, started := index.BeginLookup(1)
	require.True(t, started)
	require.Equal(t, version, retryVersion, "queue rejection must not change home version")
	_, started = index.BeginLookup(1)
	require.False(t, started)
	require.False(t, index.Evict(1))
	require.NoError(t, index.CompleteLookup(1, retryVersion, nil))
	require.Equal(t, NoHome, index.Snapshot(1).State)
	require.True(t, index.Evict(1))
}

func TestIndexLookupAdmissionIsExclusive(t *testing.T) {
	index := NewIndex()
	versions := make(chan Version, 8)
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			if version, started := index.BeginLookup(1); started {
				versions <- version
			}
		})
	}
	workers.Wait()
	require.Len(t, versions, 1, "only one caller may submit the character's read")
	version := <-versions
	require.NoError(t, index.CompleteLookup(1, version, nil))
}

func TestIndexLookupAfterEvictionUsesNewVersion(t *testing.T) {
	index := NewIndex()
	_, started := index.BeginLookup(1)
	require.True(t, started)
	require.True(t, index.RejectLookup(1))
	old := index.Snapshot(1)
	require.True(t, index.Evict(1))
	version, started := index.BeginLookup(1)
	require.True(t, started)
	require.Greater(t, version, old.Version)
	require.NoError(t, index.CompleteLookup(1, version, nil))
	current := index.Snapshot(1)
	_, err := index.ReplaceHome(1, old.Version, testHome(20))
	require.ErrorIs(t, err, ErrVersionChanged)
	assigned, err := index.ReplaceHome(1, current.Version, testHome(10))
	require.NoError(t, err)
	require.False(t, index.AcknowledgeSaved(1, old.Version))
	require.True(t, assigned.Dirty)
}

func TestIndexLookupOutlivesLastReference(t *testing.T) {
	index := NewIndex()
	body, err := index.Retain(1)
	require.NoError(t, err)
	version, started := index.BeginLookup(1)
	require.True(t, started)
	body.Release()
	require.False(t, index.Evict(1), "logout must not discard an accepted read")
	_, started = index.BeginLookup(1)
	require.False(t, started)

	home := testHome(10)
	require.NoError(t, index.CompleteLookup(1, version, &home))
	owner, owned := index.Owner(home.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
	require.True(t, index.Evict(1))
}

func TestIndexRecreatedHomeRejectsPreviousVersion(t *testing.T) {
	index := NewIndex()
	previous := hydrate(t, index, 1, nil)
	for _, objectID := range []types.EntityID{10, 11, 12} {
		updated, err := index.ReplaceHome(1, previous.Version, testHome(objectID))
		require.NoError(t, err)
		previous = updated
	}
	require.True(t, index.AcknowledgeSaved(1, previous.Version))
	require.True(t, index.Evict(1))

	none := hydrate(t, index, 1, nil)
	current, err := index.ReplaceHome(1, none.Version, testHome(20))
	require.NoError(t, err)
	require.Greater(t, current.Version, previous.Version)
	_, err = index.ReplaceHome(1, previous.Version, testHome(30))
	require.ErrorIs(t, err, ErrVersionChanged)
	require.False(t, index.AcknowledgeSaved(1, previous.Version))
	require.Equal(t, current, index.Snapshot(1))
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
	// Queue rejection leaves dirty intent without an acknowledgment. SQL errors are fatal.
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

func TestIndexReferenceSurvivesHomeVersionChanges(t *testing.T) {
	index := NewIndex()
	none := hydrate(t, index, 1, nil)
	reference, err := index.Retain(1)
	require.NoError(t, err)
	_, err = index.ReplaceHome(1, none.Version, testHome(10))
	require.NoError(t, err)
	revoked, err := index.RevokeHome(1)
	require.NoError(t, err)
	require.True(t, index.AcknowledgeSaved(1, revoked.Version))
	require.False(t, index.Evict(1))
	reference.Release()
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
	version, started := index.BeginLookup(1)
	require.True(t, started)
	terminal, err := index.ApplyTerminal(1)
	require.NoError(t, err)
	old := testHome(10)
	require.ErrorIs(t, index.CompleteLookup(1, version, &old), ErrStaleLookup)
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
	version, started := index.BeginLookup(1)
	require.True(t, started)
	old := testHome(10)
	require.NoError(t, index.CompleteLookup(1, version, &old))
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
		_, err = index.ReplaceHome(character, 0, testHome(10))
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
	version, started := index.BeginLookup(2)
	require.True(t, started)
	bad := Home{}
	require.ErrorIs(t, index.CompleteLookup(2, version, &bad), ErrInvalidHome)
	require.True(t, index.Evict(2), "invalid completion must not strand the read")
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
	version, _ := index.BeginLookup(1)
	if err := index.CompleteLookup(1, version, &home); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		index.Snapshot(1)
		index.Owner(home.Object)
	}
}
