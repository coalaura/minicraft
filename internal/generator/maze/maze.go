package maze

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "maze"

	floorY     = int32(63)
	wallMinY   = int32(64)
	wallHeight = int32(4)
	wallMaxY   = wallMinY + wallHeight - 1

	walkwayWidth = int32(2)
	cellSize     = walkwayWidth + 1
	passageStart = int32(1)
	passageEnd   = walkwayWidth
)

type direction uint8

const (
	directionNorth direction = iota
	directionEast
	directionSouth
	directionWest
)

type orientation struct {
	horizontal direction
	vertical   direction
}

type Generator struct{}

type generatedChunk struct {
	chunk game.ChunkPosition
	walls [game.ChunkWidth * game.ChunkWidth]bool
}

var (
	_ game.Generator        = Generator{}
	_ game.SectionGenerator = Generator{}
	_ game.ChunkGenerator   = Generator{}
	_ game.BoundedGenerator = Generator{}
	_ game.SpawnGenerator   = Generator{}
	_ game.GeneratedChunk   = (*generatedChunk)(nil)
)

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	if position.Y < floorY || position.Y > wallMaxY {
		return game.Air
	}

	if position.Y == floorY {
		return game.SmoothStone
	}

	if isWall(seed, position.X, position.Z) {
		return game.StoneBricks
	}

	return game.Air
}

func (Generator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	generated := generateChunk(seed, chunk)

	return generated.GenerateSection(sectionMinY, blocks)
}

func (Generator) GenerateChunk(seed int64, chunk game.ChunkPosition) game.GeneratedChunk {
	generated := generateChunk(seed, chunk)

	return &generated
}

func (Generator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return floorY, wallMaxY, true
}

func (Generator) Spawn(_ int64) game.Position {
	center := float64(cellSize/2) + 0.5

	return game.Position{
		X: center,
		Y: float64(wallMinY),
		Z: center,
	}
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func generateChunk(seed int64, chunk game.ChunkPosition) generatedChunk {
	generated := generatedChunk{chunk: chunk}

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for localZ := range int32(game.ChunkWidth) {
		worldZ := chunkMinZ + localZ

		for localX := range int32(game.ChunkWidth) {
			worldX := chunkMinX + localX
			generated.walls[localZ*game.ChunkWidth+localX] = isWall(seed, worldX, worldZ)
		}
	}

	return generated
}

func (generated *generatedChunk) GenerateSection(sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < floorY || sectionMinY > wallMaxY {
		return game.Air, true
	}

	if sectionMinY > floorY && sectionMaxY <= wallMaxY {
		allWalls := true
		allOpen := true

		for _, wall := range generated.walls {
			allWalls = allWalls && wall
			allOpen = allOpen && !wall
		}

		if allWalls {
			return game.StoneBricks, true
		}

		if allOpen {
			return game.Air, true
		}
	}

	first := game.Air
	uniform := true

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				wall := generated.walls[localZ*game.ChunkWidth+localX]
				block := blockForColumn(worldY, wall)
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

func blockForColumn(worldY int32, wall bool) game.Block {
	if worldY == floorY {
		return game.SmoothStone
	}

	if worldY < wallMinY || worldY > wallMaxY || !wall {
		return game.Air
	}

	return game.StoneBricks
}

func isWall(seed int64, worldX, worldZ int32) bool {
	localX := floorMod(worldX, cellSize)
	localZ := floorMod(worldZ, cellSize)

	verticalWall := localX == 0
	horizontalWall := localZ == 0

	if !verticalWall && !horizontalWall {
		return false
	}

	if verticalWall && horizontalWall {
		return true
	}

	mazeOrientation := orientationForSeed(seed)

	if verticalWall {
		if localZ < passageStart || localZ > passageEnd {
			return true
		}

		boundaryX := floorDiv(worldX, cellSize)
		cellZ := floorDiv(worldZ, cellSize)

		var sourceX int32
		if mazeOrientation.horizontal == directionEast {
			sourceX = boundaryX - 1
		} else {
			sourceX = boundaryX
		}

		return !cellChoosesHorizontal(seed, sourceX, cellZ)
	}

	if localX < passageStart || localX > passageEnd {
		return true
	}

	cellX := floorDiv(worldX, cellSize)
	boundaryZ := floorDiv(worldZ, cellSize)

	var sourceZ int32
	if mazeOrientation.vertical == directionNorth {
		sourceZ = boundaryZ
	} else {
		sourceZ = boundaryZ - 1
	}

	return cellChoosesHorizontal(seed, cellX, sourceZ)
}

func orientationForSeed(seed int64) orientation {
	switch mix64(uint64(seed)) & 3 {
	case 0:
		return orientation{horizontal: directionEast, vertical: directionNorth}
	case 1:
		return orientation{horizontal: directionEast, vertical: directionSouth}
	case 2:
		return orientation{horizontal: directionWest, vertical: directionSouth}
	default:
		return orientation{horizontal: directionWest, vertical: directionNorth}
	}
}

func cellChoosesHorizontal(seed int64, cellX, cellZ int32) bool {
	value := uint64(seed)
	value ^= uint64(int64(cellX)) * 0x9e3779b97f4a7c15
	value ^= uint64(int64(cellZ)) * 0xc2b2ae3d27d4eb4f

	return mix64(value)&1 == 0
}

func mix64(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31

	return value
}

func floorDiv(value, divisor int32) int32 {
	quotient := value / divisor
	if value%divisor < 0 {
		quotient--
	}

	return quotient
}

func floorMod(value, divisor int32) int32 {
	remainder := value % divisor
	if remainder < 0 {
		remainder += divisor
	}

	return remainder
}
