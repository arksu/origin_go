package components

import "testing"

func TestIsDroppedItemExpiredUsesRuntimeSeconds(t *testing.T) {
	tests := []struct {
		name     string
		dropTime int64
		now      int64
		expired  bool
	}{
		{name: "before lifetime", dropTime: 100, now: 109, expired: false},
		{name: "at lifetime", dropTime: 100, now: 110, expired: true},
		{name: "after lifetime", dropTime: 100, now: 111, expired: true},
		{name: "runtime rollback", dropTime: 100, now: 99, expired: false},
		{name: "invalid timestamp", dropTime: -1, now: 100, expired: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDroppedItemExpired(tt.dropTime, tt.now); got != tt.expired {
				t.Fatalf("IsDroppedItemExpired(%d, %d) = %v, want %v", tt.dropTime, tt.now, got, tt.expired)
			}
		})
	}
}
