package game

import "testing"

type itemPlacementBlockMappingTestCase struct {
	item  Item
	block Block
}

type copperChestTestCase struct {
	name      string
	block     Block
	item      Item
	oxidation int
	waxed     bool
}

func TestGeneratedItemCatalogueCoversVanillaItems(t *testing.T) {
	if MaxItemID != 1504 {
		t.Fatalf("max item ID = %d, want 1504", MaxItemID)
	}

	for item := Item(0); item <= MaxItemID; item++ {
		definition, ok := item.Definition()
		if !ok {
			t.Fatalf("item %d has no definition", item)
		}

		if definition.ID != item || definition.Name == "" || definition.StackSize < 1 {
			t.Fatalf("item %d definition = %+v", item, definition)
		}
	}

	if (MaxItemID + 1).Valid() {
		t.Fatal("item above catalogue is valid")
	}
}

func TestItemPlacementBlockMapping(t *testing.T) {
	placementBlockCases := map[string]itemPlacementBlockMappingTestCase{
		"stone": {item: ItemStone, block: Stone},
		"dirt":  {item: ItemDirt, block: Dirt},
		"glass": {item: ItemGlass, block: Glass},
		"log":   {item: ItemOakLog, block: OakLog},
	}

	for name, test := range placementBlockCases {
		t.Run(name, func(t *testing.T) {
			block, ok := test.item.PlacementBlock()
			if !ok || block != test.block {
				t.Fatalf("placement block = %d, %v; want %d, true", block, ok, test.block)
			}
		})
	}

	rule := ItemOakLog.PlacementRule()
	if rule != ItemPlacementAxis {
		t.Fatalf("oak log placement rule = %d, want axis", rule)
	}

	for _, test := range copperChestBlocksForTest() {
		block, ok := test.item.PlacementBlock()
		if !ok || block != test.block {
			t.Errorf("%s placement block = %d, %v; want %d, true", test.name, block, ok, test.block)
		}
	}

	placementRuleCases := map[Item]ItemPlacementRule{
		ItemOakSlab:                   ItemPlacementSlab,
		ItemOakStairs:                 ItemPlacementStairs,
		ItemOakDoor:                   ItemPlacementDoor,
		ItemOakTrapdoor:               ItemPlacementTrapdoor,
		ItemOakFenceGate:              ItemPlacementFenceGate,
		ItemOakFence:                  ItemPlacementFence,
		ItemGlassPane:                 ItemPlacementPane,
		ItemIronBars:                  ItemPlacementPane,
		ItemCobblestoneWall:           ItemPlacementWall,
		ItemIronDoor:                  ItemPlacementDoor,
		ItemIronTrapdoor:              ItemPlacementTrapdoor,
		ItemOakLeaves:                 ItemPlacementLeaves,
		ItemIronChain:                 ItemPlacementChain,
		ItemStoneButton:               ItemPlacementButton,
		ItemSnow:                      ItemPlacementSnow,
		ItemCandle:                    ItemPlacementCandle,
		ItemPointedDripstone:          ItemPlacementPointedDripstone,
		ItemFern:                      ItemPlacementPlant,
		ItemPoppy:                     ItemPlacementPlant,
		ItemCopperBars:                ItemPlacementPane,
		ItemCobweb:                    ItemPlacementDefault,
		ItemChest:                     ItemPlacementChest,
		ItemCopperChest:               ItemPlacementChest,
		ItemExposedCopperChest:        ItemPlacementChest,
		ItemWeatheredCopperChest:      ItemPlacementChest,
		ItemOxidizedCopperChest:       ItemPlacementChest,
		ItemWaxedCopperChest:          ItemPlacementChest,
		ItemWaxedExposedCopperChest:   ItemPlacementChest,
		ItemWaxedWeatheredCopperChest: ItemPlacementChest,
		ItemWaxedOxidizedCopperChest:  ItemPlacementChest,
	}

	for item, rule := range placementRuleCases {
		actual := item.PlacementRule()
		if actual != rule {
			t.Errorf("item %d placement rule = %d, want %d", item, actual, rule)
		}
	}
}

func copperChestBlocksForTest() []copperChestTestCase {
	return []copperChestTestCase{
		{name: "copper", block: CopperChest, item: ItemCopperChest},
		{name: "exposed", block: ExposedCopperChest, item: ItemExposedCopperChest, oxidation: 1},
		{name: "weathered", block: WeatheredCopperChest, item: ItemWeatheredCopperChest, oxidation: 2},
		{name: "oxidized", block: OxidizedCopperChest, item: ItemOxidizedCopperChest, oxidation: 3},
		{name: "waxed_copper", block: WaxedCopperChest, item: ItemWaxedCopperChest, waxed: true},
		{name: "waxed_exposed", block: WaxedExposedCopperChest, item: ItemWaxedExposedCopperChest, oxidation: 1, waxed: true},
		{name: "waxed_weathered", block: WaxedWeatheredCopperChest, item: ItemWaxedWeatheredCopperChest, oxidation: 2, waxed: true},
		{name: "waxed_oxidized", block: WaxedOxidizedCopperChest, item: ItemWaxedOxidizedCopperChest, oxidation: 3, waxed: true},
	}
}

func TestUnsupportedItemsAreNotMappedToPlacementStates(t *testing.T) {
	items := []Item{
		ItemWhiteBed,
		ItemTorch,
		ItemRedstone,
		ItemWaterBucket,
	}

	for _, item := range items {
		block, ok := item.PlacementBlock()
		if ok {
			t.Errorf("item %d maps to block %d", item, block)
		}
	}

	block, ok := MaxItemID.PlacementBlock()
	if ok {
		t.Errorf("last catalogue item maps to block %d", block)
	}
}
