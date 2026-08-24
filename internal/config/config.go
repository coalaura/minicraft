package config

import (
	"fmt"
	"io"
	"math"
	"os"

	"github.com/coalaura/minicraft/internal/crypto"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/plain"
	"github.com/pelletier/go-toml/v2"
)

const (
	DefaultRenderDistance = 10
	MinRenderDistance     = 2
	MaxRenderDistance     = 32
)

type ServerConfig struct {
	Hostname        string `toml:"hostname"`
	Port            uint   `toml:"port"`
	OnlineMode      bool   `toml:"online-mode"`
	Motd            string `toml:"motd"`
	MaxPlayers      *int   `toml:"max-players"`
	RenderDistance  *int32 `toml:"render-distance"`
	DefaultGameMode string `toml:"default-game-mode"`
}

type WorldConfig struct {
	Generator string       `toml:"generator"`
	Seed      int64        `toml:"seed"`
	Spawn     *SpawnConfig `toml:"spawn"`
}

type SpawnConfig struct {
	X float64 `toml:"x"`
	Y float64 `toml:"y"`
	Z float64 `toml:"z"`
}

type NetworkConfig struct {
	CompressionThreshold int `toml:"compression-threshold"`
}

type LoggingConfig struct {
	Level string `toml:"level"`
}

type Config struct {
	Key *crypto.KeyPair `toml:"-"`

	Server  ServerConfig  `toml:"server"`
	World   WorldConfig   `toml:"world"`
	Network NetworkConfig `toml:"network"`
	Logging LoggingConfig `toml:"logging"`
}

func LoadConfig() (*Config, error) {
	file, err := os.OpenFile("config.toml", os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	cfg, err := decodeConfig(file)
	if err != nil {
		return nil, err
	}

	key, err := crypto.CreateKeyPair()
	if err != nil {
		return nil, err
	}

	cfg.Key = key

	return cfg, nil
}

func decodeConfig(reader io.Reader) (*Config, error) {
	var cfg Config

	err := toml.NewDecoder(reader).DisallowUnknownFields().Decode(&cfg)
	if err != nil {
		return nil, err
	}

	cfg.SetDefaults()

	err = cfg.Validate()
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) SetDefaults() {
	if c.Server.Hostname == "" {
		c.Server.Hostname = "localhost"
	}

	if c.Server.Port == 0 {
		c.Server.Port = 25565
	}

	if c.Logging.Level == "" {
		c.Logging.Level = "print"
	}

	if c.Server.MaxPlayers == nil {
		maxPlayers := 2
		c.Server.MaxPlayers = &maxPlayers
	}

	if c.Server.RenderDistance == nil {
		renderDistance := int32(DefaultRenderDistance)
		c.Server.RenderDistance = &renderDistance
	}

	*c.Server.RenderDistance = max(*c.Server.RenderDistance, int32(MinRenderDistance))
	*c.Server.RenderDistance = min(*c.Server.RenderDistance, int32(MaxRenderDistance))

	if c.Server.DefaultGameMode == "" {
		c.Server.DefaultGameMode = "creative"
	}

	if c.Network.CompressionThreshold < 32 {
		c.Network.CompressionThreshold = 32
	}

	if c.World.Generator == "" {
		c.World.Generator = "spawn-platform"
	}
}

func (c *Config) Validate() error {
	if c.Server.Port > 65535 {
		return fmt.Errorf("server port %d is out of range", c.Server.Port)
	}

	if c.Server.MaxPlayers == nil || *c.Server.MaxPlayers < 1 {
		return fmt.Errorf("max-players must be at least 1")
	}

	if *c.Server.MaxPlayers > math.MaxInt32 {
		return fmt.Errorf("max-players must not exceed %d", math.MaxInt32)
	}

	if c.World.Spawn != nil && (!isFinite(c.World.Spawn.X) || !isFinite(c.World.Spawn.Y) || !isFinite(c.World.Spawn.Z)) {
		return fmt.Errorf("world spawn coordinates must be finite")
	}

	_, err := game.ParseGameMode(c.Server.DefaultGameMode)
	if err != nil {
		return fmt.Errorf("default-game-mode: %w", err)
	}

	return nil
}

func (c *Config) MaxPlayers() int {
	if c.Server.MaxPlayers == nil {
		return 2
	}

	return *c.Server.MaxPlayers
}

func (c *Config) RenderDistance() int32 {
	if c.Server.RenderDistance == nil {
		return DefaultRenderDistance
	}

	return *c.Server.RenderDistance
}

func (c *Config) GameMode() game.GameMode {
	mode, _ := game.ParseGameMode(c.Server.DefaultGameMode)

	return mode
}

func (c *Config) GetLogLevel() plain.Level {
	switch c.Logging.Level {
	case "debug":
		return plain.LevelDebug
	case "warn":
		return plain.LevelWarn
	case "error":
		return plain.LevelError
	}

	return plain.LevelPrint
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Hostname, c.Server.Port)
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
