package waveterrain

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
	{seed: -1, chunk: game.ChunkPosition{X: -3, Z: 2}, sectionMinY: 48},
	{seed: 0, chunk: game.ChunkPosition{X: 0, Z: 0}, sectionMinY: 64},
	{seed: 987654321, chunk: game.ChunkPosition{X: 7, Z: -5}, sectionMinY: 80},
	{seed: math.MaxInt64, chunk: game.ChunkPosition{X: -11, Z: -9}, sectionMinY: -16},
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

func TestTerrainIsDeterministicAndSeeded(t *testing.T) {
	positions := []game.BlockPosition{
		{X: math.MinInt32, Y: 64, Z: math.MaxInt32},
		{X: -17, Y: 63, Z: -16},
		{X: 0, Y: 60, Z: 0},
		{X: 16, Y: 70, Z: 17},
	}

	generated := Generator{}
	for _, position := range positions {
		first := generated.BlockAt(8, position)
		second := generated.BlockAt(8, position)
		if first != second {
			t.Fatalf("block at %+v changed from %d to %d", position, first, second)
		}
	}

	if surfaceHeight(0, 12, 0) == surfaceHeight(8, 12, 0) {
		t.Fatal("different seeds produced the same test surface height")
	}
}

func TestTerrainFillsThroughItsSurface(t *testing.T) {
	const seed = int64(1234)

	generated := Generator{}
	height := surfaceHeight(seed, -16, 16)

	if block := generated.BlockAt(seed, game.BlockPosition{X: -16, Y: height, Z: 16}); block != game.Stone {
		t.Fatalf("surface block = %d, want stone", block)
	}

	if block := generated.BlockAt(seed, game.BlockPosition{X: -16, Y: height + 1, Z: 16}); block != game.Air {
		t.Fatalf("block above surface = %d, want air", block)
	}
}

func TestEverySeedProvidesSafeSpawn(t *testing.T) {
	seeds := []int64{math.MinInt64, -34359738448, -1, 0, 1, 34359738448, math.MaxInt64}

	for _, seed := range seeds {
		if height := surfaceHeight(seed, 0, 0); height != 69 {
			t.Errorf("seed %d spawn surface height = %d, want 69", seed, height)
		}
	}
}

func TestPreparedSectionsMatchSectionGeneratorAndBlockAt(t *testing.T) {
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
		group.Add(1)

		go func() {
			defer group.Done()

			for range 8 {
				var blocks [game.SectionVolume]game.Block

				block, uniform := prepared.GenerateSection(testCase.sectionMinY, &blocks)
				if block != expectedBlock || uniform != expectedUniform || blocks != expectedBlocks {
					errors <- "prepared section changed"

					return
				}
			}
		}()
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

	for iteration := 0; b.Loop(); iteration++ {
		prepared.GenerateSection(64+int32(iteration%3)*game.ChunkWidth, &blocks)
	}
}

func BenchmarkSectionGeneratorCalls(b *testing.B) {
	generated := Generator{}

	var blocks [game.SectionVolume]game.Block

	b.ReportAllocs()

	for iteration := 0; b.Loop(); iteration++ {
		generated.GenerateSection(987654321, game.ChunkPosition{X: -3, Z: 2}, 64+int32(iteration%3)*game.ChunkWidth, &blocks)
	}
}

func TestTerrainIsContinuousAcrossChunkBoundaries(t *testing.T) {
	boundaries := []int32{math.MinInt32, -49, -48, -33, -32, -17, -16, -1, 0, 15, 16, 31, 32, 47, 48, math.MaxInt32 - 1}
	seeds := []int64{math.MinInt64, -1, 0, 42, math.MaxInt64}

	for _, seed := range seeds {
		for _, coordinate := range boundaries {
			left := surfaceHeight(seed, coordinate, coordinate)

			xRight := surfaceHeight(seed, coordinate+1, coordinate)
			zRight := surfaceHeight(seed, coordinate, coordinate+1)

			assertHeightDifference(t, "x", seed, coordinate, left, xRight)
			assertHeightDifference(t, "z", seed, coordinate, left, zRight)
		}
	}
}

func assertHeightDifference(t *testing.T, axis string, seed int64, coordinate, first, second int32) {
	t.Helper()

	difference := first - second
	if difference < 0 {
		difference = -difference
	}

	if difference > 1 {
		t.Errorf("seed %d surface jumps from %d to %d across %s=%d", seed, first, second, axis, coordinate)
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
