package actiondefs

type TargetKind string

const (
	TargetNone      TargetKind = "none"
	TargetObject    TargetKind = "object"
	TargetTile      TargetKind = "tile"
	TargetDirection TargetKind = "direction"
)

type Presentation struct {
	Label    string `json:"label"`
	MenuIcon string `json:"menuIcon"`
}

type Target struct {
	Kind     TargetKind `json:"kind"`
	Cursor   string     `json:"cursor,omitempty"`
	Approach string     `json:"approach,omitempty"`
}

const ApproachTileCenter = "tile_center"

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
	Ticks   int            `json:"ticks,omitempty"`
	Stamina float64        `json:"stamina,omitempty"`
	Repeat  bool           `json:"repeat,omitempty"`
	Combat  *CombatProfile `json:"combat,omitempty"`
}

type CombatProfile struct {
	Selection          string  `json:"selection"`
	SectorAngleDegrees float64 `json:"sectorAngleDegrees"`
	WindupMs           int64   `json:"windupMs"`
	RecoveryMs         int64   `json:"recoveryMs"`
	CooldownMs         int64   `json:"cooldownMs"`
	DamageMultiplier   float64 `json:"damageMultiplier"`
}

const (
	SelectionAll     = "all"
	SelectionNearest = "nearest"
)

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
