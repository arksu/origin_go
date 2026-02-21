package ecs

import "origin/internal/types"

// PendingAdminSpawn tracks pending admin spawn commands per player.
// When a player issues /spawn, an entry is stored here; the next MoveTo/MoveToEntity
// click provides target coordinates and triggers the actual spawn.
type PendingAdminSpawn struct {
	Entries map[types.EntityID]AdminSpawnEntry
}

// AdminSpawnEntry holds validated parameters for a deferred /spawn command.
type AdminSpawnEntry struct {
	ObjectKey string
	DefID     int
	Quality   uint32
}

func (p *PendingAdminSpawn) Set(playerID types.EntityID, entry AdminSpawnEntry) {
	p.Entries[playerID] = entry
}

func (p *PendingAdminSpawn) Get(playerID types.EntityID) (AdminSpawnEntry, bool) {
	e, ok := p.Entries[playerID]
	return e, ok
}

func (p *PendingAdminSpawn) Clear(playerID types.EntityID) {
	delete(p.Entries, playerID)
}

// PendingAdminTeleport tracks deferred admin teleports per player.
// When a player issues /tp without coordinates, the next map click supplies target coordinates.
type PendingAdminTeleport struct {
	Entries map[types.EntityID]struct{}
}

func (p *PendingAdminTeleport) Set(playerID types.EntityID) {
	p.Entries[playerID] = struct{}{}
}

func (p *PendingAdminTeleport) Get(playerID types.EntityID) bool {
	_, ok := p.Entries[playerID]
	return ok
}

func (p *PendingAdminTeleport) Clear(playerID types.EntityID) {
	delete(p.Entries, playerID)
}
