package components

import (
	"encoding/json"
	"fmt"
	"strings"

	"origin/internal/ecs"
)

// ObjectInternalState tracks runtime state and dirty flag for world objects.
// Added to all non-player entities on chunk activation; used to skip
// unchanged objects during persistence.
type ObjectInternalState struct {
	State   any
	Flags   []string
	IsDirty bool
}

type ObjectStateEnvelope struct {
	Version   int                        `json:"v"`
	Behaviors map[string]json.RawMessage `json:"behaviors,omitempty"`
}

type RuntimeObjectState struct {
	Behaviors map[string]any
}

type TreeBehaviorState struct {
	ChopPoints     int            `json:"chop_points,omitempty"`
	Stage          int            `json:"stage,omitempty"`
	NextGrowthTick uint64         `json:"next_growth_tick,omitempty"`
	Taken          map[string]int `json:"-"`
}

func (s TreeBehaviorState) MarshalJSON() ([]byte, error) {
	payload := make(map[string]any, 3+len(s.Taken))
	if s.ChopPoints > 0 {
		payload["chop_points"] = s.ChopPoints
	}
	if s.Stage > 0 {
		payload["stage"] = s.Stage
	}
	if s.NextGrowthTick > 0 {
		payload["next_growth_tick"] = s.NextGrowthTick
	}
	for actionID, count := range s.Taken {
		if actionID == "" || count <= 0 {
			continue
		}
		payload[actionID+"_taken"] = count
	}
	return json.Marshal(payload)
}

func (s *TreeBehaviorState) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("tree behavior state is nil")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*s = TreeBehaviorState{}
	for key, value := range raw {
		switch key {
		case "chop_points":
			if err := json.Unmarshal(value, &s.ChopPoints); err != nil {
				return fmt.Errorf("tree state field %q: %w", key, err)
			}
		case "stage":
			if err := json.Unmarshal(value, &s.Stage); err != nil {
				return fmt.Errorf("tree state field %q: %w", key, err)
			}
		case "next_growth_tick":
			if err := json.Unmarshal(value, &s.NextGrowthTick); err != nil {
				return fmt.Errorf("tree state field %q: %w", key, err)
			}
		default:
			if !strings.HasSuffix(key, "_taken") {
				continue
			}
			actionID := strings.TrimSuffix(key, "_taken")
			if actionID == "" {
				continue
			}
			var taken int
			if err := json.Unmarshal(value, &taken); err != nil {
				return fmt.Errorf("tree state field %q: %w", key, err)
			}
			if taken <= 0 {
				continue
			}
			if s.Taken == nil {
				s.Taken = make(map[string]int)
			}
			s.Taken[actionID] = taken
		}
	}
	return nil
}

const ObjectInternalStateComponentID ecs.ComponentID = 23

func init() {
	ecs.RegisterComponent[ObjectInternalState](ObjectInternalStateComponentID)
}
