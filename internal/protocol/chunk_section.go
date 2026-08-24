package protocol

const (
	SectionVolume           = 4096
	BiomeSectionVolume      = 64
	MinBitsPerEntry         = 4
	MaxIndirectBitsPerEntry = 8
	DirectBitsPerEntry      = 15

	MinBiomeBitsPerEntry         = 1
	MaxIndirectBiomeBitsPerEntry = 3
	MinDirectBiomeBitsPerEntry   = 4
)

// Block state IDs, matching the vanilla registry order.
const (
	AirBlockState   = 0
	StoneBlockState = 1
)

type ChunkSection struct {
	BlockCount int16
	BlockState int32
	Biome      int32

	// When Palette is empty the section is uniform and encoded as a
	// single-value palette using BlockState. Otherwise Data holds the
	// palette indices of all 4096 blocks, bit-packed without entries
	// spanning longs.
	Palette []int32
	Data    []int64
	Direct  bool

	// Biomes use the same paletted-container layout over a 4x4x4 grid.
	// When BiomePalette is empty and BiomeDirect is false, Biome is the
	// single biome for the section.
	BiomePalette      []int32
	BiomeData         []int64
	BiomeDirect       bool
	BiomeBitsPerEntry int32
}

// SectionBlocks holds the block states of one chunk section, indexed by
// (y * 256 + z * 16 + x) with local coordinates from 0 to 15.
type SectionBlocks struct {
	States [SectionVolume]int32
}

// SectionBiomes holds the biome registry IDs of one chunk section, indexed by
// (y * 16 + z * 4 + x) with local biome coordinates from 0 to 3.
type SectionBiomes struct {
	States [BiomeSectionVolume]int32
}

func (blocks *SectionBlocks) Set(localX, localY, localZ int, state int32) {
	blocks.States[localY*256+localZ*16+localX] = state
}

func (biomes *SectionBiomes) Set(localX, localY, localZ int, biome int32) {
	biomes.States[localY*16+localZ*4+localX] = biome
}

// ToSection packs the blocks into the smallest supported palette form, using
// the given biome ID for the whole section.
func (blocks *SectionBlocks) ToSection(biomeID int32) ChunkSection {
	firstState := blocks.States[0]
	uniform := true

	var blockCount int16

	for _, state := range blocks.States {
		if state != AirBlockState {
			blockCount++
		}

		if state != firstState {
			uniform = false
		}
	}

	if uniform {
		return UniformChunkSection(firstState, biomeID)
	}

	var (
		smallPalette [16]int32
		palette      []int32
		paletteIndex map[int32]int32
		paletteSize  int
	)

	for _, state := range blocks.States {
		if paletteIndex == nil {
			var known bool

			for index := 0; index < paletteSize; index++ {
				if smallPalette[index] == state {
					known = true

					break
				}
			}

			if known {
				continue
			}

			if paletteSize < len(smallPalette) {
				smallPalette[paletteSize] = state

				paletteSize++

				continue
			}

			palette = make([]int32, paletteSize, 1<<MaxIndirectBitsPerEntry)
			copy(palette, smallPalette[:])

			paletteIndex = make(map[int32]int32, 1<<MaxIndirectBitsPerEntry)

			for index, paletteState := range palette {
				paletteIndex[paletteState] = int32(index)
			}
		}

		if _, known := paletteIndex[state]; known {
			continue
		}

		if paletteSize == 1<<MaxIndirectBitsPerEntry {
			return ChunkSection{
				BlockCount: blockCount,
				Biome:      biomeID,
				Data:       packDirectStates(&blocks.States),
				Direct:     true,
			}
		}

		paletteIndex[state] = int32(paletteSize)

		palette = append(palette, state)

		paletteSize++
	}

	if paletteIndex == nil {
		palette = append([]int32(nil), smallPalette[:paletteSize]...)
	}

	bitsPerEntry := paletteBitsPerEntry(len(palette))

	data := make([]int64, packedLongCount(SectionVolume, bitsPerEntry))

	entriesPerLong := 64 / int(bitsPerEntry)

	for index, state := range blocks.States {
		var entry int32

		if paletteIndex == nil {
			for index, paletteState := range palette {
				if paletteState == state {
					entry = int32(index)

					break
				}
			}
		} else {
			entry = paletteIndex[state]
		}

		longIndex := index / entriesPerLong
		bitOffset := index % entriesPerLong * int(bitsPerEntry)

		data[longIndex] |= int64(entry) << uint(bitOffset)
	}

	return ChunkSection{
		BlockCount: blockCount,
		Biome:      biomeID,

		Palette: palette,
		Data:    data,
	}
}

