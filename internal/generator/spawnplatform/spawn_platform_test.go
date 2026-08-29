package spawnplatform_test

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
	"github.com/coalaura/minicraft/internal/generator/spawnplatform"
)

func TestGeneratorIsRegistered(t *testing.T) {
	registered, err := generator.New(spawnplatform.Name)
	if err != nil {
		t.Fatalf("create registered generator: %v", err)
	}

	if _, ok := registered.(spawnplatform.Generator); !ok {
		t.Fatalf("registered generator type = %T", registered)
	}
}

func TestSpawnPlatform(t *testing.T) {
	world := game.NewOverworld(spawnplatform.New())

	tests := map[game.BlockPosition]game.Block{
		{X: -4, Y: 69, Z: -4}: game.Stone,
		{X: 4, Y: 69, Z: 4}:   game.Stone,
		{X: -5, Y: 69, Z: 0}:  game.Air,
		{X: 0, Y: 68, Z: 0}:   game.Air,
		{X: 0, Y: 70, Z: 0}:   game.Air,
	}

	for position, expected := range tests {
		actual := world.BlockAt(position)
		if actual != expected {
			t.Errorf("block at %+v = %d, want %d", position, actual, expected)
		}
	}
}
