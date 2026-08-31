package superflat

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "superflat"

	minimumY = -64
	surfaceY = 69
	dirtMinY = surfaceY - 3
	decorY   = surfaceY + 1
)

type Generator struct{}

var (
	_ game.Generator        = Generator{}
	_ game.SectionGenerator = Generator{}
	_ game.BoundedGenerator = Generator{}
	_ game.SpawnGenerator   = Generator{}
)

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	switch {
	case position.Y < minimumY || position.Y > decorY:
		return game.Air
	case position.Y == minimumY:
		return game.Bedrock
	case position.Y < dirtMinY:
		return game.Stone
	case position.Y < surfaceY:
		return game.Dirt
	case position.Y == surfaceY:
		return game.GrassBlock
	default:
		return decorationAt(seed, position.X, position.Z)
	}
}

func (generated Generator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < minimumY || sectionMinY > decorY {
		return game.Air, true
	}

	if sectionMinY > minimumY && sectionMaxY < dirtMinY {
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
	return minimumY, decorY, true
}

func (Generator) Spawn(_ int64) game.Position {
	return game.Position{X: 0.5, Y: 70, Z: 0.5}
}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func decorationAt(seed int64, x, z int32) game.Block {
	if x >= -2 && x <= 2 && z >= -2 && z <= 2 {
		return game.Air
	}

	hash := coordinateHash(seed, x, z) % 128

	switch hash {
	case 0, 1, 2, 3:
		return game.ShortGrass
	case 4:
		return game.Dandelion
	case 5:
		return game.Poppy
	default:
		return game.Air
	}
}

func coordinateHash(seed int64, x, z int32) uint64 {
	value := uint64(seed)
	value ^= uint64(int64(x)) * 0x9e3779b97f4a7c15
	value ^= uint64(int64(z)) * 0xbf58476d1ce4e5b9
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb

	return value ^ value>>31
}
