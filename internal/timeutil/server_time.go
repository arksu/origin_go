package timeutil

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.uber.org/zap"

	constt "origin/internal/const"
	"origin/internal/persistence"
)

// ServerTimeManager handles loading and persisting server time to/from database.
// It provides a stable start time for the monotonic clock across server restarts.
type ServerTimeManager struct {
	db                *persistence.Postgres
	logger            *zap.Logger
	lastSaveTime      time.Time
	savePeriodSeconds int64
}

// NewServerTimeManager creates a new server time manager.
func NewServerTimeManager(db *persistence.Postgres, logger *zap.Logger) *ServerTimeManager {
	return &ServerTimeManager{
		db:                db,
		logger:            logger,
		savePeriodSeconds: constt.ServerTimeSavePeriodSeconds,
	}
}

// LoadServerTime loads the server time from database.
// Returns the loaded time if found and valid, otherwise returns the current time.
func (m *ServerTimeManager) LoadServerTime() time.Time {
	if m.db == nil {
		return time.Now()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gv, err := m.db.Queries().GetGlobalVar(ctx, constt.SERVER_TIME)
	switch {
	case err == nil && gv.ValueLong.Valid && gv.ValueLong.Int64 > 0:
		loadedTime := time.Unix(gv.ValueLong.Int64, 0)
		m.logger.Info("Loaded SERVER_TIME from DB",
			zap.Int64("server_time_sec", gv.ValueLong.Int64),
			zap.Time("server_time", loadedTime),
		)
		return loadedTime
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		m.logger.Warn("Failed to load SERVER_TIME from DB, using current time",
			zap.Error(err),
		)
	default:
		m.logger.Info("SERVER_TIME not found in DB, using current time")
	}

	return time.Now()
}

// MaybePersistServerTime saves the server time to database if enough time has passed since last save.
// This is called periodically during game loop to keep the server time persisted.
func (m *ServerTimeManager) MaybePersistServerTime(now time.Time) {
	if m.db == nil {
		return
	}
	if m.lastSaveTime.IsZero() {
		m.lastSaveTime = now
		return
	}

	if now.Sub(m.lastSaveTime) < time.Duration(m.savePeriodSeconds)*time.Second {
		return
	}

	m.lastSaveTime = now

	go func(t time.Time) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := m.SaveServerTime(ctx, t); err != nil {
			m.logger.Error("Failed to persist SERVER_TIME (periodic)", zap.Int64("server_time_sec", t.Unix()), zap.Error(err))
		} else {
			//m.logger.Info("Persisted SERVER_TIME (periodic)", zap.Int64("server_time_sec", t.Unix()))
		}
	}(now)
}

// SaveServerTime saves the given server time to database immediately.
// This is called during server shutdown to ensure the current time is persisted.
func (m *ServerTimeManager) SaveServerTime(ctx context.Context, t time.Time) error {
	if m.db == nil {
		return nil
	}
	return m.db.SetGlobalVarLong(ctx, constt.SERVER_TIME, t.Unix())
}

// SaveCurrentTime saves the current server time to database with logging.
// This is a convenience method for shutdown that handles context and logging.
func (m *ServerTimeManager) SaveCurrentTime(clock Clock) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverTime := clock.GameNow()
	if err := m.SaveServerTime(ctx, serverTime); err != nil {
		m.logger.Error("Failed to persist SERVER_TIME", zap.Int64("server_time_sec", serverTime.Unix()), zap.Error(err))
	} else {
		m.logger.Info("Persisted SERVER_TIME", zap.Int64("server_time_sec", serverTime.Unix()))
	}
}
