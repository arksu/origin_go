package objectdefs

import "fmt"

// BehaviorRegistry tracks known behavior keys.
// At this stage behaviors are stubs without logic.
type BehaviorRegistry struct {
	known map[string]struct{}
}

// NewBehaviorRegistry creates a registry with the given known behavior keys.
func NewBehaviorRegistry(keys ...string) *BehaviorRegistry {
	r := &BehaviorRegistry{known: make(map[string]struct{}, len(keys))}
	for _, k := range keys {
		r.known[k] = struct{}{}
	}
	return r
}

// IsRegistered returns true if the behavior key is known.
func (r *BehaviorRegistry) IsRegistered(key string) bool {
	_, ok := r.known[key]
	return ok
}

// Validate checks that every key in the slice is registered.
func (r *BehaviorRegistry) Validate(keys []string) error {
	for _, k := range keys {
		if !r.IsRegistered(k) {
			return fmt.Errorf("unknown behavior %q", k)
		}
	}
	return nil
}

// DefaultBehaviorRegistry returns a BehaviorRegistry pre-populated with
// the built-in behavior keys.
func DefaultBehaviorRegistry() *BehaviorRegistry {
	return NewBehaviorRegistry("tree", "container", "player")
}
