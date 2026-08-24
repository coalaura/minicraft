package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator/fractalvaults"
	"github.com/coalaura/minicraft/internal/generator/mengersponge"
	"github.com/coalaura/minicraft/internal/generator/spawnplatform"
	"github.com/coalaura/minicraft/internal/generator/waveterrain"
)

func BenchmarkBuildLevelChunk(b *testing.B) {
	benchmarks := []struct {
		name      string
		generator game.Generator
	}{
		{name: "spawn_platform", generator: spawnplatform.New()},
		{name: "wave_terrain", generator: waveterrain.New()},
		{name: "menger_sponge", generator: mengersponge.New()},
		{name: "fractal_vaults", generator: fractalvaults.New()},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			world := game.NewOverworld(benchmark.generator, 42)

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				_, err := buildLevelChunk(world, 17, -23)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkChunkPacketMengerSponge(b *testing.B) {
	world := game.NewOverworld(mengersponge.New(), 42)

	b.ReportAllocs()

	for b.Loop() {
		_, err := chunkPacket(world, 17, -23)
		if err != nil {
			b.Fatal(err)
		}
	}
}
