package protocol

import "testing"

func TestSectionBiomesPaletteModes(t *testing.T) {
	tests := []struct {
		name        string
		paletteSize int
		direct      bool
		bits        int32
	}{
		{name: "uniform", paletteSize: 1},
		{name: "two", paletteSize: 2, bits: 1},
		{name: "three", paletteSize: 3, bits: 2},
		{name: "eight", paletteSize: 8, bits: 3},
		{name: "direct", paletteSize: 9, direct: true, bits: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var biomes SectionBiomes

			for index := range biomes.States {
				biomes.States[index] = int32(index % test.paletteSize)
			}

			section := UniformChunkSection(AirBlockState, 0)
			section.SetBiomes(&biomes)

			if section.BiomeDirect != test.direct {
				t.Fatalf("direct = %v, want %v", section.BiomeDirect, test.direct)
			}

			if test.paletteSize == 1 {
				if len(section.BiomePalette) != 0 || len(section.BiomeData) != 0 || section.Biome != 0 {
					t.Fatalf("uniform biome section = %+v", section)
				}

				return
			}

			if section.BiomeBitsPerEntry != test.bits {
				t.Fatalf("bits = %d, want %d", section.BiomeBitsPerEntry, test.bits)
			}

			for index, want := range biomes.States {
				got := chunkSectionBiome(section, index)
				if got != want {
					t.Fatalf("biome %d = %d, want %d", index, got, want)
				}
			}
		})
	}
}

func chunkSectionBiome(section ChunkSection, index int) int32 {
	if len(section.BiomePalette) == 0 && !section.BiomeDirect {
		return section.Biome
	}

	bitsPerEntry := section.BiomeBitsPerEntry
	entriesPerLong := 64 / int(bitsPerEntry)
	longIndex := index / entriesPerLong
	bitOffset := index % entriesPerLong * int(bitsPerEntry)
	mask := int64(1<<bitsPerEntry) - 1
	value := int32(section.BiomeData[longIndex] >> uint(bitOffset) & mask)

	if section.BiomeDirect {
		return value
	}

	return section.BiomePalette[value]
}
