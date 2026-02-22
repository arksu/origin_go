package craftdefs

// Built-in quality formula ids. Recipes may override this in future for special logic.
const (
	QualityFormulaWeightedAverageFloor = "weighted_avg_floor"
)

type CraftInput struct {
	ItemKey       string `json:"itemKey"`
	Count         uint32 `json:"count"`
	QualityWeight uint32 `json:"qualityWeight"`
}

type CraftOutput struct {
	ItemKey string `json:"itemKey"`
	Count   uint32 `json:"count"`
}

type CraftDef struct {
	DefID int    `json:"defId"`
	Key   string `json:"key"`
	Name  string `json:"name"`

	Inputs  []CraftInput  `json:"inputs"`
	Outputs []CraftOutput `json:"outputs"`

	StaminaCost   float64 `json:"staminaCost"`
	TicksRequired uint32  `json:"ticksRequired"`

	RequiredSkills       []string `json:"requiredSkills,omitempty"`
	RequiredDiscovery    []string `json:"requiredDiscovery,omitempty"`
	RequiredLinkedObject string   `json:"requiredLinkedObjectKey,omitempty"`

	// QualityFormula selects result quality computation strategy.
	// Default is weighted average floor; special crafts may use custom strategies later.
	QualityFormula string `json:"qualityFormula,omitempty"`
}

type CraftsFile struct {
	Version int        `json:"v"`
	Source  string     `json:"source"`
	Crafts  []CraftDef `json:"crafts"`
}
