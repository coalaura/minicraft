package natural

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestGenerateSectionMatchesBlockAt(t *testing.T) {
	generated := Generator{}
	chunks := []game.ChunkPosition{
		{X: 0, Z: 0},
		{X: -7, Z: 11},
		{X: 29, Z: -18},
	}
	sections := []int32{-64, 32, 48, 64, 80, 96, 112, 144, 176}

	for _, chunk := range chunks {
		for _, sectionMinY := range sections {
			var blocks [game.SectionVolume]game.Block

			uniformBlock, uniform := generated.GenerateSection(42, chunk, sectionMinY, &blocks)

			for localY := range int32(game.ChunkWidth) {
				for localZ := range int32(game.ChunkWidth) {
					for localX := range int32(game.ChunkWidth) {
						position := game.BlockPosition{
							X: chunk.X*game.ChunkWidth + localX,
							Y: sectionMinY + localY,
							Z: chunk.Z*game.ChunkWidth + localZ,
						}

						want := generated.BlockAt(42, position)
						index := localY*256 + localZ*16 + localX
						got := blocks[index]

						if uniform {
							got = uniformBlock
						}

						if got != want {
							t.Fatalf("chunk %+v section %d position %+v = %d, want %d", chunk, sectionMinY, position, got, want)
						}
					}
				}
			}
		}
	}
}

func TestBiomeVariety(t *testing.T) {
	generated := Generator{}
	seen := make(map[game.Biome]bool)

	for z := int32(-2048); z <= 2048; z += 64 {
		for x := int32(-2048); x <= 2048; x += 64 {
			seen[generated.BiomeAt(42, x, z)] = true
		}
	}

	if len(seen) < 6 {
		t.Fatalf("found only %d biomes: %v", len(seen), seen)
	}
}

func TestSpawnIsOnLand(t *testing.T) {
	generated := Generator{}
	spawn := generated.Spawn(42)

	x := int32(spawn.X)
	y := int32(spawn.Y)
	z := int32(spawn.Z)

	floor := generated.BlockAt(42, game.BlockPosition{X: x, Y: y - 1, Z: z})
	if floor == game.Water || floor == game.Air {
		t.Fatalf("spawn floor = %d", floor)
	}

	block := generated.BlockAt(42, game.BlockPosition{X: x, Y: y, Z: z})
	if block == game.Water || block == game.OakLog || block == game.SpruceLog || block == game.Cactus {
		t.Fatalf("spawn obstructed by %d", block)
	}
}
