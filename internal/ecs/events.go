package ecs

import (
	constt "origin/internal/const"
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
	TopicGameplayPlayerEnterWorld = "gameplay.player.enter_world"
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
}

func (e *ChunkLoadEvent) Topic() string { return e.topic }

func NewChunkLoadEvent(entityID types.EntityID, x, y, layer int, tiles []byte) *ChunkLoadEvent {
	return &ChunkLoadEvent{
		topic:     TopicGameplayChunkLoad,
		Timestamp: time.Now(),
		EntityID:  entityID,
		X:         x,
		Y:         y,
		Layer:     layer,
		Tiles:     tiles,
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
}

func (e *ChunkUnloadEvent) Topic() string { return e.topic }

func NewChunkUnloadEvent(entityID types.EntityID, x, y, layer int) *ChunkUnloadEvent {
	return &ChunkUnloadEvent{
		topic:     TopicGameplayChunkUnload,
		Timestamp: time.Now(),
		EntityID:  entityID,
		X:         x,
		Y:         y,
		Layer:     layer,
	}
}

// ObjectMoveEvent represents an object movement event with raw data
type ObjectMoveEvent struct {
	topic     string
	Timestamp time.Time
	EntityID  types.EntityID
	X         int
	Y         int
	Heading   int
	VelocityX int
	VelocityY int
	MoveMode  constt.MoveMode // 0=Walk, 1=Run, 2=FastRun, 3=Swim
	IsMoving  bool
	TargetX   *int
	TargetY   *int
	Layer     int
}

func (e *ObjectMoveEvent) Topic() string { return e.topic }

func NewObjectMoveEvent(entityID types.EntityID, x, y, heading, velocityX, velocityY int, moveMode constt.MoveMode, isMoving bool, targetX, targetY *int, layer int) *ObjectMoveEvent {
	return &ObjectMoveEvent{
		topic:     TopicGameplayMovementMove,
		Timestamp: time.Now(),
		EntityID:  entityID,
		X:         x,
		Y:         y,
		Heading:   heading,
		VelocityX: velocityX,
		VelocityY: velocityY,
		MoveMode:  moveMode,
		IsMoving:  isMoving,
		TargetX:   targetX,
		TargetY:   targetY,
		Layer:     layer,
	}
}
