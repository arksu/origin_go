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
	Entries map[types.EntityID]AdminTeleportEntry
}

// AdminTeleportEntry holds validated parameters for a deferred /tp command.
type AdminTeleportEntry struct {
	// TargetLayer overrides current layer when set.
	TargetLayer *int
}

func (p *PendingAdminTeleport) Set(playerID types.EntityID, entry AdminTeleportEntry) {
	p.Entries[playerID] = entry
}

func (p *PendingAdminTeleport) Get(playerID types.EntityID) (AdminTeleportEntry, bool) {
	e, ok := p.Entries[playerID]
	return e, ok
}

func (p *PendingAdminTeleport) Clear(playerID types.EntityID) {
	delete(p.Entries, playerID)
}
