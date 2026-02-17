package timeutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
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
	InitialTick         uint64
	RuntimeSecondsTotal int64
}

func (m *ServerTimeManager) LoadOrInitBootstrap() (ServerTimeBootstrap, error) {
	if m.db == nil {
		return ServerTimeBootstrap{
			InitialTick:         0,
			RuntimeSecondsTotal: 0,
		}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tickValue, tickFound, err := m.loadLongGlobalVar(ctx, constt.SERVER_TICK_TOTAL)
	if err != nil {
		return ServerTimeBootstrap{}, err
	}
	runtimeValue, runtimeFound, err := m.loadLongGlobalVar(ctx, constt.SERVER_RUNTIME_SECONDS_TOTAL)
	if err != nil {
		return ServerTimeBootstrap{}, err
	}

	if tickFound && tickValue < 0 {
		return ServerTimeBootstrap{}, fmt.Errorf("invalid %s=%d", constt.SERVER_TICK_TOTAL, tickValue)
	}
	if runtimeFound && runtimeValue < 0 {
		return ServerTimeBootstrap{}, fmt.Errorf("invalid %s=%d", constt.SERVER_RUNTIME_SECONDS_TOTAL, runtimeValue)
	}

	bootstrap := ServerTimeBootstrap{
		InitialTick:         0,
		RuntimeSecondsTotal: 0,
	}
	if tickFound {
		bootstrap.InitialTick = uint64(tickValue)
	}
	if runtimeFound {
		bootstrap.RuntimeSecondsTotal = runtimeValue
	}

	if tickFound && runtimeFound {
		m.logger.Info("Loaded server time bootstrap",
			zap.Uint64("initial_tick", bootstrap.InitialTick),
			zap.Int64("runtime_seconds_total", bootstrap.RuntimeSecondsTotal),
		)
		return bootstrap, nil
	}

	if err := m.persistState(ctx, bootstrap); err != nil {
		return ServerTimeBootstrap{}, err
	}

	if !tickFound && !runtimeFound {
		m.logger.Info("Initialized server time globals",
			zap.Uint64("initial_tick", bootstrap.InitialTick),
			zap.Int64("runtime_seconds_total", bootstrap.RuntimeSecondsTotal),
		)
	} else {
		m.logger.Info("Recovered missing server time globals",
			zap.Bool("tick_found", tickFound),
			zap.Bool("runtime_found", runtimeFound),
			zap.Uint64("initial_tick", bootstrap.InitialTick),
			zap.Int64("runtime_seconds_total", bootstrap.RuntimeSecondsTotal),
		)
	}

	return bootstrap, nil
}

func (m *ServerTimeManager) PersistState(ctx context.Context, state ServerTimeBootstrap) error {
	if state.RuntimeSecondsTotal < 0 {
		return fmt.Errorf("invalid runtime seconds total=%d", state.RuntimeSecondsTotal)
	}
	if state.InitialTick > uint64(math.MaxInt64) {
		return fmt.Errorf("initial tick overflow for int64: %d", state.InitialTick)
	}

	if m.db == nil {
		return nil
	}

	return m.persistState(ctx, state)
}

func (m *ServerTimeManager) persistState(ctx context.Context, state ServerTimeBootstrap) error {
	return m.db.WithTx(ctx, func(queries *repository.Queries) error {
		if err := queries.UpsertGlobalVarLong(ctx, repository.UpsertGlobalVarLongParams{
			Name:      constt.SERVER_TICK_TOTAL,
			ValueLong: sql.NullInt64{Int64: int64(state.InitialTick), Valid: true},
		}); err != nil {
			return fmt.Errorf("persist %s: %w", constt.SERVER_TICK_TOTAL, err)
		}
		if err := queries.UpsertGlobalVarLong(ctx, repository.UpsertGlobalVarLongParams{
			Name:      constt.SERVER_RUNTIME_SECONDS_TOTAL,
			ValueLong: sql.NullInt64{Int64: state.RuntimeSecondsTotal, Valid: true},
		}); err != nil {
			return fmt.Errorf("persist %s: %w", constt.SERVER_RUNTIME_SECONDS_TOTAL, err)
		}
		return nil
	})
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
