package protocol

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
		RegistryID: "minecraft:timeline",
		Tags: []RegistryTag{
			{
				ID:      "minecraft:in_overworld",
				Entries: []int32{2, 0, 1, 3},
			},
		},
	},
}
