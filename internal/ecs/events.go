package ecs

import (
	constt "origin/internal/const"
	"origin/internal/types"

	"time"
)

const (
	TopicGameplayAll               = "gameplay.*"
	TopicGameplayCombat            = "gameplay.combat.*"
	TopicGameplayCombatDamage      = "gameplay.combat.damage_dealt"
	TopicGameplayCombatDeath       = "gameplay.combat.death"
	TopicGameplayCombatHeal        = "gameplay.combat.heal"
	TopicGameplayMovement          = "gameplay.movement.*"
	TopicGameplayMovementMoveBatch = "gameplay.movement.move_batch"
	TopicGameplayMovementTeleport  = "gameplay.movement.teleport"
	TopicGameplayPlayerEnterWorld  = "gameplay.player.enter_world"
	TopicGameplayEntity            = "gameplay.entity.*"
	TopicGameplayEntitySpawn       = "gameplay.entity.spawn"
	TopicGameplayEntityDespawn     = "gameplay.entity.despawn"
	TopicGameplayEntityUpdate      = "gameplay.entity.update"
	TopicGameplayEntityAppearance  = "gameplay.entity.appearance_changed"
	TopicGameplayLink              = "gameplay.link.*"
	TopicGameplayLinkCreated       = "gameplay.link.created"
	TopicGameplayLinkBroken        = "gameplay.link.broken"
	TopicGameplayChunk             = "gameplay.chunk.*"
	TopicGameplayChunkLoad         = "gameplay.chunk.load"
	TopicGameplayChunkUnload       = "gameplay.chunk.unload"
	TopicSystemAll                 = "system.*"
	TopicSystemTick                = "system.tick"
	TopicSystemShutdown            = "system.shutdown"
	TopicNetworkAll                = "network.*"
	TopicNetworkConnect            = "network.connect"
	TopicNetworkDisconnect         = "network.disconnect"
	TopicNetworkMessage            = "network.message"
)

// EntitySpawnEvent represents when an entity becomes visible to an observer
type EntitySpawnEvent struct {
	topic        string
	Timestamp    time.Time
	ObserverID   types.EntityID
	TargetID     types.EntityID
	TargetHandle types.Handle
	Layer        int
}

func (e *EntitySpawnEvent) Topic() string { return e.topic }

func NewEntitySpawnEvent(observerID, targetID types.EntityID, targetHandle types.Handle, layer int) *EntitySpawnEvent {
	return &EntitySpawnEvent{
		topic:        TopicGameplayEntitySpawn,
		Timestamp:    time.Now(),
		ObserverID:   observerID,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		Layer:        layer,
	}
}

// EntityDespawnEvent represents when an entity becomes invisible to an observer
type EntityDespawnEvent struct {
	topic      string
	Timestamp  time.Time
	ObserverID types.EntityID
	TargetID   types.EntityID
	Layer      int
}

func (e *EntityDespawnEvent) Topic() string { return e.topic }

func NewEntityDespawnEvent(observerID, targetID types.EntityID, layer int) *EntityDespawnEvent {
	return &EntityDespawnEvent{
		topic:      TopicGameplayEntityDespawn,
		Timestamp:  time.Now(),
		ObserverID: observerID,
		TargetID:   targetID,
		Layer:      layer,
	}
}

// PlayerEnteredWorldEvent represents when a player enters the world
type PlayerEnteredWorldEvent struct {
	topic     string
	Timestamp time.Time
	EntityID  types.EntityID
	Layer     int
	X         int
	Y         int
}

func (e *PlayerEnteredWorldEvent) Topic() string { return e.topic }

func NewPlayerEnteredWorldEvent(entityID types.EntityID, layer, x, y int) *PlayerEnteredWorldEvent {
	return &PlayerEnteredWorldEvent{
		topic:     TopicGameplayPlayerEnterWorld,
		Timestamp: time.Now(),
		EntityID:  entityID,
		Layer:     layer,
		X:         x,
		Y:         y,
	}
}

// ChunkLoadEvent represents when a chunk is loaded for a specific entity
type ChunkLoadEvent struct {
	topic     string
	Timestamp time.Time
	EntityID  types.EntityID // Entity for whom the chunk is being loaded
	X         int
	Y         int
	Layer     int
	Tiles     []byte
	Epoch     uint32
	Version   uint32 // версия чанка
}

func (e *ChunkLoadEvent) Topic() string { return e.topic }

func NewChunkLoadEvent(entityID types.EntityID, x, y, layer int, tiles []byte, epoch uint32, version uint32) *ChunkLoadEvent {
	return &ChunkLoadEvent{
		topic:     TopicGameplayChunkLoad,
		Timestamp: time.Now(),
		EntityID:  entityID,
		X:         x,
		Y:         y,
		Layer:     layer,
		Tiles:     tiles,
		Epoch:     epoch,
		Version:   version,
	}
}

