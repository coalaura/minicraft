package waveterrain

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const Name = "wave-terrain"

type Generator struct{}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	if position.Y <= surfaceHeight(seed, position.X, position.Z) {
		return game.Stone
	}

	return game.Air
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func surfaceHeight(seed int64, worldX, worldZ int32) int32 {
	xWave := triangularWave(worldX, seed, 32)
	zSeed := seed ^ (seed >> 32)
	zWave := triangularWave(worldZ, zSeed, 48)

	return 60 + xWave/4 + zWave/6
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
