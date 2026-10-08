// Package testutil supplies isolated PostgreSQL schemas for integration tests.
package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/persistence"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// NewPostgres creates a fresh schema and removes only that schema at cleanup.
// dsnEnv must point to a disposable database; schemaSQLPath is caller-relative.
func NewPostgres(t testing.TB, dsnEnv, schemaSQLPath string) *persistence.Postgres {
	t.Helper()
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("set %s to run PostgreSQL integration tests", dsnEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	admin, err := pgx.ConnectConfig(ctx, cfg)
	require.NoError(t, err)
	schema := fmt.Sprintf("object_health_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, dropErr := admin.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+quoted+" CASCADE")
		require.NoError(t, dropErr)
		require.NoError(t, admin.Close(cleanupCtx))
	})
	schemaSQL, err := os.ReadFile(schemaSQLPath)
	require.NoError(t, err)
	fixtureSQL := strings.Replace(string(schemaSQL), "create schema if not exists origin;", "create schema "+quoted+";", 1)
	fixtureSQL = strings.Replace(fixtureSQL, "set search_path to origin;", "set search_path to "+quoted+";", 1)
	_, err = admin.Exec(ctx, fixtureSQL)
	require.NoError(t, err)
	db, err := persistence.NewPostgres(ctx, &config.DatabaseConfig{
		Host: cfg.Host, Port: int(cfg.Port), User: cfg.User, Password: cfg.Password,
		Database: cfg.Database, Schema: schema, MaxConns: 4,
	}, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(db.Close)
	return db
}
