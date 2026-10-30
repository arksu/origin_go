package contracts

const (
	CropStageCount      = 4
	CropMaxHarvestItems = 32
)

// CropBehaviorConfig is immutable definition data shared by all crop instances.
type CropBehaviorConfig struct {
	Priority                  int               `json:"priority,omitempty"`
	SeedItemKey               string            `json:"seedItemKey"`
	PlantingSuccessChance     *float64          `json:"plantingSuccessChance,omitempty"`
	Stages                    []CropStageConfig `json:"stages"`
	HarvestCycleDurationTicks uint32            `json:"harvestCycleDurationTicks,omitempty"`
	HarvestStaminaCost        float64           `json:"harvestStaminaCost,omitempty"`
}

type CropStageConfig struct {
	DurationSeconds int64                `json:"durationSeconds"`
	Harvest         []CropHarvestProduct `json:"harvest,omitempty"`
}

type CropHarvestProduct struct {
	ItemKey string `json:"itemKey"`
	Min     int    `json:"min"`
	Max     int    `json:"max"`
}
