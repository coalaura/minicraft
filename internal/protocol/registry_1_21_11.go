package protocol

import "github.com/coalaura/minicraft/internal/game"

const (
	MenuGeneric9x1 int32 = iota
	MenuGeneric9x2
	MenuGeneric9x3
	MenuGeneric9x4
	MenuGeneric9x5
	MenuGeneric9x6
)

const (
	MenuBlastFurnace int32 = 10
	MenuCrafting     int32 = 12
	MenuFurnace      int32 = 14
	MenuHopper       int32 = 16
	MenuSmoker       int32 = 22
)

type itemEnchantmentTagDefinition struct {
	name     string
	category game.ItemEnchantCategory
}

func Generic9xMenuType(rows int) (int32, bool) {
	if rows < 1 || rows > 6 {
		return 0, false
	}

	return MenuGeneric9x1 + int32(rows-1), true
}

var ConfigurationRegistries = []Registry{
	{
		ID:      "minecraft:dimension_type",
		Entries: []string{"minecraft:overworld"},
	},
	{
		ID:      "minecraft:chat_type",
		Entries: []string{"minecraft:chat"},
	},
	{
		ID:      "minecraft:worldgen/biome",
		Entries: biomeRegistryEntries,
	},
	{
		ID: "minecraft:enchantment",
		Entries: []string{
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
		},
	},
	{
		ID:      "minecraft:cat_variant",
		Entries: []string{"minecraft:tabby"},
	},
	{
		ID:      "minecraft:chicken_variant",
		Entries: []string{"minecraft:temperate"},
	},
	{
		ID:      "minecraft:cow_variant",
		Entries: []string{"minecraft:temperate"},
	},
	{
		ID:      "minecraft:frog_variant",
		Entries: []string{"minecraft:temperate"},
	},
	{
		ID:      "minecraft:painting_variant",
		Entries: []string{"minecraft:kebab"},
	},
	{
		ID:      "minecraft:pig_variant",
		Entries: []string{"minecraft:temperate"},
	},
	{
		ID:      "minecraft:wolf_sound_variant",
		Entries: []string{"minecraft:classic"},
	},
	{
		ID:      "minecraft:wolf_variant",
		Entries: []string{"minecraft:pale"},
	},
	{
		ID:      "minecraft:zombie_nautilus_variant",
		Entries: []string{"minecraft:temperate"},
	},
	{
		ID: "minecraft:timeline",
		Entries: []string{
			"minecraft:day",               // 0
			"minecraft:moon",              // 1
			"minecraft:villager_schedule", // 2
			"minecraft:early_game",        // 3
		},
	},
	{
		ID: "minecraft:damage_type",
		Entries: []string{
			"minecraft:arrow",
			"minecraft:bad_respawn_point",
			"minecraft:cactus",
			"minecraft:campfire",
			"minecraft:cramming",
			"minecraft:dragon_breath",
			"minecraft:drown",
			"minecraft:dry_out",
			"minecraft:ender_pearl",
			"minecraft:explosion",
			"minecraft:fall",
			"minecraft:falling_anvil",
			"minecraft:falling_block",
			"minecraft:falling_stalactite",
			"minecraft:fireball",
			"minecraft:fireworks",
			"minecraft:fly_into_wall",
			"minecraft:freeze",
			"minecraft:generic",
			"minecraft:generic_kill",
			"minecraft:hot_floor",
			"minecraft:in_fire",
			"minecraft:in_wall",
			"minecraft:indirect_magic",
			"minecraft:lava",
			"minecraft:lightning_bolt",
			"minecraft:mace_smash",
			"minecraft:magic",
			"minecraft:mob_attack",
			"minecraft:mob_attack_no_aggro",
			"minecraft:mob_projectile",
			"minecraft:on_fire",
			"minecraft:out_of_world",
			"minecraft:outside_border",
			"minecraft:player_attack",
			"minecraft:player_explosion",
			"minecraft:sonic_boom",
			"minecraft:spear",
			"minecraft:spit",
			"minecraft:stalagmite",
			"minecraft:starve",
			"minecraft:sting",
			"minecraft:sweet_berry_bush",
			"minecraft:thorns",
			"minecraft:thrown",
			"minecraft:trident",
			"minecraft:unattributed_fireball",
			"minecraft:wind_charge",
			"minecraft:wither",
			"minecraft:wither_skull",
		},
	},
}

