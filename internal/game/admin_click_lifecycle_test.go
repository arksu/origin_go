package game

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/types"
)

func TestDeathClearsAllPendingAdminClicksForOnlyDeadPlayer(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, nil)
	for _, playerID := range []types.EntityID{1, 2} {
		ecs.GetResource[ecs.PendingAdminSpawn](w).Set(playerID, ecs.AdminSpawnEntry{})
		ecs.GetResource[ecs.PendingAdminTeleport](w).Set(playerID)
		ecs.GetResource[ecs.PendingAdminObjectInfo](w).Set(playerID)
		ecs.GetResource[ecs.PendingAdminDestroy](w).Set(playerID)
	}

	(&Shard{}).clearPlayerTransientStateForDeath(w, 1, player)

	for _, playerID := range []types.EntityID{1, 2} {
		_, spawn := ecs.GetResource[ecs.PendingAdminSpawn](w).Get(playerID)
		pending := map[string]bool{
			"spawn":   spawn,
			"tp":      ecs.GetResource[ecs.PendingAdminTeleport](w).Get(playerID),
			"info":    ecs.GetResource[ecs.PendingAdminObjectInfo](w).Get(playerID),
			"destroy": ecs.GetResource[ecs.PendingAdminDestroy](w).Get(playerID),
		}
		for command, armed := range pending {
			if armed != (playerID == 2) {
				t.Errorf("player %d command %s: pending=%v", playerID, command, armed)
			}
		}
	}
}
