package persistence

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"origin/internal/config"
	"origin/internal/db"
)

type Postgres struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewPostgres(cfg *config.Config) (*Postgres, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.PostgresURL)
	if err != nil {
		return nil, err
	}

	// Apply pool configuration
	poolConfig.MaxConns = int32(cfg.DBMaxConns)
	poolConfig.MinConns = int32(cfg.DBMinConns)
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolConfig.ConnConfig.ConnectTimeout = cfg.DBConnTimeout
	poolConfig.ConnConfig.RuntimeParams["search_path"] = cfg.DBSchema

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, err
	}

	// Test the connection
	if err := pool.Ping(context.Background()); err != nil {
		return nil, err
	}

	// Create a database/sql compatible connection for sqlc
	sqlDB := stdlib.OpenDBFromPool(pool)
	queries := db.New(sqlDB)

	return &Postgres{
		pool:    pool,
		queries: queries,
	}, nil
}

func (p *Postgres) Close() {
	p.pool.Close()
}

// LoadCharacter loads a character by ID
func (p *Postgres) LoadCharacter(ctx context.Context, id int64) (db.Character, error) {
	return p.queries.GetCharacter(ctx, id)
}

// LoadCharactersByAccountID loads all characters for an account
func (p *Postgres) LoadCharactersByAccountID(ctx context.Context, accountID int64) ([]db.Character, error) {
	return p.queries.GetCharactersByAccountID(ctx, accountID)
}

// LoadChunk loads a chunk by region, x, y, and layer
func (p *Postgres) LoadChunk(ctx context.Context, region, x, y, layer int32) (db.Chunk, error) {
	return p.queries.GetChunk(ctx, db.GetChunkParams{
		Region: region,
		X:      x,
		Y:      y,
		Layer:  layer,
	})
}

// LoadChunksByRegion loads all grids for a region
func (p *Postgres) LoadChunksByRegion(ctx context.Context, region int32) ([]db.Chunk, error) {
	return p.queries.GetChunksByRegion(ctx, region)
}

// LoadInventoryItem loads a single inventory item by ID
func (p *Postgres) LoadInventoryItem(ctx context.Context, id int64) (db.Inventory, error) {
	return p.queries.GetInventoryItem(ctx, id)
}

// LoadInventoryByParentID loads all inventory items for a parent (e.g., character ID)
func (p *Postgres) LoadInventoryByParentID(ctx context.Context, parentID int64) ([]db.Inventory, error) {
	return p.queries.GetInventoryByParentID(ctx, parentID)
}

// GetGlobalVar loads a global variable by name
func (p *Postgres) GetGlobalVar(ctx context.Context, name string) (db.GlobalVar, error) {
	return p.queries.GetGlobalVar(ctx, name)
}

func (p *Postgres) GetGlobalVarLong(ctx context.Context, name string) int64 {
	globalVar, err := p.GetGlobalVar(ctx, name)
	var result int64 = 0
	if err != nil {
		return 0
	} else if globalVar.ValueLong.Valid {
		result = globalVar.ValueLong.Int64
	}
	return result
}

// SetGlobalVarLong sets a global variable long value
func (p *Postgres) SetGlobalVarLong(ctx context.Context, name string, value int64) error {
	return p.queries.UpsertGlobalVarLong(ctx, db.UpsertGlobalVarLongParams{
		Name:      name,
		ValueLong: sql.NullInt64{Int64: value, Valid: true},
	})
}

// GetCharacterByToken loads a character by auth token
func (p *Postgres) GetCharacterByToken(ctx context.Context, token string) (db.Character, error) {
	return p.queries.GetCharacterByToken(ctx, sql.NullString{String: token, Valid: true})
}

// ClearAuthToken clears the auth token for a character
func (p *Postgres) ClearAuthToken(ctx context.Context, characterID int64) error {
	return p.queries.ClearAuthToken(ctx, characterID)
}

// Queries returns the underlying db.Queries for direct access
func (p *Postgres) Queries() *db.Queries {
	return p.queries
}
