package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	_const "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/game/world/wildweed"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// WildWeedRefillWrite is retained unchanged until both commit and publication
// finish. Entity IDs and the checked interval are its retry receipt.
type WildWeedRefillWrite struct {
	SpotID              int64
	Region              int
	RuntimeSeconds      int64
	CheckRuntimeSeconds int64
	Target              int
	Objects             []repository.Object
	MaxAllocatedID      types.EntityID
}

type WildWeedRefillStore interface {
	RefillWildWeeds(context.Context, WildWeedRefillWrite) (SpotStateCommit, error)
}

type wildWeedRefillPosition = wildweed.Position

func validateWildWeedRefill(write WildWeedRefillWrite) (map[string]wildWeedRefillPosition, error) {
	if write.SpotID <= 0 || write.Region <= 0 || write.RuntimeSeconds < 0 ||
		write.CheckRuntimeSeconds < 0 || write.CheckRuntimeSeconds > write.RuntimeSeconds ||
		write.Target < 0 || len(write.Objects) > write.Target || write.MaxAllocatedID > math.MaxInt64 {
		return nil, fmt.Errorf("refill wild weeds: invalid identity, runtime or target")
	}
	positions := make(map[string]wildWeedRefillPosition, len(write.Objects))
	for _, object := range write.Objects {
		coord := types.WorldToChunkCoord(object.X, object.Y, _const.ChunkSize, _const.CoordPerTile)
		if object.ID <= 0 || types.EntityID(object.ID) > write.MaxAllocatedID || object.TypeID <= 0 ||
			object.Region != write.Region || object.Layer != 0 || object.ChunkX != coord.X || object.ChunkY != coord.Y ||
			!object.Hp.Valid || object.Hp.Float64 <= 0 || math.IsNaN(object.Hp.Float64) || math.IsInf(object.Hp.Float64, 0) ||
			object.Quality < 10 || object.DeletedAt.Valid || !object.Data.Valid {
			return nil, fmt.Errorf("refill wild weeds: invalid object %d", object.ID)
		}
		var envelope struct {
			Version   int `json:"v"`
			Behaviors struct {
				WildWeed *components.WildWeedBehaviorState `json:"wild_weed"`
			} `json:"behaviors"`
		}
		if err := json.Unmarshal(object.Data.RawMessage, &envelope); err != nil || envelope.Version != 1 ||
			envelope.Behaviors.WildWeed == nil || envelope.Behaviors.WildWeed.SpotID != write.SpotID ||
			envelope.Behaviors.WildWeed.Stage != 0 || envelope.Behaviors.WildWeed.NextGrowthRuntimeSeconds <= write.RuntimeSeconds {
			return nil, fmt.Errorf("refill wild weeds: invalid stem state for object %d", object.ID)
		}
		key := strconv.FormatInt(object.ID, 10)
		if _, exists := positions[key]; exists {
			return nil, fmt.Errorf("refill wild weeds: duplicate object %d", object.ID)
		}
		positions[key] = wildWeedRefillPosition{X: object.X, Y: object.Y}
	}
	return positions, nil
}

