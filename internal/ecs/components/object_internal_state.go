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

type TakeBehaviorState struct {
	Taken map[string]int `json:"-"`
}

type BuildBehaviorState struct {
	BuildKey     string                   `json:"build_key,omitempty"`
	BuildDefID   int                      `json:"build_def_id,omitempty"`
	ObjectKey    string                   `json:"object_key,omitempty"`
	ObjectTypeID uint32                   `json:"object_type_id,omitempty"`
	TargetX      int                      `json:"target_x,omitempty"`
	TargetY      int                      `json:"target_y,omitempty"`
	Items        []BuildRequiredItemState `json:"items,omitempty"`
}

type BuildRequiredItemState struct {
	Slot int `json:"slot,omitempty"`

	ItemKey string `json:"item_key,omitempty"`
	ItemTag string `json:"item_tag,omitempty"`

	RequiredCount uint32 `json:"required_count,omitempty"`
	BuildCount    uint32 `json:"build_count,omitempty"`

	QualityWeight     uint32              `json:"quality_weight,omitempty"`
	BuildQualityTotal uint32              `json:"build_quality_total,omitempty"`
	PutItems          []BuildPutItemState `json:"put_items,omitempty"`
}

type BuildPutItemState struct {
	ItemKey string `json:"item_key,omitempty"`
	Quality uint32 `json:"quality,omitempty"`
	Count   uint32 `json:"count,omitempty"`
}

func (s *BuildRequiredItemState) PutCount() uint32 {
	if s == nil || len(s.PutItems) == 0 {
		return 0
	}
	var total uint32
	for _, item := range s.PutItems {
		total += item.Count
	}
	return total
}

func (s *BuildRequiredItemState) MergePutItem(itemKey string, quality uint32, count uint32) {
	if s == nil || count == 0 || itemKey == "" {
		return
	}
	for i := range s.PutItems {
		if s.PutItems[i].ItemKey == itemKey && s.PutItems[i].Quality == quality {
			s.PutItems[i].Count += count
			return
		}
	}
	s.PutItems = append(s.PutItems, BuildPutItemState{
		ItemKey: itemKey,
		Quality: quality,
		Count:   count,
	})
}

func (s *BuildBehaviorState) IsEmpty() bool {
	if s == nil {
		return true
	}
	for i := range s.Items {
		item := &s.Items[i]
		if item.BuildCount > 0 || item.PutCount() > 0 {
			return false
		}
	}
	return true
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
	appendTakenCounts(payload, s.Taken)
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
			taken, err := unmarshalTakenCountField(key, value)
			if err != nil {
				return fmt.Errorf("tree state field %q: %w", key, err)
			}
			if taken.ActionID == "" || taken.Count <= 0 {
				continue
			}
			if s.Taken == nil {
				s.Taken = make(map[string]int)
			}
			s.Taken[taken.ActionID] = taken.Count
		}
	}
	return nil
}

func (s TakeBehaviorState) MarshalJSON() ([]byte, error) {
	payload := make(map[string]any, len(s.Taken))
	appendTakenCounts(payload, s.Taken)
	return json.Marshal(payload)
}

func (s *TakeBehaviorState) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("take behavior state is nil")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*s = TakeBehaviorState{}
	for key, value := range raw {
		taken, err := unmarshalTakenCountField(key, value)
		if err != nil {
			return fmt.Errorf("take state field %q: %w", key, err)
		}
		if taken.ActionID == "" || taken.Count <= 0 {
			continue
		}
		if s.Taken == nil {
			s.Taken = make(map[string]int)
		}
		s.Taken[taken.ActionID] = taken.Count
	}
	return nil
}

type takenCountField struct {
	ActionID string
	Count    int
}

func appendTakenCounts(payload map[string]any, taken map[string]int) {
	for actionID, count := range taken {
		if actionID == "" || count <= 0 {
			continue
		}
		payload[actionID+"_taken"] = count
	}
}

func unmarshalTakenCountField(key string, value json.RawMessage) (takenCountField, error) {
	if !strings.HasSuffix(key, "_taken") {
		return takenCountField{}, nil
	}
	actionID := strings.TrimSuffix(key, "_taken")
	if actionID == "" {
		return takenCountField{}, nil
	}
	var taken int
	if err := json.Unmarshal(value, &taken); err != nil {
		return takenCountField{}, err
	}
	if taken <= 0 {
		return takenCountField{}, nil
	}
	return takenCountField{
		ActionID: actionID,
		Count:    taken,
	}, nil
}

const ObjectInternalStateComponentID ecs.ComponentID = 23

func init() {
	ecs.RegisterComponent[ObjectInternalState](ObjectInternalStateComponentID)
}
