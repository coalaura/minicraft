package game

import "testing"

type itemPlacementBlockMappingTestCase struct {
	item  Item
	block Block
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
	for name, test := range map[string]itemPlacementBlockMappingTestCase{
		"stone": {item: ItemStone, block: Stone},
		"dirt":  {item: ItemDirt, block: Dirt},
		"glass": {item: ItemGlass, block: Glass},
		"log":   {item: ItemOakLog, block: OakLog},
	} {
		t.Run(name, func(t *testing.T) {
			block, ok := test.item.PlacementBlock()
			if !ok || block != test.block {
				t.Fatalf("placement block = %d, %v; want %d, true", block, ok, test.block)
			}
		})
	}

	if rule := ItemOakLog.PlacementRule(); rule != ItemPlacementAxis {
		t.Fatalf("oak log placement rule = %d, want axis", rule)
	}

	for item, rule := range map[Item]ItemPlacementRule{
		ItemOakSlab:          ItemPlacementSlab,
		ItemOakStairs:        ItemPlacementStairs,
		ItemOakDoor:          ItemPlacementDoor,
		ItemOakTrapdoor:      ItemPlacementTrapdoor,
		ItemOakFenceGate:     ItemPlacementFenceGate,
		ItemOakFence:         ItemPlacementFence,
		ItemGlassPane:        ItemPlacementPane,
		ItemIronBars:         ItemPlacementPane,
		ItemCobblestoneWall:  ItemPlacementWall,
		ItemIronDoor:         ItemPlacementDoor,
		ItemIronTrapdoor:     ItemPlacementTrapdoor,
		ItemOakLeaves:        ItemPlacementLeaves,
		ItemIronChain:        ItemPlacementChain,
		ItemStoneButton:      ItemPlacementButton,
		ItemSnow:             ItemPlacementSnow,
		ItemCandle:           ItemPlacementCandle,
		ItemPointedDripstone: ItemPlacementPointedDripstone,
		ItemFern:             ItemPlacementPlant,
		ItemPoppy:            ItemPlacementPlant,
		ItemCopperBars:       ItemPlacementPane,
		ItemCobweb:           ItemPlacementDefault,
	} {
		if actual := item.PlacementRule(); actual != rule {
			t.Errorf("item %d placement rule = %d, want %d", item, actual, rule)
		}
	}
}

func TestUnsupportedItemsAreNotMappedToPlacementStates(t *testing.T) {
	items := []Item{
		ItemWhiteBed,
		ItemTorch,
		ItemChest,
		ItemRedstone,
		ItemWaterBucket,
	}

	for _, item := range items {
		if block, ok := item.PlacementBlock(); ok {
			t.Errorf("item %d maps to block %d", item, block)
		}
	}

	if block, ok := MaxItemID.PlacementBlock(); ok {
		t.Errorf("last catalogue item maps to block %d", block)
	}
}
