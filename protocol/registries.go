package protocol

type ConfigRegistry struct {
	id      string
	entries []string
}

var ConfigRegistries = []ConfigRegistry{
	{
		id:      "minecraft:dimension_type",
		entries: []string{"minecraft:overworld"},
	},
	{
		id:      "minecraft:worldgen/biome",
		entries: []string{"minecraft:plains"},
	},
	{
		id:      "minecraft:cat_variant",
		entries: []string{"minecraft:tabby"},
	},
	{
		id:      "minecraft:chicken_variant",
		entries: []string{"minecraft:temperate"},
	},
	{
		id:      "minecraft:cow_variant",
		entries: []string{"minecraft:temperate"},
	},
	{
		id:      "minecraft:frog_variant",
		entries: []string{"minecraft:temperate"},
	},
	{
		id:      "minecraft:painting_variant",
		entries: []string{"minecraft:kebab"},
	},
	{
		id:      "minecraft:pig_variant",
		entries: []string{"minecraft:temperate"},
	},
	{
		id:      "minecraft:wolf_sound_variant",
		entries: []string{"minecraft:classic"},
	},
	{
		id:      "minecraft:wolf_variant",
		entries: []string{"minecraft:pale"},
	},
	{
		id:      "minecraft:zombie_nautilus_variant",
		entries: []string{"minecraft:temperate"},
	},
	{
		id: "minecraft:timeline",
		entries: []string{
			"minecraft:day",               // 0
			"minecraft:moon",              // 1
			"minecraft:villager_schedule", // 2
			"minecraft:early_game",        // 3
		},
	},
	{
		id: "minecraft:damage_type",
		entries: []string{
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
