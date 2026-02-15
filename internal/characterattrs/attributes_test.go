package characterattrs

import (
	"encoding/json"
	"testing"
)

func TestNormalize_OnlyRequiredAndMinOne(t *testing.T) {
	input := Values{
		INT:           5,
		STR:           0,
		PER:           -10,
		Name("EXTRA"): 99,
	}

	got := Normalize(input)

	if len(got) != 9 {
		t.Fatalf("expected 9 attributes, got %d", len(got))
	}
	if got[INT] != 5 {
		t.Fatalf("expected INT=5, got %d", got[INT])
	}
	if got[STR] != 1 {
		t.Fatalf("expected STR=1, got %d", got[STR])
	}
	if got[PER] != 1 {
		t.Fatalf("expected PER=1, got %d", got[PER])
	}
	if _, ok := got[Name("EXTRA")]; ok {
		t.Fatalf("extra key should be removed")
	}
}

func TestFromRaw_FailSafe(t *testing.T) {
	raw := json.RawMessage(`{"INT":4,"STR":"bad","PER":0,"EXTRA":42}`)
	got, changed := FromRaw(raw)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if got[INT] != 4 {
		t.Fatalf("expected INT=4, got %d", got[INT])
	}
	if got[STR] != 1 {
		t.Fatalf("expected STR=1, got %d", got[STR])
	}
	if got[PER] != 1 {
		t.Fatalf("expected PER=1, got %d", got[PER])
	}
	if len(got) != 9 {
		t.Fatalf("expected 9 attributes, got %d", len(got))
	}
}

func TestGet_FailSafe(t *testing.T) {
	values := Values{
		INT: 10,
		STR: 0,
	}
	if got := Get(values, INT); got != 10 {
		t.Fatalf("expected INT=10, got %d", got)
	}
	if got := Get(values, STR); got != 1 {
		t.Fatalf("expected STR=1, got %d", got)
	}
	if got := Get(values, CHA); got != 1 {
		t.Fatalf("expected CHA=1 for missing key, got %d", got)
	}
}
