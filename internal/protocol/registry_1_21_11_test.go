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
	expected := []string{
		"minecraft:protection", "minecraft:fire_protection", "minecraft:feather_falling", "minecraft:blast_protection",
		"minecraft:projectile_protection", "minecraft:respiration", "minecraft:aqua_affinity", "minecraft:thorns",
		"minecraft:depth_strider", "minecraft:frost_walker", "minecraft:binding_curse", "minecraft:soul_speed",
		"minecraft:swift_sneak", "minecraft:sharpness", "minecraft:smite", "minecraft:bane_of_arthropods",
		"minecraft:knockback", "minecraft:fire_aspect", "minecraft:looting", "minecraft:sweeping_edge",
		"minecraft:efficiency", "minecraft:silk_touch", "minecraft:unbreaking", "minecraft:fortune",
		"minecraft:power", "minecraft:punch", "minecraft:flame", "minecraft:infinity",
		"minecraft:luck_of_the_sea", "minecraft:lure", "minecraft:loyalty", "minecraft:impaling",
		"minecraft:riptide", "minecraft:lunge", "minecraft:channeling", "minecraft:multishot",
		"minecraft:quick_charge", "minecraft:piercing", "minecraft:density", "minecraft:breach",
		"minecraft:wind_burst", "minecraft:mending", "minecraft:vanishing_curse",
	}

	for _, registry := range ConfigurationRegistries {
		if registry.ID != "minecraft:enchantment" {
			continue
		}

		if !slices.Equal(registry.Entries, expected) {
			t.Fatalf("enchantment registry entries = %v, want %v", registry.Entries, expected)
		}

		for index, name := range expected {
			registryID, found := registry.EntryID(name)
			if !found || registryID != int32(index) {
				t.Fatalf("%s registry ID = %d, found %t; want %d, true", name, registryID, found, index)
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
	requiredItemTags := []string{
		"armor", "bow", "chest_armor", "crossbow", "durability", "equippable", "fire_aspect", "fishing", "foot_armor", "head_armor",
		"leg_armor", "lunge", "mace", "melee_weapon", "mining", "mining_loot", "sharp_weapon", "sweeping", "trident", "vanishing", "weapon",
	}

	for _, name := range requiredItemTags {
		id := "minecraft:enchantable/" + name
		entries, exists := itemTags[id]

		if !exists || len(entries) == 0 {
			t.Errorf("required item tag %s = %v, exists %t", id, entries, exists)
		}
	}

	if !slices.Contains(itemTags["minecraft:enchantable/mace"], int32(game.ItemMace)) {
		t.Fatal("mace enchantable tag does not contain the mace")
	}

	if !slices.Contains(itemTags["minecraft:enchantable/chest_armor"], int32(game.ItemLeatherChestplate)) {
		t.Fatal("chest armor enchantable tag does not contain the leather chestplate")
	}

	enchantmentTags := registries["minecraft:enchantment"]
	requiredEnchantmentTags := []string{"armor", "boots", "bow", "crossbow", "damage", "mining", "riptide"}

	for _, name := range requiredEnchantmentTags {
		id := "minecraft:exclusive_set/" + name
		entries, exists := enchantmentTags[id]

		if !exists || len(entries) == 0 {
			t.Errorf("required enchantment tag %s = %v, exists %t", id, entries, exists)
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
