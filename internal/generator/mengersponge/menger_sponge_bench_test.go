package mengersponge

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func BenchmarkBlockAt(b *testing.B) {
	generator := Generator{}

	positions := [...]game.BlockPosition{
		{X: 0, Y: -64, Z: 0},
		{X: 127, Y: 63, Z: -255},
		{X: 1024, Y: 128, Z: 2048},
		{X: -8192, Y: 319, Z: 4096},
	}

	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		_ = generator.BlockAt(0, positions[index%len(positions)])
	}
}

func BenchmarkPreparedChunkSections(b *testing.B) {
	prepared := Generator{}.GenerateChunk(0, game.ChunkPosition{X: -7, Z: 11})

	sections := [...]int32{-80, -72, -64, 64, 304, 312, 320}

	var blocks [game.SectionVolume]game.Block

	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		_, _ = prepared.GenerateSection(sections[index%len(sections)], &blocks)
	}
}

func BenchmarkRepeatedSectionPreparation(b *testing.B) {
	generated := Generator{}

	chunk := game.ChunkPosition{X: -7, Z: 11}

	sections := [...]int32{-80, -72, -64, 64, 304, 312, 320}

	var blocks [game.SectionVolume]game.Block

	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		_, _ = generated.GenerateSection(0, chunk, sections[index%len(sections)], &blocks)
	}
}
