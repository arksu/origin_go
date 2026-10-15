package hearth

import (
	"errors"
	"math"
	"sync"

	"origin/internal/types"
)

const SurfaceLayer = 0

var (
	ErrInvalidCharacter = errors.New("invalid hearth character")
	ErrInvalidHome      = errors.New("invalid hearth home")
	ErrUnknownHome      = errors.New("hearth home is not loaded")
	ErrVersionChanged   = errors.New("hearth home version changed")
	ErrObjectOwned      = errors.New("hearth object already has an owner")
	ErrTerminal         = errors.New("hearth owner is terminal")
	ErrStaleLookup      = errors.New("stale hearth lookup")
)

type ObjectKey struct {
	Region int
	ID     types.EntityID
}

// Home owns its metadata; it never retains ECS components or raw-cache pointers.
type Home struct {
	Object ObjectKey
	X      int
	Y      int
	Layer  int
}

type HomeState uint8

const (
	Unknown HomeState = iota
	NoHome
	CurrentHome
)

// Version never repeats within an Index, including after eviction and recreation.
type Version uint64

type Snapshot struct {
	State           HomeState
	Home            Home
	Version         Version
	Dirty           bool
	PendingTerminal bool
	Terminal        bool
}

func (s Snapshot) CanAcquire() bool {
	return s.State != Unknown && !s.PendingTerminal && !s.Terminal
}

func (s Snapshot) CanReturn() bool {
	return s.State == CurrentHome && !s.PendingTerminal && !s.Terminal
}

type indexEntry struct {
	snapshot   Snapshot
	loading    bool
	references int
}

// Index is feature-owned metadata, not a world mutation or persistence queue.
// Home mutations require the owning execution path. Its lock never spans ECS,
// I/O, callbacks or acquiring a shard lock.
type Index struct {
	mu          sync.RWMutex
	characters  map[types.EntityID]*indexEntry
	owners      map[ObjectKey]types.EntityID
	nextVersion Version
}

func NewIndex() *Index {
	return &Index{
		characters: make(map[types.EntityID]*indexEntry),
		owners:     make(map[ObjectKey]types.EntityID),
	}
}

func (i *Index) Snapshot(character types.EntityID) Snapshot {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if entry := i.characters[character]; entry != nil {
		return entry.snapshot
	}
	return Snapshot{State: Unknown}
}

// Owner reports eligible runtime ownership only. Unknown metadata or a pending
// terminal owner is not proof of an orphan; cleanup uses Snapshot.
func (i *Index) Owner(object ObjectKey) (types.EntityID, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	character, exists := i.owners[object]
	if !exists || !i.characters[character].snapshot.CanReturn() {
		return 0, false
	}
	return character, true
}

// BeginLookup admits one read for an unknown character and returns its observed
// version. Once submitted, the read must reach CompleteLookup exactly once,
// even after logout or a runtime change. Only a rejected submission may release it.
func (i *Index) BeginLookup(character types.EntityID) (Version, bool) {
	if !validID(character) {
		return 0, false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.entryLocked(character)
	if entry.snapshot.State != Unknown || entry.loading || entry.snapshot.Terminal {
		return 0, false
	}
	entry.loading = true
	return entry.snapshot.Version, true
}

// A nil home is a successful lookup of known absence, not a failed SQL read.
// Hydration may prepare cleanup metadata while PendingTerminal blocks admission.
// The submitter delivers each accepted read result exactly once. A stale result
// still releases loading, which prevents eviction until that read completes.
func (i *Index) CompleteLookup(character types.EntityID, expected Version, home *Home) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.characters[character]
	if entry == nil || !entry.loading {
		return ErrStaleLookup
	}
	entry.loading = false
	if entry.snapshot.Version != expected || entry.snapshot.Terminal {
		return ErrStaleLookup
	}
	if home == nil {
		entry.snapshot.State = NoHome
		return nil
	}
	if !validHome(*home) {
		return ErrInvalidHome
	}
	if owner, exists := i.owners[home.Object]; exists && owner != character {
		return ErrObjectOwned
	}
	entry.snapshot.State = CurrentHome
	entry.snapshot.Home = *home
	i.owners[home.Object] = character
	return nil
}

// RejectLookup releases a read only after its queue submission was rejected.
// The BeginLookup caller invokes it once, immediately after rejection guarantees
// no job or result can follow. It must never cancel an accepted read or be retried.
func (i *Index) RejectLookup(character types.EntityID) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.characters[character]
	if entry == nil || !entry.loading {
		return false
	}
	entry.loading = false
	return true
}

