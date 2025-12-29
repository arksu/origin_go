package network

import (
	"origin/internal/proto"
)

// ClientAdapter adapts Client to work with systems.NetworkClient interface
type ClientAdapter struct {
	client *Client
}

// SendPacket implements systems.NetworkClient interface
func (a *ClientAdapter) SendPacket(packetType proto.PacketType, msg interface{}) error {
	// Convert interface{} to protobuf message or handle map[string]interface{}
	switch v := msg.(type) {
	case map[string]interface{}:
		// Convert map to protobuf message based on packet type
		return a.sendMapAsPacket(packetType, v)
	default:
		return nil
	}
}

// sendMapAsPacket converts a map to the appropriate protobuf message
func (a *ClientAdapter) sendMapAsPacket(packetType proto.PacketType, data map[string]interface{}) error {
	switch packetType {
	case proto.PacketObjectAdd:
		msg := &proto.S2CObjectAdd{
			EntityId: getUint64(data, "entity_id"),
			TypeId:   getInt32(data, "type_id"),
			X:        getInt32(data, "x"),
			Y:        getInt32(data, "y"),
			Heading:  getInt32(data, "heading"),
			Resource: getString(data, "resource"),
		}
		return a.client.SendPacket(packetType, msg)
	case proto.PacketObjectDel:
		msg := &proto.S2CObjectDelete{
			EntityId: getUint64(data, "entity_id"),
		}
		return a.client.SendPacket(packetType, msg)
	case proto.PacketObjectMove:
		msg := &proto.S2CObjectMove{
			EntityId: getUint64(data, "entity_id"),
			X:        getInt32(data, "x"),
			Y:        getInt32(data, "y"),
		}
		return a.client.SendPacket(packetType, msg)
	default:
		return nil
	}
}

// Helper functions to extract typed values from map
func getUint64(m map[string]interface{}, key string) uint64 {
	if v, ok := m[key].(uint64); ok {
		return v
	}
	return 0
}

func getInt32(m map[string]interface{}, key string) int32 {
	if v, ok := m[key].(int32); ok {
		return v
	}
	return 0
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
