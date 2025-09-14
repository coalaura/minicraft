package config

import (
	"fmt"
	"os"

	"github.com/coalaura/minicraft/crypto"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Key *crypto.KeyPair `toml:"-"`

	Hostname   string `toml:"hostname"`
	Port       uint   `toml:"port"`
	OnlineMode bool   `toml:"online-mode"`

	Motd       string `toml:"motd"`
	MaxPlayers int    `toml:"max-players"`

	CompressionThreshold int `toml:"compression-threshold"`
}

func LoadConfig() (*Config, error) {
	file, err := os.OpenFile("config.toml", os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	var cfg Config

	err = toml.NewDecoder(file).Decode(&cfg)
	if err != nil {
		return nil, err
	}

	key, err := crypto.CreateKeyPair()
	if err != nil {
		return nil, err
	}

	cfg.Key = key

	cfg.SetDefaults()

	return &cfg, nil
}

func (c *Config) SetDefaults() {
	if c.Hostname == "" {
		c.Hostname = "localhost"
	}

	if c.Port == 0 {
		c.Port = 25565
	}

	if c.MaxPlayers == 0 {
		c.MaxPlayers = 2
	}

	if c.CompressionThreshold < 32 {
		c.CompressionThreshold = 32
	}
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Hostname, c.Port)
}
