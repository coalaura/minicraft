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

type LevelChunkBlockEntity struct {
	X    byte
	Z    byte
	Y    int16
	Type int32
	Data []byte
}

type LevelChunkWithLight struct {
	Position      ChunkPosition
	Sections      []ChunkSection
	BlockEntities []LevelChunkBlockEntity

	SkyLightMask        []int64
	BlockLightMask      []int64
	EmptySkyLightMask   []int64
	EmptyBlockLightMask []int64

	SkyLight   [][]byte
	BlockLight [][]byte
}

type UpdateLight struct {
	Position ChunkPosition

	SkyLightMask        []int64
	BlockLightMask      []int64
	EmptySkyLightMask   []int64
	EmptyBlockLightMask []int64

	SkyLight   [][]byte
	BlockLight [][]byte
}

var (
	fullBrightSkyLight    = newFullBrightSkyLight()
	overworldSkyLight     = newOverworldSkyLight(OverworldLightSectionCount)
	openOverworldSkyLight = newOverworldSkyLight(OverworldLightSectionCount - 1)
	overworldLightMask    = []int64{(int64(1) << OverworldLightSectionCount) - 1}
	openSkyLightMask      = []int64{overworldLightMask[0] &^ 1}
	bottomLightMask       = []int64{1}
	emptyLightMask        = []int64{overworldLightMask[0]}
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

		encodeSectionBiomes(&sections, section)
	}

	err := sections.Err()
	if err != nil {
		wr.err = err

		return
	}

	// Chunk data: one length-prefixed byte array holding all sections.
	wr.Bytes(sections.Buffer.Bytes())

	wr.VarInt(int32(len(p.BlockEntities)))

	for _, entity := range p.BlockEntities {
		entity.Encode(wr)
	}

	encodeLightData(wr, p.SkyLightMask, p.BlockLightMask, p.EmptySkyLightMask, p.EmptyBlockLightMask, p.SkyLight, p.BlockLight)
}

func (p LevelChunkBlockEntity) Encode(wr *PacketWriter) {
	wr.Byte(p.X<<4 | p.Z)
	wr.Short(p.Y)
	wr.VarInt(p.Type)

	if len(p.Data) == 0 {
		// Optional anonymous NBT is absent when its root tag is TAG_End.
		wr.Byte(0)

		return
	}

	wr.Raw(p.Data)
}

func (p UpdateLight) Encode(wr *PacketWriter) {
	wr.VarInt(p.Position.X)
	wr.VarInt(p.Position.Z)

	encodeLightData(wr, p.SkyLightMask, p.BlockLightMask, p.EmptySkyLightMask, p.EmptyBlockLightMask, p.SkyLight, p.BlockLight)
}

func encodeLightData(wr *PacketWriter, skyMask, blockMask, emptySkyMask, emptyBlockMask []int64, skyLight, blockLight [][]byte) {
	wr.VarInt(int32(len(skyMask)))

	for _, mask := range skyMask {
		wr.Long(mask)
	}

	wr.VarInt(int32(len(blockMask)))

	for _, mask := range blockMask {
		wr.Long(mask)
	}

	wr.VarInt(int32(len(emptySkyMask)))

	for _, mask := range emptySkyMask {
		wr.Long(mask)
	}

	wr.VarInt(int32(len(emptyBlockMask)))

	for _, mask := range emptyBlockMask {
		wr.Long(mask)
	}

	wr.VarInt(int32(len(skyLight)))

	for _, array := range skyLight {
		wr.Bytes(array)
	}

	wr.VarInt(int32(len(blockLight)))

	for _, array := range blockLight {
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

func NewOpenOverworldLight(x, z int32) UpdateLight {
	return UpdateLight{
		Position:            ChunkPosition{X: x, Z: z},
		SkyLightMask:        openSkyLightMask,
		BlockLightMask:      []int64{0},
		EmptySkyLightMask:   bottomLightMask,
		EmptyBlockLightMask: emptyLightMask,
		SkyLight:            openOverworldSkyLight,
	}
}

func encodeSectionBiomes(wr *PacketWriter, section ChunkSection) {
	if len(section.BiomePalette) == 0 && !section.BiomeDirect {
		wr.Byte(0)
		wr.VarInt(section.Biome)

		return
	}

	wr.Byte(byte(section.BiomeBitsPerEntry))

	if !section.BiomeDirect {
		wr.VarInt(int32(len(section.BiomePalette)))

		for _, biome := range section.BiomePalette {
			wr.VarInt(biome)
		}
	}

	for _, value := range section.BiomeData {
		wr.Long(value)
	}
}

func newFullBrightSkyLight() []byte {
	light := make([]byte, SkyLightArrayLength)

	for index := range light {
		light[index] = 0xff
	}

	return light
}

func newOverworldSkyLight(sectionCount int) [][]byte {
	light := make([][]byte, sectionCount)

	for index := range light {
		light[index] = fullBrightSkyLight
	}

	return light
}
