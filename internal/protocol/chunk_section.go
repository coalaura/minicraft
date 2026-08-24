package protocol

const (
	SectionVolume           = 4096
	MinBitsPerEntry         = 4
	MaxIndirectBitsPerEntry = 8
	DirectBitsPerEntry      = 15
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
}

// SectionBlocks holds the block states of one chunk section, indexed by
// (y * 256 + z * 16 + x) with local coordinates from 0 to 15.
type SectionBlocks struct {
	States [SectionVolume]int32
}

func (blocks *SectionBlocks) Set(localX, localY, localZ int, state int32) {
	blocks.States[localY*256+localZ*16+localX] = state
}

// ToSection packs the blocks into the smallest supported palette form, using
// the given biome ID for the whole section.
func (blocks *SectionBlocks) ToSection(biomeID int32) ChunkSection {
	firstState := blocks.States[0]
	var blockCount int16

	uniform := true

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

func packedLongCount(entries int, bitsPerEntry int32) int {
	entriesPerLong := 64 / int(bitsPerEntry)
	return (entries + entriesPerLong - 1) / entriesPerLong
}
