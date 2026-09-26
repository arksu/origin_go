package core

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// Run against a disposable schema from migrations/schema.sql. All writes roll
// back, including failures, so this also exercises the real affected-row API.
func TestChunkPersistencePostgres(t *testing.T) {
	dsn := os.Getenv("ORIGIN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORIGIN_TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	queries := repository.New(tx)
	params := repository.UpsertChunkParams{Region: 99, X: -1, Y: -1, Layer: 0, TilesData: []byte{35, 35, 35, 35}, Version: 0, LastTick: 42}
	if count, err := queries.UpsertChunk(ctx, params); err != nil || count != 1 {
		t.Fatalf("initial write: %d %v", count, err)
	}
	key := repository.GetChunkParams{Region: 99, X: -1, Y: -1, Layer: 0}
	row, err := queries.GetChunk(ctx, key)
	if err != nil || row.Version != 0 {
		t.Fatalf("initial version: %#v %v", row, err)
	}
	params.Version = 17
	if count, err := queries.UpsertChunk(ctx, params); err != nil || count != 1 {
		t.Fatalf("advanced write: %d %v", count, err)
	}
	row, err = queries.GetChunk(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	chunk := NewChunk(types.ChunkCoord{X: -1, Y: -1}, 99, 0, 2)
	if err := chunk.RestoreTiles(row.TilesData, uint64(row.LastTick), uint32(row.Version)); err != nil {
		t.Fatal(err)
	}
	chunk.SetTile(0, 0, types.TilePlowed)
	if err := chunk.saveTiles(ctx, queries, 0); err != nil {
		t.Fatal(err)
	}
	row, err = queries.GetChunk(ctx, key)
	if err != nil || row.Version != 18 || row.LastTick != 42 || row.TilesData[0] != types.TilePlowed || chunk.TilesDirty() {
		t.Fatalf("saved snapshot: %#v %v", row, err)
	}
	params.Version = 17
	if count, err := queries.UpsertChunk(ctx, params); err != nil || count != 0 {
		t.Fatalf("stale write not rejected: %d %v", count, err)
	}
	row, err = queries.GetChunk(ctx, key)
	if err != nil || row.Version != 18 || row.TilesData[0] != types.TilePlowed {
		t.Fatalf("stale write replaced tiles: %#v %v", row, err)
	}
	params.Version, params.TilesData = row.Version, row.TilesData
	if count, err := queries.UpsertChunk(ctx, params); err != nil || count != 1 {
		t.Fatalf("equal version retry rejected: %d %v", count, err)
	}
}
