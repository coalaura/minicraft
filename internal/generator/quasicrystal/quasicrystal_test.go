package quasicrystal

import (
	"math"
	"sync"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

type sectionTestCase struct {
	seed        int64
	chunk       game.ChunkPosition
	sectionMinY int32
}

var sectionTestCases = []sectionTestCase{
	{seed: -1, chunk: game.ChunkPosition{X: -3, Z: 2}, sectionMinY: 32},
	{seed: -1, chunk: game.ChunkPosition{X: -3, Z: 2}, sectionMinY: 48},
	{seed: 0, chunk: game.ChunkPosition{X: 0, Z: 0}, sectionMinY: 64},
	{seed: 987654321, chunk: game.ChunkPosition{X: 7, Z: -5}, sectionMinY: 80},
	{seed: math.MaxInt64, chunk: game.ChunkPosition{X: -11, Z: -9}, sectionMinY: 112},
	{seed: math.MaxInt64, chunk: game.ChunkPosition{X: -11, Z: -9}, sectionMinY: 128},
}

func TestGeneratorIsRegistered(t *testing.T) {
	registered, err := generator.New(Name)
	if err != nil {
		t.Fatalf("create registered generator: %v", err)
	}

	if _, ok := registered.(Generator); !ok {
		t.Fatalf("registered generator type = %T", registered)
	}
}

func TestSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}
	seeds := []int64{math.MinInt64, -1, 0, 1, math.MaxInt64}

	for _, seed := range seeds {
		spawn := generated.Spawn(seed)
		position := game.BlockPosition{X: int32(spawn.X), Y: int32(spawn.Y), Z: int32(spawn.Z)}

		spawnBlock := generated.BlockAt(seed, position)
		if spawnBlock != game.Air {
			t.Fatalf("seed %d spawn block = %d, want air", seed, spawnBlock)
		}

		position.Y--

		belowSpawnBlock := generated.BlockAt(seed, position)
		if belowSpawnBlock == game.Air {
			t.Fatalf("seed %d block below spawn is air", seed)
		}
	}
}

func TestGeneratorIsDeterministicAndSeeded(t *testing.T) {
	generated := Generator{}
	positions := []game.BlockPosition{
		{X: -213, Y: surfaceY, Z: 91},
		{X: -47, Y: surfaceY + 2, Z: -119},
		{X: 73, Y: surfaceY, Z: 164},
		{X: 281, Y: surfaceY + 7, Z: -33},
	}

	for _, position := range positions {
		first := generated.BlockAt(12345, position)
		second := generated.BlockAt(12345, position)

		if first != second {
			t.Fatalf("block at %+v changed from %d to %d", position, first, second)
		}
	}

	seededDifference := false

	for x := int32(-128); x <= 128 && !seededDifference; x += 11 {
		for z := int32(-128); z <= 128; z += 13 {
			first := describeColumn(1, x, z)
			second := describeColumn(2, x, z)

			if first.path != second.path || first.pathFamily != second.pathFamily || first.reliefHeight != second.reliefHeight || math.Abs(first.field-second.field) > 0.000001 {
				seededDifference = true

				break
			}
		}
	}

	if !seededDifference {
		t.Fatal("different seeds produced identical sampled quasicrystal columns")
	}
}

func TestSectionGenerationMatchesBlockAt(t *testing.T) {
	generated := Generator{}

	for _, testCase := range sectionTestCases {
		var legacyBlocks [game.SectionVolume]game.Block
		legacyBlock, legacyUniform := generated.GenerateSection(testCase.seed, testCase.chunk, testCase.sectionMinY, &legacyBlocks)

		prepared := generated.GenerateChunk(testCase.seed, testCase.chunk)
		var preparedBlocks [game.SectionVolume]game.Block
		preparedBlock, preparedUniform := prepared.GenerateSection(testCase.sectionMinY, &preparedBlocks)

		if legacyBlock != preparedBlock || legacyUniform != preparedUniform || legacyBlocks != preparedBlocks {
			t.Fatalf("prepared section differs for seed %d chunk %+v section %d", testCase.seed, testCase.chunk, testCase.sectionMinY)
		}

		assertSectionMatchesBlockAt(t, generated, testCase, preparedBlock, preparedUniform, &preparedBlocks)
	}
}

func TestPreparedSectionsAreDeterministicWhenConcurrent(t *testing.T) {
	generated := Generator{}
	testCase := sectionTestCases[1]
	prepared := generated.GenerateChunk(testCase.seed, testCase.chunk)

	var expectedBlocks [game.SectionVolume]game.Block
	expectedBlock, expectedUniform := prepared.GenerateSection(testCase.sectionMinY, &expectedBlocks)

	var group sync.WaitGroup
	errors := make(chan string, 16)

	for range 16 {
		group.Go(func() {
			for range 8 {
				var blocks [game.SectionVolume]game.Block

				block, uniform := prepared.GenerateSection(testCase.sectionMinY, &blocks)
				if block != expectedBlock || uniform != expectedUniform || blocks != expectedBlocks {
					errors <- "prepared section changed"

					return
				}
			}
		})
	}

	group.Wait()
	close(errors)

	for err := range errors {
		t.Fatal(err)
	}
}

func BenchmarkPreparedChunkSections(b *testing.B) {
	generated := Generator{}
	prepared := generated.GenerateChunk(987654321, game.ChunkPosition{X: -3, Z: 2})
	var blocks [game.SectionVolume]game.Block

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		prepared.GenerateSection(64+int32(iteration%3)*game.ChunkWidth, &blocks)
	}
}

func BenchmarkSectionGeneratorCalls(b *testing.B) {
	generated := Generator{}
	var blocks [game.SectionVolume]game.Block

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		generated.GenerateSection(987654321, game.ChunkPosition{X: -3, Z: 2}, 64+int32(iteration%3)*game.ChunkWidth, &blocks)
	}
}

func TestGenerationBoundsContainAllStructures(t *testing.T) {
	generated := Generator{}

	minY, maxY, ok := generated.GenerationBounds(0, game.ChunkPosition{})
	if !ok {
		t.Fatal("generator reported empty bounds")
	}

	if minY != foundationMinY {
		t.Fatalf("minimum generation Y = %d, want %d", minY, foundationMinY)
	}

	if maxY != maxBuildY {
		t.Fatalf("maximum generation Y = %d, want %d", maxY, maxBuildY)
	}

	if surfaceY+maxStructureHeight > maxY {
		t.Fatalf("maximum structure Y %d exceeds generation bound %d", surfaceY+maxStructureHeight, maxY)
	}
}

func assertSectionMatchesBlockAt(t *testing.T, generated Generator, testCase sectionTestCase, block game.Block, uniform bool, blocks *[game.SectionVolume]game.Block) {
	t.Helper()

	for localY := range int32(game.ChunkWidth) {
		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				worldX := testCase.chunk.X*game.ChunkWidth + localX
				worldY := testCase.sectionMinY + localY
				worldZ := testCase.chunk.Z*game.ChunkWidth + localZ
				index := localY*256 + localZ*16 + localX
				want := generated.BlockAt(testCase.seed, game.BlockPosition{X: worldX, Y: worldY, Z: worldZ})

				if uniform {
					if block != want {
						t.Fatalf("uniform block at (%d, %d, %d) = %d, want %d", worldX, worldY, worldZ, block, want)
					}

					continue
				}

				if blocks[index] != want {
					t.Fatalf("section block at (%d, %d, %d) = %d, want %d", worldX, worldY, worldZ, blocks[index], want)
				}
			}
		}
	}
}
