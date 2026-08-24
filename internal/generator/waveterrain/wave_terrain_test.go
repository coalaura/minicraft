package waveterrain

import (
	"math"
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

func TestTerrainIsDeterministicAndSeeded(t *testing.T) {
	positions := []game.BlockPosition{
		{X: math.MinInt32, Y: 64, Z: math.MaxInt32},
		{X: -17, Y: 63, Z: -16},
		{X: 0, Y: 60, Z: 0},
		{X: 16, Y: 70, Z: 17},
	}

	generated := Generator{}
	for _, position := range positions {
		first := generated.BlockAt(8, position)
		second := generated.BlockAt(8, position)
		if first != second {
			t.Fatalf("block at %+v changed from %d to %d", position, first, second)
		}
	}

	if surfaceHeight(0, 0, 0) == surfaceHeight(8, 0, 0) {
		t.Fatal("different seeds produced the same test surface height")
	}
}

func TestTerrainFillsThroughItsSurface(t *testing.T) {
	const seed = int64(1234)

	generated := Generator{}
	height := surfaceHeight(seed, -16, 16)

	if block := generated.BlockAt(seed, game.BlockPosition{X: -16, Y: height, Z: 16}); block != game.Stone {
		t.Fatalf("surface block = %d, want stone", block)
	}

	if block := generated.BlockAt(seed, game.BlockPosition{X: -16, Y: height + 1, Z: 16}); block != game.Air {
		t.Fatalf("block above surface = %d, want air", block)
	}
}

func TestTerrainIsContinuousAcrossChunkBoundaries(t *testing.T) {
	boundaries := []int32{-17, -16, -1, 0, 15, 16}

	for _, worldX := range boundaries {
		left := surfaceHeight(42, worldX, 0)
		right := surfaceHeight(42, worldX+1, 0)
		difference := left - right
		if difference < 0 {
			difference = -difference
		}

		if difference > 1 {
			t.Errorf("surface jumps from %d to %d between x=%d and x=%d", left, right, worldX, worldX+1)
		}
	}
}
