package ecs

import "testing"

func TestClearPendingAdminClicksIsIsolated(t *testing.T) {
	w := NewWorldForTesting()
	GetResource[PendingAdminSpawn](w).Set(1, AdminSpawnEntry{})
	GetResource[PendingAdminTeleport](w).Set(1)
	GetResource[PendingAdminObjectInfo](w).Set(1)
	GetResource[PendingAdminDestroy](w).Set(1)
	GetResource[PendingAdminDestroy](w).Set(2)
	ClearPendingAdminClicks(w, 1)
	_, spawn := GetResource[PendingAdminSpawn](w).Get(1)
	if spawn || GetResource[PendingAdminTeleport](w).Get(1) || GetResource[PendingAdminObjectInfo](w).Get(1) || GetResource[PendingAdminDestroy](w).Get(1) {
		t.Fatal("pending action survived cleanup")
	}
	if !GetResource[PendingAdminDestroy](w).Get(2) {
		t.Fatal("other player's selection cleared")
	}
}
