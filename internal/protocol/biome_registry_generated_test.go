package protocol

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestBiomeRegistryEntries(t *testing.T) {
	if len(biomeRegistryEntries) != int(game.BiomeCount) {
		t.Fatalf("len(biomeRegistryEntries) = %d, want %d", len(biomeRegistryEntries), game.BiomeCount)
	}

	for id, name := range game.BiomeNames {
		want := "minecraft:" + name
		if biomeRegistryEntries[id] != want {
			t.Errorf("biome registry entry %d = %q, want %q", id, biomeRegistryEntries[id], want)
		}
	}
}

func TestConfigurationBiomeRegistryUsesGeneratedEntries(t *testing.T) {
	for _, registry := range ConfigurationRegistries {
		if registry.ID == "minecraft:worldgen/biome" {
			if len(registry.Entries) != len(biomeRegistryEntries) {
				t.Fatalf("biome registry length = %d, want %d", len(registry.Entries), len(biomeRegistryEntries))
			}

			if &registry.Entries[0] != &biomeRegistryEntries[0] {
				t.Fatal("configuration biome registry does not reference generated entries")
			}

			return
		}
	}

	t.Fatal("configuration biome registry is missing")
}