var ConfigurationTags = []RegistryTags{
	{
		RegistryID: "minecraft:block",
		Tags:       generatedBlockTags,
	},
	{
		RegistryID: "minecraft:item",
		Tags:       itemEnchantmentTags(),
	},
	{
		RegistryID: "minecraft:enchantment",
		Tags: []RegistryTag{
			registryTag("minecraft:enchantment", "minecraft:exclusive_set/armor", "minecraft:protection", "minecraft:blast_protection", "minecraft:fire_protection", "minecraft:projectile_protection"),
			registryTag("minecraft:enchantment", "minecraft:exclusive_set/boots", "minecraft:frost_walker", "minecraft:depth_strider"),
			registryTag("minecraft:enchantment", "minecraft:exclusive_set/bow", "minecraft:infinity", "minecraft:mending"),
			registryTag("minecraft:enchantment", "minecraft:exclusive_set/crossbow", "minecraft:multishot", "minecraft:piercing"),
			registryTag("minecraft:enchantment", "minecraft:exclusive_set/damage", "minecraft:sharpness", "minecraft:smite", "minecraft:bane_of_arthropods", "minecraft:impaling", "minecraft:density", "minecraft:breach"),
			registryTag("minecraft:enchantment", "minecraft:exclusive_set/mining", "minecraft:fortune", "minecraft:silk_touch"),
			registryTag("minecraft:enchantment", "minecraft:exclusive_set/riptide", "minecraft:loyalty", "minecraft:channeling"),
		},
	},
	{
		RegistryID: "minecraft:entity_type",
		Tags: []RegistryTag{
			{
				ID:      "minecraft:arrows",
				Entries: []int32{6, 123},
			},
			{
				ID:      "minecraft:sensitive_to_impaling",
				Entries: []int32{137, 7, 63, 40, 27, 107, 110, 136, 35, 127, 61, 130, 88, 152},
			},
			{
				ID:      "minecraft:sensitive_to_bane_of_arthropods",
				Entries: []int32{11, 42, 114, 124, 22},
			},
			{
				ID:      "minecraft:sensitive_to_smite",
				Entries: []int32{115, 128, 146, 116, 16, 97, 151, 20, 150, 153, 154, 149, 38, 67, 152, 145, 99},
			},
		},
	},
	{
		RegistryID: "minecraft:timeline",
		Tags: []RegistryTag{
			{
				ID:      "minecraft:in_overworld",
				Entries: []int32{2, 0, 1, 3},
			},
		},
	},
}

func itemEnchantmentTags() []RegistryTag {
	definitions := []itemEnchantmentTagDefinition{
		{name: "armor", category: game.ItemEnchantCategoryArmor},
		{name: "bow", category: game.ItemEnchantCategoryBow},
		{name: "chest_armor", category: game.ItemEnchantCategoryChestArmor},
		{name: "crossbow", category: game.ItemEnchantCategoryCrossbow},
		{name: "durability", category: game.ItemEnchantCategoryDurability},
		{name: "equippable", category: game.ItemEnchantCategoryEquippable},
		{name: "fire_aspect", category: game.ItemEnchantCategoryFireAspect},
		{name: "fishing", category: game.ItemEnchantCategoryFishing},
		{name: "foot_armor", category: game.ItemEnchantCategoryFootArmor},
		{name: "head_armor", category: game.ItemEnchantCategoryHeadArmor},
		{name: "leg_armor", category: game.ItemEnchantCategoryLegArmor},
		{name: "lunge", category: game.ItemEnchantCategoryLunge},
		{name: "mace", category: game.ItemEnchantCategoryMace},
		{name: "melee_weapon", category: game.ItemEnchantCategoryMeleeWeapon},
		{name: "mining", category: game.ItemEnchantCategoryMining},
		{name: "mining_loot", category: game.ItemEnchantCategoryMiningLoot},
		{name: "sharp_weapon", category: game.ItemEnchantCategorySharpWeapon},
		{name: "sweeping", category: game.ItemEnchantCategorySweeping},
		{name: "trident", category: game.ItemEnchantCategoryTrident},
		{name: "vanishing", category: game.ItemEnchantCategoryVanishing},
		{name: "weapon", category: game.ItemEnchantCategoryWeapon},
	}

	tags := make([]RegistryTag, 0, len(definitions))

	for _, definition := range definitions {
		entries := make([]int32, 0)

		for itemID := range int(game.MaxItemID) + 1 {
			itemDefinition, exists := game.Item(itemID).Definition()
			if exists && itemDefinition.EnchantCategories&definition.category != 0 {
				entries = append(entries, int32(itemID))
			}
		}

		tags = append(tags, RegistryTag{ID: "minecraft:enchantable/" + definition.name, Entries: entries})
	}

	return tags
}

func registryTag(registryID string, tagID string, entryNames ...string) RegistryTag {
	entries := make([]int32, 0, len(entryNames))

	for _, entryName := range entryNames {
		for _, registry := range ConfigurationRegistries {
			if registry.ID != registryID {
				continue
			}

			entryID, exists := registry.EntryID(entryName)
			if !exists {
				panic("unknown " + registryID + " registry entry " + entryName)
			}

			entries = append(entries, entryID)

			break
		}
	}

	return RegistryTag{ID: tagID, Entries: entries}
}
