package spawnplatform

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "spawn-platform"

	platformY      = 69
	platformRadius = 4
)

type Generator struct{}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func (Generator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	if position.Y != platformY {
		return game.Air
	}

	if position.X < -platformRadius || position.X > platformRadius {
		return game.Air
	}

	if position.Z < -platformRadius || position.Z > platformRadius {
		return game.Air
	}

	return game.Stone
}

func (Generator) GenerateSection(_ int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	if sectionMinY > platformY || sectionMinY+game.ChunkWidth-1 < platformY || !chunkIntersectsPlatform(chunk) {
		return game.Air, true
	}

	clear(blocks[:])

	localY := platformY - sectionMinY

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for localZ := range int32(game.ChunkWidth) {
		worldZ := chunkMinZ + localZ
		if worldZ < -platformRadius || worldZ > platformRadius {
			continue
		}

		for localX := range int32(game.ChunkWidth) {
			worldX := chunkMinX + localX
			if worldX < -platformRadius || worldX > platformRadius {
				continue
			}

			blocks[localY*256+localZ*16+localX] = game.Stone
		}
	}

	return game.Air, false
}

func (Generator) GenerationBounds(_ int64, chunk game.ChunkPosition) (int32, int32, bool) {
	if !chunkIntersectsPlatform(chunk) {
		return 0, 0, false
	}

	return platformY, platformY, true
}

func (Generator) Spawn(_ int64) game.Position {
	return game.Position{X: 0.5, Y: 70, Z: 0.5}
}

func chunkIntersectsPlatform(chunk game.ChunkPosition) bool {
	minX := chunk.X * game.ChunkWidth
	minZ := chunk.Z * game.ChunkWidth

	maxX := minX + game.ChunkWidth - 1
	maxZ := minZ + game.ChunkWidth - 1

	return maxX >= -platformRadius && minX <= platformRadius && maxZ >= -platformRadius && minZ <= platformRadius
}
