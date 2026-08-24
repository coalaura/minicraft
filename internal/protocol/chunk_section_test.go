package protocol

import "testing"

func TestSectionBlocksPaletteModes(t *testing.T) {
	tests := []struct {
		name         string
		paletteSize  int
		direct       bool
		uniform      bool
		bitsPerEntry int32
	}{
		{name: "uniform", paletteSize: 1, uniform: true},
		{name: "small", paletteSize: 8, bitsPerEntry: 4},
		{name: "sixteen", paletteSize: 16, bitsPerEntry: 4},
		{name: "seventeen", paletteSize: 17, bitsPerEntry: 5},
		{name: "indirect maximum", paletteSize: 256, bitsPerEntry: 8},
		{name: "direct", paletteSize: 300, direct: true, bitsPerEntry: DirectBitsPerEntry},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var blocks SectionBlocks

			for index := range blocks.States {
				blocks.States[index] = int32(index % test.paletteSize)
			}

			section := blocks.ToSection(7)
			if section.Direct != test.direct {
				t.Fatalf("direct = %v, want %v", section.Direct, test.direct)
			}

			if test.uniform {
				if len(section.Palette) != 0 || len(section.Data) != 0 || section.BlockState != 0 {
					t.Fatalf("uniform section = %+v", section)
				}

				return
			}

			if !test.direct && len(section.Palette) != test.paletteSize {
				t.Fatalf("palette size = %d, want %d", len(section.Palette), test.paletteSize)
			}

			expectedLongs := packedLongCount(SectionVolume, test.bitsPerEntry)
			if len(section.Data) != expectedLongs {
				t.Fatalf("data longs = %d, want %d", len(section.Data), expectedLongs)
			}

			for index := range blocks.States {
				state := chunkSectionState(section, index)
				if state != blocks.States[index] {
					t.Fatalf("state %d = %d, want %d", index, state, blocks.States[index])
				}
			}
		})
	}
}

func TestUniformChunkSectionCountsBlocks(t *testing.T) {
	air := UniformChunkSection(AirBlockState, 3)
	if air.BlockCount != 0 || air.Biome != 3 {
		t.Fatalf("air section = %+v", air)
	}

	stone := UniformChunkSection(StoneBlockState, 4)
	if stone.BlockCount != SectionVolume || stone.BlockState != StoneBlockState {
		t.Fatalf("stone section = %+v", stone)
	}
}

func chunkSectionState(section ChunkSection, index int) int32 {
	if len(section.Palette) == 0 && !section.Direct {
		return section.BlockState
	}

	bitsPerEntry := int32(DirectBitsPerEntry)

	if !section.Direct {
		bitsPerEntry = paletteBitsPerEntry(len(section.Palette))
	}

	entriesPerLong := 64 / int(bitsPerEntry)
	longIndex := index / entriesPerLong
	bitOffset := index % entriesPerLong * int(bitsPerEntry)
	mask := int64(1<<bitsPerEntry) - 1
	value := int32(section.Data[longIndex] >> uint(bitOffset) & mask)

	if section.Direct {
		return value
	}

	return section.Palette[value]
}
