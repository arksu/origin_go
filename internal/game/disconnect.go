package game

import (
	"context"
	"log"
	"origin/internal/ecs/systems"

	"origin/internal/db"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

// DisconnectPlayer handles player disconnection cleanup
// - Stops movement
// - Saves final position to database
// - Removes player from shard
// - Despawns entity from ECS world
func (s *Shard) DisconnectPlayer(ctx context.Context, queries *db.Queries, h ecs.Handle) error {
	w := s.world

	// Get player component to retrieve character ID
	player, ok := ecs.GetComponent[components.Player](w, h)
	if !ok {
		log.Printf("DisconnectPlayer: handle %d has no Player component", h)
		return nil
	}

	// Stop movement - clear velocity and movement target
	systems.StopMovement(w, h)

	// Get final position for database save
	pos, ok := ecs.GetComponent[components.Position](w, h)
	if ok {
		// Save position to database
		err := queries.UpdateCharacterPosition(ctx, db.UpdateCharacterPositionParams{
			ID: player.CharacterID,
			X:  int32(pos.X),
			Y:  int32(pos.Y),
		})
		if err != nil {
			log.Printf("DisconnectPlayer: failed to save position for character %d: %v", player.CharacterID, err)
			// Continue with cleanup even if save fails
		} else {
			log.Printf("DisconnectPlayer: saved position (%.2f, %.2f) for character %d", pos.X, pos.Y, player.CharacterID)
		}
	}

	// Unregister client from network flush system
	s.NetworkFlush().UnregisterClient(h)

	// Remove player from shard tracking
	s.RemovePlayer(h)

	// Despawn entity from ECS world (removes all components)
	w.Despawn(h)

	log.Printf("DisconnectPlayer: cleaned up player %s (CharID=%d, Handle=%d)", player.Name, player.CharacterID, h)
	return nil
}
