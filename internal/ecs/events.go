package ecs

import (
	"origin/internal/types"
	"time"
)

const (
	TopicGameplayAll              = "gameplay.*"
	TopicGameplayCombat           = "gameplay.combat.*"
	TopicGameplayCombatDamage     = "gameplay.combat.damage_dealt"
	TopicGameplayCombatDeath      = "gameplay.combat.death"
	TopicGameplayCombatHeal       = "gameplay.combat.heal"
	TopicGameplayMovement         = "gameplay.movement.*"
	TopicGameplayMovementMove     = "gameplay.movement.move"
	TopicGameplayMovementTeleport = "gameplay.movement.teleport"
	TopicGameplayEntity           = "gameplay.entity.*"
	TopicGameplayEntitySpawn      = "gameplay.entity.spawn"
	TopicGameplayEntityDespawn    = "gameplay.entity.despawn"
	TopicGameplayEntityUpdate     = "gameplay.entity.update"
	TopicGameplayChunk            = "gameplay.chunk.*"
	TopicGameplayChunkLoad        = "gameplay.chunk.load"
	TopicGameplayChunkUnload      = "gameplay.chunk.unload"
	TopicSystemAll                = "system.*"
	TopicSystemTick               = "system.tick"
	TopicSystemShutdown           = "system.shutdown"
	TopicNetworkAll               = "network.*"
	TopicNetworkConnect           = "network.connect"
	TopicNetworkDisconnect        = "network.disconnect"
	TopicNetworkMessage           = "network.message"
)

// EntitySpawnEvent represents when an entity becomes visible to an observer
type EntitySpawnEvent struct {
	topic        string
	Timestamp    time.Time
	ObserverID   types.EntityID
	TargetID     types.EntityID
	TargetHandle types.Handle
}

func (e *EntitySpawnEvent) Topic() string { return e.topic }

func NewEntitySpawnEvent(observerID, targetID types.EntityID, targetHandle types.Handle) *EntitySpawnEvent {
	return &EntitySpawnEvent{
		topic:        TopicGameplayEntitySpawn,
		Timestamp:    time.Now(),
		ObserverID:   observerID,
		TargetID:     targetID,
		TargetHandle: targetHandle,
	}
}

// EntityDespawnEvent represents when an entity becomes invisible to an observer
type EntityDespawnEvent struct {
	topic      string
	Timestamp  time.Time
	ObserverID types.EntityID
	TargetID   types.EntityID
}

func (e *EntityDespawnEvent) Topic() string { return e.topic }

func NewEntityDespawnEvent(observerID, targetID types.EntityID) *EntityDespawnEvent {
	return &EntityDespawnEvent{
		topic:      TopicGameplayEntityDespawn,
		Timestamp:  time.Now(),
		ObserverID: observerID,
		TargetID:   targetID,
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
		topic:     TopicGameplayEntitySpawn, // Reuse entity spawn topic for now
		Timestamp: time.Now(),
		EntityID:  entityID,
		Layer:     layer,
		X:         x,
		Y:         y,
	}
}

// ChunkLoadEvent represents when a chunk is loaded
type ChunkLoadEvent struct {
	topic     string
	Timestamp time.Time
	X         int
	Y         int
	Layer     int
}

func (e *ChunkLoadEvent) Topic() string { return e.topic }

func NewChunkLoadEvent(x, y, layer int) *ChunkLoadEvent {
	return &ChunkLoadEvent{
		topic:     TopicGameplayChunkLoad,
		Timestamp: time.Now(),
		X:         x,
		Y:         y,
		Layer:     layer,
	}
}

// ChunkUnloadEvent represents when a chunk is unloaded
type ChunkUnloadEvent struct {
	topic     string
	Timestamp time.Time
	X         int
	Y         int
	Layer     int
}

func (e *ChunkUnloadEvent) Topic() string { return e.topic }

func NewChunkUnloadEvent(x, y, layer int) *ChunkUnloadEvent {
	return &ChunkUnloadEvent{
		topic:     TopicGameplayChunkUnload,
		Timestamp: time.Now(),
		X:         x,
		Y:         y,
		Layer:     layer,
	}
}

// ObjectMoveEvent represents an object movement event for network transmission
type ObjectMoveEvent struct {
	topic     string
	Timestamp time.Time
	EntityID  types.EntityID
	Movement  interface{} // Using interface{} to avoid import cycle with proto package
}

func (e *ObjectMoveEvent) Topic() string { return e.topic }

func NewObjectMoveEvent(entityID types.EntityID, movement interface{}) *ObjectMoveEvent {
	return &ObjectMoveEvent{
		topic:     TopicGameplayMovementMove,
		Timestamp: time.Now(),
		EntityID:  entityID,
		Movement:  movement,
	}
}
