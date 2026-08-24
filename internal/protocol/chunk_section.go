package protocol

const (
	SectionVolume   = 4096
	MinBitsPerEntry = 4
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
}

// SectionBlocks holds the block states of one chunk section, indexed by
// (y * 256 + z * 16 + x) with local coordinates from 0 to 15.
type SectionBlocks struct {
	States [SectionVolume]int32
}

func (blocks *SectionBlocks) Set(localX, localY, localZ int, state int32) {
	blocks.States[localY*256+localZ*16+localX] = state
}

// ToSection packs the blocks into an encodable chunk section with an
// indirect palette, using the given biome ID for the whole section.
func (blocks *SectionBlocks) ToSection(biomeID int32) ChunkSection {
	palette := make([]int32, 0, 4)
	paletteIndex := make(map[int32]int32, 4)

	var data []int64

	var blockCount int16

	for index, state := range blocks.States {
		if state != AirBlockState {
			blockCount++
		}

		entry, known := paletteIndex[state]
		if !known {
			entry = int32(len(palette))
			paletteIndex[state] = entry
			palette = append(palette, state)
		}

		longIndex := index / 16

		if longIndex >= len(data) {
			data = append(data, 0)
		}

		data[longIndex] |= int64(entry) << uint(index%16*MinBitsPerEntry)
	}

	return ChunkSection{
		BlockCount: blockCount,
		Biome:      biomeID,

		Palette: palette,
		Data:    data,
	}
}

func paletteBitsPerEntry(paletteSize int) int32 {
	bitsPerEntry := int32(MinBitsPerEntry)

	for int32(1)<<bitsPerEntry < int32(paletteSize) {
		bitsPerEntry++
	}

	return bitsPerEntry
}
