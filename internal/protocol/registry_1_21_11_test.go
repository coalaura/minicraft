package protocol

import "testing"

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

	if _, valid := Generic9xMenuType(0); valid {
		t.Fatal("zero-row generic menu type is valid")
	}

	if _, valid := Generic9xMenuType(7); valid {
		t.Fatal("seven-row generic menu type is valid")
	}
}

func TestCraftingMenuRegistryID12111(t *testing.T) {
	if MenuCrafting != 12 {
		t.Fatalf("crafting menu registry ID = %d, want 12", MenuCrafting)
	}
}
