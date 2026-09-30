package server

import (
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestNaturalSpawnSourceSupportOverrides(t *testing.T) {
	expected := map[string]bool{
		"soul_sand":      true,
		"mud":            true,
		"carved_pumpkin": true,
		"jack_o_lantern": true,
		"redstone_lamp":  true,
		"bedrock":        false,
		"glass":          false,
		"tinted_glass":   false,
		"ice":            false,
		"frosted_ice":    false,
		"magma_block":    false,
		"barrier":        false,
		"scaffolding":    false,
	}
	checked := make(map[string]int, len(expected)+64)

	for block := game.Block(0); block <= game.MaxBlockState; block++ {
		definition, _ := block.Definition()

		want, explicit := expected[definition.Name]
		family := strings.HasSuffix(definition.Name, "_leaves") || strings.HasSuffix(definition.Name, "_trapdoor") || strings.HasSuffix(definition.Name, "_stained_glass") || strings.HasSuffix(definition.Name, "copper_grate")

		if !explicit && !family {
			continue
		}

		if naturalSpawnBlocks[block].Support != want {
			t.Fatalf("%s state %d support = %t, want %t", definition.Name, block, naturalSpawnBlocks[block].Support, want)
		}

		checked[definition.Name]++
	}

	for name := range expected {
		if checked[name] == 0 {
			t.Fatalf("support exception %s was not checked", name)
		}
	}

	if len(checked) < len(expected)+40 {
		t.Fatalf("support family coverage only reached %d block kinds", len(checked))
	}
}

func TestNaturalSpawnSourceSignalOverrides(t *testing.T) {
	names := []string{"daylight_detector", "detector_rail", "repeater", "comparator", "jukebox", "lectern", "lever", "observer", "redstone_block", "redstone_torch", "redstone_wall_torch", "redstone_wire", "sculk_sensor", "calibrated_sculk_sensor", "target", "trapped_chest", "tripwire_hook"}

	for _, name := range names {
		if !naturalSignalSource(name) {
			t.Fatalf("source isSignalSource override missing for %s", name)
		}
	}

	for block := game.Block(0); block <= game.MaxBlockState; block++ {
		definition, _ := block.Definition()
		family := strings.HasSuffix(definition.Name, "_button") || strings.HasSuffix(definition.Name, "_pressure_plate") || strings.HasSuffix(definition.Name, "lightning_rod")

		if (naturalSignalSource(definition.Name) || family) && naturalSpawnBlocks[block].Empty {
			t.Fatalf("signal source %s state %d allowed as empty", definition.Name, block)
		}
	}

	if naturalSignalSource("rail") {
		t.Fatal("ordinary rail incorrectly treated as a signal source")
	}

	if naturalSpawnBlocks[game.Rail].Empty {
		t.Fatal("ordinary rail must still be excluded by prevent_mob_spawning_inside")
	}
}

func TestNaturalSpawnSourceHazards(t *testing.T) {
	names := []string{"fire", "soul_fire", "lava", "lava_cauldron", "magma_block", "wither_rose", "sweet_berry_bush", "cactus", "powder_snow"}

	for _, name := range names {
		block, exists := game.BlockByName(name)
		if !exists || naturalSpawnBlocks[block].Empty {
			t.Fatalf("source hazard %s allowed as empty", name)
		}
	}

	for block := game.Block(0); block <= game.MaxBlockState; block++ {
		definition, _ := block.Definition()
		if definition.Name != "campfire" && definition.Name != "soul_campfire" {
			continue
		}

		lit, _ := block.Property("lit")
		waterlogged, _ := block.Property("waterlogged")

		want := lit == "false" && waterlogged == "false"

		var storage [1]game.AABB

		boxes := block.AppendCollisionBoxes(storage[:0], game.BlockPosition{})
		if len(boxes) != 1 || boxes[0] != (game.AABB{MaxX: 1, MaxY: 0.4375, MaxZ: 1}) {
			t.Fatalf("campfire collision differs from source: %+v", boxes)
		}

		if naturalSpawnBlocks[block].Empty != want {
			t.Fatalf("%s state %d (lit=%s, waterlogged=%s) empty=%t, want %t", definition.Name, block, lit, waterlogged, naturalSpawnBlocks[block].Empty, want)
		}
	}
}

func TestNaturalSpawnCommittedTagFlags(t *testing.T) {
	checked := 0

	for _, registry := range protocol.ConfigurationTags {
		if registry.RegistryID != "minecraft:block" {
			continue
		}

		for _, tag := range registry.Tags {
			for _, identifier := range tag.Entries {
				definition, _ := game.BlockByID(game.BlockID(identifier))

				for block := definition.MinState; block <= definition.MaxState; block++ {
					flags := naturalSpawnBlocks[block]

					switch tag.ID {
					case "minecraft:animals_spawnable_on":
						if !flags.Animals {
							t.Fatalf("animal tag missing for state %d", block)
						}
					case "minecraft:bats_spawnable_on":
						if !flags.Bats {
							t.Fatalf("bat tag missing for state %d", block)
						}
					case "minecraft:prevent_mob_spawning_inside", "minecraft:buttons", "minecraft:pressure_plates", "minecraft:lightning_rods", "minecraft:fire":
						if flags.Empty {
							t.Fatalf("%s state %d incorrectly empty", tag.ID, block)
						}
					case "minecraft:leaves", "minecraft:trapdoors":
						if flags.Support {
							t.Fatalf("%s state %d incorrectly supports spawning", tag.ID, block)
						}
					default:
						continue
					}

					checked++
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no committed block-tag states checked")
	}
}
