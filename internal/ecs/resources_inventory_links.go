package ecs

import (
	"time"

	constt "origin/internal/const"
	"origin/internal/types"
)

// InventoryRefKey uniquely identifies an inventory container: (kind, owner_id, inventory_key)
type InventoryRefKey struct {
	Kind    constt.InventoryKind
	OwnerID types.EntityID
	Key     uint32
}

// InventoryRefIndex provides O(1) lookup from InventoryRef to Handle
type InventoryRefIndex struct {
	index map[InventoryRefKey]types.Handle
}

func (idx *InventoryRefIndex) Add(kind constt.InventoryKind, ownerID types.EntityID, key uint32, handle types.Handle) {
	idx.index[InventoryRefKey{Kind: kind, OwnerID: ownerID, Key: key}] = handle
}

func (idx *InventoryRefIndex) Remove(kind constt.InventoryKind, ownerID types.EntityID, key uint32) {
	delete(idx.index, InventoryRefKey{Kind: kind, OwnerID: ownerID, Key: key})
}

func (idx *InventoryRefIndex) Lookup(kind constt.InventoryKind, ownerID types.EntityID, key uint32) (types.Handle, bool) {
	h, ok := idx.index[InventoryRefKey{Kind: kind, OwnerID: ownerID, Key: key}]
	return h, ok
}

// RemoveAllByOwner removes all inventory refs for the owner and returns removed handles.
func (idx *InventoryRefIndex) RemoveAllByOwner(ownerID types.EntityID) []types.Handle {
	if len(idx.index) == 0 {
		return nil
	}

	removed := make([]types.Handle, 0, 2)
	for key, handle := range idx.index {
		if key.OwnerID != ownerID {
			continue
		}
		delete(idx.index, key)
		removed = append(removed, handle)
	}
	return removed
}

// PlayerLink stores the current one-to-one link between a player and a world target.
type PlayerLink struct {
	PlayerID     types.EntityID
	PlayerHandle types.Handle
	TargetID     types.EntityID
	TargetHandle types.Handle

	// Snapshot positions taken at link creation.
	PlayerX float64
	PlayerY float64
	TargetX float64
	TargetY float64

	CreatedAt time.Time
}

// LinkIntent stores explicit player intent to link with a specific target.
// Link is created only when collision confirms physical contact with this target.
type LinkIntent struct {
	TargetID     types.EntityID
	TargetHandle types.Handle
	CreatedAt    time.Time
}

// LinkState keeps active links and reverse index in ECS resource storage.
//
// Invariants:
// - one player has at most one active link (LinkedByPlayer)
// - PlayersByTarget is the reverse index for fast fanout.
type LinkState struct {
	LinkedByPlayer  map[types.EntityID]PlayerLink
	PlayersByTarget map[types.EntityID]map[types.EntityID]struct{}
	IntentByPlayer  map[types.EntityID]LinkIntent
}

func (s *LinkState) SetIntent(playerID, targetID types.EntityID, targetHandle types.Handle, createdAt time.Time) {
	s.IntentByPlayer[playerID] = LinkIntent{
		TargetID:     targetID,
		TargetHandle: targetHandle,
		CreatedAt:    createdAt,
	}
}

func (s *LinkState) ClearIntent(playerID types.EntityID) {
	delete(s.IntentByPlayer, playerID)
}

func (s *LinkState) SetLink(link PlayerLink) {
	// Keep reverse index consistent when relinking.
	if prev, ok := s.LinkedByPlayer[link.PlayerID]; ok && prev.TargetID != link.TargetID {
		if players, found := s.PlayersByTarget[prev.TargetID]; found {
			delete(players, link.PlayerID)
			if len(players) == 0 {
				delete(s.PlayersByTarget, prev.TargetID)
			}
		}
	}

	s.LinkedByPlayer[link.PlayerID] = link

	players, ok := s.PlayersByTarget[link.TargetID]
	if !ok {
		players = make(map[types.EntityID]struct{}, 4)
		s.PlayersByTarget[link.TargetID] = players
	}
	players[link.PlayerID] = struct{}{}
}

func (s *LinkState) RemoveLink(playerID types.EntityID) (PlayerLink, bool) {
	link, ok := s.LinkedByPlayer[playerID]
	if !ok {
		return PlayerLink{}, false
	}

	delete(s.LinkedByPlayer, playerID)

	if players, found := s.PlayersByTarget[link.TargetID]; found {
		delete(players, playerID)
		if len(players) == 0 {
			delete(s.PlayersByTarget, link.TargetID)
		}
	}

	return link, true
}

func (s *LinkState) GetLink(playerID types.EntityID) (PlayerLink, bool) {
	link, ok := s.LinkedByPlayer[playerID]
	return link, ok
}

// BreakLinkForPlayer removes the active gameplay link for a player, clears link intent,
// and publishes LinkBrokenEvent when a link existed.
func BreakLinkForPlayer(w *World, playerID types.EntityID, reason LinkBreakReason) (PlayerLink, bool, error) {
	if w == nil || playerID == 0 {
		return PlayerLink{}, false, nil
	}
	linkState := GetResource[LinkState](w)
	link, removed := linkState.RemoveLink(playerID)
	if !removed {
		linkState.ClearIntent(playerID)
		return PlayerLink{}, false, nil
	}

	// Keep a newer retarget intent alive only if it points elsewhere.
	if intent, hasIntent := linkState.IntentByPlayer[playerID]; !hasIntent || intent.TargetID == link.TargetID {
		linkState.ClearIntent(playerID)
	}

	if w.eventBus == nil {
		return link, true, nil
	}
	if err := w.eventBus.PublishSync(NewLinkBrokenEvent(w.Layer, playerID, link.TargetID, reason)); err != nil {
		return link, true, err
	}
	return link, true, nil
}

