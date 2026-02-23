package builddefs

type BuildInput struct {
	ItemKey       string `json:"itemKey,omitempty"`
	ItemTag       string `json:"itemTag,omitempty"`
	Count         uint32 `json:"count"`
	QualityWeight uint32 `json:"qualityWeight"`
}

type BuildDef struct {
	DefID int    `json:"defId"`
	Key   string `json:"key"`
	Name  string `json:"name"`

	Inputs []BuildInput `json:"inputs"`

	StaminaCost   float64 `json:"staminaCost"`
	TicksRequired uint32  `json:"ticksRequired"`

	RequiredSkills    []string `json:"requiredSkills,omitempty"`
	RequiredDiscovery []string `json:"requiredDiscovery,omitempty"`

	AllowedTiles    []int `json:"allowedTiles,omitempty"`
	DisallowedTiles []int `json:"disallowedTiles,omitempty"`

	ObjectKey string `json:"objectKey"`
}

type BuildsFile struct {
	Version int        `json:"v"`
	Source  string     `json:"source"`
	Builds  []BuildDef `json:"builds"`
}
