package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator/superflat"
	"github.com/coalaura/minicraft/internal/protocol"
)

type splitBiomeGenerator struct{}

type verticalBiomeGenerator struct{}

func (splitBiomeGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return game.Air
}

func (splitBiomeGenerator) BiomeAt(_ int64, x, _, _ int32) game.Biome {
	if x&15 < 8 {
		return game.BiomePlains
	}

	return game.BiomeForest
}

func (splitBiomeGenerator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return 0, 0, false
}

func (verticalBiomeGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return game.Air
}

func (verticalBiomeGenerator) BiomeAt(_ int64, _, y, _ int32) game.Biome {
	biomes := [...]game.Biome{
		game.BiomePlains,
		game.BiomeForest,
		game.BiomeDesert,
		game.BiomeTaiga,
	}

	return biomes[(y-protocol.OverworldMinY)/4%4]
}

func (verticalBiomeGenerator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return 0, 0, false
}

func TestBuildLevelChunkUsesGeneratorBiomes(t *testing.T) {
	world := game.NewOverworld(splitBiomeGenerator{}, 42)

	chunk, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	for sectionIndex, section := range chunk.Sections {
		if len(section.BiomePalette) != 2 {
			t.Fatalf("section %d biome palette = %v", sectionIndex, section.BiomePalette)
		}

		if section.BiomeBitsPerEntry != 1 {
			t.Fatalf("section %d biome bits = %d", sectionIndex, section.BiomeBitsPerEntry)
		}

		for localY := range 4 {
			for localZ := range 4 {
				for localX := range 4 {
					want := int32(game.BiomeForest)

					if localX < 2 {
						want = int32(game.BiomePlains)
					}

					got := sectionBiomeAt(section, localX, localY, localZ)
					if got != want {
						t.Fatalf("section %d biome (%d, %d, %d) = %d, want %d", sectionIndex, localX, localY, localZ, got, want)
					}
				}
			}
		}
	}
}

func TestBuildLevelChunkSamplesBiomesInThreeDimensions(t *testing.T) {
	world := game.NewOverworld(verticalBiomeGenerator{}, 42)

	chunk, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	want := [...]game.Biome{
		game.BiomePlains,
		game.BiomeForest,
		game.BiomeDesert,
		game.BiomeTaiga,
	}

	section := chunk.Sections[0]

	for localY, biome := range want {
		for localZ := range 4 {
			for localX := range 4 {
				got := sectionBiomeAt(section, localX, localY, localZ)
				if got != int32(biome) {
					t.Fatalf("biome (%d, %d, %d) = %d, want %d", localX, localY, localZ, got, biome)
				}
			}
		}
	}
}

func TestBuildLevelChunkDefaultsToPlainsBiomes(t *testing.T) {
	assertDefaultPlainsBiomes(t, game.NewOverworld(nil))

	fullbrightWorld := game.NewOverworld(superflat.New())

	assertDefaultPlainsBiomes(t, fullbrightWorld)

	normalWorld := game.NewOverworld(superflat.New())

	normalWorld.SetLightingMode(game.LightingNormal)

	assertDefaultPlainsBiomes(t, normalWorld)
}

func assertDefaultPlainsBiomes(t *testing.T, world *game.World) {
	t.Helper()

	chunk, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	for sectionIndex, section := range chunk.Sections {
		if section.Biome != int32(game.BiomePlains) {
			t.Fatalf("section %d biome = %d, want plains %d", sectionIndex, section.Biome, game.BiomePlains)
		}
	}
}

func sectionBiomeAt(section protocol.ChunkSection, localX, localY, localZ int) int32 {
	if len(section.BiomePalette) == 0 && !section.BiomeDirect {
		return section.Biome
	}

	index := localY*16 + localZ*4 + localX
	entriesPerLong := 64 / int(section.BiomeBitsPerEntry)
	longIndex := index / entriesPerLong
	bitOffset := index % entriesPerLong * int(section.BiomeBitsPerEntry)
	mask := int64(1<<section.BiomeBitsPerEntry) - 1
	value := int32(section.BiomeData[longIndex] >> uint(bitOffset) & mask)

	if section.BiomeDirect {
		return value
	}

	return section.BiomePalette[value]
}
