package hearth

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"

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

// Generation survives eviction through the Index's monotonic sequence.
type Version struct {
	Generation uint64
	Revision   uint64
}

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

type LookupToken struct {
	character  types.EntityID
	generation uint64
	lookup     uint64
	revision   uint64
}

type indexEntry struct {
	snapshot   Snapshot
	lookup     uint64
	references int
}

// Index is feature-owned metadata, not a world mutation or persistence queue.
// Home mutations require the owning execution path. Its lock never spans ECS,
// I/O, callbacks or acquiring a shard lock.
type Index struct {
	mu             sync.RWMutex
	characters     map[types.EntityID]*indexEntry
	owners         map[ObjectKey]types.EntityID
	nextGeneration uint64
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

// BeginLookup coalesces one read for an unknown character. CompleteLookup or
// CancelLookup must finish it, even when a runtime change makes it obsolete.
func (i *Index) BeginLookup(character types.EntityID) (LookupToken, bool) {
	if !validID(character) {
		return LookupToken{}, false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.entryLocked(character)
	if entry.snapshot.State != Unknown || entry.lookup != 0 || entry.snapshot.Terminal {
		return LookupToken{}, false
	}
	entry.lookup = i.generationLocked()
	return LookupToken{
		character: character, generation: entry.snapshot.Version.Generation,
		lookup: entry.lookup, revision: entry.snapshot.Version.Revision,
	}, true
}

// A nil home is a successful lookup of known absence, not a failed SQL read.
// Hydration may prepare cleanup metadata while PendingTerminal blocks admission.
func (i *Index) CompleteLookup(token LookupToken, home *Home) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	entry, matches := i.lookupLocked(token)
	if !matches {
		return ErrStaleLookup
	}
	entry.lookup = 0
	if entry.snapshot.Version.Revision != token.revision || entry.snapshot.Terminal {
		return ErrStaleLookup
	}
	if home == nil {
		entry.snapshot.State = NoHome
		return nil
	}
	if !validHome(*home) {
		return ErrInvalidHome
	}
	if owner, exists := i.owners[home.Object]; exists && owner != token.character {
		return ErrObjectOwned
	}
	entry.snapshot.State = CurrentHome
	entry.snapshot.Home = *home
	i.owners[home.Object] = token.character
	return nil
}

func (i *Index) CancelLookup(token LookupToken) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	entry, matches := i.lookupLocked(token)
	if matches {
		entry.lookup = 0
	}
	return matches
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
	index      *Index
	character  types.EntityID
	generation uint64
	released   atomic.Bool
}

func (i *Index) Retain(character types.EntityID) (*Reference, error) {
	if !validID(character) {
		return nil, ErrInvalidCharacter
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.entryLocked(character)
	entry.references++
	return &Reference{index: i, character: character, generation: entry.snapshot.Version.Generation}, nil
}

func (r *Reference) Release() {
	if r == nil || r.index == nil || !r.released.CompareAndSwap(false, true) {
		return
	}
	i := r.index
	i.mu.Lock()
	defer i.mu.Unlock()
	if entry := i.characters[r.character]; entry != nil && entry.snapshot.Version.Generation == r.generation {
		entry.references--
	}
}

func (i *Index) Evict(character types.EntityID) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.characters[character]
	if entry == nil || entry.snapshot.Dirty || entry.snapshot.PendingTerminal || entry.lookup != 0 || entry.references != 0 {
		return false
	}
	i.removeOwnerLocked(entry)
	delete(i.characters, character)
	return true
}

func (i *Index) entryLocked(character types.EntityID) *indexEntry {
	entry := i.characters[character]
	if entry == nil {
		entry = &indexEntry{snapshot: Snapshot{Version: Version{Generation: i.generationLocked()}}}
		i.characters[character] = entry
	}
	return entry
}

func (i *Index) lookupLocked(token LookupToken) (*indexEntry, bool) {
	entry := i.characters[token.character]
	return entry, entry != nil && token.lookup != 0 && entry.lookup == token.lookup && entry.snapshot.Version.Generation == token.generation
}

func (i *Index) removeOwnerLocked(entry *indexEntry) {
	if entry.snapshot.State == CurrentHome {
		delete(i.owners, entry.snapshot.Home.Object)
	}
}

func (i *Index) dirtyLocked(entry *indexEntry) {
	if entry.snapshot.Version.Revision == math.MaxUint64 {
		panic("hearth revision exhausted")
	}
	entry.snapshot.Version.Revision++
	entry.snapshot.Dirty = true
}

func (i *Index) generationLocked() uint64 {
	if i.nextGeneration == math.MaxUint64 {
		panic("hearth generation exhausted")
	}
	i.nextGeneration++
	return i.nextGeneration
}

func validID(id types.EntityID) bool {
	return id != 0 && id <= math.MaxInt64
}

func validHome(home Home) bool {
	return validID(home.Object.ID) && home.Layer == SurfaceLayer &&
		home.Object.Region >= math.MinInt32 && home.Object.Region <= math.MaxInt32 &&
		home.X >= math.MinInt32 && home.X <= math.MaxInt32 && home.Y >= math.MinInt32 && home.Y <= math.MaxInt32
}
