package server

import (
	"fmt"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func buildChunkBiomes(world *game.World, chunk game.ChunkPosition) (protocol.SectionBiomes, bool, error) {
	generator, ok := world.Generator.(game.BiomeGenerator)
	if !ok {
		return protocol.SectionBiomes{}, false, nil
	}

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	var biomes protocol.SectionBiomes

	for localZ := range 4 {
		for localX := range 4 {
			worldX := chunkMinX + int32(localX*4+2)
			worldZ := chunkMinZ + int32(localZ*4+2)

			biome := generator.BiomeAt(world.Seed, worldX, worldZ)
			if !biome.Valid() {
				return protocol.SectionBiomes{}, false, fmt.Errorf("unsupported biome %d at x=%d z=%d", biome, worldX, worldZ)
			}

			for localY := range 4 {
				biomes.Set(localX, localY, localZ, int32(biome))
			}
		}
	}

	return biomes, true, nil
}
