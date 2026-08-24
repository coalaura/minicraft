package mengersponge

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

func TestGeneratorIsRegistered(t *testing.T) {
	registered, err := generator.New(Name)
	if err != nil {
		t.Fatalf("create registered generator: %v", err)
	}

	if _, ok := registered.(Generator); !ok {
		t.Fatalf("registered generator type = %T", registered)
	}
}

func TestGeneratorSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}

	spawn := generated.Spawn(0)

	position := game.BlockPosition{
		X: int32(spawn.X),
		Y: int32(spawn.Y),
		Z: int32(spawn.Z),
	}

	if block := generated.BlockAt(0, position); block != game.Air {
		t.Fatalf("spawn block = %d, want air", block)
	}

	position.Y++

	if block := generated.BlockAt(0, position); block != game.Air {
		t.Fatalf("block above spawn = %d, want air", block)
	}

	position.Y -= 2

	if block := generated.BlockAt(0, position); block != game.Stone {
		t.Fatalf("block below spawn = %d, want stone", block)
	}
}

func TestGeneratorBuildsMengerPattern(t *testing.T) {
	generated := Generator{}

	tests := []struct {
		name  string
		x     int32
		y     int32
		z     int32
		block game.Block
	}{
		{
			name:  "origin",
			x:     0,
			y:     minBuildY,
			z:     0,
			block: game.Stone,
		},
		{
			name:  "small opening",
			x:     1,
			y:     minBuildY,
			z:     1,
			block: game.Air,
		},
		{
			name:  "small strut",
			x:     1,
			y:     minBuildY,
			z:     0,
			block: game.Stone,
		},
		{
			name:  "next scale opening",
			x:     3,
			y:     minBuildY,
			z:     3,
			block: game.Air,
		},
		{
			name:  "next scale strut",
			x:     3,
			y:     minBuildY,
			z:     0,
			block: game.Stone,
		},
		{
			name:  "vertical opening",
			x:     1,
			y:     minBuildY + 1,
			z:     0,
			block: game.Air,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position := game.BlockPosition{
				X: test.x,
				Y: test.y,
				Z: test.z,
			}

			if block := generated.BlockAt(0, position); block != test.block {
				t.Fatalf("block at %+v = %d, want %d", position, block, test.block)
			}
		})
	}
}

func TestGeneratorExtendsToArbitrarilyLargeCoordinates(t *testing.T) {
	generated := Generator{}

	const farScale = int32(387420489) // 3^18

	strut := game.BlockPosition{
		X: farScale,
		Y: minBuildY,
		Z: 0,
	}

	if block := generated.BlockAt(0, strut); block != game.Stone {
		t.Fatalf("far strut = %d, want stone", block)
	}

	opening := game.BlockPosition{
		X: farScale,
		Y: minBuildY,
		Z: farScale,
	}

	if block := generated.BlockAt(0, opening); block != game.Air {
		t.Fatalf("far opening = %d, want air", block)
	}
}

func TestGeneratorIsSymmetricAcrossNegativeCoordinates(t *testing.T) {
	generated := Generator{}

	positions := []game.BlockPosition{
		{X: 1, Y: minBuildY, Z: 1},
		{X: 27, Y: 72, Z: 9},
		{X: 729, Y: 200, Z: 243},
	}

	for _, positive := range positions {
		expected := generated.BlockAt(0, positive)

		variants := []game.BlockPosition{
			{X: -positive.X, Y: positive.Y, Z: positive.Z},
			{X: positive.X, Y: positive.Y, Z: -positive.Z},
			{X: -positive.X, Y: positive.Y, Z: -positive.Z},
		}

		for _, variant := range variants {
			if block := generated.BlockAt(0, variant); block != expected {
				t.Errorf(
					"block at %+v = %d, want %d to match %+v",
					variant,
					block,
					expected,
					positive,
				)
			}
		}
	}
}

func TestGeneratorStopsAtBuildHeight(t *testing.T) {
	generated := Generator{}

	positions := []game.BlockPosition{
		{X: 0, Y: minBuildY - 1, Z: 0},
		{X: 0, Y: maxBuildY + 1, Z: 0},
	}

	for _, position := range positions {
		if block := generated.BlockAt(0, position); block != game.Air {
			t.Errorf("block outside build height at %+v = %d, want air", position, block)
		}
	}
}
