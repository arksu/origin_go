package game

import (
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func (s *Shard) SendCustomWindow(playerID types.EntityID, window *netproto.S2C_CustomWindow) {
	if window == nil {
		return
	}
	// Serialization is immediate; the sender owns its epoch, not the caller's snapshot.
	state := &netproto.S2C_CustomWindow{EntityId: window.EntityId, Title: window.Title, Fields: window.Fields}
	s.sendWorldTransition(playerID, &netproto.ServerMessage{Payload: &netproto.ServerMessage_CustomWindow{CustomWindow: state}})
}

func (s *Shard) SendCustomWindowClosed(playerID, targetID types.EntityID) {
	s.sendWorldTransition(playerID, &netproto.ServerMessage{Payload: &netproto.ServerMessage_CustomWindowClosed{
		CustomWindowClosed: &netproto.S2C_CustomWindowClosed{EntityId: uint64(targetID)},
	}})
}