// ChunkUnloadEvent represents when a chunk is unloaded for a specific entity
type ChunkUnloadEvent struct {
	topic     string
	Timestamp time.Time
	EntityID  types.EntityID // Entity for whom the chunk is being unloaded
	X         int
	Y         int
	Layer     int
	Epoch     uint32
}

func (e *ChunkUnloadEvent) Topic() string { return e.topic }

func NewChunkUnloadEvent(entityID types.EntityID, x, y, layer int, epoch uint32) *ChunkUnloadEvent {
	return &ChunkUnloadEvent{
		topic:     TopicGameplayChunkUnload,
		Timestamp: time.Now(),
		EntityID:  entityID,
		X:         x,
		Y:         y,
		Layer:     layer,
		Epoch:     epoch,
	}
}

// MoveBatchEntry holds movement data for a single entity within a batch.
type MoveBatchEntry struct {
	EntityID types.EntityID
	Handle   types.Handle
	// Carry visual relation for observers (0 when not carried). Set by the movement producer
	// so the visibility dispatcher can stay allocation/lookup stable on the hot path.
	CarriedByEntityID types.EntityID
	X                 int
	Y                 int
	Heading           float64
	VelocityX         int
	VelocityY         int
	MoveMode          constt.MoveMode
	IsMoving          bool
	TargetX           *int
	TargetY           *int
	ServerTimeMs      int64
	MoveSeq           uint32
	IsTeleport        bool
}

// ObjectMoveBatchEvent carries all movement updates for one tick in a single event.
type ObjectMoveBatchEvent struct {
	topic   string
	Layer   int
	Entries []MoveBatchEntry
}

func (e *ObjectMoveBatchEvent) Topic() string { return e.topic }

func NewObjectMoveBatchEvent(layer int, entries []MoveBatchEntry) *ObjectMoveBatchEvent {
	return &ObjectMoveBatchEvent{
		topic:   TopicGameplayMovementMoveBatch,
		Layer:   layer,
		Entries: entries,
	}
}

type LinkBreakReason string

const (
	LinkBreakMoved   LinkBreakReason = "moved"
	LinkBreakRelink  LinkBreakReason = "relink"
	LinkBreakDespawn LinkBreakReason = "despawn"
	LinkBreakClosed  LinkBreakReason = "closed"
)

// LinkCreatedEvent is published synchronously when player-target link is established.
type LinkCreatedEvent struct {
	topic     string
	Timestamp time.Time
	Layer     int
	PlayerID  types.EntityID
	TargetID  types.EntityID
}

func (e *LinkCreatedEvent) Topic() string { return e.topic }

func NewLinkCreatedEvent(layer int, playerID, targetID types.EntityID) *LinkCreatedEvent {
	return &LinkCreatedEvent{
		topic:     TopicGameplayLinkCreated,
		Timestamp: time.Now(),
		Layer:     layer,
		PlayerID:  playerID,
		TargetID:  targetID,
	}
}

// LinkBrokenEvent is published synchronously when player-target link is broken.
type LinkBrokenEvent struct {
	topic       string
	Timestamp   time.Time
	Layer       int
	PlayerID    types.EntityID
	TargetID    types.EntityID
	BreakReason LinkBreakReason
}

// EntityAppearanceChangedEvent is published when object's Appearance.Resource changes.
// Network layer rebroadcasts this as ObjectSpawn-upsert to visible observers.
type EntityAppearanceChangedEvent struct {
	topic        string
	Timestamp    time.Time
	Layer        int
	TargetID     types.EntityID
	TargetHandle types.Handle
}

func (e *EntityAppearanceChangedEvent) Topic() string { return e.topic }

func NewEntityAppearanceChangedEvent(layer int, targetID types.EntityID, targetHandle types.Handle) *EntityAppearanceChangedEvent {
	return &EntityAppearanceChangedEvent{
		topic:        TopicGameplayEntityAppearance,
		Timestamp:    time.Now(),
		Layer:        layer,
		TargetID:     targetID,
		TargetHandle: targetHandle,
	}
}

func (e *LinkBrokenEvent) Topic() string { return e.topic }

func NewLinkBrokenEvent(layer int, playerID, targetID types.EntityID, reason LinkBreakReason) *LinkBrokenEvent {
	return &LinkBrokenEvent{
		topic:       TopicGameplayLinkBroken,
		Timestamp:   time.Now(),
		Layer:       layer,
		PlayerID:    playerID,
		TargetID:    targetID,
		BreakReason: reason,
	}
}