// ReplaceHome commits an already-prepared owning-shard completion. PendingTerminal
// blocks new admissions, not a completion already ordered before terminal apply.
// Applied terminal state and a changed home version always reject assignment.
func (i *Index) ReplaceHome(character types.EntityID, expected Version, home Home) (Snapshot, error) {
	if !validID(character) {
		return Snapshot{}, ErrInvalidCharacter
	}
	if !validHome(home) {
		return Snapshot{}, ErrInvalidHome
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.characters[character]
	if entry == nil || entry.snapshot.State == Unknown {
		return Snapshot{}, ErrUnknownHome
	}
	if entry.snapshot.Terminal {
		return Snapshot{}, ErrTerminal
	}
	if entry.snapshot.Version != expected {
		return Snapshot{}, ErrVersionChanged
	}
	if owner, exists := i.owners[home.Object]; exists && owner != character {
		return Snapshot{}, ErrObjectOwned
	}
	if entry.snapshot.State == CurrentHome && entry.snapshot.Home == home {
		return entry.snapshot, nil
	}
	i.dirtyLocked(entry)
	i.removeOwnerLocked(entry)
	entry.snapshot.State, entry.snapshot.Home = CurrentHome, home
	i.owners[home.Object] = character
	return entry.snapshot, nil
}

func (i *Index) RevokeHome(character types.EntityID) (Snapshot, error) {
	if !validID(character) {
		return Snapshot{}, ErrInvalidCharacter
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.entryLocked(character)
	if entry.snapshot.State != NoHome {
		i.dirtyLocked(entry)
		i.removeOwnerLocked(entry)
		entry.snapshot.State, entry.snapshot.Home = NoHome, Home{}
	}
	return entry.snapshot, nil
}

// MarkTerminalPending can be called from another layer or HTTP notification;
// it only blocks eligibility, leaving current association changes to its owner.
func (i *Index) MarkTerminalPending(character types.EntityID) error {
	if !validID(character) {
		return ErrInvalidCharacter
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.entryLocked(character)
	if !entry.snapshot.Terminal {
		entry.snapshot.PendingTerminal = true
	}
	return nil
}

// ApplyTerminal runs on the owning path and revokes its latest home, not a home
// captured earlier by the notifying thread.
func (i *Index) ApplyTerminal(character types.EntityID) (Snapshot, error) {
	if !validID(character) {
		return Snapshot{}, ErrInvalidCharacter
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.entryLocked(character)
	if !entry.snapshot.Terminal {
		i.dirtyLocked(entry)
		i.removeOwnerLocked(entry)
		entry.snapshot.State, entry.snapshot.Home = NoHome, Home{}
		entry.snapshot.Terminal, entry.snapshot.PendingTerminal = true, false
	}
	return entry.snapshot, nil
}

// AcknowledgeSaved only clears the exact saved revision. The future saver must
// hold a Reference while its copied snapshot is in flight; no worker exists here.
func (i *Index) AcknowledgeSaved(character types.EntityID, version Version) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.characters[character]
	if entry == nil || !entry.snapshot.Dirty || entry.snapshot.Version != version {
		return false
	}
	entry.snapshot.Dirty = false
	return true
}

// Reference retains cache metadata for a body, session, loaded object or write.
// It does not reserve a gameplay object. Release is safe to call more than once.
// Keep the pointer returned by Retain; a Reference must not be copied after use.
type Reference struct {
	index     *Index
	character types.EntityID
	released  bool // protected by index.mu
}

func (i *Index) Retain(character types.EntityID) (*Reference, error) {
	if !validID(character) {
		return nil, ErrInvalidCharacter
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.entryLocked(character)
	entry.references++
	return &Reference{index: i, character: character}, nil
}

func (r *Reference) Release() {
	if r == nil || r.index == nil {
		return
	}
	i := r.index
	i.mu.Lock()
	defer i.mu.Unlock()
	if r.released {
		return
	}
	r.released = true
	// This reference prevents eviction until its first release.
	i.characters[r.character].references--
}

func (i *Index) Evict(character types.EntityID) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.characters[character]
	if entry == nil || entry.snapshot.Dirty || entry.snapshot.PendingTerminal || entry.loading || entry.references != 0 {
		return false
	}
	i.removeOwnerLocked(entry)
	delete(i.characters, character)
	return true
}

func (i *Index) entryLocked(character types.EntityID) *indexEntry {
	entry := i.characters[character]
	if entry == nil {
		entry = &indexEntry{snapshot: Snapshot{Version: i.versionLocked()}}
		i.characters[character] = entry
	}
	return entry
}

func (i *Index) removeOwnerLocked(entry *indexEntry) {
	if entry.snapshot.State == CurrentHome {
		delete(i.owners, entry.snapshot.Home.Object)
	}
}

func (i *Index) dirtyLocked(entry *indexEntry) {
	entry.snapshot.Version = i.versionLocked()
	entry.snapshot.Dirty = true
}

func (i *Index) versionLocked() Version {
	if i.nextVersion == math.MaxUint64 {
		panic("hearth version exhausted")
	}
	i.nextVersion++
	return i.nextVersion
}

func validID(id types.EntityID) bool {
	return id != 0 && id <= math.MaxInt64
}

func validHome(home Home) bool {
	return validID(home.Object.ID) && home.Layer == SurfaceLayer &&
		home.Object.Region >= math.MinInt32 && home.Object.Region <= math.MaxInt32 &&
		home.X >= math.MinInt32 && home.X <= math.MaxInt32 && home.Y >= math.MinInt32 && home.Y <= math.MaxInt32
}
