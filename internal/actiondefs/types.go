package actiondefs

type Audience string

const (
	AudiencePlayer Audience = "player"
	AudienceNPC    Audience = "npc"
)

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
	Slots        []string `json:"slots"`
	ItemKey      string   `json:"itemKey,omitempty"`
	ItemTag      string   `json:"itemTag,omitempty"`
	DamageSource bool     `json:"damageSource,omitempty"` // Server-only weapon selector.
}

type Requirements struct {
	Skills    []string               `json:"skills,omitempty"`
	Equipment []EquipmentRequirement `json:"equipment,omitempty"`
}

type Execution struct {
	WindupMs uint32  `json:"windupMs,omitempty"`
	Ticks    int     `json:"ticks,omitempty"`
	Stamina  float64 `json:"stamina,omitempty"`
	Repeat   bool    `json:"repeat,omitempty"`
}

type Sector struct {
	Range float64
	Angle float64 // Full sector width in radians.
}

type HitMode string

const (
	HitAll     HitMode = "all"
	HitNearest HitMode = "nearest"
)

type Combat struct {
	HitMode          HitMode  `json:"hitMode"`
	DamageMultiplier float64  `json:"damageMultiplier,omitempty"`
	RawDamage        *float64 `json:"rawDamage,omitempty"`
}

type Definition struct {
	ID           string       `json:"id"`
	Audience     Audience     `json:"audience,omitempty"`
	Presentation Presentation `json:"presentation"`
	Target       Target       `json:"target"`
	Requirements Requirements `json:"requirements"`
	Execution    Execution    `json:"execution"`
	Cooldown     uint32       `json:"cooldown,omitempty"`
	IsRepeatable *bool        `json:"isRepeatable,omitempty"`
	Sector       *Sector      `json:"-"`
	Combat       *Combat      `json:"combat,omitempty"`

	SourceFile string `json:"-"`
}

func (definition *Definition) Repeatable() bool {
	return definition != nil && definition.IsRepeatable != nil && *definition.IsRepeatable
}

// PlayerAvailable preserves the audience of existing definitions that omit the field.
func (definition *Definition) PlayerAvailable() bool {
	return definition != nil && (definition.Audience == "" || definition.Audience == AudiencePlayer)
}
