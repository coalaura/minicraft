package superflat

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

type uniformSectionTestCase struct {
	sectionMinY int32
	expected    game.Block
}

func TestGeneratorRegistration(t *testing.T) {
	registered, err := generator.New(Name)
	if err != nil {
		t.Fatalf("create registered generator: %v", err)
	}

	if _, ok := registered.(Generator); !ok {
		t.Fatalf("registered generator type = %T", registered)
	}
}

func TestWorldLayers(t *testing.T) {
	tests := []blockTestCase{
		{name: "below world", position: game.BlockPosition{X: -100, Y: minimumY - 1, Z: 100}, expected: game.Air},
		{name: "bedrock", position: game.BlockPosition{X: 100, Y: minimumY, Z: -100}, expected: game.Bedrock},
		{name: "stone", position: game.BlockPosition{X: -100, Y: dirtMinY - 1, Z: -100}, expected: game.Stone},
		{name: "bottom dirt", position: game.BlockPosition{X: 100, Y: dirtMinY, Z: 100}, expected: game.Dirt},
		{name: "top dirt", position: game.BlockPosition{Y: surfaceY - 1}, expected: game.Dirt},
		{name: "surface", position: game.BlockPosition{X: -100, Y: surfaceY, Z: 100}, expected: game.GrassBlock},
		{name: "above decoration", position: game.BlockPosition{Y: decorY + 1}, expected: game.Air},
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
	generated := Generator{}
	sections := []int32{-64, 64}

	for _, sectionMinY := range sections {
		var blocks [game.SectionVolume]game.Block

		uniformBlock, uniform := generated.GenerateSection(0, game.ChunkPosition{X: -3, Z: 5}, sectionMinY, &blocks)
		if uniform {
			t.Fatalf("boundary section at %d unexpectedly uniform with block %d", sectionMinY, uniformBlock)
		}

		assertSectionMatchesBlockAt(t, generated, game.ChunkPosition{X: -3, Z: 5}, sectionMinY, blocks)
	}
}

func TestGeneratedSectionsUseUniformFastPath(t *testing.T) {
	generated := Generator{}
	tests := []uniformSectionTestCase{
		{sectionMinY: -80, expected: game.Air},
		{sectionMinY: -48, expected: game.Stone},
		{sectionMinY: 80, expected: game.Air},
	}

	for _, test := range tests {
		var blocks [game.SectionVolume]game.Block

		actual, uniform := generated.GenerateSection(0, game.ChunkPosition{}, test.sectionMinY, &blocks)
		if !uniform || actual != test.expected {
			t.Fatalf("section at %d = %d, %v; want %d, true", test.sectionMinY, actual, uniform, test.expected)
		}
	}
}

func TestDecorationsAreSparseAndSpawnStaysClear(t *testing.T) {
	generated := Generator{}
	seen := make(map[game.Block]bool)
	decorations := 0

	for z := int32(-128); z <= 128; z++ {
		for x := int32(-128); x <= 128; x++ {
			block := generated.BlockAt(1234, game.BlockPosition{X: x, Y: decorY, Z: z})
			if block == game.Air {
				continue
			}

			seen[block] = true
			decorations++
		}
	}

	decorationBlocks := []game.Block{game.ShortGrass, game.Dandelion, game.Poppy}

	for _, block := range decorationBlocks {
		if !seen[block] {
			t.Fatalf("decoration block %d was not generated", block)
		}
	}

	if decorations < 2000 || decorations > 4000 {
		t.Fatalf("generated %d decorations in 257x257 area, want sparse coverage", decorations)
	}

	for z := int32(-2); z <= 2; z++ {
		for x := int32(-2); x <= 2; x++ {
			if generated.BlockAt(1234, game.BlockPosition{X: x, Y: decorY, Z: z}) != game.Air {
				t.Fatalf("spawn decoration at %d, %d", x, z)
			}
		}
	}
}

func TestDecorationsDependOnSeed(t *testing.T) {
	generated := Generator{}
	different := false

	for z := int32(-32); z <= 32 && !different; z++ {
		for x := int32(-32); x <= 32; x++ {
			position := game.BlockPosition{X: x, Y: decorY, Z: z}
			if generated.BlockAt(1, position) != generated.BlockAt(2, position) {
				different = true

				break
			}
		}
	}

	if !different {
		t.Fatal("decoration layout does not vary with seed")
	}
}

func TestGenerationBoundsAndSpawn(t *testing.T) {
	generated := Generator{}
	minimum, maximum, valid := generated.GenerationBounds(1234, game.ChunkPosition{X: -9, Z: 12})

	if !valid || minimum != minimumY || maximum != decorY {
		t.Fatalf("generation bounds = %d, %d, %v; want %d, %d, true", minimum, maximum, valid, minimumY, decorY)
	}

	spawn := generated.Spawn(1234)
	if spawn != (game.Position{X: 0.5, Y: 70, Z: 0.5}) {
		t.Fatalf("spawn = %+v", spawn)
	}

	if generated.BlockAt(1234, game.BlockPosition{Y: decorY}) != game.Air {
		t.Fatal("spawn is obstructed by decoration")
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
