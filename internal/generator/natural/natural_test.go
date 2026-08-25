package natural

import (
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type generatedChunkSnapshot struct {
	sections [5][game.SectionVolume]game.Block
}

var benchmarkNaturalBlock game.Block

func TestGenerateSectionMatchesBlockAt(t *testing.T) {
	generated := Generator{}

	chunks := []game.ChunkPosition{
		{X: 0, Z: 0},
		{X: -7, Z: 11},
		{X: 29, Z: -18},
	}

	sections := []int32{-64, 32, 48, 64, 80, 96, 112, 144, 176}

	for _, chunk := range chunks {
		prepared := generated.GenerateChunk(42, chunk)

		for _, sectionMinY := range sections {
			var blocks [game.SectionVolume]game.Block

			uniformBlock, uniform := prepared.GenerateSection(sectionMinY, &blocks)

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

func TestChunkGenerationIsDeterministicAcrossOrderAndConcurrency(t *testing.T) {
	generator := Generator{}

	chunks := []game.ChunkPosition{
		{X: -7, Z: 11},
		{X: 0, Z: 0},
		{X: 29, Z: -18},
	}

	want := make(map[game.ChunkPosition]generatedChunkSnapshot, len(chunks))

	for _, chunk := range chunks {
		want[chunk] = snapshotChunk(generator, 42, chunk)
	}

	var wait sync.WaitGroup

	errors := make(chan string, len(chunks))

	for _, chunk := range slices.Backward(chunks) {
		wait.Go(func() {
			got := snapshotChunk(generator, 42, chunk)
			if !reflect.DeepEqual(got, want[chunk]) {
				errors <- "chunk changed when generated concurrently or in reverse order"
			}
		})
	}

	wait.Wait()
	close(errors)

	for message := range errors {
		t.Fatal(message)
	}
}

func TestBiomeVariety(t *testing.T) {
	generated := Generator{}
	seen := make(map[game.Biome]bool)

	for z := int32(-2048); z <= 2048; z += 64 {
		for x := int32(-2048); x <= 2048; x += 64 {
			seen[generated.BiomeAt(42, x, 0, z)] = true
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

func TestWorldMetadataMatchesNaturalSeaLevel(t *testing.T) {
	metadata := (Generator{}).WorldMetadata(42)
	if metadata.SeaLevel != seaLevel {
		t.Fatalf("sea level = %d, want %d", metadata.SeaLevel, seaLevel)
	}
}

func BenchmarkChunkSections(b *testing.B) {
	generator := Generator{}

	chunk := game.ChunkPosition{X: 17, Z: -23}

	sections := [...]int32{-64, -48, -32, -16, 0, 16, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176}

	b.Run("prepared_chunk", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			prepared := generator.GenerateChunk(42, chunk)

			var blocks [game.SectionVolume]game.Block

			for _, sectionMinY := range sections {
				block, _ := prepared.GenerateSection(sectionMinY, &blocks)
				benchmarkNaturalBlock = block
			}
		}
	})

	b.Run("repeated_section_preparation", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			var blocks [game.SectionVolume]game.Block

			for _, sectionMinY := range sections {
				block, _ := generator.GenerateSection(42, chunk, sectionMinY, &blocks)
				benchmarkNaturalBlock = block
			}
		}
	})
}

func snapshotChunk(generator Generator, seed int64, chunk game.ChunkPosition) generatedChunkSnapshot {
	sectionMinY := [...]int32{-64, 48, 64, 80, 176}

	prepared := generator.GenerateChunk(seed, chunk)

	var snapshot generatedChunkSnapshot

	for index, minY := range sectionMinY {
		var blocks [game.SectionVolume]game.Block

		uniformBlock, uniform := prepared.GenerateSection(minY, &blocks)
		if uniform {
			for blockIndex := range blocks {
				blocks[blockIndex] = uniformBlock
			}
		}

		snapshot.sections[index] = blocks
	}

	return snapshot
}
