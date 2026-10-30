package components

import (
	"encoding/json"
	"fmt"
)

// CropBehaviorState uses the existing persistent object behavior envelope.
type CropBehaviorState struct {
	Stage                    int   `json:"stage"`
	NextGrowthRuntimeSeconds int64 `json:"next_growth_runtime_seconds,omitempty"`
}

func (s CropBehaviorState) Validate() error {
	if s.Stage < 0 || s.Stage > 3 || s.NextGrowthRuntimeSeconds < 0 {
		return fmt.Errorf("invalid crop state")
	}
	if (s.Stage < 3) != (s.NextGrowthRuntimeSeconds > 0) {
		return fmt.Errorf("crop growth deadline does not match stage")
	}
	return nil
}

func (s *CropBehaviorState) UnmarshalJSON(data []byte) error {
	type payload CropBehaviorState
	var decoded payload
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	state := CropBehaviorState(decoded)
	if err := state.Validate(); err != nil {
		return err
	}
	*s = state
	return nil
}
