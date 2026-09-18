package components

import "testing"

func TestStationStateConsumeResourceRejectsUnderflow(t *testing.T) {
	state := StationState{
		Resources: map[string]uint32{"thread": 2},
	}

	if !state.ConsumeResource("thread", 1) {
		t.Fatal("expected first consumption to succeed")
	}
	if got := state.Resources["thread"]; got != 1 {
		t.Fatalf("remaining resource = %d, want 1", got)
	}
	if state.ConsumeResource("thread", 2) {
		t.Fatal("expected underflow to be rejected")
	}
	if got := state.Resources["thread"]; got != 1 {
		t.Fatalf("resource changed after rejected consumption: got %d, want 1", got)
	}
}

func TestStationStateSnapshotIsIndependent(t *testing.T) {
	state := StationState{
		Capabilities: []string{"cooking"},
		Values:       map[string]float64{"temperature": 800},
		Resources:    map[string]uint32{"fuel": 5},
	}

	snapshot := state.Snapshot()
	snapshot.Capabilities[0] = "changed"
	snapshot.Values["temperature"] = 10
	snapshot.Resources["fuel"] = 0

	if state.Capabilities[0] != "cooking" || state.Values["temperature"] != 800 || state.Resources["fuel"] != 5 {
		t.Fatal("snapshot mutated the authoritative station state")
	}
}
