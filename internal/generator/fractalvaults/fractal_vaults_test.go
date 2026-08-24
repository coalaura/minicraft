package fractalvaults

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestGeneratorSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}
	for _, seed := range []int64{0, 1, -1, 123456789} {
		spawn := generated.Spawn(seed)
		position := game.BlockPosition{X: int32(spawn.X), Y: int32(spawn.Y), Z: int32(spawn.Z)}
		if block := generated.BlockAt(seed, position); block != game.Air {
			t.Fatalf("seed %d spawn block = %d, want air", seed, block)
		}
		position.Y--
		if block := generated.BlockAt(seed, position); block != game.Stone {
			t.Fatalf("seed %d block below spawn = %d, want stone", seed, block)
		}
	}
}
func TestGeneratorBuildsHierarchicalWalls(t *testing.T) {
	generated := Generator{}
	seed := int64(0)
	tests := []struct {
		name string
		x    int32
		y    int32
		z    int32
		want game.Block
	}{
		{name: "interior", x: 4, y: 64, z: 4, want: game.Air},
		{name: "small wall", x: 9, y: 69, z: 7, want: game.Stone},
		{name: "small wall ends", x: 9, y: 70, z: 7, want: game.Air},
		{name: "medium wall rises", x: 27, y: 74, z: 7, want: game.Stone},
		{name: "medium wall ends", x: 27, y: 75, z: 7, want: game.Air},
		{name: "arch opening", x: 27, y: 70, z: 4, want: game.Air},
		{name: "pillar stays solid", x: 27, y: 70, z: 9, want: game.Stone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position := game.BlockPosition{X: test.x, Y: test.y, Z: test.z}
			if block := generated.BlockAt(seed, position); block != test.want {
				t.Fatalf("block at %+v = %d, want %d", position, block, test.want)
			}
		})
	}
}
func TestGeneratorRepeatsAcrossNegativeCoordinates(t *testing.T) {
	generated := Generator{}
	positive := game.BlockPosition{X: 81, Y: 75, Z: 7}
	negative := game.BlockPosition{X: -81, Y: 75, Z: 7}
	if generated.BlockAt(0, positive) != game.Stone {
		t.Fatal("positive hierarchical wall is not stone")
	}
	if generated.BlockAt(0, negative) != game.Stone {
		t.Fatal("negative hierarchical wall is not stone")
	}
}
