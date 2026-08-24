package mengersponge

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "menger-sponge"

	minBuildY = int32(-64)
	maxBuildY = int32(319)
)

type Generator struct{}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	if position.Y < minBuildY || position.Y > maxBuildY {
		return game.Air
	}

	x := absoluteCoordinate(position.X)
	y := int64(position.Y - minBuildY)
	z := absoluteCoordinate(position.Z)

	if !isMengerSolid(x, y, z) {
		return game.Air
	}

	return game.Stone
}

func (Generator) GenerateSection(_ int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	if sectionMinY > maxBuildY || sectionMinY+game.ChunkWidth-1 < minBuildY {
		return game.Air, true
	}

	var (
		xMasks   [game.ChunkWidth]uint32
		yMasks   [game.ChunkWidth]uint32
		yInRange [game.ChunkWidth]bool
		zMasks   [game.ChunkWidth]uint32
	)

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for local := range int32(game.ChunkWidth) {
		xMasks[local] = ternaryCenterMask(absoluteCoordinate(chunkMinX + local))
		zMasks[local] = ternaryCenterMask(absoluteCoordinate(chunkMinZ + local))

		worldY := sectionMinY + local
		if worldY >= minBuildY && worldY <= maxBuildY {
			yInRange[local] = true
			yMasks[local] = ternaryCenterMask(int64(worldY - minBuildY))
		}
	}

	first := game.Air
	uniform := true

	for localY := range game.ChunkWidth {
		for localZ := range game.ChunkWidth {
			for localX := range game.ChunkWidth {
				block := game.Air

				if yInRange[localY] {
					block = game.Stone
					xMask := xMasks[localX]
					yMask := yMasks[localY]
					zMask := zMasks[localZ]

					if xMask&yMask != 0 || xMask&zMask != 0 || yMask&zMask != 0 {
						block = game.Air
					}
				}

				index := localY*256 + localZ*16 + localX
				blocks[index] = block

				if index == 0 {
					first = block
				} else if block != first {
					uniform = false
				}
			}
		}
	}

	return first, uniform
}

func (Generator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return minBuildY, maxBuildY, true
}

func (Generator) Spawn(_ int64) game.Position {
	return game.Position{
		X: 3.5,
		Y: 65,
		Z: 2.5,
	}
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func isMengerSolid(x, y, z int64) bool {
	for x != 0 || y != 0 || z != 0 {
		centeredAxes := 0

		if x%3 == 1 {
			centeredAxes++
		}

		if y%3 == 1 {
			centeredAxes++
		}

		if z%3 == 1 {
			centeredAxes++
		}

		if centeredAxes >= 2 {
			return false
		}

		x /= 3
		y /= 3
		z /= 3
	}

	return true
}

func absoluteCoordinate(coordinate int32) int64 {
	value := int64(coordinate)

	if value < 0 {
		return -value
	}

	return value
}

func ternaryCenterMask(value int64) uint32 {
	var (
		mask uint32
		bit  uint
	)

	for value != 0 {
		if value%3 == 1 {
			mask |= 1 << bit
		}

		value /= 3
		bit++
	}

	return mask
}
