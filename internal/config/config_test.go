package config

import (
	"math"
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestConfigDefaults(t *testing.T) {
	var cfg Config

	cfg.SetDefaults()

	if cfg.World.Generator != "spawn-platform" {
		t.Fatalf("world generator = %q, want spawn-platform", cfg.World.Generator)
	}

	if cfg.RenderDistance() != DefaultRenderDistance {
		t.Fatalf("render distance = %d, want %d", cfg.RenderDistance(), DefaultRenderDistance)
	}

	if cfg.GameMode() != game.GameModeCreative {
		t.Fatalf("game mode = %d, want creative", cfg.GameMode())
	}

	if !cfg.AllowBlockBreaking() {
		t.Fatal("block breaking is disabled by default")
	}

	if cfg.World.Spawn != nil {
		t.Fatalf("spawn = %+v, want omitted", cfg.World.Spawn)
	}
}

func TestConfigDecodesSectionsAndOptionalSpawn(t *testing.T) {
	input := `
[server]
max-players = 12
render-distance = 18
default-game-mode = "spectator"
allow-block-breaking = false

[world]
generator = "wave-terrain"
seed = -42
spawn = { x = 12.5, y = 80.0, z = -7.5 }
`

	cfg, err := decodeConfig(strings.NewReader(input))
	if err != nil {
		t.Fatalf("decode config: %v", err)
	}

	if cfg.World.Seed != -42 || cfg.World.Spawn == nil || cfg.World.Spawn.Z != -7.5 {
		t.Fatalf("world config = %+v", cfg.World)
	}

	if cfg.GameMode() != game.GameModeSpectator {
		t.Fatalf("game mode = %d, want spectator", cfg.GameMode())
	}

	if cfg.AllowBlockBreaking() {
		t.Fatal("block breaking is enabled, want disabled")
	}
}

func TestConfigRejectsInvalidGameMode(t *testing.T) {
	cfg := Config{Server: ServerConfig{MaxPlayers: new(1), DefaultGameMode: "builder"}}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "builder") {
		t.Fatalf("validation error = %v, want invalid game mode", err)
	}
}

func TestRenderDistanceIsClamped(t *testing.T) {
	for configured, expected := range map[int32]int32{
		-10: int32(MinRenderDistance),
		0:   int32(MinRenderDistance),
		1:   int32(MinRenderDistance),
		64:  int32(MaxRenderDistance),
	} {
		cfg := Config{Server: ServerConfig{RenderDistance: &configured}}

		cfg.SetDefaults()

		if cfg.RenderDistance() != expected {
			t.Errorf("render distance %d became %d, want %d", configured, cfg.RenderDistance(), expected)
		}
	}
}

func TestExplicitZeroMaxPlayersIsRejected(t *testing.T) {
	_, err := decodeConfig(strings.NewReader("[server]\nmax-players = 0"))
	if err == nil || !strings.Contains(err.Error(), "max-players") {
		t.Fatalf("decode error = %v, want max-players validation error", err)
	}
}

func TestLegacyFlatConfigIsRejected(t *testing.T) {
	_, err := decodeConfig(strings.NewReader("hostname = 'example.org'\nmax-players = 10"))
	if err == nil {
		t.Fatal("legacy flat config decoded without an error")
	}
}

func TestMaxPlayersRejectsProtocolOverflow(t *testing.T) {
	input := "[server]\nmax-players = 2147483648"

	_, err := decodeConfig(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "must not exceed") {
		t.Fatalf("decode error = %v, want upper-bound validation error", err)
	}
}

func TestSpawnCoordinatesMustBeFinite(t *testing.T) {
	for name, value := range map[string]float64{
		"nan":      math.NaN(),
		"positive": math.Inf(1),
		"negative": math.Inf(-1),
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Config{
				Server: ServerConfig{MaxPlayers: new(1), DefaultGameMode: "creative"},
				World:  WorldConfig{Spawn: &SpawnConfig{X: value}},
			}

			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), "finite") {
				t.Fatalf("validation error = %v, want finite-coordinate error", err)
			}
		})
	}
}
