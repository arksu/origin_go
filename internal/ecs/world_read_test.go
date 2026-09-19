package ecs

import "testing"

func TestComponentReadsDoNotCreateStorage(t *testing.T) {
	for _, operation := range []string{"get", "has"} {
		t.Run(operation, func(t *testing.T) {
			w := NewWorldWithCapacity(16, nil, 0)
			handle := w.Spawn(1, nil)
			if w.GetStorage(BenchPositionID) != nil {
				t.Fatal("expected position storage to be absent")
			}
			if operation == "get" {
				value, found := GetComponent[BenchPosition](w, handle)
				if found || value != (BenchPosition{}) {
					t.Fatalf("missing component returned %+v, %v", value, found)
				}
			} else if HasComponent[BenchPosition](w, handle) {
				t.Fatal("missing component reported as present")
			}
			if w.GetStorage(BenchPositionID) != nil {
				t.Fatal("reading a missing component created storage under a read lock")
			}

			AddComponent(w, handle, BenchPosition{X: 42})
			position, found := GetComponent[BenchPosition](w, handle)
			if !found || position.X != 42 || !HasComponent[BenchPosition](w, handle) {
				t.Fatal("component added after an absent lookup must remain readable")
			}
		})
	}
}
