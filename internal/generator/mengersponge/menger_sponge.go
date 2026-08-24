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
