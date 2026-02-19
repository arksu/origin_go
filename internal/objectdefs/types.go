package objectdefs

import "encoding/json"

// ObjectDef represents a single object definition.
type ObjectDef struct {
	DefID                     int                        `json:"defId"`
	Key                       string                     `json:"key"`
	Name                      string                     `json:"name,omitempty"`
	Static                    *bool                      `json:"static,omitempty"`
	ContextMenuEvenForOneItem *bool                      `json:"contextMenuEvenForOneItem,omitempty"`
	HP                        int                        `json:"hp,omitempty"`
	Components                *Components                `json:"components,omitempty"`
	Resource                  string                     `json:"resource,omitempty"`
	Appearance                []Appearance               `json:"appearance,omitempty"`
	Behaviors                 map[string]json.RawMessage `json:"behaviors,omitempty"`

	// resolved at load time
	IsStatic                       bool                `json:"-"`
	ContextMenuEvenForOneItemValue bool                `json:"-"`
	BehaviorOrder                  []string            `json:"-"`
	BehaviorPriorities             map[string]int      `json:"-"`
	TreeConfig                     *TreeBehaviorConfig `json:"-"`
	TakeConfig                     *TakeBehaviorConfig `json:"-"`
}

// Components describes ECS components to attach when loading the object.
type Components struct {
	Collider  *ColliderDef   `json:"collider,omitempty"`
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

// TreeBehaviorConfig contains numeric/tree-specific config only.
// Behavior logic itself is implemented in code.
type TreeBehaviorConfig struct {
	Priority int               `json:"priority,omitempty"`
	Stages   []TreeStageConfig `json:"stages"`
}

type TreeStageConfig struct {
	ChopPointsTotal   int              `json:"chopPointsTotal"`
	StageDuration     int              `json:"stageDurationTicks"`
	AllowChop         bool             `json:"allowChop"`
	SpawnChopObject   []string         `json:"spawnChopObject,omitempty"`
	SpawnChopItem     []string         `json:"spawnChopItem,omitempty"`
	Take              []TakeConfig     `json:"take,omitempty"`
	TransformToDefKey string           `json:"transformToDefKey,omitempty"`
}

type TakeConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ItemDefKey string `json:"itemDefKey"`
	Count      int    `json:"count"`
}

type TakeBehaviorConfig struct {
	Priority int          `json:"priority,omitempty"`
	Items    []TakeConfig `json:"items"`
}

// ObjectsFile represents a JSONC file containing object definitions.
type ObjectsFile struct {
	Version int         `json:"v"`
	Source  string      `json:"source"`
	Objects []ObjectDef `json:"objects"`
}

func (d *ObjectDef) HasBehavior(key string) bool {
	if d == nil || len(d.Behaviors) == 0 || key == "" {
		return false
	}
	_, ok := d.Behaviors[key]
	return ok
}

func (d *ObjectDef) PriorityForBehavior(key string) int {
	if d == nil || len(d.BehaviorPriorities) == 0 {
		return 100
	}
	priority, ok := d.BehaviorPriorities[key]
	if !ok {
		return 100
	}
	return priority
}

func (d *ObjectDef) CopyBehaviorOrder() []string {
	if d == nil || len(d.BehaviorOrder) == 0 {
		return nil
	}

	behaviors := make([]string, len(d.BehaviorOrder))
	copy(behaviors, d.BehaviorOrder)
	return behaviors
}
