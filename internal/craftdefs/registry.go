package craftdefs

import "sync"

type Registry struct {
	byID  map[int]*CraftDef
	byKey map[string]*CraftDef
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

func NewRegistry(crafts []CraftDef) *Registry {
	r := &Registry{
		byID:  make(map[int]*CraftDef, len(crafts)),
		byKey: make(map[string]*CraftDef, len(crafts)),
	}
	for i := range crafts {
		craft := &crafts[i]
		r.byID[craft.DefID] = craft
		r.byKey[craft.Key] = craft
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
	out := make([]*CraftDef, 0, len(r.byID))
	for _, v := range r.byID {
		out = append(out, v)
	}
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
	globalRegistry = r
}

func Global() *Registry {
	return globalRegistry
}
