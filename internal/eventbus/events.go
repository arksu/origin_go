package eventbus

import "time"

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

type DamageDealtEvent struct {
	topic      string
	Timestamp  time.Time
	AttackerID uint64
	TargetID   uint64
	Damage     float64
	DamageType string
	IsCritical bool
}

func (e *DamageDealtEvent) Topic() string { return e.topic }

func NewDamageDealtEvent(attackerID, targetID uint64, damage float64, damageType string, isCritical bool) *DamageDealtEvent {
	return &DamageDealtEvent{
		topic:      TopicGameplayCombatDamage,
		Timestamp:  time.Now(),
		AttackerID: attackerID,
		TargetID:   targetID,
		Damage:     damage,
		DamageType: damageType,
		IsCritical: isCritical,
	}
}

type DeathEvent struct {
	topic     string
	Timestamp time.Time
	EntityID  uint64
	KillerID  uint64
	Cause     string
}

func (e *DeathEvent) Topic() string { return e.topic }

func NewDeathEvent(entityID, killerID uint64, cause string) *DeathEvent {
	return &DeathEvent{
		topic:     TopicGameplayCombatDeath,
		Timestamp: time.Now(),
		EntityID:  entityID,
		KillerID:  killerID,
		Cause:     cause,
	}
}

type EntitySpawnEvent struct {
	topic      string
	Timestamp  time.Time
	EntityID   uint64
	EntityType string
	X, Y, Z    float64
	Layer      int32
}

func (e *EntitySpawnEvent) Topic() string { return e.topic }

func NewEntitySpawnEvent(entityID uint64, entityType string, x, y, z float64, layer int32) *EntitySpawnEvent {
	return &EntitySpawnEvent{
		topic:      TopicGameplayEntitySpawn,
		Timestamp:  time.Now(),
		EntityID:   entityID,
		EntityType: entityType,
		X:          x,
		Y:          y,
		Z:          z,
		Layer:      layer,
	}
}

type EntityDespawnEvent struct {
	topic     string
	Timestamp time.Time
	EntityID  uint64
	Reason    string
}

func (e *EntityDespawnEvent) Topic() string { return e.topic }

func NewEntityDespawnEvent(entityID uint64, reason string) *EntityDespawnEvent {
	return &EntityDespawnEvent{
		topic:     TopicGameplayEntityDespawn,
		Timestamp: time.Now(),
		EntityID:  entityID,
		Reason:    reason,
	}
}

type MovementEvent struct {
	topic        string
	Timestamp    time.Time
	EntityID     uint64
	FromX, FromY float64
	ToX, ToY     float64
}

func (e *MovementEvent) Topic() string { return e.topic }

func NewMovementEvent(entityID uint64, fromX, fromY, toX, toY float64) *MovementEvent {
	return &MovementEvent{
		topic:     TopicGameplayMovementMove,
		Timestamp: time.Now(),
		EntityID:  entityID,
		FromX:     fromX,
		FromY:     fromY,
		ToX:       toX,
		ToY:       toY,
	}
}

type ChunkLoadEvent struct {
	topic     string
	Timestamp time.Time
	ChunkX    int
	ChunkY    int
	Layer     int32
}

func (e *ChunkLoadEvent) Topic() string { return e.topic }

func NewChunkLoadEvent(chunkX, chunkY int, layer int32) *ChunkLoadEvent {
	return &ChunkLoadEvent{
		topic:     TopicGameplayChunkLoad,
		Timestamp: time.Now(),
		ChunkX:    chunkX,
		ChunkY:    chunkY,
		Layer:     layer,
	}
}

type ChunkUnloadEvent struct {
	topic     string
	Timestamp time.Time
	ChunkX    int
	ChunkY    int
	Layer     int32
}

func (e *ChunkUnloadEvent) Topic() string { return e.topic }

func NewChunkUnloadEvent(chunkX, chunkY int, layer int32) *ChunkUnloadEvent {
	return &ChunkUnloadEvent{
		topic:     TopicGameplayChunkUnload,
		Timestamp: time.Now(),
		ChunkX:    chunkX,
		ChunkY:    chunkY,
		Layer:     layer,
	}
}

type TickEvent struct {
	topic      string
	Timestamp  time.Time
	TickNumber uint64
	DeltaTime  float64
}

func (e *TickEvent) Topic() string { return e.topic }

func NewTickEvent(tickNumber uint64, deltaTime float64) *TickEvent {
	return &TickEvent{
		topic:      TopicSystemTick,
		Timestamp:  time.Now(),
		TickNumber: tickNumber,
		DeltaTime:  deltaTime,
	}
}

type NetworkConnectEvent struct {
	topic     string
	Timestamp time.Time
	ClientID  uint64
	Address   string
}

func (e *NetworkConnectEvent) Topic() string { return e.topic }

func NewNetworkConnectEvent(clientID uint64, address string) *NetworkConnectEvent {
	return &NetworkConnectEvent{
		topic:     TopicNetworkConnect,
		Timestamp: time.Now(),
		ClientID:  clientID,
		Address:   address,
	}
}

type NetworkDisconnectEvent struct {
	topic     string
	Timestamp time.Time
	ClientID  uint64
	Reason    string
}

func (e *NetworkDisconnectEvent) Topic() string { return e.topic }

func NewNetworkDisconnectEvent(clientID uint64, reason string) *NetworkDisconnectEvent {
	return &NetworkDisconnectEvent{
		topic:     TopicNetworkDisconnect,
		Timestamp: time.Now(),
		ClientID:  clientID,
		Reason:    reason,
	}
}
