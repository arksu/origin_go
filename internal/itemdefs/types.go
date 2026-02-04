package itemdefs

// ItemDef represents a single item definition.
type ItemDef struct {
	DefID    int      `json:"defId"`
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Tags     []string `json:"tags"`
	Size     Size     `json:"size"`
	Stack    *Stack   `json:"stack"`
	Allowed  Allowed  `json:"allowed"`
	Resource string   `json:"resource,omitempty"`
	Visual   *Visual  `json:"visual,omitempty"`

	// Container describes nested inventory capabilities for this item (e.g. seed bag).
	// If nil, the item is not a container.
	Container *ContainerDef `json:"container,omitempty"`
}

// Size represents item dimensions in inventory grid.
type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

// Stack represents stacking behavior.
type Stack struct {
	Mode string `json:"mode"`
	Max  int    `json:"max"`
}

// StackModeNone indicates item cannot be stacked.
const StackModeNone = "none"

// StackModeStack indicates item can be stacked.
const StackModeStack = "stack"

// Allowed represents where an item can be placed.
type Allowed struct {
	Hand           *bool    `json:"hand,omitempty"`
	Grid           *bool    `json:"grid,omitempty"`
	EquipmentSlots []string `json:"equipmentSlots,omitempty"`
}

type ContainerDef struct {
	Size Size `json:"size"`
	// ContentRules limit what items can be placed into this container.
	Rules ContentRules `json:"rules"`
}

// ContentRules defines allow/deny constraints for items placed inside the container.
type ContentRules struct {
	// If non-empty: item must have at least one of these tags.
	AllowTags []string `json:"allowTags,omitempty"`

	// If item has any of these tags => forbidden.
	DenyTags []string `json:"denyTags,omitempty"`

	// Optional точечный whitelist по key (удобнее, чем typeId).
	AllowItemKeys []string `json:"allowItemKeys,omitempty"`

	// Optional точечный blacklist по key.
	DenyItemKeys []string `json:"denyItemKeys,omitempty"`
}

// Visual defines rules for computing resource path dynamically.
type Visual struct {
	NestedInventory *NestedInventoryVisual `json:"nestedInventory,omitempty"`
}

// NestedInventoryVisual defines resource paths based on nested inventory state.
type NestedInventoryVisual struct {
	HasItems string `json:"hasItems"` // Resource when nested inventory has items
	Empty    string `json:"empty"`    // Resource when nested inventory is empty
}

// ResolveResource computes the resource path based on visual rules and item state.
// If no visual rules apply, returns the base resource field.
func (def *ItemDef) ResolveResource(hasNestedItems bool) string {
	if def.Visual != nil && def.Visual.NestedInventory != nil {
		if hasNestedItems && def.Visual.NestedInventory.HasItems != "" {
			return def.Visual.NestedInventory.HasItems
		}
		if !hasNestedItems && def.Visual.NestedInventory.Empty != "" {
			return def.Visual.NestedInventory.Empty
		}
	}
	return def.Resource
}

// ItemsFile represents a JSON file containing item definitions.
type ItemsFile struct {
	Version int       `json:"v"`
	Source  string    `json:"source"`
	Items   []ItemDef `json:"items"`
}
