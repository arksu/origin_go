package config

import (
	"errors"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Game     GameConfig     `mapstructure:"game"`
	EntityID EntityIDConfig `mapstructure:"entity_id"`
	Network  NetworkConfig  `mapstructure:"network"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	Schema          string        `mapstructure:"schema"`
	MaxConns        int32         `mapstructure:"max_conns"`
	MinConns        int32         `mapstructure:"min_conns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `mapstructure:"max_conn_idle_time"`
}

type GameConfig struct {
	Env                      string        `mapstructure:"env"`
	TickRate                 int           `mapstructure:"tick_rate"`
	PlayerActiveChunkRadius  int           `mapstructure:"player_active_chunk_radius"`
	PlayerPreloadChunkRadius int           `mapstructure:"player_preload_chunk_radius"`
	PlayerSaveInterval       time.Duration `mapstructure:"player_save_interval"`
	Region                   int           `mapstructure:"region"`
	MaxLayers                int           `mapstructure:"max_layers"`
	DisconnectDelay          int           `mapstructure:"disconnect_delay"`
	ChunkLRUCapacity         int           `mapstructure:"chunk_lru_capacity"`
	ChunkLRUTTL              int           `mapstructure:"chunk_lru_ttl"`
	MaxEntities              int           `mapstructure:"max_entities"`
	EventBusMinWorkers       int           `mapstructure:"event_bus_min_workers"`
	EventBusMaxWorkers       int           `mapstructure:"event_bus_max_workers"`
	WorkerPoolSize           int           `mapstructure:"worker_pool_size"`
	LoadWorkers              int           `mapstructure:"load_workers"`
	SaveWorkers              int           `mapstructure:"save_workers"`
	SpawnTimeout             time.Duration `mapstructure:"spawn_timeout"`
	NearSpawnRadius          int           `mapstructure:"near_spawn_radius"`
	NearSpawnTries           int           `mapstructure:"near_spawn_tries"`
	RandomSpawnTries         int           `mapstructure:"random_spawn_tries"`
	WorldMinXChunks          int           `mapstructure:"world_min_x_chunks"`
	WorldMinYChunks          int           `mapstructure:"world_min_y_chunks"`
	WorldWidthChunks         int           `mapstructure:"world_width_chunks"`
	WorldHeightChunks        int           `mapstructure:"world_height_chunks"`
	WorldMarginTiles         int           `mapstructure:"world_margin_tiles"`
	SendChannelBuffer        int           `mapstructure:"send_channel_buffer"`
	PprofEnabled             bool          `mapstructure:"pprof_enabled"`

	// Command queue settings
	CommandQueueSize            int `mapstructure:"command_queue_size"`               // Max commands in queue (default: 500)
	MaxPacketsPerSecond         int `mapstructure:"max_packets_per_second"`           // Per-client rate limit (default: 40)
	MaxCommandsPerTickPerClient int `mapstructure:"max_commands_per_tick_per_client"` // Fairness limit (default: 20)

	// Chat settings
	ChatLocalRadius   int `mapstructure:"chat_local_radius"`    // Radius for local chat (default: 1000)
	ChatMaxLen        int `mapstructure:"chat_max_len"`         // Max chat message length (default: 256)
	ChatMinIntervalMs int `mapstructure:"chat_min_interval_ms"` // Min interval between messages in ms (default: 400)

	ObjectBehaviorBudgetPerTick int `mapstructure:"object_behavior_budget_per_tick"` // Max dirty behavior objects processed per tick (default: 512)
}

type EntityIDConfig struct {
	RangeSize int `mapstructure:"range_size"`
}

