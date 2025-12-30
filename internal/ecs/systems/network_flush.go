package systems

import (
	"log"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/proto"

	protog "google.golang.org/protobuf/proto"
)

// NetworkClient is an interface for sending packets to clients
// This avoids circular dependency with the network package
type NetworkClient interface {
	SendPacket(packetType proto.PacketType, msg protog.Message) error
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
		objectAdd    []*proto.S2CObjectAdd
		objectDelete []*proto.S2CObjectDelete
		objectMove   []*proto.S2CObjectMove
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

			msg := &proto.S2CObjectAdd{
				EntityId: uint64(meta.EntityID),
				TypeId:   int32(meta.EntityType),
				X:        int32(pos.X),
				Y:        int32(pos.Y),
				Heading:  0,
				Resource: "",
			}
			clientEvents[client].objectAdd = append(clientEvents[client].objectAdd, msg)
		} else {
			// Object left visibility - send S2CObjectDelete
			meta, ok := metaStorage.Get(event.Target)
			if !ok {
				continue
			}

			msg := &proto.S2CObjectDelete{
				EntityId: uint64(meta.EntityID),
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

		msg := &proto.S2CObjectMove{
			EntityId: uint64(event.EntityID),
			X:        event.X,
			Y:        event.Y,
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
