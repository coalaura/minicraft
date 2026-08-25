package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator/babel"
	"github.com/coalaura/minicraft/internal/generator/backrooms"
	"github.com/coalaura/minicraft/internal/generator/fractalvaults"
	"github.com/coalaura/minicraft/internal/generator/mengersponge"
	"github.com/coalaura/minicraft/internal/generator/natural"
	"github.com/coalaura/minicraft/internal/generator/quasicrystal"
	"github.com/coalaura/minicraft/internal/generator/spawnplatform"
	"github.com/coalaura/minicraft/internal/generator/waveterrain"
	"github.com/coalaura/minicraft/internal/protocol"
)

type emissiveBenchmarkGenerator struct{}

type chunkBenchmarkCase struct {
	name      string
	generator game.Generator
}

type lightingModeBenchmarkCase struct {
	name string
	mode game.LightingMode
}

func (emissiveBenchmarkGenerator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	if position.Y == 64 && (position.X+position.Z)&1 == 0 {
		return game.Glowstone
	}

	return game.Air
}

func (generator emissiveBenchmarkGenerator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	if sectionMinY != 64 {
		return game.Air, true
	}

	for localZ := range game.ChunkWidth {
		for localX := range game.ChunkWidth {
			position := game.BlockPosition{
				X: chunk.X*game.ChunkWidth + int32(localX),
				Y: sectionMinY,
				Z: chunk.Z*game.ChunkWidth + int32(localZ),
			}

			blocks[localZ*game.ChunkWidth+localX] = generator.BlockAt(seed, position)
		}
	}

	return 0, false
}

func (emissiveBenchmarkGenerator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return 64, 64, true
}

func BenchmarkBuildLevelChunk(b *testing.B) {
	benchmarks := []chunkBenchmarkCase{
		{name: "spawn_platform", generator: spawnplatform.New()},
		{name: "natural", generator: natural.New()},
		{name: "wave_terrain", generator: waveterrain.New()},
		{name: "menger_sponge", generator: mengersponge.New()},
		{name: "quasicrystal", generator: quasicrystal.New()},
		{name: "babel", generator: babel.New()},
		{name: "backrooms", generator: backrooms.New()},
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

func BenchmarkLightingModes(b *testing.B) {
	benchmarks := []chunkBenchmarkCase{
		{name: "open", generator: nil},
		{name: "ordinary_terrain", generator: natural.New()},
		{name: "wave_terrain", generator: waveterrain.New()},
		{name: "menger_sponge", generator: mengersponge.New()},
		{name: "quasicrystal", generator: quasicrystal.New()},
		{name: "babel", generator: babel.New()},
		{name: "backrooms", generator: backrooms.New()},
		{name: "fractal_vaults", generator: fractalvaults.New()},
		{name: "many_emitters", generator: emissiveBenchmarkGenerator{}},
	}

	for _, benchmark := range benchmarks {
		for _, mode := range []lightingModeBenchmarkCase{
			{name: "normal", mode: game.LightingNormal},
			{name: "fullbright", mode: game.LightingFullbright},
		} {
			b.Run(benchmark.name+"/"+mode.name, func(b *testing.B) {
				world := game.NewOverworld(benchmark.generator, 42)

				world.SetLightingMode(mode.mode)

				b.ReportAllocs()

				for b.Loop() {
					_, err := buildLevelChunk(world, 17, -23)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkNormalChunkPacket(b *testing.B) {
	benchmarks := []chunkBenchmarkCase{
		{name: "ordinary_terrain", generator: natural.New()},
		{name: "menger_sponge", generator: mengersponge.New()},
		{name: "backrooms", generator: backrooms.New()},
		{name: "many_emitters", generator: emissiveBenchmarkGenerator{}},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			world := game.NewOverworld(benchmark.generator, 42)

			world.SetLightingMode(game.LightingNormal)

			b.ReportAllocs()

			for b.Loop() {
				_, err := chunkPacket(world, 17, -23)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDynamicRelighting(b *testing.B) {
	position := game.BlockPosition{X: 8, Y: 70, Z: 8}

	change := []game.BlockChange{{Position: position, Replacement: game.Torch}}

	b.Run("add_emitter", func(b *testing.B) {
		world := normalLightingTestWorld()

		b.ReportAllocs()

		for b.Loop() {
			world.SetBlock(position, game.Torch)

			updates, err := buildChangedLightUpdates(world, change)
			if err != nil {
				b.Fatal(err)
			}

			if len(updates) == 0 {
				b.Fatal("no light updates")
			}

			world.SetBlock(position, game.Air)
		}
	})

	b.Run("remove_emitter", func(b *testing.B) {
		world := normalLightingTestWorld()

		world.SetBlock(position, game.Torch)

		b.ReportAllocs()

		for b.Loop() {
			world.SetBlock(position, game.Air)

			updates, err := buildChangedLightUpdates(world, change)
			if err != nil {
				b.Fatal(err)
			}

			if len(updates) == 0 {
				b.Fatal("no light updates")
			}

			world.SetBlock(position, game.Torch)
		}
	})
}

func BenchmarkLightUpdateEncoding(b *testing.B) {
	world := normalLightingTestWorld()

	world.SetBlock(game.BlockPosition{X: 8, Y: 70, Z: 8}, game.Glowstone)

	update, err := buildChunkLight(world, 0, 0)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		var writer protocol.PacketWriter

		update.Encode(&writer)

		err := writer.Err()
		if err != nil {
			b.Fatal(err)
		}
	}
}
