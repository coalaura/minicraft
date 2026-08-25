package server

import "github.com/coalaura/minicraft/internal/game"

type preparedChunkGeneration struct {
	generator game.Generator
	generated game.GeneratedChunk
	seed      int64
	position  game.ChunkPosition
}

func prepareChunkGeneration(world *game.World, position game.ChunkPosition) preparedChunkGeneration {
	prepared := preparedChunkGeneration{
		generator: world.Generator,
		seed:      world.Seed,
		position:  position,
	}

	if generator, ok := world.Generator.(game.ChunkGenerator); ok {
		prepared.generated = generator.GenerateChunk(world.Seed, position)
	}

	return prepared
}

func (prepared preparedChunkGeneration) GenerateSection(sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	if prepared.generated != nil {
		return prepared.generated.GenerateSection(sectionMinY, blocks)
	}

	if generator, ok := prepared.generator.(game.SectionGenerator); ok {
		return generator.GenerateSection(prepared.seed, prepared.position, sectionMinY, blocks)
	}

	return generateSectionBlocks(prepared.generator, prepared.seed, prepared.position, sectionMinY, blocks)
}

func (prepared preparedChunkGeneration) BiomeAt(x, y, z int32) (game.Biome, bool) {
	if generator, ok := prepared.generated.(game.GeneratedChunkBiomeGenerator); ok {
		return generator.BiomeAt(x, y, z), true
	}

	if generator, ok := prepared.generator.(game.BiomeGenerator); ok {
		return generator.BiomeAt(prepared.seed, x, y, z), true
	}

	return 0, false
}
