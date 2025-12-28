package game

import (
	"context"
	"log"
	"math/rand"

	"origin/internal/config"
	"origin/internal/db"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

// SpawnMethod represents the method used to spawn an entity
type SpawnMethod int

const (
	SpawnMethodOriginal SpawnMethod = iota // Spawned at original position
	SpawnMethodNearby                      // Spawned at nearby position
	SpawnMethodRandom                      // Spawned at random position
	SpawnMethodFailed                      // Failed to spawn
)

// SpawnResult contains the result of a spawn attempt
type SpawnResult struct {
	Handle   ecs.Handle   // Runtime handle for ECS operations
	EntityID ecs.EntityID // Global unique ID for persistence/replication
	X        int
	Y        int
	Success  bool
	Method   SpawnMethod
}

// SpawnPlayer spawns a player entity into the world with fallback logic:
// 1. Try original position from database
// 2. If collision, try nearby positions within 2 tiles
// 3. If still failed, try random position in the layer
func (s *Shard) SpawnPlayer(ctx context.Context, cfg *config.Config, queries *db.Queries, region int, characterID int64, name string, x, y int) SpawnResult {
	// Try original position
	if s.canSpawnAt(ctx, queries, region, x, y) {
		return s.doSpawnPlayer(characterID, name, x, y, SpawnMethodOriginal)
	}
	log.Printf("SpawnPlayer: original position (%d, %d) blocked, trying nearby", x, y)

	// Try nearby positions within 1 tiles (1 * COORD_PER_TILE)
	nearbyOffset := COORD_PER_TILE
	offsets := []struct{ dx, dy int }{
		{nearbyOffset, 0},
		{-nearbyOffset, 0},
		{0, nearbyOffset},
		{0, -nearbyOffset},
		{nearbyOffset, nearbyOffset},
		{nearbyOffset, -nearbyOffset},
		{-nearbyOffset, nearbyOffset},
		{-nearbyOffset, -nearbyOffset},
	}

	for _, off := range offsets {
		newX, newY := x+off.dx, y+off.dy
		if s.canSpawnAt(ctx, queries, region, newX, newY) {
			return s.doSpawnPlayer(characterID, name, newX, newY, SpawnMethodNearby)
		}
	}
	log.Printf("SpawnPlayer: nearby positions blocked, trying random")

	// Try random positions in the layer
	maxWorldX := cfg.RegionWidthChunks * CHUNK_SIZE * COORD_PER_TILE
	maxWorldY := cfg.RegionHeightChunks * CHUNK_SIZE * COORD_PER_TILE

	// Try up to 100 random positions
	for i := 0; i < 100; i++ {
		randX := rand.Intn(maxWorldX)
		randY := rand.Intn(maxWorldY)
		if s.canSpawnAt(ctx, queries, region, randX, randY) {
			return s.doSpawnPlayer(characterID, name, randX, randY, SpawnMethodRandom)
		}
	}

	log.Printf("SpawnPlayer: failed to find valid spawn position after 100 random attempts")
	return SpawnResult{Success: false, Method: SpawnMethodFailed}
}

// canSpawnAt checks if a position is valid for spawning (walkable tile, no entity collision)
func (s *Shard) canSpawnAt(ctx context.Context, queries *db.Queries, region, x, y int) bool {
	// Bounds check
	if x < 0 || y < 0 {
		return false
	}

	// Calculate chunk coordinates
	chunkX := x / (CHUNK_SIZE * COORD_PER_TILE)
	chunkY := y / (CHUNK_SIZE * COORD_PER_TILE)

	// Load chunk and check tile walkability
	chunk, err := s.chunkManager.GetOrLoadChunk(ctx, queries, region, chunkX, chunkY, s.layer)
	if err != nil {
		log.Printf("canSpawnAt: failed to load chunk (%d, %d): %v", chunkX, chunkY, err)
		return false
	}

	// Check if tile is walkable
	if !chunk.IsPositionWalkable(x, y) {
		return false
	}

	// TODO: Check for entity collisions at this position
	// For now, we only check tile walkability

	return true
}

// doSpawnPlayer creates the player entity with all necessary components
func (s *Shard) doSpawnPlayer(characterID int64, name string, x, y int, method SpawnMethod) SpawnResult {
	// Use character ID from database as global entity ID
	entityID := ecs.EntityID(characterID)

	// Register entity in ECS world - returns runtime handle
	h := s.world.Spawn(entityID)
	if h == ecs.InvalidHandle {
		log.Printf("SpawnPlayer: failed to allocate handle for entity %d", entityID)
		return SpawnResult{Success: false, Method: SpawnMethodFailed}
	}

	// Add Player component
	ecs.AddComponent(s.world, h, components.Player{
		CharacterID: characterID,
		Name:        name,
	})

	// Add Position component
	ecs.AddComponent(s.world, h, components.Position{
		X: float64(x),
		Y: float64(y),
	})
	ecs.AddComponent(s.world, h, components.Velocity{
		X: 0,
		Y: 0,
	})

	// Add Speed component (default player speed)
	ecs.AddComponent(s.world, h, components.Speed{
		Value: 100.0, // TODO: make speed configurable
	})

	// Add Collider component for player
	ecs.AddComponent(s.world, h, components.Collider{
		Width:  5,
		Height: 5,
		Layer:  1,    // Player collision layer
		Mask:   0xFF, // Collide with everything
	})

	// Track player in shard by handle
	s.AddPlayer(h)

	log.Printf("SpawnPlayer: spawned player %s (CharID=%d, Handle=%d, EntityID=%d) at (%d, %d) via %v",
		name, characterID, h, entityID, x, y, method)

	return SpawnResult{
		Handle:   h,
		EntityID: entityID,
		X:        x,
		Y:        y,
		Success:  true,
		Method:   method,
	}
}
