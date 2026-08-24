package spawnplatform

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const Name = "spawn-platform"

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
	const (
		platformY      = 69
		platformRadius = 4
	)

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

func (Generator) Spawn(_ int64) game.Position {
	return game.Position{X: 0.5, Y: 70, Z: 0.5}
}
