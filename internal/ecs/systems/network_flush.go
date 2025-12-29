package systems

import (
	"log"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/proto"
)

// NetworkClient is an interface for sending packets to clients
// This avoids circular dependency with the network package
type NetworkClient interface {
	SendPacket(packetType proto.PacketType, msg interface{}) error
}

// NetworkClientAdapter wraps a real client to match the interface
type NetworkClientAdapter struct {
	sendFunc func(proto.PacketType, interface{}) error
}

func (a *NetworkClientAdapter) SendPacket(packetType proto.PacketType, msg interface{}) error {
	return a.sendFunc(packetType, msg)
}

// NetworkFlushSystem sends accumulated network events to clients
// Runs at priority 500 (last, after all game logic)
type NetworkFlushSystem struct {
	ecs.BaseSystem
	visibilitySystem *VisibilitySystem
	broadcastSystem  *MovementBroadcastSystem
	clientRouter     map[ecs.Handle]NetworkClient
}

// NewNetworkFlushSystem creates a new network flush system
func NewNetworkFlushSystem(visSystem *VisibilitySystem, broadcastSystem *MovementBroadcastSystem) *NetworkFlushSystem {
	return &NetworkFlushSystem{
		BaseSystem:       ecs.NewBaseSystem("NetworkFlushSystem", 500),
		visibilitySystem: visSystem,
		broadcastSystem:  broadcastSystem,
		clientRouter:     make(map[ecs.Handle]NetworkClient),
	}
}

// RegisterClient registers a client for a player handle
func (s *NetworkFlushSystem) RegisterClient(h ecs.Handle, client NetworkClient) {
	s.clientRouter[h] = client
}

// UnregisterClient removes a client from the router
func (s *NetworkFlushSystem) UnregisterClient(h ecs.Handle) {
	delete(s.clientRouter, h)
}

// Update sends all accumulated events to clients
func (s *NetworkFlushSystem) Update(w *ecs.World, dt float64) {
	// Get component storages
	metaStorage := ecs.GetOrCreateStorage[components.EntityMeta](w)
	posStorage := ecs.GetOrCreateStorage[components.Position](w)

	// Group events by client
	type ClientEvents struct {
		objectAdd    []interface{}
		objectDelete []interface{}
		objectMove   []interface{}
	}
	clientEvents := make(map[NetworkClient]*ClientEvents)

	// Process visibility events (enter/leave)
	for _, event := range s.visibilitySystem.Events() {
		client, ok := s.clientRouter[event.Observer]
		if !ok {
			continue
		}

		if clientEvents[client] == nil {
			clientEvents[client] = &ClientEvents{}
		}

		if event.Enter {
			// Object entered visibility - send S2CObjectAdd
			meta, ok := metaStorage.Get(event.Target)
			if !ok {
				continue
			}

			pos, ok := posStorage.Get(event.Target)
			if !ok {
				continue
			}

			// Create object add message (will be marshaled by network layer)
			msg := map[string]interface{}{
				"entity_id": uint64(meta.EntityID),
				"type_id":   int32(meta.EntityType),
				"x":         int32(pos.X),
				"y":         int32(pos.Y),
				"heading":   int32(0),
				"resource":  "",
			}
			clientEvents[client].objectAdd = append(clientEvents[client].objectAdd, msg)
		} else {
			// Object left visibility - send S2CObjectDelete
			meta, ok := metaStorage.Get(event.Target)
			if !ok {
				continue
			}

			msg := map[string]interface{}{
				"entity_id": uint64(meta.EntityID),
			}
			clientEvents[client].objectDelete = append(clientEvents[client].objectDelete, msg)
		}
	}

	// Process movement events
	for _, event := range s.broadcastSystem.Events() {
		client, ok := s.clientRouter[event.Observer]
		if !ok {
			continue
		}

		if clientEvents[client] == nil {
			clientEvents[client] = &ClientEvents{}
		}

		msg := map[string]interface{}{
			"entity_id": uint64(event.EntityID),
			"x":         event.X,
			"y":         event.Y,
		}
		clientEvents[client].objectMove = append(clientEvents[client].objectMove, msg)
	}

	// Send packets to each client in priority order
	for client, events := range clientEvents {
		// 1. Send deletes first (remove objects that left visibility)
		for _, msg := range events.objectDelete {
			if err := client.SendPacket(proto.PacketObjectDel, msg); err != nil {
				log.Printf("Failed to send ObjectDelete: %v", err)
			}
		}

		// 2. Send adds (new objects entering visibility)
		for _, msg := range events.objectAdd {
			if err := client.SendPacket(proto.PacketObjectAdd, msg); err != nil {
				log.Printf("Failed to send ObjectAdd: %v", err)
			}
		}

		// 3. Send moves (position updates for visible objects)
		for _, msg := range events.objectMove {
			if err := client.SendPacket(proto.PacketObjectMove, msg); err != nil {
				log.Printf("Failed to send ObjectMove: %v", err)
			}
		}
	}
}
