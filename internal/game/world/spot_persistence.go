package world

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"origin/internal/persistence"
	"origin/internal/persistence/repository"
)

var ErrSpotRevisionConflict = errors.New("spot revision conflict")

// SpotStateWrite owns an immutable snapshot until SaveSpotState returns. Retrying
// it must retain the same expected revision, state and runtime timestamp.
type SpotStateWrite struct {
	ID               int64
	Region           int
	ExpectedRevision int64
	State            json.RawMessage
	RuntimeSeconds   int64
}

type SpotStore interface {
	LoadSpots(context.Context, int) ([]repository.Spot, error)
	SaveSpotState(context.Context, SpotStateWrite) error
}

type SpotStoreDB struct {
	db *persistence.Postgres
}

func NewSpotStoreDB(db *persistence.Postgres) *SpotStoreDB {
	return &SpotStoreDB{db: db}
}

func (s *SpotStoreDB) LoadSpots(ctx context.Context, region int) ([]repository.Spot, error) {
	if s == nil || s.db == nil || ctx == nil || region <= 0 {
		return nil, fmt.Errorf("load spots: invalid database, context or region")
	}
	rows, err := s.db.Queries().LoadSpotsByRegion(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("load spots: %w", err)
	}
	return rows, nil
}

func (s *SpotStoreDB) SaveSpotState(ctx context.Context, write SpotStateWrite) error {
	if s == nil || s.db == nil || ctx == nil || write.ID <= 0 || write.Region <= 0 ||
		write.ExpectedRevision <= 0 || write.ExpectedRevision == math.MaxInt64 || write.RuntimeSeconds < 0 {
		return fmt.Errorf("save spot state: invalid database, context or snapshot")
	}
	if !validSpotState(write.State) {
		return fmt.Errorf("save spot state: state must be a JSON object")
	}
	rows, err := s.db.Queries().SaveSpotState(ctx, repository.SaveSpotStateParams{
		ID: write.ID, Region: write.Region, ExpectedRevision: write.ExpectedRevision,
		State: write.State, LastRuntimeSeconds: write.RuntimeSeconds,
	})
	if err != nil {
		return fmt.Errorf("save spot state: %w", err)
	}
	if rows == 0 {
		return ErrSpotRevisionConflict
	}
	return nil
}

func validSpotState(state json.RawMessage) bool {
	trimmed := bytes.TrimSpace(state)
	return len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed)
}
