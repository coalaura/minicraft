package testworld

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

type blockTestCase struct {
	name     string
	position game.BlockPosition
	expected game.Block
}

type sectionTestCase struct {
	chunk       game.ChunkPosition
	sectionMinY int32
}

type uniformSectionTestCase struct {
	name        string
	sectionMinY int32
	expected    game.Block
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

func TestWorldLayersAndSurfaceMarkers(t *testing.T) {
	tests := []blockTestCase{
		{name: "below world", position: game.BlockPosition{Y: minimumY - 1}, expected: game.Air},
		{name: "bedrock", position: game.BlockPosition{Y: minimumY}, expected: game.Bedrock},
		{name: "foundation", position: game.BlockPosition{Y: surfaceY - 1}, expected: game.Stone},
		{name: "plain surface", position: game.BlockPosition{X: 3, Y: surfaceY, Z: 5}, expected: game.StoneBricks},
		{name: "positive chunk border", position: game.BlockPosition{X: 16, Y: surfaceY, Z: 5}, expected: game.DeepslateBricks},
		{name: "negative chunk border", position: game.BlockPosition{X: -16, Y: surfaceY, Z: -5}, expected: game.DeepslateBricks},
		{name: "positive chunk corner", position: game.BlockPosition{X: 32, Y: surfaceY, Z: 48}, expected: game.ReinforcedDeepslate},
		{name: "negative chunk corner", position: game.BlockPosition{X: -32, Y: surfaceY, Z: -48}, expected: game.ReinforcedDeepslate},
		{name: "positive chunk center", position: game.BlockPosition{X: 8, Y: surfaceY, Z: 24}, expected: game.ChiseledTuffBricks},
		{name: "negative chunk center", position: game.BlockPosition{X: -8, Y: surfaceY, Z: -24}, expected: game.ChiseledTuffBricks},
		{name: "x axis", position: game.BlockPosition{X: 7, Y: surfaceY, Z: 0}, expected: game.PolishedBlackstoneBricks},
		{name: "z axis", position: game.BlockPosition{X: 0, Y: surfaceY, Z: -7}, expected: game.PolishedBlackstoneBricks},
		{name: "origin", position: game.BlockPosition{Y: surfaceY}, expected: game.ReinforcedDeepslate},
		{name: "above surface", position: game.BlockPosition{Y: surfaceY + 1}, expected: game.Air},
	}

	generated := Generator{}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := generated.BlockAt(1234, test.position)
			if actual != test.expected {
				t.Fatalf("block at %+v = %d, want %d", test.position, actual, test.expected)
			}
		})
	}
}

func TestGeneratedSectionsMatchBlockAt(t *testing.T) {
	sections := []sectionTestCase{
		{chunk: game.ChunkPosition{X: -2, Z: -1}, sectionMinY: -64},
		{chunk: game.ChunkPosition{X: -1, Z: 1}, sectionMinY: 64},
		{chunk: game.ChunkPosition{X: 2, Z: 3}, sectionMinY: 64},
	}

	generated := Generator{}

	for _, section := range sections {
		var blocks [game.SectionVolume]game.Block

		uniformBlock, uniform := generated.GenerateSection(0, section.chunk, section.sectionMinY, &blocks)
		if uniform {
			t.Fatalf("boundary section %+v at %d unexpectedly uniform with block %d", section.chunk, section.sectionMinY, uniformBlock)
		}

		assertSectionMatchesBlockAt(t, generated, section.chunk, section.sectionMinY, blocks)
	}
}

func TestGeneratedSectionsUseUniformFastPath(t *testing.T) {
	generated := Generator{}

	tests := []uniformSectionTestCase{
		{name: "below foundation", sectionMinY: -80, expected: game.Air},
		{name: "foundation", sectionMinY: -48, expected: game.Stone},
		{name: "above surface", sectionMinY: 80, expected: game.Air},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var blocks [game.SectionVolume]game.Block

			actual, uniform := generated.GenerateSection(0, game.ChunkPosition{X: -7, Z: 11}, test.sectionMinY, &blocks)
			if !uniform || actual != test.expected {
				t.Fatalf("section block, uniform = %d, %v; want %d, true", actual, uniform, test.expected)
			}
		})
	}
}

func TestSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}
	spawn := generated.Spawn(0)

	if spawn != (game.Position{X: 0.5, Y: 70, Z: 0.5}) {
		t.Fatalf("spawn = %+v", spawn)
	}

	if generated.BlockAt(0, game.BlockPosition{Y: 69}) != game.ReinforcedDeepslate {
		t.Fatal("spawn is not supported by the origin marker")
	}

	if generated.BlockAt(0, game.BlockPosition{Y: 70}) != game.Air || generated.BlockAt(0, game.BlockPosition{Y: 71}) != game.Air {
		t.Fatal("spawn space is obstructed")
	}
}

func TestGenerationBoundsIncludeFoundationAndSurface(t *testing.T) {
	generated := Generator{}

	minimum, maximum, valid := generated.GenerationBounds(1234, game.ChunkPosition{X: -9, Z: 12})
	if !valid || minimum != minimumY || maximum != surfaceY {
		t.Fatalf("generation bounds = %d, %d, %v; want %d, %d, true", minimum, maximum, valid, minimumY, surfaceY)
	}
}

func assertSectionMatchesBlockAt(t *testing.T, generated Generator, chunk game.ChunkPosition, sectionMinY int32, blocks [game.SectionVolume]game.Block) {
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

				index := localY*256 + localZ*16 + localX
				expected := generated.BlockAt(0, position)

				if blocks[index] != expected {
					t.Fatalf("block at %+v = %d, want %d", position, blocks[index], expected)
				}
			}
		}
	}
}
