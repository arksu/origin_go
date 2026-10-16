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
	ErrObjectOwned      = errors.New("hearth object already has an owner")
	ErrTerminal         = errors.New("hearth owner is terminal")
)

type ObjectKey struct {
	Region int
	ID     types.EntityID
}

// Home owns its metadata; it never retains ECS components or raw-cache pointers.
type Home struct {
	Object    ObjectKey
	X         int
	Y         int
	Layer     int
	OwnerName string
}

type HomeState uint8

const (
	Unknown HomeState = iota
	NoHome
	CurrentHome
)

type Snapshot struct {
	State           HomeState
	Home            Home
	PendingTerminal bool
	Terminal        bool
}

func (s Snapshot) CanAcquire() bool {
	return s.State != Unknown && !s.PendingTerminal && !s.Terminal
}

func (s Snapshot) CanReturn() bool {
	return s.State == CurrentHome && !s.PendingTerminal && !s.Terminal
}

// Index is feature-owned metadata, not a world mutation or persistence queue.
// Home mutations require the owning execution path. Its lock never spans ECS,
// I/O, callbacks or acquiring a shard lock.
type Index struct {
	mu         sync.RWMutex
	characters map[types.EntityID]*Snapshot
	owners     map[ObjectKey]types.EntityID
}

func NewIndex() *Index {
	return &Index{
		characters: make(map[types.EntityID]*Snapshot),
		owners:     make(map[ObjectKey]types.EntityID),
	}
}

func (i *Index) Snapshot(character types.EntityID) Snapshot {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if entry := i.characters[character]; entry != nil {
		return *entry
	}
	return Snapshot{State: Unknown}
}

// SnapshotByObject includes pending ownership so restoration can distinguish an
// ineligible current home from an orphan.
func (i *Index) SnapshotByObject(object ObjectKey) (types.EntityID, Snapshot, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	character, exists := i.owners[object]
	if !exists {
		return 0, Snapshot{}, false
	}
	return character, *i.characters[character], true
}

// Owner reports eligible runtime ownership only. A pending terminal owner is
// not proof of an orphan; cleanup uses SnapshotByObject.
func (i *Index) Owner(object ObjectKey) (types.EntityID, bool) {
	character, snapshot, exists := i.SnapshotByObject(object)
	if !exists || !snapshot.CanReturn() {
		return 0, false
	}
	return character, true
}

// LoadIfUnknown hydrates copied metadata under the owning surface shard lock.
// Known runtime state wins over saved data. A nil home establishes known absence;
// pending terminal state is preserved while preparing cleanup metadata.
func (i *Index) LoadIfUnknown(character types.EntityID, home *Home) error {
	if !validID(character) {
		return ErrInvalidCharacter
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if entry := i.characters[character]; entry != nil && (entry.State != Unknown || entry.Terminal) {
		return nil
	}
	if home != nil {
		if !validHome(*home) {
			return ErrInvalidHome
		}
		if owner, exists := i.owners[home.Object]; exists && owner != character {
			return ErrObjectOwned
		}
	}
	entry := i.entryLocked(character)
	if home == nil {
		entry.State = NoHome
	} else {
		entry.State, entry.Home = CurrentHome, *home
		i.owners[home.Object] = character
	}
	return nil
}

// ReplaceHome commits an already-prepared owning-shard completion. PendingTerminal
// blocks new admissions, not a completion already ordered before terminal apply.
// Applied terminal state always rejects assignment.
func (i *Index) ReplaceHome(character types.EntityID, home Home) (Snapshot, error) {
	if !validID(character) {
		return Snapshot{}, ErrInvalidCharacter
	}
	if !validHome(home) {
		return Snapshot{}, ErrInvalidHome
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.characters[character]
	if entry == nil || entry.State == Unknown {
		return Snapshot{}, ErrUnknownHome
	}
	if entry.Terminal {
		return Snapshot{}, ErrTerminal
	}
	if owner, exists := i.owners[home.Object]; exists && owner != character {
		return Snapshot{}, ErrObjectOwned
	}
	if entry.State == CurrentHome && entry.Home == home {
		return *entry, nil
	}
	i.removeOwnerLocked(entry)
	entry.State, entry.Home = CurrentHome, home
	i.owners[home.Object] = character
	return *entry, nil
}

func (i *Index) RevokeHome(character types.EntityID) (Snapshot, error) {
	if !validID(character) {
		return Snapshot{}, ErrInvalidCharacter
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entry := i.entryLocked(character)
	if entry.State != NoHome {
		i.removeOwnerLocked(entry)
		entry.State, entry.Home = NoHome, Home{}
	}
	return *entry, nil
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
	if !entry.Terminal {
		entry.PendingTerminal = true
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
	if !entry.Terminal {
		i.removeOwnerLocked(entry)
		entry.State, entry.Home = NoHome, Home{}
		entry.Terminal, entry.PendingTerminal = true, false
	}
	return *entry, nil
}

func (i *Index) entryLocked(character types.EntityID) *Snapshot {
	entry := i.characters[character]
	if entry == nil {
		entry = &Snapshot{}
		i.characters[character] = entry
	}
	return entry
}

func (i *Index) removeOwnerLocked(entry *Snapshot) {
	if entry.State == CurrentHome {
		delete(i.owners, entry.Home.Object)
	}
}

func validID(id types.EntityID) bool {
	return id != 0 && id <= math.MaxInt64
}

func validHome(home Home) bool {
	return validID(home.Object.ID) && home.Layer == SurfaceLayer &&
		home.Object.Region >= math.MinInt32 && home.Object.Region <= math.MaxInt32 &&
		home.X >= math.MinInt32 && home.X <= math.MaxInt32 && home.Y >= math.MinInt32 && home.Y <= math.MaxInt32
}
