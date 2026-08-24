package server

import (
	"reflect"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator/fractalvaults"
	"github.com/coalaura/minicraft/internal/generator/mengersponge"
	"github.com/coalaura/minicraft/internal/generator/spawnplatform"
	"github.com/coalaura/minicraft/internal/generator/waveterrain"
	"github.com/coalaura/minicraft/internal/protocol"
)

type unsupportedBlockGenerator struct{}

func (unsupportedBlockGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return game.MaxBlockState + 1
}

func TestLevelChunksQueryWorldAcrossBoundaries(t *testing.T) {
	world := game.NewOverworld(spawnplatform.New())

	right, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatalf("build right chunk: %v", err)
	}

	left, err := buildLevelChunk(world, -1, 0)
	if err != nil {
		t.Fatalf("build left chunk: %v", err)
	}

	rightAgain, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatalf("rebuild right chunk: %v", err)
	}

	if !reflect.DeepEqual(right, rightAgain) {
		t.Fatal("chunk changed when generated in a different order")
	}

	assertChunkBlockState(t, left, 15, 69, 0, protocol.StoneBlockState)
	assertChunkBlockState(t, right, 0, 69, 0, protocol.StoneBlockState)
	assertChunkBlockState(t, right, 4, 69, 0, protocol.StoneBlockState)
	assertChunkBlockState(t, right, 5, 69, 0, protocol.AirBlockState)
}

func TestLevelChunksIncludeWorldOverrides(t *testing.T) {
	world := game.NewOverworld(spawnplatform.New())

	world.SetBlock(game.BlockPosition{X: 0, Y: 69, Z: 0}, game.Air)
	world.SetBlock(game.BlockPosition{X: 5, Y: 69, Z: 0}, game.Stone)

	chunk, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatalf("build overridden chunk: %v", err)
	}

	assertChunkBlockState(t, chunk, 0, 69, 0, protocol.AirBlockState)
	assertChunkBlockState(t, chunk, 5, 69, 0, protocol.StoneBlockState)
}

func TestBulkGeneratorsMatchBlockAt(t *testing.T) {
	tests := []struct {
		name       string
		generator  game.Generator
		seed       int64
		chunks     []game.ChunkPosition
		sectionMin []int32
	}{
		{
			name:       "spawn platform",
			generator:  spawnplatform.New(),
			chunks:     []game.ChunkPosition{{}, {X: -1}, {X: 2, Z: 2}},
			sectionMin: []int32{48, 64, 80},
		},
		{
			name:       "wave terrain",
			generator:  waveterrain.New(),
			seed:       -918273645,
			chunks:     []game.ChunkPosition{{}, {X: -3, Z: 5}},
			sectionMin: []int32{-64, 48, 64, 80},
		},
		{
			name:       "Menger sponge",
			generator:  mengersponge.New(),
			chunks:     []game.ChunkPosition{{}, {X: -7, Z: 11}},
			sectionMin: []int32{-80, -70, -64, 0, 304, 310, 320},
		},
		{
			name:       "fractal vaults",
			generator:  fractalvaults.New(),
			seed:       123456789,
			chunks:     []game.ChunkPosition{{}, {X: -4, Z: 9}},
			sectionMin: []int32{48, 64, 80, 96},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bulk, ok := test.generator.(game.SectionGenerator)
			if !ok {
				t.Fatal("generator does not implement SectionGenerator")
			}

			for _, chunk := range test.chunks {
				for _, sectionMinY := range test.sectionMin {
					assertBulkSectionMatches(t, test.generator, bulk, test.seed, chunk, sectionMinY)
				}
			}
		})
	}
}

func TestChunkBatchValidatesGeneratedBlocksBeforeWriting(t *testing.T) {
	session, connection := newChunkTestSession(game.Position{})

	session.Runtime.World = &game.World{Generator: unsupportedBlockGenerator{}}

	sent, err := session.sendChunkBatch([]LoadedChunk{{}})
	if err == nil {
		t.Fatal("send chunk batch with unsupported block succeeded")
	}

	if sent != 0 {
		t.Fatalf("sent chunk count = %d, want 0", sent)
	}

	if packets := connection.packets(t); len(packets) != 0 {
		t.Fatalf("invalid chunk wrote packets: %v", connection.packetIDs(t))
	}
}

func assertChunkBlockState(t *testing.T, chunk protocol.LevelChunkWithLight, localX int, worldY int, localZ int, expected int32) {
	t.Helper()

	sectionIndex := (worldY - protocol.OverworldMinY) / ChunkWidth
	localY := (worldY - protocol.OverworldMinY) % ChunkWidth
	section := chunk.Sections[sectionIndex]

	actual := section.BlockState

	if len(section.Palette) != 0 {
		blockIndex := localY*256 + localZ*16 + localX
		packed := section.Data[blockIndex/16]
		paletteIndex := packed >> uint(blockIndex%16*protocol.MinBitsPerEntry) & 0x0F
		actual = section.Palette[paletteIndex]
	}

	if actual != expected {
		t.Fatalf(
			"block (%d, %d, %d) state = %d, want %d",
			localX,
			worldY,
			localZ,
			actual,
			expected,
		)
	}
}

func assertBulkSectionMatches(t *testing.T, generator game.Generator, bulk game.SectionGenerator, seed int64, chunk game.ChunkPosition, sectionMinY int32) {
	t.Helper()

	var blocks [game.SectionVolume]game.Block

	uniformBlock, uniform := bulk.GenerateSection(seed, chunk, sectionMinY, &blocks)

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for localY := range int32(game.ChunkWidth) {
		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				expected := generator.BlockAt(seed, game.BlockPosition{
					X: chunkMinX + localX,
					Y: sectionMinY + localY,
					Z: chunkMinZ + localZ,
				})

				actual := uniformBlock
				if !uniform {
					actual = blocks[localY*256+localZ*16+localX]
				}

				if actual != expected {
					t.Fatalf(
						"chunk %+v section %d local (%d, %d, %d) = %d, want %d",
						chunk,
						sectionMinY,
						localX,
						localY,
						localZ,
						actual,
						expected,
					)
				}
			}
		}
	}
}
