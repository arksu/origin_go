package components

import (
	"encoding/json"
	"fmt"

	"origin/internal/ecs"
)

type ActionCooldown struct {
	StartedAtMs int64 `json:"startedAtMs"`
	ExpiresAtMs int64 `json:"expiresAtMs"`
}

// ActionCooldowns outlives active actions and is saved with the character.
type ActionCooldowns struct {
	ByAction map[string]ActionCooldown
}

const ActionCooldownsComponentID ecs.ComponentID = 37

func init() {
	ecs.RegisterComponent[ActionCooldowns](ActionCooldownsComponentID)
}

func UnmarshalActionCooldowns(raw []byte) (ActionCooldowns, error) {
	cooldowns := ActionCooldowns{ByAction: make(map[string]ActionCooldown)}
	if len(raw) == 0 {
		return cooldowns, nil
	}
	if err := json.Unmarshal(raw, &cooldowns.ByAction); err != nil {
		return ActionCooldowns{}, fmt.Errorf("parse action cooldowns: %w", err)
	}
	if cooldowns.ByAction == nil {
		return ActionCooldowns{}, fmt.Errorf("action cooldowns must be an object")
	}
	for id, cooldown := range cooldowns.ByAction {
		if id == "" || cooldown.StartedAtMs < 0 || cooldown.ExpiresAtMs <= cooldown.StartedAtMs {
			return ActionCooldowns{}, fmt.Errorf("invalid cooldown for action %q", id)
		}
	}
	return cooldowns, nil
}

// MarshalActive makes an owned snapshot before the asynchronous save worker runs.
func (cooldowns ActionCooldowns) MarshalActive(nowMs int64) (string, error) {
	active := make(map[string]ActionCooldown)
	for id, cooldown := range cooldowns.ByAction {
		if cooldown.ExpiresAtMs > nowMs {
			active[id] = cooldown
		}
	}
	encoded, err := json.Marshal(active)
	return string(encoded), err
}
