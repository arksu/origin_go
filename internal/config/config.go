package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Server
	ListenAddr         string
	RegionID           int
	RegionWidthChunks  int
	RegionHeightChunks int

	// Database
	PostgresURL       string
	DBSchema          string
	DBMaxConns        int
	DBMinConns        int
	DBMaxConnIdleTime time.Duration
	DBMaxConnLifetime time.Duration
	DBConnTimeout     time.Duration

	// Game settings
	TickRate       time.Duration
	WorldBorder    int
	MaxPlayers     int
	MaxBots        int
	ChunkLoadRange int // 3x3 around player

	// Performance
	TickBudget           time.Duration
	PositionSaveInterval time.Duration

	// Entity ID allocation
	EntityIdRangeSize int
}

func Load() *Config {
	return &Config{
		ListenAddr:           getEnv("LISTEN_ADDR", ":8080"),
		RegionID:             getIntEnv("REGION_ID", 1),
		RegionHeightChunks:   getIntEnv("REGION_HEIGHT", 100),
		RegionWidthChunks:    getIntEnv("REGION_WIDTH", 100),
		PostgresURL:          getEnv("DB_URL", "postgres://origingo:origingo@localhost:5432/origingo"),
		DBSchema:             getEnv("DB_SCHEMA", "origin"),
		DBMaxConns:           getIntEnv("DB_MAX_CONNS", 25),
		DBMinConns:           getIntEnv("DB_MIN_CONNS", 5),
		DBMaxConnIdleTime:    getDurationEnv("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
		DBMaxConnLifetime:    getDurationEnv("DB_MAX_CONN_LIFETIME", time.Hour),
		DBConnTimeout:        getDurationEnv("DB_CONN_TIMEOUT", 5*time.Second),
		WorldBorder:          getIntEnv("WORLD_BORDER", 20),
		MaxPlayers:           getIntEnv("MAX_PLAYERS", 1000),
		MaxBots:              getIntEnv("MAX_BOTS", 5000),
		ChunkLoadRange:       getIntEnv("CHUNK_LOAD_RANGE", 1),                 // 3x3 = range 1
		TickRate:             getDurationEnv("TICK_RATE", 33*time.Millisecond), // ~30 ticks/sec
		TickBudget:           getDurationEnv("TICK_BUDGET", 30*time.Millisecond),
		PositionSaveInterval: getDurationEnv("POSITION_SAVE_INTERVAL", time.Second),
		EntityIdRangeSize:    getIntEnv("ENTITY_ID_RANGE_SIZE", 1000),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
