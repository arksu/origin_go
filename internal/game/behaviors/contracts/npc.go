package contracts

// NPCBehaviorConfig is immutable content shared by every instance of a definition.
// Profiles select decision logic; adding a species does not consume a component ID.
type NPCBehaviorConfig struct {
	Priority        int     `json:"priority,omitempty"`
	Profile         string  `json:"profile"`
	Speed           float64 `json:"speed"`
	IdleMinSeconds  float64 `json:"idleMinSeconds"`
	IdleMaxSeconds  float64 `json:"idleMaxSeconds"`
	WanderMinTiles  float64 `json:"wanderMinTiles"`
	WanderMaxTiles  float64 `json:"wanderMaxTiles"`
	HomeRadiusTiles float64 `json:"homeRadiusTiles"`
	MaxWalkSeconds  float64 `json:"maxWalkSeconds"`
}
