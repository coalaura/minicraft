package babel

import (
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

type generatedSectionSnapshot struct {
	block   game.Block
	uniform bool
	blocks  [game.SectionVolume]game.Block
}

var benchmarkSectionBlocks [game.SectionVolume]game.Block

func TestGeneratorRegisters(t *testing.T) {
	generated, err := generator.New(Name)
	if err != nil {
		t.Fatalf("create babel generator: %v", err)
	}

	if generated == nil {
		t.Fatal("babel generator is nil")
	}
}

func TestGeneratorSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}

	for _, seed := range []int64{0, 1, -1, 123456789, -987654321} {
		spawn := generated.Spawn(seed)

		position := game.BlockPosition{X: int32(spawn.X), Y: int32(spawn.Y), Z: int32(spawn.Z)}

		block := generated.BlockAt(seed, position)
		if block != game.Air {
			t.Fatalf("seed %d spawn block = %d, want air", seed, block)
		}

		position.Y++

		block = generated.BlockAt(seed, position)
		if block != game.Air {
			t.Fatalf("seed %d block above spawn = %d, want air", seed, block)
		}

		position.Y -= 2

		block = generated.BlockAt(seed, position)
		if block == game.Air {
			t.Fatalf("seed %d block below spawn is air", seed)
		}
	}
}

func TestGeneratorHasRecursiveStreetHierarchy(t *testing.T) {
	generated := Generator{}

	seed := int64(42)

	originX, originZ := cityOrigins(seed)

	grand := game.BlockPosition{X: int32(originX), Y: baseFloorY, Z: int32(originZ + 29)}
	boulevard := game.BlockPosition{X: int32(originX + boulevardScale), Y: baseFloorY, Z: int32(originZ + 29)}
	street := game.BlockPosition{X: int32(originX + lotScale), Y: baseFloorY, Z: int32(originZ + 29)}

	block := generated.BlockAt(seed, grand)
	if block != game.BlackConcrete {
		t.Fatalf("grand avenue block = %d, want black concrete", block)
	}

	block = generated.BlockAt(seed, boulevard)
	if block != game.GrayConcrete {
		t.Fatalf("boulevard block = %d, want gray concrete", block)
	}

	block = generated.BlockAt(seed, street)
	if block != game.GrayConcrete {
		t.Fatalf("street block = %d, want gray concrete", block)
	}
}

func TestGeneratorUsesArchitecturalMaterialVariety(t *testing.T) {
	generated := Generator{}

	seed := int64(1337)

	originX, originZ := cityOrigins(seed)

	seen := make(map[game.Block]struct{})

	for y := baseFloorY; y <= 184; y += 4 {
		for z := int64(0); z < districtScale*2; z += 4 {
			for x := int64(0); x < districtScale*2; x += 4 {
				block := generated.BlockAt(seed, game.BlockPosition{
					X: int32(originX + x),
					Y: y,
					Z: int32(originZ + z),
				})

				if block != game.Air {
					seen[block] = struct{}{}
				}
			}
		}
	}

	if len(seen) < 12 {
		t.Fatalf("sampled only %d non-air block types, want at least 12", len(seen))
	}

	glassFound := false

	for _, glass := range []game.Block{
		game.LightBlueStainedGlass,
		game.GrayStainedGlass,
		game.PurpleStainedGlass,
		game.CyanStainedGlass,
		game.OrangeStainedGlass,
		game.MagentaStainedGlass,
		game.BlueStainedGlass,
	} {
		if _, ok := seen[glass]; ok {
			glassFound = true

			break
		}
	}

	if !glassFound {
		t.Fatal("sample contains no stained-glass facade blocks")
	}
}

func TestGenerateSectionMatchesBlockAt(t *testing.T) {
	generated := Generator{}

	seed := int64(-24680)

	originX, originZ := cityOrigins(seed)

	chunk := game.ChunkPosition{
		X: blockChunkCoordinate(int32(originX + lotScale + 12)),
		Z: blockChunkCoordinate(int32(originZ + lotScale + 12)),
	}

	sectionMinY := int32(80)

	var blocks [game.SectionVolume]game.Block

	_, uniform := generated.GenerateSection(seed, chunk, sectionMinY, &blocks)
	if uniform {
		t.Fatal("sample building section unexpectedly reported as uniform")
	}

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for localY := range int32(game.ChunkWidth) {
		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				position := game.BlockPosition{
					X: chunkMinX + localX,
					Y: sectionMinY + localY,
					Z: chunkMinZ + localZ,
				}

				index := localY*256 + localZ*16 + localX

				want := generated.BlockAt(seed, position)

				if blocks[index] != want {
					t.Fatalf("section block at %+v = %d, want %d", position, blocks[index], want)
				}
			}
		}
	}
}

