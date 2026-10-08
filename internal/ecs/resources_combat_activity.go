package ecs

import (
	"errors"
	"math"

	"origin/internal/types"
)

const CombatLogoutHoldMs int64 = 30_000

var (
	ErrCombatActivityUnprepared = errors.New("combat activity: target is not prepared")
	ErrInvalidCombatActivity    = errors.New("combat activity: invalid timestamp or state")
)

// CombatState is runtime-only metadata. HasEvent distinguishes no combat from
// an accepted event at UnixMs zero. Copies belong to runtime transfer/cache state.
type CombatState struct {
	HasEvent                bool
	LastCombatEventAtUnixMs int64
}

type combatActivityRecord struct {
	identity types.EntityID
	state    CombatState
}

// CombatActivityState owns the exact-handle registrations used by creature
// damage and logout. All access requires the owning shard lock. Preparation may
// allocate; event validation and recording reuse existing map keys.
type CombatActivityState struct {
	records map[types.Handle]combatActivityRecord
}

func NewCombatActivityState() *CombatActivityState {
	return &CombatActivityState{records: make(map[types.Handle]combatActivityRecord)}
}

// Prepare is idempotent for the same exact identity; it never resets activity.
func (s *CombatActivityState) Prepare(handle types.Handle, identity types.EntityID) bool {
	if s == nil || handle == types.InvalidHandle || identity == 0 {
		return false
	}
	if previous, exists := s.records[handle]; exists {
		return previous.identity == identity
	}
	if s.records == nil {
		s.records = make(map[types.Handle]combatActivityRecord)
	}
	s.records[handle] = combatActivityRecord{identity: identity}
	return true
}

func (s *CombatActivityState) IsPrepared(handle types.Handle, identity types.EntityID) bool {
	if s == nil {
		return false
	}
	record, exists := s.records[handle]
	return exists && record.identity == identity
}

func (s *CombatActivityState) Identity(handle types.Handle) (types.EntityID, bool) {
	if s == nil {
		return 0, false
	}
	record, exists := s.records[handle]
	return record.identity, exists
}

func (s *CombatActivityState) PreparedCount() int {
	if s == nil {
		return 0
	}
	return len(s.records)
}

func ValidateCombatState(state CombatState) error {
	if (!state.HasEvent && state.LastCombatEventAtUnixMs != 0) || state.LastCombatEventAtUnixMs < 0 ||
		state.LastCombatEventAtUnixMs > math.MaxInt64-CombatLogoutHoldMs {
		return ErrInvalidCombatActivity
	}
	return nil
}

// ValidateEvent precedes acceptance/payment, so recording the prepared event
// has no remaining fallible checks during a same-lock combat commit.
func (s *CombatActivityState) ValidateEvent(handle types.Handle, identity types.EntityID, now int64) error {
	if s == nil {
		return ErrCombatActivityUnprepared
	}
	record, exists := s.records[handle]
	if !exists || record.identity != identity {
		return ErrCombatActivityUnprepared
	}
	if now < 0 || now > math.MaxInt64-CombatLogoutHoldMs {
		return ErrInvalidCombatActivity
	}
	return ValidateCombatState(record.state)
}

// RecordPreparedEvent consumes a validated event within the same shard-locked
// call. Even equal timestamps are combat events; callers must request logout
// rechecking after this method because the hit can also have entered KO.
func (s *CombatActivityState) RecordPreparedEvent(handle types.Handle, now int64) {
	record := s.records[handle]
	if !record.state.HasEvent || now > record.state.LastCombatEventAtUnixMs {
		record.state = CombatState{HasEvent: true, LastCombatEventAtUnixMs: now}
		s.records[handle] = record
	}
}

func (s *CombatActivityState) Capture(handle types.Handle) (CombatState, bool) {
	if s == nil {
		return CombatState{}, false
	}
	record, exists := s.records[handle]
	return record.state, exists
}

// Restore replaces only a prepared record and rejects invalid snapshots before
// mutation. It is used before publication of a transferred or cached body.
func (s *CombatActivityState) Restore(handle types.Handle, state CombatState) error {
	if s == nil {
		return ErrCombatActivityUnprepared
	}
	record, exists := s.records[handle]
	if !exists {
		return ErrCombatActivityUnprepared
	}
	if err := ValidateCombatState(state); err != nil {
		return err
	}
	record.state = state
	s.records[handle] = record
	return nil
}

func (s *CombatActivityState) Release(handle types.Handle) {
	if s != nil {
		delete(s.records, handle)
	}
}
