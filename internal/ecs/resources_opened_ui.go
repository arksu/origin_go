package ecs

import "origin/internal/types"

// OpenedWindowsState tracks client-opened UI windows per player entity.
// Used to avoid computing/pushing expensive UI payloads when the UI is closed.
type OpenedWindowsState struct {
	ByPlayer map[types.EntityID]map[string]struct{}
}

func (s *OpenedWindowsState) Open(playerID types.EntityID, name string) {
	if s == nil || playerID == 0 || name == "" {
		return
	}
	if s.ByPlayer == nil {
		s.ByPlayer = make(map[types.EntityID]map[string]struct{}, 32)
	}
	set, ok := s.ByPlayer[playerID]
	if !ok {
		set = make(map[string]struct{}, 4)
		s.ByPlayer[playerID] = set
	}
	set[name] = struct{}{}
}

func (s *OpenedWindowsState) Close(playerID types.EntityID, name string) {
	if s == nil || playerID == 0 || name == "" {
		return
	}
	set, ok := s.ByPlayer[playerID]
	if !ok {
		return
	}
	delete(set, name)
	if len(set) == 0 {
		delete(s.ByPlayer, playerID)
	}
}

func (s *OpenedWindowsState) IsOpen(playerID types.EntityID, name string) bool {
	if s == nil || playerID == 0 || name == "" {
		return false
	}
	set, ok := s.ByPlayer[playerID]
	if !ok {
		return false
	}
	_, exists := set[name]
	return exists
}

func (s *OpenedWindowsState) ClearPlayer(playerID types.EntityID) {
	if s == nil || playerID == 0 {
		return
	}
	delete(s.ByPlayer, playerID)
}
