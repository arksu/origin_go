package components

import (
	"encoding/json"
	"fmt"
)

// WildWeedBehaviorState belongs to the ordinary persistent object envelope.
type WildWeedBehaviorState struct {
	SpotID                   int64 `json:"spot_id"`
	Stage                    int   `json:"stage"`
	NextGrowthRuntimeSeconds int64 `json:"next_growth_runtime_seconds,omitempty"`
}

func (s *WildWeedBehaviorState) UnmarshalJSON(data []byte) error {
	type payload WildWeedBehaviorState
	var decoded payload
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.SpotID < 0 || decoded.Stage < 0 || decoded.Stage > 3 || decoded.NextGrowthRuntimeSeconds < 0 {
		return fmt.Errorf("invalid wild weed state")
	}
	if decoded.Stage < 3 && decoded.NextGrowthRuntimeSeconds == 0 {
		return fmt.Errorf("growing wild weed requires a runtime deadline")
	}
	*s = WildWeedBehaviorState(decoded)
	return nil
}
