package natural

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "natural"

	seaLevel      = int32(63)
	minimumY      = int32(-64)
	maximumY      = int32(175)
	treeCellSize  = int32(8)
	maxTreeRadius = int32(2)
	maxTreeHeight = int32(9)
)

const (
	treeNone treeKind = iota
	treeOak
	treeSpruce
)

const (
	saltWarpX       uint64 = 0x9e3779b97f4a7c15
	saltWarpZ       uint64 = 0xbf58476d1ce4e5b9
	saltContinents  uint64 = 0x94d049bb133111eb
	saltElevation   uint64 = 0xd6e8feb86659fd93
	saltMountains   uint64 = 0xa0761d6478bd642f
	saltRidges      uint64 = 0xe7037ed1a0b428db
	saltRivers      uint64 = 0x8ebc6af09c88c6e3
	saltTemperature uint64 = 0x589965cc75374cc3
	saltHumidity    uint64 = 0x1d8e4e27c47d124f
	saltSurface     uint64 = 0xeb44accab455d165
	saltDecor       uint64 = 0x9c06faf4d023e3ab
	saltTree        uint64 = 0xc2b2ae3d27d4eb4f
	saltBedrock     uint64 = 0x165667b19e3779f9
)

type Generator struct{}

type generatedChunk struct {
	seed      int64
	position  game.ChunkPosition
	columns   [game.ChunkWidth * game.ChunkWidth]column
	minHeight int32
	maxHeight int32
	trees     []tree
}

type column struct {
	height        int32
	biome         game.Biome
	temperature   float64
	humidity      float64
	riverStrength float64
	beach         bool
}

type treeKind uint8

type tree struct {
	x      int32
	z      int32
	baseY  int32
	height int32
	kind   treeKind
}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	if position.Y < minimumY || position.Y > maximumY {
		return game.Air
	}

	terrain := columnAt(seed, position.X, position.Z)
	block := terrainBlockAt(seed, position, terrain)

	if block != game.Air {
		return block
	}

	feature, ok := treeFeatureAt(seed, position)
	if ok {
		return feature
	}

	feature, ok = cactusFeatureAt(seed, position, terrain)
	if ok {
		return feature
	}

	feature, ok = surfaceDecorationAt(seed, position, terrain)
	if ok {
		return feature
	}

	return game.Air
}

func (Generator) BiomeAt(seed int64, x, _ int32, z int32) game.Biome {
	return columnAt(seed, x, z).biome
}

func (Generator) GenerateChunk(seed int64, chunkPosition game.ChunkPosition) game.GeneratedChunk {
	chunkMinX := chunkPosition.X * game.ChunkWidth
	chunkMinZ := chunkPosition.Z * game.ChunkWidth

	generated := &generatedChunk{
		seed:      seed,
		position:  chunkPosition,
		minHeight: math.MaxInt32,
		maxHeight: math.MinInt32,
	}

	for localZ := range int32(game.ChunkWidth) {
		for localX := range int32(game.ChunkWidth) {
			terrain := columnAt(seed, chunkMinX+localX, chunkMinZ+localZ)

			generated.columns[localZ*game.ChunkWidth+localX] = terrain
			generated.minHeight = min(generated.minHeight, terrain.height)
			generated.maxHeight = max(generated.maxHeight, terrain.height)
		}
	}

	generated.trees = treesForChunk(seed, chunkPosition)

	return generated
}

func (Generator) GenerateSection(seed int64, chunkPosition game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	generated := Generator{}.GenerateChunk(seed, chunkPosition)
	return generated.GenerateSection(sectionMinY, blocks)
}

func (generated *generatedChunk) GenerateSection(sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < minimumY || sectionMinY > maximumY {
		return game.Air, true
	}

	if sectionMinY >= minimumY+game.ChunkWidth && sectionMaxY <= 31 {
		return game.Stone, true
	}

	chunkMinX := generated.position.X * game.ChunkWidth
	chunkMinZ := generated.position.Z * game.ChunkWidth

	if sectionMinY > max(generated.maxHeight, seaLevel)+maxTreeHeight+2 {
		return game.Air, true
	}

	if sectionMaxY < generated.minHeight-7 && sectionMinY > minimumY+4 {
		return game.Stone, true
	}

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				index := localY*256 + localZ*16 + localX
				position := game.BlockPosition{
					X: chunkMinX + localX,
					Y: worldY,
					Z: chunkMinZ + localZ,
				}

				blocks[index] = terrainBlockAt(generated.seed, position, generated.columns[localZ*game.ChunkWidth+localX])
			}
		}
	}

	applyPreparedTrees(generated.position, sectionMinY, generated.trees, blocks)
	applyColumnFeatures(generated.seed, generated.position, sectionMinY, &generated.columns, blocks)

	first := blocks[0]

	for _, block := range blocks[1:] {
		if block != first {
			return game.Air, false
		}
	}

	return first, true
}

func (generated *generatedChunk) BiomeAt(x, _ int32, z int32) game.Biome {
	localX := x - generated.position.X*game.ChunkWidth
	localZ := z - generated.position.Z*game.ChunkWidth

	if localX < 0 || localX >= game.ChunkWidth || localZ < 0 || localZ >= game.ChunkWidth {
		return columnAt(generated.seed, x, z).biome
	}

	return generated.columns[localZ*game.ChunkWidth+localX].biome
}

func (Generator) WorldMetadata(_ int64) game.WorldMetadata {
	return game.WorldMetadata{SeaLevel: seaLevel}
}

func (Generator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return minimumY, maximumY, true
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}
