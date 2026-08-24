package fractalvaults

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "fractal-vaults"

	floorY            = 63
	baseScale         = int64(9)
	maxHierarchyLevel = 4
)

type Generator struct{}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	originX, originZ := origins(seed)

	levelX := wallLevel(int64(position.X), originX)
	levelZ := wallLevel(int64(position.Z), originZ)

	return blockAtLevels(position.X, position.Y, position.Z, originX, originZ, levelX, levelZ)
}

func (Generator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < floorY || sectionMinY > floorY+int32(wallHeight(maxHierarchyLevel)) {
		return game.Air, true
	}

	originX, originZ := origins(seed)

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	var (
		xLevels [game.ChunkWidth]int
		zLevels [game.ChunkWidth]int
	)

	for local := int32(0); local < game.ChunkWidth; local++ {
		xLevels[local] = wallLevel(int64(chunkMinX+local), originX)
		zLevels[local] = wallLevel(int64(chunkMinZ+local), originZ)
	}

	first := game.Air
	uniform := true

	for localY := range int32(game.ChunkWidth) {
		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				block := blockAtLevels(
					chunkMinX+localX,
					sectionMinY+localY,
					chunkMinZ+localZ,
					originX,
					originZ,
					xLevels[localX],
					zLevels[localZ],
				)

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
	return floorY, floorY + int32(wallHeight(maxHierarchyLevel)), true
}

func blockAtLevels(worldX, worldY, worldZ int32, originX, originZ int64, levelX, levelZ int) game.Block {

	if worldY == floorY {
		return game.Stone
	}

	if worldY < floorY {
		return game.Air
	}

	if levelX < 0 && levelZ < 0 {
		return game.Air
	}

	relativeY := int64(worldY - floorY)

	if levelX >= 0 && levelZ >= 0 {
		height := wallHeight(levelX)

		other := wallHeight(levelZ)
		if other > height {
			height = other
		}

		if relativeY <= height {
			return game.Stone
		}

		return game.Air
	}

	if levelX >= 0 {
		if relativeY > wallHeight(levelX) || archOpen(int64(worldZ), originZ, levelX, relativeY) {
			return game.Air
		}

		return game.Stone
	}

	if relativeY > wallHeight(levelZ) || archOpen(int64(worldX), originX, levelZ, relativeY) {
		return game.Air
	}

	return game.Stone
}

func (Generator) Spawn(seed int64) game.Position {
	originX, originZ := origins(seed)

	return game.Position{
		X: float64(originX) + 4.5,
		Y: floorY + 1,
		Z: float64(originZ) + 4.5,
	}
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func wallLevel(coordinate, origin int64) int {
	offset := coordinate - origin

	for level := maxHierarchyLevel; level >= 0; level-- {
		scale := scaleForLevel(level)

		local := positiveRemainder(offset, scale)

		distance := local

		if other := scale - local; other < distance {
			distance = other
		}

		if distance <= int64(level/2) {
			return level
		}
	}

	return -1
}

func scaleForLevel(level int) int64 {
	scale := baseScale

	for range level {
		scale *= 3
	}

	return scale
}

func wallHeight(level int) int64 {
	return 6 + int64(level*5)
}

func archOpen(coordinate, origin int64, level int, relativeY int64) bool {
	local := positiveRemainder(coordinate-origin, baseScale)

	center := baseScale / 2

	distance := local - center

	if distance < 0 {
		distance = -distance
	}

	radius := min(int64(1+level/2), 3)

	if distance > radius {
		return false
	}

	radiusSquared := radius * radius

	openingHeight := int64(3)
	openingHeight += (radiusSquared - distance*distance) * (wallHeight(level) - 5) / radiusSquared

	return relativeY <= openingHeight
}

func origins(seed int64) (int64, int64) {
	return positiveRemainder(seed, baseScale), positiveRemainder(seed/baseScale, baseScale)
}

func positiveRemainder(value, divisor int64) int64 {
	remainder := value % divisor
	if remainder < 0 {
		remainder += divisor
	}

	return remainder
}
