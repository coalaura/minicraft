package quasicrystal

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

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

		if block := generated.BlockAt(seed, position); block != game.Air {
			t.Fatalf("seed %d spawn block = %d, want air", seed, block)
		}

		position.Y--
		if block := generated.BlockAt(seed, position); block == game.Air {
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
	seed := int64(987654321)
	chunks := []game.ChunkPosition{
		{X: -3, Z: 2},
		{X: 0, Z: 0},
		{X: 7, Z: -5},
	}
	sectionHeights := []int32{48, 64, 80, 96, 112}

	for _, chunk := range chunks {
		for _, sectionMinY := range sectionHeights {
			var blocks [game.SectionVolume]game.Block

			block, uniform := generated.GenerateSection(seed, chunk, sectionMinY, &blocks)
			if uniform {
				for index, generatedBlock := range blocks {
					if generatedBlock != 0 && generatedBlock != block {
						t.Fatalf("uniform section contains unexpected block %d at index %d", generatedBlock, index)
					}
				}
			}

			for localY := range int32(game.ChunkWidth) {
				for localZ := range int32(game.ChunkWidth) {
					for localX := range int32(game.ChunkWidth) {
						worldX := chunk.X*game.ChunkWidth + localX
						worldY := sectionMinY + localY
						worldZ := chunk.Z*game.ChunkWidth + localZ
						index := localY*256 + localZ*16 + localX
						want := generated.BlockAt(seed, game.BlockPosition{X: worldX, Y: worldY, Z: worldZ})

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
