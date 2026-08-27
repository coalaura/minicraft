package main

import "testing"

type placementRuleTestCase struct {
	name       string
	properties []BlockProperty
	box        string
	rule       string
}

func TestBlockPlacementRuleFamilies(t *testing.T) {
	tests := []placementRuleTestCase{
		{name: "grass_block", properties: properties("snowy"), box: "block", rule: "ItemPlacementDefault"},
		{name: "oak_leaves", properties: properties("distance", "persistent", "waterlogged"), box: "block", rule: "ItemPlacementLeaves"},
		{name: "iron_chain", properties: properties("axis", "waterlogged"), box: "block", rule: "ItemPlacementChain"},
		{name: "stone_button", properties: properties("face", "facing", "powered"), box: "empty", rule: "ItemPlacementButton"},
		{name: "stone_pressure_plate", properties: properties("powered"), box: "empty", rule: "ItemPlacementPressurePlate"},
		{name: "short_grass", box: "empty", rule: "ItemPlacementPlant"},
		{name: "snow", properties: properties("layers"), box: "empty", rule: "ItemPlacementSnow"},
		{name: "candle", properties: properties("candles", "lit", "waterlogged"), box: "block", rule: "ItemPlacementCandle"},
		{name: "pointed_dripstone", properties: properties("thickness", "vertical_direction", "waterlogged"), box: "block", rule: "ItemPlacementPointedDripstone"},
		{name: "copper_grate", properties: properties("waterlogged"), box: "block", rule: "ItemPlacementDefault"},
		{name: "iron_door", properties: properties("facing", "half", "hinge", "open", "powered"), box: "block", rule: "ItemPlacementDoor"},
		{name: "copper_bars", properties: properties("east", "north", "south", "waterlogged", "west"), box: "block", rule: "ItemPlacementPane"},
		{name: "cobweb", box: "empty", rule: "ItemPlacementDefault"},
		{name: "mushroom_stem", properties: properties("down", "east", "north", "south", "up", "west"), box: "block", rule: "ItemPlacementDefault"},
		{name: "barrel", properties: properties("facing", "open"), box: "block", rule: "ItemPlacementDirectionalFacing"},
		{name: "chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := BlockDefinition{Name: test.name, DefaultState: 1, Properties: test.properties, BoundingBox: test.box}

			rule := blockPlacementRule(block)
			if rule != test.rule {
				t.Fatalf("placement rule = %q, want %q", rule, test.rule)
			}
		})
	}
}

func TestUnsupportedBlockEntityPlacementFamiliesRemainExcluded(t *testing.T) {
	for _, name := range []string{"white_bed", "oak_sign", "white_banner", "white_shulker_box", "decorated_pot", "oak_shelf", "suspicious_sand"} {
		block := BlockDefinition{Name: name, DefaultState: 1, BoundingBox: "block"}

		rule := blockPlacementRule(block)
		if rule != "" {
			t.Errorf("%s placement rule = %q", name, rule)
		}
	}

	paleMossCarpet := BlockDefinition{
		Name:         "pale_moss_carpet",
		DefaultState: 1,
		BoundingBox:  "block",
		Properties:   properties("bottom", "east", "north", "south", "west"),
	}

	rule := blockPlacementRule(paleMossCarpet)
	if rule != "" {
		t.Errorf("pale moss carpet placement rule = %q", rule)
	}
}

func properties(names ...string) []BlockProperty {
	properties := make([]BlockProperty, len(names))

	for index, name := range names {
		properties[index] = BlockProperty{Name: name}
	}

	return properties
}
