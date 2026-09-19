package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"origin/internal/persistence/repository"
)

type deletionQueriesStub struct {
	deleteError    error
	inventoryError error
	object         repository.SoftDeleteObjectParams
	deletedOwner   int64
}

func (q *deletionQueriesStub) SoftDeleteObject(_ context.Context, target repository.SoftDeleteObjectParams) (int64, error) {
	q.object = target
	return target.ID, q.deleteError
}

func (q *deletionQueriesStub) DeleteInventoriesByOwner(_ context.Context, owner int64) error {
	q.deletedOwner = owner
	return q.inventoryError
}

func TestDeleteObjectRows(t *testing.T) {
	dbFailure := errors.New("database unavailable")
	for _, tc := range []struct {
		name                string
		deleteError         error
		inventoryError      error
		allowMissing        bool
		wantError           error
		wantInventoryDelete bool
	}{
		{"unsaved spawn", sql.ErrNoRows, nil, true, nil, true},
		{"wrapped missing row", fmt.Errorf("query: %w", sql.ErrNoRows), nil, true, nil, true},
		{"saved object", nil, nil, true, nil, true},
		{"database failure", dbFailure, nil, true, dbFailure, false},
		{"inventory failure after absent object", sql.ErrNoRows, dbFailure, true, dbFailure, true},
		{"missing pickup must fail", sql.ErrNoRows, nil, false, sql.ErrNoRows, false},
		{"saved pickup", nil, nil, false, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queries := &deletionQueriesStub{deleteError: tc.deleteError, inventoryError: tc.inventoryError}
			err := deleteObjectRows(context.Background(), queries, 1, 487952, tc.allowMissing)
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("delete returned %v, want %v", err, tc.wantError)
			}
			if queries.object.Region != 1 || queries.object.ID != 487952 {
				t.Fatalf("wrong object target: %+v", queries.object)
			}
			if tc.wantInventoryDelete && queries.deletedOwner != 487952 {
				t.Fatal("owned inventories were not deleted")
			}
			if !tc.wantInventoryDelete && queries.deletedOwner != 0 {
				t.Fatal("deleted inventories after failed object deletion")
			}
		})
	}
}
