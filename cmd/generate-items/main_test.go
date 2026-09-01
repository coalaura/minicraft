package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

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
		{name: "hopper", properties: properties("enabled", "facing"), box: "block", rule: "ItemPlacementHopper"},
		{name: "chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
		{name: "copper_chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
		{name: "exposed_copper_chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
		{name: "weathered_copper_chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
		{name: "oxidized_copper_chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
		{name: "waxed_copper_chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
		{name: "waxed_exposed_copper_chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
		{name: "waxed_weathered_copper_chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
		{name: "waxed_oxidized_copper_chest", properties: properties("facing", "type", "waterlogged"), box: "block", rule: "ItemPlacementChest"},
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
	unsupportedBlocks := []string{"white_bed", "oak_sign", "white_banner", "white_shulker_box", "decorated_pot", "oak_shelf", "suspicious_sand"}

	for _, name := range unsupportedBlocks {
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

func TestConsumableSourcesAndGeneratedItemsAreCurrent(t *testing.T) {
	manifest, err := readConsumables(filepath.Join("..", "..", "data", "item_consumables.json"))
	if err != nil {
		t.Fatalf("read consumables: %v", err)
	}

	for source, expected := range manifest.Sources {
		path := filepath.Join("..", "..", "..", "reference", "client_source", filepath.FromSlash(source))

		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read consumable source %s: %v", source, err)
		}

		digest := sha256.Sum256(contents)

		actual := hex.EncodeToString(digest[:])
		if actual != expected {
			t.Errorf("consumable source %s digest = %s, want %s", source, actual, expected)
		}
	}

	items, err := readItems(filepath.Join("..", "..", "data", "items.json"))
	if err != nil {
		t.Fatalf("read items: %v", err)
	}

	blocks, err := readBlocks(filepath.Join("..", "..", "data", "blocks.json"))
	if err != nil {
		t.Fatalf("read blocks: %v", err)
	}

	generated, err := generate(items, blocks, manifest)
	if err != nil {
		t.Fatalf("generate items: %v", err)
	}

	committed, err := os.ReadFile(filepath.Join("..", "..", "internal", "game", "items_generated.go"))
	if err != nil {
		t.Fatalf("read generated items: %v", err)
	}

	if !bytes.Equal(generated, committed) {
		t.Fatal("items_generated.go is stale; run go generate ./internal/game")
	}
}

func properties(names ...string) []BlockProperty {
	properties := make([]BlockProperty, len(names))

	for index, name := range names {
		properties[index] = BlockProperty{Name: name}
	}

	return properties
}
