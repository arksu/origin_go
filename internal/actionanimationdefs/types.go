package actionanimationdefs

type Source struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	ID        string `json:"id"`
}

type Equipment struct {
	Slot      string `json:"slot"`
	VisualKey string `json:"visual_key"`
}

type Variant struct {
	Clip      string      `json:"clip"`
	Equipment []Equipment `json:"equipment"`
}

type Frame struct {
	Width   int `json:"width"`
	Height  int `json:"height"`
	OriginX int `json:"origin_x"`
	OriginY int `json:"origin_y"`
}

type Preview struct {
	Label      string      `json:"label"`
	DurationMs float64     `json:"duration_ms"`
	Equipment  []Equipment `json:"equipment"`
}

type Definition struct {
	Key                  string    `json:"key"`
	Source               Source    `json:"source"`
	Actor                string    `json:"actor"`
	Variants             []Variant `json:"variants"`
	Eligibility          []string  `json:"eligibility"`
	Facing               string    `json:"facing"`
	BlendMs              float64   `json:"blend_ms"`
	Frame                Frame     `json:"frame"`
	UnbindEquipmentSlots []string  `json:"unbind_equipment_slots,omitempty"`
	Preview              *Preview  `json:"preview,omitempty"`
	SourceFile           string    `json:"-"`
}