func TestGeneratedChunkMatchesSectionAndBlockAt(t *testing.T) {
	tests := []struct {
		seed  int64
		chunk game.ChunkPosition
	}{
		{seed: 0, chunk: game.ChunkPosition{X: 0, Z: 0}},
		{seed: -24680, chunk: game.ChunkPosition{X: -19, Z: 23}},
		{seed: 987654321, chunk: game.ChunkPosition{X: 31, Z: -37}},
		{seed: math.MinInt64, chunk: game.ChunkPosition{X: math.MaxInt32, Z: math.MinInt32}},
	}

	sectionMinYs := []int32{-64, 48, 64, 96, 256, 272}

	generator := Generator{}

	for _, test := range tests {
		t.Run(fmt.Sprintf("seed_%d_chunk_%d_%d", test.seed, test.chunk.X, test.chunk.Z), func(t *testing.T) {
			prepared := generator.GenerateChunk(test.seed, test.chunk)

			for _, sectionMinY := range sectionMinYs {
				var preparedBlocks [game.SectionVolume]game.Block

				preparedBlock, preparedUniform := prepared.GenerateSection(sectionMinY, &preparedBlocks)

				var sectionBlocks [game.SectionVolume]game.Block

				sectionBlock, sectionUniform := generator.GenerateSection(test.seed, test.chunk, sectionMinY, &sectionBlocks)

				if preparedBlock != sectionBlock || preparedUniform != sectionUniform {
					t.Fatalf("section y=%d result = (%d, %t), fallback = (%d, %t)", sectionMinY, preparedBlock, preparedUniform, sectionBlock, sectionUniform)
				}

				if !preparedUniform && preparedBlocks != sectionBlocks {
					t.Fatalf("section y=%d prepared blocks differ from fallback", sectionMinY)
				}

				assertSectionMatchesBlockAt(t, generator, test.seed, test.chunk, sectionMinY, preparedBlock, preparedUniform, &preparedBlocks)
			}
		})
	}
}

func TestGeneratedChunksAreDeterministicAcrossOrderAndConcurrency(t *testing.T) {
	seed := int64(-7046029254386353131)

	chunks := []game.ChunkPosition{
		{X: -25, Z: -17},
		{X: 0, Z: 0},
		{X: 19, Z: -31},
		{X: 48, Z: 64},
	}

	generator := Generator{}

	want := make([]generatedSectionSnapshot, len(chunks))

	for index, chunk := range chunks {
		want[index] = snapshotGeneratedSection(generator.GenerateChunk(seed, chunk), 96)
	}

	got := make([]generatedSectionSnapshot, len(chunks))

	var waitGroup sync.WaitGroup

	waitGroup.Add(len(chunks))

	for reverseIndex := range chunks {
		index := len(chunks) - 1 - reverseIndex

		go func() {
			defer waitGroup.Done()

			got[index] = snapshotGeneratedSection(generator.GenerateChunk(seed, chunks[index]), 96)
		}()
	}

	waitGroup.Wait()

	for index := range chunks {
		if got[index] != want[index] {
			t.Fatalf("chunk %+v changed across generation order or concurrency", chunks[index])
		}
	}
}

func BenchmarkChunkSections(b *testing.B) {
	generator := Generator{}

	seed := int64(42)

	chunk := game.ChunkPosition{X: 17, Z: -23}

	sectionMinYs := make([]int32, 0, 24)

	for sectionMinY := int32(-64); sectionMinY <= 304; sectionMinY += game.ChunkWidth {
		sectionMinYs = append(sectionMinYs, sectionMinY)
	}

	b.Run("prepared_chunk", func(b *testing.B) {
		prepared := generator.GenerateChunk(seed, chunk)

		b.ReportAllocs()

		for b.Loop() {
			for _, sectionMinY := range sectionMinYs {
				prepared.GenerateSection(sectionMinY, &benchmarkSectionBlocks)
			}
		}
	})

	b.Run("repeated_section_preparation", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			for _, sectionMinY := range sectionMinYs {
				generator.GenerateSection(seed, chunk, sectionMinY, &benchmarkSectionBlocks)
			}
		}
	})
}

func TestGeneratorIsDeterministicAcrossNegativeCoordinates(t *testing.T) {
	generated := Generator{}

	seed := int64(987654321)

	positions := []game.BlockPosition{
		{X: -4096, Y: 64, Z: -4096},
		{X: -1234, Y: 97, Z: 567},
		{X: 812, Y: 143, Z: -991},
		{X: 2048, Y: 86, Z: 2048},
	}

	for _, position := range positions {
		first := generated.BlockAt(seed, position)
		second := generated.BlockAt(seed, position)

		if first != second {
			t.Fatalf("block at %+v changed from %d to %d", position, first, second)
		}
	}
}

func TestGenerationBounds(t *testing.T) {
	generated := Generator{}

	minY, maxY, ok := generated.GenerationBounds(0, game.ChunkPosition{X: -1000, Z: 1000})
	if !ok {
		t.Fatal("babel unexpectedly reported an empty chunk")
	}

	if minY != foundationMinY || maxY != maxBuildY {
		t.Fatalf("bounds = %d..%d, want %d..%d", minY, maxY, foundationMinY, maxBuildY)
	}
}

func assertSectionMatchesBlockAt(t *testing.T, generator Generator, seed int64, chunk game.ChunkPosition, sectionMinY int32, uniformBlock game.Block, uniform bool, blocks *[game.SectionVolume]game.Block) {
	t.Helper()

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for localY := range int32(game.ChunkWidth) {
		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				position := game.BlockPosition{
					X: chunkMinX + localX,
					Y: sectionMinY + localY,
					Z: chunkMinZ + localZ,
				}

				want := generator.BlockAt(seed, position)

				got := uniformBlock

				if !uniform {
					got = blocks[localY*256+localZ*16+localX]
				}

				if got != want {
					t.Fatalf("block at %+v = %d, want %d", position, got, want)
				}
			}
		}
	}
}

func snapshotGeneratedSection(generated game.GeneratedChunk, sectionMinY int32) generatedSectionSnapshot {
	var snapshot generatedSectionSnapshot

	snapshot.block, snapshot.uniform = generated.GenerateSection(sectionMinY, &snapshot.blocks)

	return snapshot
}

func blockChunkCoordinate(coordinate int32) int32 {
	chunk := coordinate / game.ChunkWidth
	if coordinate%game.ChunkWidth < 0 {
		chunk--
	}

	return chunk
}