type NetworkConfig struct {
	ReadBufferSize  int           `mapstructure:"read_buffer_size"`
	WriteBufferSize int           `mapstructure:"write_buffer_size"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	MaxMessageSize  int           `mapstructure:"max_message_size"`
}

func Load(logger *zap.Logger) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, err
		}
		logger.Info("Config file not found, using defaults and environment variables")
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	cfg.Game.Env = strings.ToLower(strings.TrimSpace(cfg.Game.Env))
	switch cfg.Game.Env {
	case "dev", "stage", "prod":
	default:
		logger.Fatal("Invalid game.env value (allowed: dev|stage|prod)",
			zap.String("game.env", cfg.Game.Env),
		)
	}

	// Validate critical game config values
	if cfg.Game.WorldWidthChunks <= 0 || cfg.Game.WorldHeightChunks <= 0 {
		logger.Fatal("Invalid world dimensions: world_width_chunks and world_height_chunks must be > 0",
			zap.Int("world_width_chunks", cfg.Game.WorldWidthChunks),
			zap.Int("world_height_chunks", cfg.Game.WorldHeightChunks),
		)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5430)
	v.SetDefault("database.user", "origin")
	v.SetDefault("database.password", "origin")
	v.SetDefault("database.database", "origin")
	v.SetDefault("database.schema", "origin")
	v.SetDefault("database.max_conns", 50)
	v.SetDefault("database.min_conns", 20)
	v.SetDefault("database.max_conn_lifetime", 30*time.Minute)
	v.SetDefault("database.max_conn_idle_time", 5*time.Minute)

	// Game defaults
	v.SetDefault("game.env", "dev")
	v.SetDefault("game.tick_rate", 10)
	v.SetDefault("game.player_active_chunk_radius", 1)
	v.SetDefault("game.player_preload_chunk_radius", 2)
	v.SetDefault("game.player_save_interval", 30*time.Second)
	v.SetDefault("game.region", 1)
	v.SetDefault("game.max_layers", 3)
	v.SetDefault("game.disconnect_delay", 3)
	v.SetDefault("game.chunk_lru_capacity", 2000)
	v.SetDefault("game.chunk_lru_ttl", 20)
	v.SetDefault("game.max_entities", 1048576)
	v.SetDefault("game.event_bus_min_workers", 32)
	v.SetDefault("game.event_bus_max_workers", 64)
	v.SetDefault("game.worker_pool_size", 4)
	v.SetDefault("game.load_workers", 48)
	v.SetDefault("game.save_workers", 10)
	v.SetDefault("game.spawn_timeout", 30*time.Second)
	v.SetDefault("game.near_spawn_radius", 20)
	v.SetDefault("game.near_spawn_tries", 5)
	v.SetDefault("game.random_spawn_tries", 5)
	v.SetDefault("game.world_min_x_chunks", 0)
	v.SetDefault("game.world_min_y_chunks", 0)
	v.SetDefault("game.world_width_chunks", 50)
	v.SetDefault("game.world_height_chunks", 50)
	v.SetDefault("game.world_margin_tiles", 50)
	v.SetDefault("game.send_channel_buffer", 132000)
	v.SetDefault("game.pprof_enabled", false)
	v.SetDefault("game.command_queue_size", 2000)
	v.SetDefault("game.max_packets_per_second", 40)
	v.SetDefault("game.max_commands_per_tick_per_client", 20)
	v.SetDefault("game.chat_local_radius", 1000)
	v.SetDefault("game.chat_max_len", 256)
	v.SetDefault("game.chat_min_interval_ms", 400)
	v.SetDefault("game.object_behavior_budget_per_tick", 512)

	// EntityID defaults
	v.SetDefault("entity_id.range_size", 1000)

	// Network defaults
	v.SetDefault("network.read_buffer_size", 4096)
	v.SetDefault("network.write_buffer_size", 4096)
	v.SetDefault("network.read_timeout", 60*time.Second)
	v.SetDefault("network.write_timeout", 10*time.Second)
	v.SetDefault("network.max_message_size", 65536)
}

func MustLoad(logger *zap.Logger) *Config {
	cfg, err := Load(logger)
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}
	return cfg
}
