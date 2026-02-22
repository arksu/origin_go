package craftdefs

import "sync"

type Registry struct {
	byID  map[int]*CraftDef
	byKey map[string]*CraftDef
	all   []*CraftDef
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

func NewRegistry(crafts []CraftDef) *Registry {
	r := &Registry{
		byID:  make(map[int]*CraftDef, len(crafts)),
		byKey: make(map[string]*CraftDef, len(crafts)),
		all:   make([]*CraftDef, 0, len(crafts)),
	}
	for i := range crafts {
		craft := &crafts[i]
		r.byID[craft.DefID] = craft
		r.byKey[craft.Key] = craft
		r.all = append(r.all, craft)
	}
	return r
}

func (r *Registry) GetByID(defID int) (*CraftDef, bool) {
	if r == nil {
		return nil, false
	}
	v, ok := r.byID[defID]
	return v, ok
}

func (r *Registry) GetByKey(key string) (*CraftDef, bool) {
	if r == nil {
		return nil, false
	}
	v, ok := r.byKey[key]
	return v, ok
}

func (r *Registry) All() []*CraftDef {
	if r == nil {
		return nil
	}
	out := make([]*CraftDef, len(r.all))
	copy(out, r.all)
	return out
}

func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	return len(r.byID)
}

func SetGlobal(r *Registry) {
	registryOnce.Do(func() {
		globalRegistry = r
	})
}

func SetGlobalForTesting(r *Registry) {
	registryOnce = sync.Once{}
	globalRegistry = r
}

func Global() *Registry {
	return globalRegistry
}
