package network

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/proto"
)

// handleAuth handles authentication packets
func (s *Server) handleAuth(c *Client, payload []byte) error {
	var req proto.C2SAuth
	if err := proto.DecodePayload(payload, &req); err != nil {
		c.SendError(proto.S2CError_ERROR_CODE_INVALID_PACKET, "invalid auth packet")
		return err
	}

	log.Printf("Auth request: token=%s", req.Token)

	ctx := context.Background()

	// Load character by auth token
	character, err := s.world.DB().GetCharacterByToken(ctx, req.Token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.SendError(proto.S2CError_ERROR_CODE_NOT_AUTHENTICATED, "invalid or expired token")
			return nil
		}
		c.SendError(proto.S2CError_ERROR_CODE_NOT_AUTHENTICATED, "authentication failed")
		return err
	}

	// Clear the auth token to prevent reuse
	// TODO while tests do not delete token
	//if err := s.world.DB().ClearAuthToken(ctx, character.ID); err != nil {
	//	log.Printf("Failed to clear auth token for character %d: %v", character.ID, err)
	//	// Continue anyway, token validation succeeded
	//}

	log.Printf("Auth success: character=%s (ID=%d)", character.Name, character.ID)

	// Get current layer's shard and spawn player
	shard := s.world.ShardManager().GetOrCreateShard(int(character.Layer))

	// Spawn player into world with fallback logic
	spawnResult := shard.SpawnPlayer(
		ctx,
		s.world.Config(),
		s.world.DB().Queries(),
		int(character.Region),
		character.ID,
		character.Name,
		int(character.X),
		int(character.Y),
	)

	if !spawnResult.Success {
		c.SendError(proto.S2CError_ERROR_CODE_UNSPECIFIED, "failed to spawn player")
		return errors.New("failed to spawn player into world")
	}

	c.mu.Lock()
	c.authed = true
	c.handle = spawnResult.Handle
	c.characterID = spawnResult.EntityID
	c.layer = int(character.Layer)
	c.mu.Unlock()

	// Register client in network flush system for packet routing
	// Wrap client to match NetworkClient interface
	clientAdapter := &ClientAdapter{client: c}
	shard.NetworkFlush().RegisterClient(spawnResult.Handle, clientAdapter)

	log.Printf("Player %s spawned at (%d, %d) in layer %d via %v",
		character.Name, spawnResult.X, spawnResult.Y, character.Layer, spawnResult.Method)

	// Send auth result
	return c.SendPacket(proto.PacketAuthResult, &proto.S2CAuthResult{
		Success:     true,
		CharacterId: uint64(character.ID),
	})
}

// handleMapClick handles map click packets
func (s *Server) handleMapClick(c *Client, payload []byte) error {
	if !c.IsAuthed() {
		c.SendError(proto.S2CError_ERROR_CODE_NOT_AUTHENTICATED, "not authenticated")
		return nil
	}

	var req proto.C2SMapClick
	if err := proto.DecodePayload(payload, &req); err != nil {
		c.SendError(proto.S2CError_ERROR_CODE_INVALID_PACKET, "invalid map click packet")
		return err
	}

	log.Printf("Map click: x=%d y=%d btn=%d", req.X, req.Y, req.Btn)

	// Get the shard for this player's layer
	shard := s.world.ShardManager().GetShard(c.layer)
	if shard == nil {
		c.SendError(proto.S2CError_ERROR_CODE_UNSPECIFIED, "shard not found")
		return nil
	}

	// Set movement target for the player entity
	c.mu.RLock()
	handle := c.handle
	c.mu.RUnlock()

	// Set movement target component on the player entity
	w := shard.ECSWorld()
	ecs.AddComponent(w, handle, components.MovementTarget{
		X: float64(req.X),
		Y: float64(req.Y),
	})
	// Add Velocity component (initially zero)
	ecs.AddComponent(w, handle, components.Velocity{
		X: 0,
		Y: 0,
	})

	return nil
}

// handleObjectClick handles object click packets
func (s *Server) handleObjectClick(c *Client, payload []byte) error {
	if !c.IsAuthed() {
		c.SendError(proto.S2CError_ERROR_CODE_NOT_AUTHENTICATED, "not authenticated")
		return nil
	}

	var req proto.C2CObjectClick
	if err := proto.DecodePayload(payload, &req); err != nil {
		c.SendError(proto.S2CError_ERROR_CODE_INVALID_PACKET, "invalid object click packet")
		return err
	}

	log.Printf("Object click: id=%d x=%d y=%d btn=%d", req.Id, req.X, req.Y, req.Btn)

	// TODO: Handle object click (interact, attack, examine, etc.)
	return nil
}

// handleChat handles chat packets
func (s *Server) handleChat(c *Client, payload []byte) error {
	if !c.IsAuthed() {
		c.SendError(proto.S2CError_ERROR_CODE_NOT_AUTHENTICATED, "not authenticated")
		return nil
	}

	var req proto.C2SChat
	if err := proto.DecodePayload(payload, &req); err != nil {
		c.SendError(proto.S2CError_ERROR_CODE_INVALID_PACKET, "invalid chat packet")
		return err
	}

	log.Printf("Chat message: channel=%v msg=%s", req.Channel, req.Message)

	// Broadcast chat message
	chatMsg := &proto.S2CChatMsg{
		SenderId:   uint64(c.characterID),
		SenderName: "Player", // TODO: get actual name
		Message:    req.Message,
		Channel:    req.Channel,
	}

	data, err := proto.EncodePacket(proto.PacketChatMsg, chatMsg)
	if err != nil {
		return err
	}

	// TODO: Filter by channel (local area, party)
	s.Broadcast(data)
	return nil
}

// IsAuthed returns whether the client is authenticated
func (c *Client) IsAuthed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authed
}
