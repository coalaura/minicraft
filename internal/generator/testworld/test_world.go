package testworld

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "test-world"

	minimumY = -64
	surfaceY = 69
)

type Generator struct{}

var (
	_ game.Generator        = Generator{}
	_ game.SectionGenerator = Generator{}
	_ game.BoundedGenerator = Generator{}
	_ game.SpawnGenerator   = Generator{}
)

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	switch {
	case position.Y < minimumY || position.Y > surfaceY:
		return game.Air
	case position.Y == minimumY:
		return game.Bedrock
	case position.Y < surfaceY:
		return game.Stone
	default:
		return surfaceBlock(position.X, position.Z)
	}
}

func (generated Generator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < minimumY || sectionMinY > surfaceY {
		return game.Air, true
	}

	if sectionMinY > minimumY && sectionMaxY < surfaceY {
		return game.Stone, true
	}

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			worldZ := chunkMinZ + localZ

			for localX := range int32(game.ChunkWidth) {
				worldX := chunkMinX + localX
				position := game.BlockPosition{X: worldX, Y: worldY, Z: worldZ}
				index := localY*256 + localZ*16 + localX

				blocks[index] = generated.BlockAt(seed, position)
			}
		}
	}

	return game.Air, false
}

func (Generator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return minimumY, surfaceY, true
}

func (Generator) Spawn(_ int64) game.Position {
	return game.Position{X: 0.5, Y: 70, Z: 0.5}
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func surfaceBlock(x, z int32) game.Block {
	xBorder := x%game.ChunkWidth == 0
	zBorder := z%game.ChunkWidth == 0

	if xBorder && zBorder {
		return game.ReinforcedDeepslate
	}

	if x == 0 || z == 0 {
		return game.PolishedBlackstoneBricks
	}

	if x&15 == 8 && z&15 == 8 {
		return game.ChiseledTuffBricks
	}

	if xBorder || zBorder {
		return game.DeepslateBricks
	}

	return game.StoneBricks
}
