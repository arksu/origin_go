package builddefs

import "sync"

type Registry struct {
	byID  map[int]*BuildDef
	byKey map[string]*BuildDef
	all   []*BuildDef
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

func NewRegistry(builds []BuildDef) *Registry {
	r := &Registry{
		byID:  make(map[int]*BuildDef, len(builds)),
		byKey: make(map[string]*BuildDef, len(builds)),
		all:   make([]*BuildDef, 0, len(builds)),
	}
	for i := range builds {
		build := &builds[i]
		r.byID[build.DefID] = build
		r.byKey[build.Key] = build
		r.all = append(r.all, build)
	}
	return r
}

func (r *Registry) GetByID(defID int) (*BuildDef, bool) {
	if r == nil {
		return nil, false
	}
	v, ok := r.byID[defID]
	return v, ok
}

func (r *Registry) GetByKey(key string) (*BuildDef, bool) {
	if r == nil {
		return nil, false
	}
	v, ok := r.byKey[key]
	return v, ok
}

func (r *Registry) All() []*BuildDef {
	if r == nil {
		return nil
	}
	out := make([]*BuildDef, len(r.all))
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
