package game

import "testing"

type blockSoundTestCase struct {
	block       Block
	soundType   BlockSoundType
	place       SoundEvent
	volume      float32
	pitch       float32
	replaceable bool
}

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

	_, ok = wire.StateForProperties(3, 0, 0, 0, 0)
	if ok {
		t.Fatal("out-of-range property index succeeded")
	}
}

func TestBlockStatePropertiesCanBeReadAndChanged(t *testing.T) {
	state, ok := OakStairs.WithProperties(
		BlockPropertyValue{Name: "facing", Value: "east"},
		BlockPropertyValue{Name: "half", Value: "top"},
		BlockPropertyValue{Name: "shape", Value: "inner_left"},
	)

	if !ok {
		t.Fatal("resolve oak stair properties")
	}

	stairsProperties := map[string]string{"facing": "east", "half": "top", "shape": "inner_left", "waterlogged": "false"}

	for property, want := range stairsProperties {
		actual, found := state.Property(property)
		if !found || actual != want {
			t.Errorf("property %s = %q, %v; want %q, true", property, actual, found, want)
		}
	}

	changed, ok := state.WithProperties(BlockPropertyValue{Name: "shape", Value: "outer_right"})
	if !ok || blockPropertyForTest(t, changed, "facing") != "east" || blockPropertyForTest(t, changed, "shape") != "outer_right" {
		t.Fatalf("changed state = %d, %v", changed, ok)
	}

	invalid := [][]BlockPropertyValue{
		{{Name: "missing", Value: "x"}},
		{{Name: "facing", Value: "up"}},
		{{Name: "half", Value: "top"}, {Name: "half", Value: "bottom"}},
	}

	for _, values := range invalid {
		_, valid := state.WithProperties(values...)
		if valid {
			t.Errorf("invalid values resolved: %+v", values)
		}
	}
}

func TestEveryGeneratedStateRoundTripsThroughProperties(t *testing.T) {
	for state := Block(0); state <= MaxBlockState; state++ {
		definition, _ := state.Definition()

		values := make([]BlockPropertyValue, 0, len(definition.Properties))

		for _, property := range definition.Properties {
			value, ok := state.Property(property.Name)
			if !ok {
				t.Fatalf("state %d missing property %s", state, property.Name)
			}

			values = append(values, BlockPropertyValue{Name: property.Name, Value: value})
		}

		resolved, ok := definition.StateForPropertyValues(values...)
		if !ok || resolved != state {
			t.Fatalf("state %d round trip = %d, %v", state, resolved, ok)
		}
	}
}

func TestGeneratedBlockSoundAndReplaceabilityMetadata(t *testing.T) {
	tests := map[string]blockSoundTestCase{
		"stone":       {block: Stone, soundType: BlockSoundStone, place: SoundBlockStonePlace, volume: 1, pitch: 1},
		"grass":       {block: GrassBlock, soundType: BlockSoundGrass, place: SoundBlockGrassPlace, volume: 1, pitch: 1},
		"short grass": {block: ShortGrass, soundType: BlockSoundGrass, place: SoundBlockGrassPlace, volume: 1, pitch: 1, replaceable: true},
		"wood":        {block: OakPlanks, soundType: BlockSoundWood, place: SoundBlockWoodPlace, volume: 1, pitch: 1},
		"glass":       {block: Glass, soundType: BlockSoundGlass, place: SoundBlockGlassPlace, volume: 1, pitch: 1},
		"snow":        {block: Snow, soundType: BlockSoundSnow, place: SoundBlockSnowPlace, volume: 1, pitch: 1, replaceable: true},
		"metal":       {block: GoldBlock, soundType: BlockSoundMetal, place: SoundBlockMetalPlace, volume: 1, pitch: 1.5},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			definition, valid := test.block.Definition()
			if !valid {
				t.Fatal("block definition is missing")
			}

			sound := test.block.SoundType()
			if definition.Sound != test.soundType || sound.Place != test.place || sound.Volume != test.volume || sound.Pitch != test.pitch {
				t.Fatalf("sound metadata = type %d sound %+v", definition.Sound, sound)
			}

			if test.block.Replaceable() != test.replaceable {
				t.Fatalf("replaceable = %v, want %v", test.block.Replaceable(), test.replaceable)
			}
		})
	}

	if Dandelion.Replaceable() {
		t.Fatal("flower is replaceable")
	}
}

func blockPropertyForTest(t *testing.T, block Block, name string) string {
	t.Helper()

	value, ok := block.Property(name)
	if !ok {
		t.Fatalf("block %d has no %s property", block, name)
	}

	return value
}
