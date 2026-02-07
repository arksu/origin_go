package objectdefs

// ObjectDef represents a single object definition.
type ObjectDef struct {
	DefID      int           `json:"defId"`
	Key        string        `json:"key"`
	Static     *bool         `json:"static,omitempty"`
	HP         int           `json:"hp,omitempty"`
	Components *Components   `json:"components,omitempty"`
	Resource   string        `json:"resource,omitempty"`
	Appearance []Appearance  `json:"appearance,omitempty"`
	Behavior   []string      `json:"behavior,omitempty"`

	// resolved at load time
	IsStatic bool `json:"-"`
}

// Components describes ECS components to attach when loading the object.
type Components struct {
	Collider  *ColliderDef  `json:"collider,omitempty"`
	Inventory []InventoryDef `json:"inventory,omitempty"`
}

// ColliderDef describes collision box dimensions in game coordinates.
type ColliderDef struct {
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	Layer uint64  `json:"layer,omitempty"`
	Mask  uint64  `json:"mask,omitempty"`
}

// InventoryDef describes an inventory container to create for the object.
type InventoryDef struct {
	W    int    `json:"w"`
	H    int    `json:"h"`
	Kind string `json:"kind,omitempty"` // default "grid"
	Key  uint32 `json:"key,omitempty"`  // default 0
}

// Appearance describes a conditional visual override.
type Appearance struct {
	ID       string          `json:"id"`
	When     *AppearanceWhen `json:"when,omitempty"`
	Resource string          `json:"resource"`
}

// AppearanceWhen describes conditions for an appearance override.
type AppearanceWhen struct {
	Flags []string `json:"flags,omitempty"`
}

// ObjectsFile represents a JSONC file containing object definitions.
type ObjectsFile struct {
	Version int         `json:"v"`
	Source  string      `json:"source"`
	Objects []ObjectDef `json:"objects"`
}
