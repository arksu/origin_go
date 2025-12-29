package proto

import (
	"encoding/binary"
	"errors"

	"google.golang.org/protobuf/proto"
)

// PacketType represents the type of packet (uint16 header)
type PacketType uint16

const (
	// Client -> Server
	PacketAuth        PacketType = 0x0001
	PacketMapClick    PacketType = 0x0002
	PacketObjectClick PacketType = 0x0003
	PacketChat        PacketType = 0x0004

	// Server -> Client
	PacketAuthResult PacketType = 0x0010
	PacketObjectAdd  PacketType = 0x0011
	PacketObjectDel  PacketType = 0x0012
	PacketObjectMove PacketType = 0x0013
	PacketInventory  PacketType = 0x0014
	PacketChunk      PacketType = 0x0015
	PacketChatMsg    PacketType = 0x0016
	PacketError      PacketType = 0x0017
	PacketWorldState PacketType = 0x0018
)

var (
	ErrInvalidPacket  = errors.New("invalid packet")
	ErrPacketTooSmall = errors.New("packet too small")
)

// Packet represents a network packet with type header and protobuf payload
type Packet struct {
	Type    PacketType
	Payload []byte
}

// EncodePacket encodes a protobuf message into a packet with type header
// Format: [2 bytes type][protobuf payload]
func EncodePacket(packetType PacketType, msg proto.Message) ([]byte, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	data := make([]byte, 2+len(payload))
	binary.LittleEndian.PutUint16(data[:2], uint16(packetType))
	copy(data[2:], payload)

	return data, nil
}

// DecodePacket decodes raw bytes into packet type and payload
func DecodePacket(data []byte) (PacketType, []byte, error) {
	if len(data) < 2 {
		return 0, nil, ErrPacketTooSmall
	}

	packetType := PacketType(binary.LittleEndian.Uint16(data[:2]))
	payload := data[2:]

	return packetType, payload, nil
}

// DecodePayload decodes protobuf payload into a message
func DecodePayload(payload []byte, msg proto.Message) error {
	return proto.Unmarshal(payload, msg)
}
