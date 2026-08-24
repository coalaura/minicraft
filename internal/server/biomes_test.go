package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type splitBiomeGenerator struct{}

func (splitBiomeGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return game.Air
}

func (splitBiomeGenerator) BiomeAt(_ int64, x, _ int32) game.Biome {
	if x&15 < 8 {
		return game.BiomePlains
	}

	return game.BiomeForest
}

func (splitBiomeGenerator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
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
	}
}
