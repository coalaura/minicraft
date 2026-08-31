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

func TestEnchantmentRegistryOrder12111(t *testing.T) {
	for _, registry := range ConfigurationRegistries {
		if registry.ID != "minecraft:enchantment" {
			continue
		}

		if !slices.Equal(registry.Entries, generatedEnchantmentRegistryEntries) {
			t.Fatalf("enchantment registry entries = %v, want generated authority", registry.Entries)
		}

		for index, name := range generatedEnchantmentRegistryEntries {
			registryID, found := registry.EntryID(name)
			if !found || registryID != int32(index) {
				t.Fatalf("%s registry ID = %d, found %t; want %d, true", name, registryID, found, index)
			}

			definition, valid := game.Enchantment(index).Definition()
			if !valid || name != "minecraft:"+definition.Name {
				t.Fatalf("registry entry %d = %s, game definition = %+v, valid %t", index, name, definition, valid)
			}
		}

		return
	}

	t.Fatal("enchantment registry is missing")
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

func TestConfigurationFluidTags12111(t *testing.T) {
	var fluidTags []RegistryTag

	for _, registry := range ConfigurationTags {
		if registry.RegistryID == "minecraft:fluid" {
			fluidTags = registry.Tags

			break
		}
	}

	if len(fluidTags) != 2 {
		t.Fatalf("configuration fluid tags = %v, want water and lava", fluidTags)
	}

	tags := make(map[string][]int32, len(fluidTags))

	for _, tag := range fluidTags {
		tags[tag.ID] = tag.Entries
	}

	if !slices.Equal(tags["minecraft:water"], []int32{2, 1}) {
		t.Errorf("water fluid tag = %v, want source and flowing water", tags["minecraft:water"])
	}

	if !slices.Equal(tags["minecraft:lava"], []int32{4, 3}) {
		t.Errorf("lava fluid tag = %v, want source and flowing lava", tags["minecraft:lava"])
	}
}

func TestConfigurationEnchantmentDependencyTags12111(t *testing.T) {
	registries := make(map[string]map[string][]int32)

	for _, registry := range ConfigurationTags {
		tags := make(map[string][]int32, len(registry.Tags))

		for _, tag := range registry.Tags {
			tags[tag.ID] = tag.Entries
		}

		registries[registry.RegistryID] = tags
	}

	itemTags := registries["minecraft:item"]
	if len(itemTags) != len(generatedItemEnchantmentTags) {
		t.Fatalf("item enchantment tags = %d, want %d", len(itemTags), len(generatedItemEnchantmentTags))
	}

	for _, expected := range generatedItemEnchantmentTags {
		if !slices.Equal(itemTags[expected.ID], expected.Entries) {
			t.Errorf("item tag %s = %v, want %v", expected.ID, itemTags[expected.ID], expected.Entries)
		}
	}

	if !slices.Contains(itemTags["minecraft:enchantable/mace"], int32(game.ItemMace)) {
		t.Fatal("mace enchantable tag does not contain the mace")
	}

	if !slices.Contains(itemTags["minecraft:enchantable/chest_armor"], int32(game.ItemLeatherChestplate)) {
		t.Fatal("chest armor enchantable tag does not contain the leather chestplate")
	}

	enchantmentTags := registries["minecraft:enchantment"]
	if len(enchantmentTags) != len(generatedEnchantmentTags) {
		t.Fatalf("enchantment tags = %d, want %d", len(enchantmentTags), len(generatedEnchantmentTags))
	}

	for _, expected := range generatedEnchantmentTags {
		if !slices.Equal(enchantmentTags[expected.ID], expected.Entries) {
			t.Errorf("enchantment tag %s = %v, want %v", expected.ID, enchantmentTags[expected.ID], expected.Entries)
		}
	}

	miningExclusive := enchantmentTags["minecraft:exclusive_set/mining"]
	if !slices.Equal(miningExclusive, []int32{23, 21}) {
		t.Fatalf("mining exclusive enchantments = %v, want fortune and silk touch holder IDs", miningExclusive)
	}

	entityTypeTags := registries["minecraft:entity_type"]
	expectedEntityTypeTags := map[string][]int32{
		"minecraft:arrows":                          {6, 123},
		"minecraft:sensitive_to_impaling":           {137, 7, 63, 40, 27, 107, 110, 136, 35, 127, 61, 130, 88, 152},
		"minecraft:sensitive_to_bane_of_arthropods": {11, 42, 114, 124, 22},
		"minecraft:sensitive_to_smite":              {115, 128, 146, 116, 16, 97, 151, 20, 150, 153, 154, 149, 38, 67, 152, 145, 99},
	}

	for id, expected := range expectedEntityTypeTags {
		if !slices.Equal(entityTypeTags[id], expected) {
			t.Errorf("required entity type tag %s = %v, want %v", id, entityTypeTags[id], expected)
		}
	}
}
