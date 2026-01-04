package persistence

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/persistence/repository"
)

type Postgres struct {
	pool    *pgxpool.Pool
	db      *sql.DB
	queries *repository.Queries
	logger  *zap.Logger
}

func NewPostgres(ctx context.Context, cfg *config.DatabaseConfig, logger *zap.Logger) (*Postgres, error) {
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?search_path=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.Schema,
	)

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	queries := repository.New(db)

	logger.Info("Connected to PostgreSQL",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database))

	return &Postgres{
		pool:    pool,
		db:      db,
		queries: queries,
		logger:  logger,
	}, nil
}

func (p *Postgres) Close() {
	if p.pool != nil {
		p.pool.Close()
		p.logger.Info("PostgreSQL connection pool closed")
	}
}

func (p *Postgres) Pool() *pgxpool.Pool {
	return p.pool
}

func (p *Postgres) Queries() *repository.Queries {
	return p.queries
}

func (p *Postgres) GetGlobalVarLong(ctx context.Context, name string) int64 {
	gv, err := p.queries.GetGlobalVar(ctx, name)
	if err != nil {
		return 0
	}
	if gv.ValueLong.Valid {
		return gv.ValueLong.Int64
	}
	return 0
}

func (p *Postgres) SetGlobalVarLong(ctx context.Context, name string, value int64) error {
	return p.queries.UpsertGlobalVarLong(ctx, repository.UpsertGlobalVarLongParams{
		Name:      name,
		ValueLong: sql.NullInt64{Int64: value, Valid: true},
	})
}

func (p *Postgres) WithTx(ctx context.Context, fn func(*repository.Queries) error) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := p.queries.WithTx(tx)
	if err := fn(qtx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
