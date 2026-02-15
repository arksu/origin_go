package timeutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	constt "origin/internal/const"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
)

type ServerTimeManager struct {
	db     *persistence.Postgres
	logger *zap.Logger
}

func NewServerTimeManager(db *persistence.Postgres, logger *zap.Logger) *ServerTimeManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ServerTimeManager{
		db:     db,
		logger: logger,
	}
}

type ServerTimeBootstrap struct {
	ServerStartTime time.Time
	InitialTick     uint64
}

func (m *ServerTimeManager) LoadOrInitBootstrap(now time.Time, tickRate int) (ServerTimeBootstrap, error) {
	if tickRate <= 0 {
		return ServerTimeBootstrap{}, fmt.Errorf("tick rate must be > 0")
	}
	if m.db == nil {
		return ServerTimeBootstrap{
			ServerStartTime: now,
			InitialTick:     0,
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	startValue, startFound, err := m.loadLongGlobalVar(ctx, constt.SERVER_START_TIME)
	if err != nil {
		return ServerTimeBootstrap{}, err
	}
	storedTickRate, tickRateFound, err := m.loadLongGlobalVar(ctx, constt.SERVER_TICK_RATE)
	if err != nil {
		return ServerTimeBootstrap{}, err
	}

	switch {
	case !startFound && !tickRateFound:
		if err := m.db.WithTx(ctx, func(queries *repository.Queries) error {
			if err := queries.UpsertGlobalVarLong(ctx, repository.UpsertGlobalVarLongParams{
				Name:      constt.SERVER_START_TIME,
				ValueLong: sql.NullInt64{Int64: now.Unix(), Valid: true},
			}); err != nil {
				return fmt.Errorf("persist %s: %w", constt.SERVER_START_TIME, err)
			}
			if err := queries.UpsertGlobalVarLong(ctx, repository.UpsertGlobalVarLongParams{
				Name:      constt.SERVER_TICK_RATE,
				ValueLong: sql.NullInt64{Int64: int64(tickRate), Valid: true},
			}); err != nil {
				return fmt.Errorf("persist %s: %w", constt.SERVER_TICK_RATE, err)
			}
			return nil
		}); err != nil {
			return ServerTimeBootstrap{}, err
		}
		m.logger.Info("Initialized server time bootstrap",
			zap.Int64("server_start_time_sec", now.Unix()),
			zap.Int("tick_rate", tickRate),
		)
		return ServerTimeBootstrap{
			ServerStartTime: time.Unix(now.Unix(), 0),
			InitialTick:     0,
		}, nil
	case startFound != tickRateFound:
		return ServerTimeBootstrap{}, fmt.Errorf("invalid bootstrap globals: %s and %s must both exist",
			constt.SERVER_START_TIME, constt.SERVER_TICK_RATE)
	}

	if storedTickRate <= 0 {
		return ServerTimeBootstrap{}, fmt.Errorf("invalid %s=%d", constt.SERVER_TICK_RATE, storedTickRate)
	}
	if int(storedTickRate) != tickRate {
		return ServerTimeBootstrap{}, fmt.Errorf("tick rate mismatch: config=%d persisted=%d", tickRate, storedTickRate)
	}
	if startValue <= 0 {
		return ServerTimeBootstrap{}, fmt.Errorf("invalid %s=%d", constt.SERVER_START_TIME, startValue)
	}

	serverStartTime := time.Unix(startValue, 0)
	if serverStartTime.After(now) {
		return ServerTimeBootstrap{}, fmt.Errorf("%s is in the future: %s", constt.SERVER_START_TIME, serverStartTime.UTC().Format(time.RFC3339))
	}

	tickPeriod := time.Second / time.Duration(tickRate)
	elapsed := now.Sub(serverStartTime)
	initialTick := uint64(elapsed / tickPeriod)
	clockNow := serverStartTime.Add(time.Duration(initialTick) * tickPeriod)

	m.logger.Info("Loaded server time bootstrap",
		zap.Int64("server_start_time_sec", startValue),
		zap.Int64("elapsed_seconds", int64(elapsed.Seconds())),
		zap.Int("tick_rate", tickRate),
		zap.Uint64("initial_tick", initialTick),
		zap.Time("clock_now", clockNow),
	)

	return ServerTimeBootstrap{
		ServerStartTime: serverStartTime,
		InitialTick:     initialTick,
	}, nil
}

func (m *ServerTimeManager) loadLongGlobalVar(ctx context.Context, key string) (int64, bool, error) {
	if m.db == nil {
		return 0, false, nil
	}

	row, err := m.db.Queries().GetGlobalVar(ctx, key)
	switch {
	case err == nil:
		if !row.ValueLong.Valid {
			return 0, false, nil
		}
		return row.ValueLong.Int64, true, nil
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	default:
		return 0, false, fmt.Errorf("load %s: %w", key, err)
	}
}
