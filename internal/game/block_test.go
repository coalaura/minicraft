package game

import "testing"

func TestGeneratedBlockCatalogueCoversVanillaStates(t *testing.T) {
	if MaxBlockID != 1165 {
		t.Fatalf("max block ID = %d, want 1165", MaxBlockID)
	}

	if MaxBlockState != 29670 {
		t.Fatalf("max block state = %d, want 29670", MaxBlockState)
	}

	for state := Block(0); state <= MaxBlockState; state++ {
		definition, ok := state.Definition()
		if !ok {
			t.Fatalf("state %d has no definition", state)
		}

		if state < definition.MinState || state > definition.MaxState {
			t.Fatalf("state %d outside definition range %d..%d", state, definition.MinState, definition.MaxState)
		}
	}

	if (MaxBlockState + 1).Valid() {
		t.Fatal("state above catalogue is valid")
	}
}

func TestBlockStatePropertyResolution(t *testing.T) {
	grass, ok := BlockByID(GrassBlockID)
	if !ok {
		t.Fatal("grass block definition is missing")
	}

	snowy, ok := grass.StateForProperties(0)
	if !ok || snowy != 8 {
		t.Fatalf("snowy grass state = %d, %v; want 8, true", snowy, ok)
	}

	notSnowy, ok := grass.StateForProperties(1)
	if !ok || notSnowy != GrassBlock {
		t.Fatalf("default grass state = %d, %v; want %d, true", notSnowy, ok, GrassBlock)
	}

	wire, ok := BlockByID(RedstoneWireID)
	if !ok {
		t.Fatal("redstone wire definition is missing")
	}

	last, ok := wire.StateForProperties(2, 2, 15, 2, 2)
	if !ok || last != wire.MaxState {
		t.Fatalf("last redstone wire state = %d, %v; want %d, true", last, ok, wire.MaxState)
	}

	if _, ok = wire.StateForProperties(3, 0, 0, 0, 0); ok {
		t.Fatal("out-of-range property index succeeded")
	}
}
