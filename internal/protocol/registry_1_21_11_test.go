package protocol

import (
	"slices"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestGenericContainerMenuRegistryIDs12111(t *testing.T) {
	actual := [...]int32{
		MenuGeneric9x1,
		MenuGeneric9x2,
		MenuGeneric9x3,
		MenuGeneric9x4,
		MenuGeneric9x5,
		MenuGeneric9x6,
	}

	for index, registryID := range actual {
		if registryID != int32(index) {
			t.Fatalf("generic 9x%d registry ID = %d, want %d", index+1, registryID, index)
		}

		resolved, valid := Generic9xMenuType(index + 1)
		if !valid || resolved != registryID {
			t.Fatalf("generic 9x%d resolved registry ID = %d, valid %v", index+1, resolved, valid)
		}
	}

	_, validZeroRows := Generic9xMenuType(0)

	if validZeroRows {
		t.Fatal("zero-row generic menu type is valid")
	}

	_, validSevenRows := Generic9xMenuType(7)

	if validSevenRows {
		t.Fatal("seven-row generic menu type is valid")
	}
}

func TestCraftingMenuRegistryID12111(t *testing.T) {
	if MenuCrafting != 12 {
		t.Fatalf("crafting menu registry ID = %d, want 12", MenuCrafting)
	}
}

func TestConfigurationBlockMiningTags12111(t *testing.T) {
	var blockTags []RegistryTag

	for _, registry := range ConfigurationTags {
		if registry.RegistryID == "minecraft:block" {
			blockTags = registry.Tags

			break
		}
	}

	if len(blockTags) == 0 {
		t.Fatal("configuration block tags are missing")
	}

	var pickaxeEntries []int32

	incorrectNetheriteFound := false

	for _, tag := range blockTags {
		switch tag.ID {
		case "minecraft:mineable/pickaxe":
			pickaxeEntries = tag.Entries
		case "minecraft:incorrect_for_netherite_tool":
			incorrectNetheriteFound = true

			if len(tag.Entries) != 0 {
				t.Fatalf("netherite incorrect-tool entries = %v, want none", tag.Entries)
			}
		}
	}

	stoneBricksID := int32(game.StoneBricksID)
	if !slices.Contains(pickaxeEntries, stoneBricksID) {
		t.Fatalf("pickaxe tag does not contain stone bricks block ID %d", stoneBricksID)
	}

	if !incorrectNetheriteFound {
		t.Fatal("netherite incorrect-tool tag is missing")
	}
}
