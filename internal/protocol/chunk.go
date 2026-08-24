package protocol

const (
	OverworldSectionCount      = 24
	OverworldLightSectionCount = OverworldSectionCount + 2
	SkyLightArrayLength        = 2048

	OverworldMinY = -64
)

type ChunkPosition struct {
	X int32
	Z int32
}

type LevelChunkWithLight struct {
	Position ChunkPosition
	Sections []ChunkSection

	SkyLightMask        []int64
	BlockLightMask      []int64
	EmptySkyLightMask   []int64
	EmptyBlockLightMask []int64

	SkyLight   [][]byte
	BlockLight [][]byte
}

var (
	fullBrightSkyLight = newFullBrightSkyLight()
	overworldSkyLight  = newOverworldSkyLight()
	overworldLightMask = []int64{(int64(1) << OverworldLightSectionCount) - 1}
)

func (p LevelChunkWithLight) Encode(wr *PacketWriter) {
	wr.Int(p.Position.X)
	wr.Int(p.Position.Z)

	// Heightmaps: none, client falls back to minimum heights.
	wr.VarInt(0)

	var sections PacketWriter

	for _, section := range p.Sections {
		sections.Short(section.BlockCount)

		if len(section.Palette) == 0 {
			// Block states: bits per entry 0, single-value palette.
			sections.Byte(0)
			sections.VarInt(section.BlockState)
		} else if section.Direct {
			sections.Byte(DirectBitsPerEntry)

			for _, value := range section.Data {
				sections.Long(value)
			}
		} else {
			bitsPerEntry := paletteBitsPerEntry(len(section.Palette))

			// Block states: indirect palette with bit-packed entries.
			// Since 1.21.5 the data array has no length prefix; its
			// size is derived from the bits per entry.
			sections.Byte(byte(bitsPerEntry))

			sections.VarInt(int32(len(section.Palette)))

			for _, state := range section.Palette {
				sections.VarInt(state)
			}

			for _, value := range section.Data {
				sections.Long(value)
			}
		}

		// Biomes: bits per entry 0, single-value palette.
		sections.Byte(0)
		sections.VarInt(section.Biome)
	}

	err := sections.Err()
	if err != nil {
		wr.err = err

		return
	}

	// Chunk data: one length-prefixed byte array holding all sections.
	wr.Bytes(sections.Buffer.Bytes())

	// Block entities: none.
	wr.VarInt(0)

	wr.VarInt(int32(len(p.SkyLightMask)))

	for _, mask := range p.SkyLightMask {
		wr.Long(mask)
	}

	wr.VarInt(int32(len(p.BlockLightMask)))

	for _, mask := range p.BlockLightMask {
		wr.Long(mask)
	}

	wr.VarInt(int32(len(p.EmptySkyLightMask)))

	for _, mask := range p.EmptySkyLightMask {
		wr.Long(mask)
	}

	wr.VarInt(int32(len(p.EmptyBlockLightMask)))

	for _, mask := range p.EmptyBlockLightMask {
		wr.Long(mask)
	}

	wr.VarInt(int32(len(p.SkyLight)))

	for _, array := range p.SkyLight {
		wr.Bytes(array)
	}

	wr.VarInt(int32(len(p.BlockLight)))

	for _, array := range p.BlockLight {
		wr.Bytes(array)
	}
}

func NewEmptyOverworldChunk(x, z int32, biomeID int32) LevelChunkWithLight {
	sections := make([]ChunkSection, OverworldSectionCount)

	for i := range sections {
		sections[i] = ChunkSection{Biome: biomeID}
	}

	return LevelChunkWithLight{
		Position: ChunkPosition{X: x, Z: z},
		Sections: sections,

		SkyLightMask: overworldLightMask,

		SkyLight: overworldSkyLight,
	}
}

func newFullBrightSkyLight() []byte {
	light := make([]byte, SkyLightArrayLength)

	for index := range light {
		light[index] = 0xff
	}

	return light
}

func newOverworldSkyLight() [][]byte {
	light := make([][]byte, OverworldLightSectionCount)

	for index := range light {
		light[index] = fullBrightSkyLight
	}

	return light
}
