package components

import (
	"encoding/json"
	"fmt"
	"origin/internal/types"
)

const MaxDryingItems = 4

// DryingBehaviorState lives in the existing world-object state envelope.
type DryingBehaviorState struct {
	Entries []DryingItemState `json:"entries,omitempty"`
}

type DryingItemState struct {
	InputItemID              types.EntityID `json:"input_item_id"`
	InputTypeID              uint32         `json:"input_type_id"`
	CompletionRuntimeSeconds int64          `json:"completion_runtime_seconds"`
}

func (s *DryingBehaviorState) UnmarshalJSON(data []byte) error {
	type payload DryingBehaviorState
	var decoded payload
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if len(decoded.Entries) > MaxDryingItems {
		return fmt.Errorf("drying state exceeds %d entries", MaxDryingItems)
	}
	for i, entry := range decoded.Entries {
		if entry.InputItemID == 0 || entry.InputTypeID == 0 || entry.CompletionRuntimeSeconds <= 0 {
			return fmt.Errorf("drying state entry %d is invalid", i)
		}
		for _, prior := range decoded.Entries[:i] {
			if prior.InputItemID == entry.InputItemID {
				return fmt.Errorf("drying state repeats input item %d", entry.InputItemID)
			}
		}
	}
	*s = DryingBehaviorState(decoded)
	return nil
}
