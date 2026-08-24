package game

import "testing"

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
	for name, test := range map[string]struct {
		item  Item
		block Block
	}{
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
}

func TestComplexItemsAreNotMappedToPlacementStates(t *testing.T) {
	items := []Item{
		ItemOakDoor,
		ItemWhiteBed,
		ItemOakSlab,
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