// SetBiomes replaces the section's biome container with the smallest valid
// palette representation for the provided 4x4x4 biome grid.
func (section *ChunkSection) SetBiomes(biomes *SectionBiomes) {
	firstBiome := biomes.States[0]
	uniform := true

	for _, biome := range biomes.States[1:] {
		if biome != firstBiome {
			uniform = false

			break
		}
	}

	section.Biome = firstBiome
	section.BiomePalette = nil
	section.BiomeData = nil
	section.BiomeDirect = false
	section.BiomeBitsPerEntry = 0

	if uniform {
		return
	}

	palette := make([]int32, 0, 1<<MaxIndirectBiomeBitsPerEntry)
	paletteIndex := make(map[int32]int32, 1<<MaxIndirectBiomeBitsPerEntry)
	indices := [BiomeSectionVolume]int32{}

	direct := false

	for index, biome := range biomes.States {
		entry, known := paletteIndex[biome]
		if !known {
			if len(palette) == 1<<MaxIndirectBiomeBitsPerEntry {
				direct = true

				break
			}

			entry = int32(len(palette))
			paletteIndex[biome] = entry
			palette = append(palette, biome)
		}

		indices[index] = entry
	}

	if direct {
		bitsPerEntry := directBiomeBits(&biomes.States)

		section.BiomeDirect = true
		section.BiomeBitsPerEntry = bitsPerEntry
		section.BiomeData = packBiomeValues(&biomes.States, bitsPerEntry)

		return
	}

	bitsPerEntry := biomePaletteBitsPerEntry(len(palette))

	section.BiomePalette = palette
	section.BiomeBitsPerEntry = bitsPerEntry
	section.BiomeData = packBiomeValues(&indices, bitsPerEntry)
}

func UniformChunkSection(state, biomeID int32) ChunkSection {
	var blockCount int16

	if state != AirBlockState {
		blockCount = SectionVolume
	}

	return ChunkSection{
		BlockCount: blockCount,
		BlockState: state,
		Biome:      biomeID,
	}
}

func paletteBitsPerEntry(paletteSize int) int32 {
	bitsPerEntry := int32(MinBitsPerEntry)

	for int32(1)<<bitsPerEntry < int32(paletteSize) {
		bitsPerEntry++
	}

	return bitsPerEntry
}

func biomePaletteBitsPerEntry(paletteSize int) int32 {
	bitsPerEntry := int32(MinBiomeBitsPerEntry)

	for int32(1)<<bitsPerEntry < int32(paletteSize) {
		bitsPerEntry++
	}

	return bitsPerEntry
}

func directBiomeBits(states *[BiomeSectionVolume]int32) int32 {
	maxBiome := int32(0)

	for _, biome := range states {
		maxBiome = max(maxBiome, biome)
	}

	bitsPerEntry := int32(MinDirectBiomeBitsPerEntry)

	for int32(1)<<bitsPerEntry <= maxBiome {
		bitsPerEntry++
	}

	return bitsPerEntry
}

func packDirectStates(states *[SectionVolume]int32) []int64 {
	data := make([]int64, packedLongCount(SectionVolume, DirectBitsPerEntry))

	entriesPerLong := 64 / DirectBitsPerEntry

	for index, state := range states {
		longIndex := index / entriesPerLong
		bitOffset := index % entriesPerLong * DirectBitsPerEntry

		data[longIndex] |= int64(state) << uint(bitOffset)
	}

	return data
}

func packBiomeValues(values *[BiomeSectionVolume]int32, bitsPerEntry int32) []int64 {
	data := make([]int64, packedLongCount(BiomeSectionVolume, bitsPerEntry))
	entriesPerLong := 64 / int(bitsPerEntry)

	for index, value := range values {
		longIndex := index / entriesPerLong
		bitOffset := index % entriesPerLong * int(bitsPerEntry)

		data[longIndex] |= int64(value) << uint(bitOffset)
	}

	return data
}

func packedLongCount(entries int, bitsPerEntry int32) int {
	entriesPerLong := 64 / int(bitsPerEntry)
	return (entries + entriesPerLong - 1) / entriesPerLong
}
