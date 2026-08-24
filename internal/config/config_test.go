package config

import "testing"

func TestWorldGeneratorDefault(t *testing.T) {
	var cfg Config

	cfg.SetDefaults()

	if cfg.WorldGenerator != "spawn-platform" {
		t.Fatalf("world generator = %q, want spawn-platform", cfg.WorldGenerator)
	}
}
