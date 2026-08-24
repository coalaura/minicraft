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

func TestDefaultSeedProvidesSafeSpawn(t *testing.T) {
	if height := surfaceHeight(0, 0, 0); height != 69 {
		t.Fatalf("default spawn surface height = %d, want 69", height)
	}
}

func TestTerrainIsContinuousAcrossChunkBoundaries(t *testing.T) {
	boundaries := []int32{math.MinInt32, -49, -48, -33, -32, -17, -16, -1, 0, 15, 16, 31, 32, 47, 48, math.MaxInt32 - 1}
	seeds := []int64{math.MinInt64, -1, 0, 42, math.MaxInt64}

	for _, seed := range seeds {
		for _, coordinate := range boundaries {
			left := surfaceHeight(seed, coordinate, coordinate)
			xRight := surfaceHeight(seed, coordinate+1, coordinate)
			zRight := surfaceHeight(seed, coordinate, coordinate+1)

			assertHeightDifference(t, "x", seed, coordinate, left, xRight)
			assertHeightDifference(t, "z", seed, coordinate, left, zRight)
		}
	}
}

func assertHeightDifference(t *testing.T, axis string, seed int64, coordinate, first, second int32) {
	t.Helper()

	difference := first - second
	if difference < 0 {
		difference = -difference
	}

	if difference > 1 {
		t.Errorf("seed %d surface jumps from %d to %d across %s=%d", seed, first, second, axis, coordinate)
	}
}
