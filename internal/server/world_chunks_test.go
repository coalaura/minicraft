package server

import (
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator/fractalvaults"
	"github.com/coalaura/minicraft/internal/generator/mengersponge"
	"github.com/coalaura/minicraft/internal/generator/spawnplatform"
	"github.com/coalaura/minicraft/internal/generator/waveterrain"
	"github.com/coalaura/minicraft/internal/protocol"
)

type unsupportedBlockGenerator struct{}

type countingChunkGenerator struct {
	chunkCalls   atomic.Int32
	blockCalls   atomic.Int32
	sectionCalls atomic.Int32
	biomeCalls   atomic.Int32
}

type countingGeneratedChunk struct {
	generator *countingChunkGenerator
}

type bulkGeneratorTestCase struct {
	name       string
	generator  game.Generator
	seed       int64
	chunks     []game.ChunkPosition
	sectionMin []int32
}

func (unsupportedBlockGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return game.MaxBlockState + 1
}

func (generator *countingChunkGenerator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	generator.blockCalls.Add(1)

	if position.Y == 0 {
		return game.Stone
	}

	return game.Air
}

func (generator *countingChunkGenerator) GenerateChunk(_ int64, _ game.ChunkPosition) game.GeneratedChunk {
	generator.chunkCalls.Add(1)

	return &countingGeneratedChunk{generator: generator}
}

func (generator *countingChunkGenerator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return protocol.OverworldMinY, protocol.OverworldMinY + protocol.OverworldSectionCount*game.ChunkWidth - 1, true
}

func (generated *countingGeneratedChunk) GenerateSection(sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	generated.generator.sectionCalls.Add(1)

	if sectionMinY != 0 {
		return game.Air, true
	}

	clear(blocks[:])

	for localZ := range game.ChunkWidth {
		for localX := range game.ChunkWidth {
			blocks[localZ*game.ChunkWidth+localX] = game.Stone
		}
	}

	return game.Air, false
}

func (generated *countingGeneratedChunk) BiomeAt(_, _, _ int32) game.Biome {
	generated.generator.biomeCalls.Add(1)

	return game.BiomePlains
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

func TestWholeChunkGenerationIsPreparedOnceAndIncludesOverrides(t *testing.T) {
	generator := &countingChunkGenerator{}

	world := game.NewOverworld(generator, 42)

	world.SetBlocks([]game.BlockChange{
		{Position: game.BlockPosition{X: 3, Y: 0, Z: 4}, Replacement: game.Air},
		{Position: game.BlockPosition{X: 5, Y: 1, Z: 6}, Replacement: game.Stone},
	})

	generator.blockCalls.Store(0)

	chunk, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	calls := generator.chunkCalls.Load()
	if calls != 1 {
		t.Fatalf("GenerateChunk calls = %d, want 1", calls)
	}

	calls = generator.sectionCalls.Load()
	if calls != protocol.OverworldSectionCount {
		t.Fatalf("GenerateSection calls = %d, want %d", calls, protocol.OverworldSectionCount)
	}

	calls = generator.biomeCalls.Load()
	if calls != protocol.OverworldSectionCount*protocol.BiomeSectionVolume {
		t.Fatalf("BiomeAt calls = %d, want %d", calls, protocol.OverworldSectionCount*protocol.BiomeSectionVolume)
	}

	calls = generator.blockCalls.Load()
	if calls != 0 {
		t.Fatalf("BlockAt calls = %d, want 0", calls)
	}

	assertChunkBlockState(t, chunk, 2, 0, 4, protocol.StoneBlockState)
	assertChunkBlockState(t, chunk, 3, 0, 4, protocol.AirBlockState)
	assertChunkBlockState(t, chunk, 5, 1, 6, protocol.StoneBlockState)

	for sectionIndex, section := range chunk.Sections {
		if section.Biome != int32(game.BiomePlains) {
			t.Fatalf("section %d biome = %d, want %d", sectionIndex, section.Biome, game.BiomePlains)
		}
	}
}

func TestNormalLightingPreparesEachContextChunkOnce(t *testing.T) {
	generator := &countingChunkGenerator{}

	world := game.NewOverworld(generator, 42)

	world.SetLightingMode(game.LightingNormal)

	chunk, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	calls := generator.chunkCalls.Load()
	if calls != 9 {
		t.Fatalf("GenerateChunk calls = %d, want 9", calls)
	}

	calls = generator.sectionCalls.Load()
	if calls != 9*protocol.OverworldSectionCount {
		t.Fatalf("GenerateSection calls = %d, want %d", calls, 9*protocol.OverworldSectionCount)
	}

	calls = generator.biomeCalls.Load()
	if calls != protocol.OverworldSectionCount*protocol.BiomeSectionVolume {
		t.Fatalf("BiomeAt calls = %d, want %d", calls, protocol.OverworldSectionCount*protocol.BiomeSectionVolume)
	}

	calls = generator.blockCalls.Load()
	if calls != 0 {
		t.Fatalf("BlockAt calls = %d, want 0", calls)
	}

	assertChunkBlockState(t, chunk, 2, 0, 4, protocol.StoneBlockState)
}

func TestBulkGeneratorsMatchBlockAt(t *testing.T) {
	tests := []bulkGeneratorTestCase{
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

	packets := connection.packets(t)
	if len(packets) != 0 {
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