// OpenContainerState tracks per-player opened world object containers and nested refs.
//
// Invariants:
// - One player can have only one opened root world object at a time.
// - Opened refs are tracked per player and reverse-indexed for fanout updates.
type OpenContainerState struct {
	// PendingAutoOpenByPlayer stores object IDs that must be opened
	// immediately after successful link creation.
	PendingAutoOpenByPlayer map[types.EntityID]types.EntityID

	// OpenRootByPlayer maps player -> opened root world-object owner_id.
	OpenRootByPlayer map[types.EntityID]types.EntityID

	// PlayersByRoot maps world-object owner_id -> set(playerID).
	PlayersByRoot map[types.EntityID]map[types.EntityID]struct{}

	// OpenRefsByPlayer maps player -> set(opened inventory refs), includes root and nested.
	OpenRefsByPlayer map[types.EntityID]map[InventoryRefKey]struct{}

	// PlayersByRef maps inventory ref -> set(playerID) for update fanout.
	PlayersByRef map[InventoryRefKey]map[types.EntityID]struct{}
}

func (s *OpenContainerState) SetPendingAutoOpen(playerID, targetID types.EntityID) {
	s.PendingAutoOpenByPlayer[playerID] = targetID
}

func (s *OpenContainerState) ClearPendingAutoOpen(playerID types.EntityID) {
	delete(s.PendingAutoOpenByPlayer, playerID)
}

func (s *OpenContainerState) GetPendingAutoOpen(playerID types.EntityID) (types.EntityID, bool) {
	targetID, ok := s.PendingAutoOpenByPlayer[playerID]
	return targetID, ok
}

func (s *OpenContainerState) SetRootOpened(playerID, rootOwnerID types.EntityID) {
	if prevRoot, ok := s.OpenRootByPlayer[playerID]; ok && prevRoot != rootOwnerID {
		if players, found := s.PlayersByRoot[prevRoot]; found {
			delete(players, playerID)
			if len(players) == 0 {
				delete(s.PlayersByRoot, prevRoot)
			}
		}
	}

	s.OpenRootByPlayer[playerID] = rootOwnerID
	players, ok := s.PlayersByRoot[rootOwnerID]
	if !ok {
		players = make(map[types.EntityID]struct{}, 4)
		s.PlayersByRoot[rootOwnerID] = players
	}
	players[playerID] = struct{}{}
}

func (s *OpenContainerState) GetOpenedRoot(playerID types.EntityID) (types.EntityID, bool) {
	rootID, ok := s.OpenRootByPlayer[playerID]
	return rootID, ok
}

func (s *OpenContainerState) OpenRef(playerID types.EntityID, key InventoryRefKey) {
	refs, ok := s.OpenRefsByPlayer[playerID]
	if !ok {
		refs = make(map[InventoryRefKey]struct{}, 4)
		s.OpenRefsByPlayer[playerID] = refs
	}
	refs[key] = struct{}{}

	players, ok := s.PlayersByRef[key]
	if !ok {
		players = make(map[types.EntityID]struct{}, 4)
		s.PlayersByRef[key] = players
	}
	players[playerID] = struct{}{}
}

func (s *OpenContainerState) CloseRef(playerID types.EntityID, key InventoryRefKey) bool {
	refs, ok := s.OpenRefsByPlayer[playerID]
	if !ok {
		return false
	}
	if _, found := refs[key]; !found {
		return false
	}

	delete(refs, key)
	if len(refs) == 0 {
		delete(s.OpenRefsByPlayer, playerID)
	}

	if players, found := s.PlayersByRef[key]; found {
		delete(players, playerID)
		if len(players) == 0 {
			delete(s.PlayersByRef, key)
		}
	}
	return true
}

func (s *OpenContainerState) IsRefOpened(playerID types.EntityID, key InventoryRefKey) bool {
	refs, ok := s.OpenRefsByPlayer[playerID]
	if !ok {
		return false
	}
	_, found := refs[key]
	return found
}

func (s *OpenContainerState) PlayersOpenedRef(key InventoryRefKey) map[types.EntityID]struct{} {
	return s.PlayersByRef[key]
}

// CloseAllForPlayer removes all opened refs and root state for player and returns closed refs.
func (s *OpenContainerState) CloseAllForPlayer(playerID types.EntityID) []InventoryRefKey {
	closed := make([]InventoryRefKey, 0, 4)
	refs, hasRefs := s.OpenRefsByPlayer[playerID]
	if hasRefs {
		for key := range refs {
			closed = append(closed, key)
		}
	}

	for _, key := range closed {
		s.CloseRef(playerID, key)
	}

	if rootID, ok := s.OpenRootByPlayer[playerID]; ok {
		delete(s.OpenRootByPlayer, playerID)
		if players, found := s.PlayersByRoot[rootID]; found {
			delete(players, playerID)
			if len(players) == 0 {
				delete(s.PlayersByRoot, rootID)
			}
		}
	}

	delete(s.PendingAutoOpenByPlayer, playerID)
	return closed
}
