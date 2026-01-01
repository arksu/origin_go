```go
// Состояние, которое нужно синхронизировать
NetworkState {
EntityID: EntityID
Position: Vec3
Rotation: float32 (optional)
Health: HealthSnapshot (optional)
Animation: AnimationState (optional)
Velocity: Vec2 (optional)

    ComponentFlags: uint64  // Битовая маска присутствующих компонентов
    Timestamp: uint64       // Server tick
}

// Snapshot для клиента
ClientSnapshot {
Tick: uint64
Entities: []NetworkState
RemovedEntities: []EntityID
Events: []GameEvent  // Combat hits, item pickups, etc
}

// Кэш предыдущего состояния для delta compression
PreviousStateCache {
PerClient: map[ClientID]map[EntityID]NetworkState
}

```