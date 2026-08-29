package game

import (
	"slices"
	"testing"
)

type generatedNameLookupTestCase struct {
	name string
	ok   bool
}

func TestItemByName(t *testing.T) {
	itemCases := []generatedNameLookupTestCase{
		{name: "stone", ok: true},
		{name: "minecraft:stone", ok: true},
		{name: "", ok: false},
		{name: "other:stone", ok: false},
		{name: "minecraft:", ok: false},
		{name: "minecraft:stone:extra", ok: false},
	}

	for _, test := range itemCases {
		item, ok := ItemByName(test.name)
		if ok != test.ok {
			t.Errorf("ItemByName(%q) = %d, %v; want ok %v", test.name, item, ok, test.ok)
			continue
		}

		if ok && item != ItemStone {
			t.Errorf("ItemByName(%q) = %d, want %d", test.name, item, ItemStone)
		}
	}
}

func TestBlockByNameReturnsDefaultState(t *testing.T) {
	blockCases := []generatedNameLookupTestCase{
		{name: "grass_block", ok: true},
		{name: "minecraft:grass_block", ok: true},
		{name: "", ok: false},
		{name: "other:grass_block", ok: false},
		{name: "minecraft:", ok: false},
		{name: "minecraft:grass_block:extra", ok: false},
	}

	for _, test := range blockCases {
		block, ok := BlockByName(test.name)
		if ok != test.ok {
			t.Errorf("BlockByName(%q) = %d, %v; want ok %v", test.name, block, ok, test.ok)
			continue
		}

		if ok && block != GrassBlock {
			t.Errorf("BlockByName(%q) = %d, want %d", test.name, block, GrassBlock)
		}
	}
}

func TestGeneratedNameCataloguesAreSorted(t *testing.T) {
	catalogues := map[string][]string{
		"items":  ItemNames,
		"blocks": BlockNames,
	}

	for name, catalogue := range catalogues {
		if !slices.IsSorted(catalogue) {
			t.Errorf("%s catalogue is not sorted", name)
		}

		if slices.ContainsFunc(catalogue, func(item string) bool { return item == "" }) {
			t.Errorf("%s catalogue contains an empty name", name)
		}
	}

	if !slices.Contains(ItemNames, "stone") {
		t.Error("item catalogue does not contain stone")
	}

	if !slices.Contains(BlockNames, "grass_block") {
		t.Error("block catalogue does not contain grass_block")
	}
}
