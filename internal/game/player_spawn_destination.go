package game

import (
	"context"
	"errors"
	"fmt"

	"origin/internal/characterattrs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

var ErrSpawnDestinationUnavailable = errors.New("spawn destination unavailable")

// Identity includes both object identity and position: replacing a destination
// at the same coordinates still requires a fresh owning-world validation.
type playerSpawnDestination struct {
	X, Y, Region int
	ObjectID     types.EntityID
}

// A finalizer runs once after successful spawning under the owning shard lock.
// It cannot fail: the body already exists and must be attached or queued for logout.
type playerSpawnFinalizer func(types.Handle, repository.Character)

// The resolver and validator run under shard.mu. Preparation may release it,
// but the final resolution, validation, exact spawn and finalizer share one critical section.
func (g *Game) spawnPlayerAtResolvedDestination(
	ctx context.Context,
	shard *Shard,
	character repository.Character,
	runtime *playerRuntimeState,
	resolve func(*Shard) (playerSpawnDestination, error),
	validate func(*Shard, playerSpawnDestination) error,
	policy SpawnCollisionPolicy,
	finalize playerSpawnFinalizer,
) (types.Handle, repository.Character, error) {
	playerID := types.EntityID(character.ID)
	normalizedAttributes, _ := characterattrs.FromRaw(character.Attributes)
	profileExperience, profileSkills, profileDiscovery := loadCharacterProfileData(character, g.logger)
	var prepared playerSpawnDestination
	hasPrepared := false
	for {
		shard.mu.Lock()
		if err := ctx.Err(); err != nil {
			shard.UnregisterEntityAOI(playerID)
			shard.mu.Unlock()
			return types.InvalidHandle, character, err
		}
		destination, err := resolve(shard)
		if err != nil {
			shard.UnregisterEntityAOI(playerID)
			shard.mu.Unlock()
			return types.InvalidHandle, character, fmt.Errorf("%w: %w", ErrSpawnDestinationUnavailable, err)
		}
		if !hasPrepared || destination != prepared {
			// RegisterEntity intentionally ignores an existing registration. Retire
			// the unpublished reservation before preparing a changed destination.
			shard.UnregisterEntityAOI(playerID)
			shard.mu.Unlock()
			if err := shard.PrepareEntityAOI(ctx, playerID, destination.X, destination.Y); err != nil {
				shard.mu.Lock()
				shard.UnregisterEntityAOI(playerID)
				shard.mu.Unlock()
				return types.InvalidHandle, character, err
			}
			prepared, hasPrepared = destination, true
			continue
		}
		if validate != nil {
			if err := validate(shard, destination); err != nil {
				shard.UnregisterEntityAOI(playerID)
				shard.mu.Unlock()
				return types.InvalidHandle, character, fmt.Errorf("%w: %w", ErrSpawnDestinationUnavailable, err)
			}
		}
		defer shard.unlockAndCompletePermanentDeaths()
		character.Layer, character.X, character.Y = shard.layer, destination.X, destination.Y
		setup := g.buildPlayerSetupWithRuntimeFunc(ctx, character, spawnPos{X: destination.X, Y: destination.Y}, normalizedAttributes, profileExperience, profileSkills, profileDiscovery, runtime)
		handle, err := shard.trySpawnPlayerWithPolicyLocked(destination.X, destination.Y, character, setup, policy)
		if handle == types.InvalidHandle {
			shard.UnregisterEntityAOI(playerID)
		} else if finalize != nil {
			finalize(handle, character)
		}
		return handle, character, err
	}
}
