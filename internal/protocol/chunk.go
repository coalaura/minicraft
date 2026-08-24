package protocol

import "bytes"

const (
	OverworldSectionCount      = 24
	OverworldLightSectionCount = OverworldSectionCount + 2
	SkyLightArrayLength        = 2048
)

type ChunkPosition struct {
	X int32
	Z int32
}

type ChunkSection struct {
	BlockCount int16
	BlockState int32
	Biome      int32
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

func (p LevelChunkWithLight) Encode(wr *PacketWriter) {
	wr.Int(p.Position.X)
	wr.Int(p.Position.Z)

	// Heightmaps: none, client falls back to minimum heights.
	wr.VarInt(0)

	var sections PacketWriter

	for _, section := range p.Sections {
		sections.Short(section.BlockCount)

		// Block states: bits per entry 0, single-value palette.
		sections.Byte(0)
		sections.VarInt(section.BlockState)

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

	skyLight := make([][]byte, OverworldLightSectionCount)

	fullBright := bytes.Repeat([]byte{0xFF}, SkyLightArrayLength)

	for i := range skyLight {
		skyLight[i] = fullBright
	}

	return LevelChunkWithLight{
		Position: ChunkPosition{X: x, Z: z},
		Sections: sections,

		SkyLightMask: []int64{(int64(1) << OverworldLightSectionCount) - 1},

		SkyLight: skyLight,
	}
}
