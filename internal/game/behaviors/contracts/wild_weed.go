package contracts

type WildWeedBehaviorConfig struct {
	Priority     int      `json:"priority,omitempty"`
	SeedOutcomes []string `json:"seedOutcomes"`
}
