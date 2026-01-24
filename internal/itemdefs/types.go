package itemdefs

// ItemDef represents a single item definition.
type ItemDef struct {
	DefID   int      `json:"defId"`
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Tags    []string `json:"tags"`
	Size    Size     `json:"size"`
	Stack   *Stack   `json:"stack"`
	Allowed Allowed  `json:"allowed"`
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

// Allowed represents where an item can be placed.
type Allowed struct {
	Hand           *bool    `json:"hand,omitempty"`
	Grid           *bool    `json:"grid,omitempty"`
	EquipmentSlots []string `json:"equipmentSlots,omitempty"`
}

// ItemsFile represents a JSON file containing item definitions.
type ItemsFile struct {
	Version int       `json:"v"`
	Source  string    `json:"source"`
	Items   []ItemDef `json:"items"`
}

// StackModeNone indicates item cannot be stacked.
const StackModeNone = "none"

// StackModeStack indicates item can be stacked.
const StackModeStack = "stack"
