package world

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

func TestIsMissingReplacementSource(t *testing.T) {
	if !isMissingReplacementSource(sql.ErrNoRows) {
		t.Fatal("expected sql.ErrNoRows to identify an unsaved replacement source")
	}
	if !isMissingReplacementSource(fmt.Errorf("soft-delete replacement source: %w", sql.ErrNoRows)) {
		t.Fatal("expected wrapped sql.ErrNoRows to identify an unsaved replacement source")
	}
	if isMissingReplacementSource(errors.New("database unavailable")) {
		t.Fatal("unexpectedly accepted a real persistence failure")
	}
}
