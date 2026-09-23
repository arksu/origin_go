package actiondefs

type TargetKind string

const (
	TargetNone   TargetKind = "none"
	TargetObject TargetKind = "object"
	TargetTile   TargetKind = "tile"
)

type Presentation struct {
	Label    string `json:"label"`
	MenuIcon string `json:"menuIcon"`
}

type Target struct {
	Kind   TargetKind `json:"kind"`
	Cursor string     `json:"cursor,omitempty"`
}

type EquipmentRequirement struct {
	Slots   []string `json:"slots"`
	ItemKey string   `json:"itemKey,omitempty"`
	ItemTag string   `json:"itemTag,omitempty"`
}

type Requirements struct {
	Skills    []string               `json:"skills,omitempty"`
	Equipment []EquipmentRequirement `json:"equipment,omitempty"`
}

type Execution struct {
	Ticks   int     `json:"ticks,omitempty"`
	Stamina float64 `json:"stamina,omitempty"`
}

type Definition struct {
	ID           string       `json:"id"`
	Presentation Presentation `json:"presentation"`
	Target       Target       `json:"target"`
	Requirements Requirements `json:"requirements"`
	Execution    Execution    `json:"execution"`
	IsRepeatable *bool        `json:"isRepeatable,omitempty"`

	SourceFile string `json:"-"`
}

func (definition *Definition) Repeatable() bool {
	return definition != nil && definition.IsRepeatable != nil && *definition.IsRepeatable
}