// RefillWildWeeds merges into the current row while its lock is held. It never
// rewrites a caller's snapshot, so destruction of another member cannot be lost.
func (s *SpotStoreDB) RefillWildWeeds(ctx context.Context, write WildWeedRefillWrite) (SpotStateCommit, error) {
	if s == nil || s.db == nil || ctx == nil {
		return SpotStateCommit{}, fmt.Errorf("refill wild weeds: invalid database or context")
	}
	positions, err := validateWildWeedRefill(write)
	if err != nil {
		return SpotStateCommit{}, err
	}
	positionsJSON, err := json.Marshal(positions)
	if err != nil {
		return SpotStateCommit{}, err
	}
	var commit SpotStateCommit
	err = s.db.WithTx(ctx, func(q *repository.Queries) error {
		spot, err := q.LockWildWeedSpot(ctx, repository.LockWildWeedSpotParams{ID: write.SpotID, Region: write.Region})
		if err != nil {
			return fmt.Errorf("lock WWW spot %d: %w", write.SpotID, err)
		}
		var state struct {
			WWW struct {
				Objects   map[string]json.RawMessage `json:"objects"`
				LastCheck *int64                     `json:"last_check_runtime_seconds"`
			} `json:"www"`
		}
		if err := json.Unmarshal(spot.State, &state); err != nil || state.WWW.Objects == nil ||
			(state.WWW.LastCheck != nil && *state.WWW.LastCheck < 0) {
			return fmt.Errorf("refill wild weeds: malformed manifest in spot %d", write.SpotID)
		}
		present := 0
		for id, position := range positions {
			if raw, exists := state.WWW.Objects[id]; exists {
				var saved wildWeedRefillPosition
				if err := json.Unmarshal(raw, &saved); err != nil || saved != position {
					return fmt.Errorf("refill wild weeds: conflicting object %s in spot %d", id, write.SpotID)
				}
				present++
			}
		}
		if present != 0 && present != len(positions) {
			return fmt.Errorf("refill wild weeds: partial retry receipt in spot %d", write.SpotID)
		}
		// A known set of IDs proves a previous commit even when filling the spot
		// removed its cursor. Empty checks are identified by the advanced cursor.
		if present > 0 || (len(positions) == 0 && ((len(state.WWW.Objects) >= write.Target && state.WWW.LastCheck == nil) ||
			(len(state.WWW.Objects) < write.Target && state.WWW.LastCheck != nil && *state.WWW.LastCheck >= write.CheckRuntimeSeconds))) {
			commit = SpotStateCommit{SpotID: spot.ID, Snapshot: SpotStateSnapshot{
				State: spot.State, Revision: spot.Revision, LastRuntimeSeconds: spot.LastRuntimeSeconds,
			}}
			return nil
		}
		if len(state.WWW.Objects)+len(positions) > write.Target && len(positions) > 0 {
			return fmt.Errorf("refill wild weeds: target exceeded in spot %d", write.SpotID)
		}
		counts := make(map[types.ChunkCoord]int)
		for _, object := range write.Objects {
			inserted, err := q.InsertWildWeedObject(ctx, repository.InsertWildWeedObjectParams{
				ID: object.ID, TypeID: object.TypeID, Region: object.Region, X: object.X, Y: object.Y, Layer: object.Layer,
				ChunkX: object.ChunkX, ChunkY: object.ChunkY, Heading: object.Heading, Quality: object.Quality,
				Hp: object.Hp, OwnerID: object.OwnerID, Data: object.Data, CreateTick: object.CreateTick, LastTick: object.LastTick,
			})
			if err != nil {
				return fmt.Errorf("insert WWW object %d: %w", object.ID, err)
			}
			if inserted != 1 {
				return fmt.Errorf("insert WWW object %d: identity already exists outside this manifest", object.ID)
			}
			counts[types.ChunkCoord{X: object.ChunkX, Y: object.ChunkY}]++
		}
		// Deterministic lock order also avoids deadlocks between overlapping spots.
		coords := make([]types.ChunkCoord, 0, len(counts))
		for coord := range counts {
			coords = append(coords, coord)
		}
		sort.Slice(coords, func(i, j int) bool {
			return coords[i].Y < coords[j].Y || (coords[i].Y == coords[j].Y && coords[i].X < coords[j].X)
		})
		for _, coord := range coords {
			updated, err := q.IncrementChunkEntityCount(ctx, repository.IncrementChunkEntityCountParams{
				Region: write.Region, X: coord.X, Y: coord.Y, Layer: 0, Count: counts[coord],
			})
			if err != nil {
				return fmt.Errorf("increment WWW chunk count: %w", err)
			}
			if updated != 1 {
				return fmt.Errorf("increment WWW chunk count: missing chunk %v", coord)
			}
		}
		row, err := q.RefillWildWeedSpotState(ctx, repository.RefillWildWeedSpotStateParams{
			ID: write.SpotID, Region: write.Region, Objects: positionsJSON, Target: write.Target,
			CheckRuntimeSeconds: write.CheckRuntimeSeconds, RuntimeSeconds: write.RuntimeSeconds,
		})
		if err != nil {
			return fmt.Errorf("merge WWW spot %d: %w", write.SpotID, err)
		}
		if len(write.Objects) > 0 {
			if err := q.UpsertGlobalVarLongMax(ctx, repository.UpsertGlobalVarLongMaxParams{
				Name: _const.LAST_USED_ID, ValueLong: sql.NullInt64{Int64: int64(write.MaxAllocatedID), Valid: true},
			}); err != nil {
				return fmt.Errorf("checkpoint WWW entity IDs: %w", err)
			}
		}
		commit = SpotStateCommit{SpotID: write.SpotID, Snapshot: SpotStateSnapshot{
			State: row.State, Revision: row.Revision, LastRuntimeSeconds: row.LastRuntimeSeconds,
		}}
		return nil
	})
	if err != nil {
		return SpotStateCommit{}, err
	}
	return commit, nil
}
