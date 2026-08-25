package server

import (
	"fmt"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const defaultBiomeID = int32(game.BiomePlains)

type chunkBiomes struct {
	sections [protocol.OverworldSectionCount]protocol.SectionBiomes
	present  bool
}

func buildSectionBiomes(prepared preparedChunkGeneration, sectionMinY int32) (protocol.SectionBiomes, bool, error) {
	chunk := prepared.position

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	var biomes protocol.SectionBiomes

	for localY := range 4 {
		for localZ := range 4 {
			for localX := range 4 {
				worldX := chunkMinX + int32(localX*4+2)
				worldY := sectionMinY + int32(localY*4+2)
				worldZ := chunkMinZ + int32(localZ*4+2)

				biome, ok := prepared.BiomeAt(worldX, worldY, worldZ)
				if !ok {
					for index := range biomes.States {
						biomes.States[index] = defaultBiomeID
					}

					return biomes, true, nil
				}

				if !biome.Valid() {
					return protocol.SectionBiomes{}, false, fmt.Errorf("unsupported biome %d at x=%d y=%d z=%d", biome, worldX, worldY, worldZ)
				}

				biomes.Set(localX, localY, localZ, int32(biome))
			}
		}
	}

	return biomes, true, nil
}
