package contracts

const WildWeedGrowthSeconds int64 = 8 * 60 * 60

type WildWeedBehaviorConfig struct {
	Priority int `json:"priority,omitempty"`
}
