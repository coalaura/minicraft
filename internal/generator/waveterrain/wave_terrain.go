package waveterrain

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const Name = "wave-terrain"

type Generator struct{}

type generatedChunk struct {
	heights   [game.ChunkWidth * game.ChunkWidth]int32
	minHeight int32
	maxHeight int32
}

var (
	_ game.Generator        = Generator{}
	_ game.SectionGenerator = Generator{}
	_ game.ChunkGenerator   = Generator{}
	_ game.BoundedGenerator = Generator{}
	_ game.SpawnGenerator   = Generator{}
	_ game.GeneratedChunk   = (*generatedChunk)(nil)
)

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	if position.Y <= surfaceHeight(seed, position.X, position.Z) {
		return game.Stone
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

func (generated *generatedChunk) GenerateSection(sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY <= generated.minHeight {
		return game.Stone, true
	}

	if sectionMinY > generated.maxHeight {
		return game.Air, true
	}

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				index := localY*256 + localZ*16 + localX

				if worldY <= generated.heights[localZ*game.ChunkWidth+localX] {
					blocks[index] = game.Stone
				} else {
					blocks[index] = game.Air
				}
			}
		}
	}

	return game.Air, false
}

func (Generator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return -1 << 31, 69, true
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

func generateChunk(seed int64, chunk game.ChunkPosition) generatedChunk {
	generated := generatedChunk{
		minHeight: int32(1<<31 - 1),
		maxHeight: int32(-1 << 31),
	}

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for localZ := range int32(game.ChunkWidth) {
		for localX := range int32(game.ChunkWidth) {
			height := surfaceHeight(seed, chunkMinX+localX, chunkMinZ+localZ)

			generated.heights[localZ*game.ChunkWidth+localX] = height

			generated.minHeight = min(generated.minHeight, height)
			generated.maxHeight = max(generated.maxHeight, height)
		}
	}

	return generated
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func surfaceHeight(seed int64, worldX, worldZ int32) int32 {
	xDepth := waveDepth(worldX, seed, 32)
	zSeed := seed ^ (seed >> 32)
	zDepth := waveDepth(worldZ, zSeed, 48)

	return 69 - xDepth/4 - zDepth/6
}

func waveDepth(coordinate int32, offset, period int64) int32 {
	depth := triangularWave(coordinate, offset, period) - triangularWave(0, offset, period)
	if depth < 0 {
		depth = -depth
	}

	return depth
}

func triangularWave(coordinate int32, offset, period int64) int32 {
	phase := positiveRemainder(int64(coordinate), period)
	phase = (phase + positiveRemainder(offset, period)) % period

	if phase > period/2 {
		phase = period - phase
	}

	return int32(phase)
}

func positiveRemainder(value, divisor int64) int64 {
	remainder := value % divisor
	if remainder < 0 {
		remainder += divisor
	}

	return remainder
}
