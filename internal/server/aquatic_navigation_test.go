package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestSwimStartNodeUsesBoundingBoxConvention(t *testing.T) {
	position := game.Position{X: 3.4, Y: 10.6, Z: -2.1}

	node := swimStartNode(position, 0.6, 0.4)
	want := game.BlockPosition{X: 3, Y: 11, Z: -3}

	if node != want {
		t.Fatalf("node = %+v, want %+v", node, want)
	}
}

func TestSwimNodeEvaluatorRequiresWaterAndClearCollision(t *testing.T) {
	runtime := newAquaticNavigationRuntime()

	node := game.BlockPosition{X: 0, Y: 10, Z: 0}

	fillAquaticNavigationWater(runtime.World, -1, 2, 9, 12, -1, 2)

	if !runtime.swimNodeValid(node, 0.6, 0.8) {
		t.Fatal("water node should be valid")
	}

	runtime.World.SetBlock(game.BlockPosition{X: 1, Y: 10}, game.Air)

	if runtime.swimNodeValid(node, 1.2, 0.8) {
		t.Fatal("node overlapping a dry cell should be invalid")
	}

	runtime.World.SetBlock(game.BlockPosition{X: 1, Y: 10}, game.Water)
	runtime.World.SetBlock(node, game.Stone)

	if runtime.swimNodeValid(node, 0.6, 0.8) {
		t.Fatal("node overlapping a collision box should be invalid")
	}
}

func TestFindSwimPathMovesVerticallyAndReusesDestination(t *testing.T) {
	runtime := newAquaticNavigationRuntime()

	fillAquaticNavigationWater(runtime.World, -1, 2, 9, 14, -1, 2)

	destination := make([]game.Position, 0, 8)
	path := runtime.findSwimPathInto(destination, game.Position{X: 0.5, Y: 10, Z: 0.5}, game.Position{X: 0.5, Y: 12, Z: 0.5}, 0.6, 0.8, 8)

	if len(path) != 2 {
		t.Fatalf("path length = %d, want 2: %+v", len(path), path)
	}

	if path[0] != (game.Position{X: 0.5, Y: 11, Z: 0.5}) || path[1] != (game.Position{X: 0.5, Y: 12, Z: 0.5}) {
		t.Fatalf("path = %+v, want vertical waypoints", path)
	}

	if &path[0] != &destination[:cap(destination)][0] {
		t.Fatal("path did not reuse destination storage")
	}
}

func TestFindSwimPathRejectsDiagonalCornerCutting(t *testing.T) {
	runtime := newAquaticNavigationRuntime()

	start := game.BlockPosition{X: 0, Y: 10, Z: 0}
	goal := game.BlockPosition{X: 1, Y: 10, Z: 1}

	runtime.World.SetBlock(start, game.Water)
	runtime.World.SetBlock(goal, game.Water)

	path := runtime.findSwimPath(game.Position{X: 0.5, Y: 10, Z: 0.5}, game.Position{X: 1.5, Y: 10, Z: 1.5}, 0.6, 0.8, 8)

	if len(path) != 0 {
		t.Fatalf("path through blocked diagonal = %+v, want none", path)
	}
}

func TestAdvanceSwimNavigationOnlyShortcutsThroughWater(t *testing.T) {
	runtime := newAquaticNavigationRuntime()

	fillAquaticNavigationWater(runtime.World, -1, 4, 9, 12, -1, 2)

	navigation := swimNavigation{}

	path := []game.Position{
		{X: 1.5, Y: 10, Z: 0.5},
		{X: 2.5, Y: 10, Z: 0.5},
		{X: 3.5, Y: 10, Z: 0.5},
	}

	navigation.SetPath(path, 0.8, path[len(path)-1])

	runtime.World.SetBlock(game.BlockPosition{X: 2, Y: 10}, game.Air)

	waypoint, active := runtime.advanceSwimNavigation(game.Position{X: 0.5, Y: 10, Z: 0.5}, 0.6, 0.8, &navigation)

	if !active || waypoint != path[0] || navigation.Index != 0 {
		t.Fatalf("dry shortcut = waypoint %+v, active %t, index %d", waypoint, active, navigation.Index)
	}

	runtime.World.SetBlock(game.BlockPosition{X: 2, Y: 10}, game.Water)

	waypoint, active = runtime.advanceSwimNavigation(game.Position{X: 0.5, Y: 10, Z: 0.5}, 0.6, 0.8, &navigation)

	if !active || waypoint != path[2] || navigation.Index != 2 {
		t.Fatalf("clear shortcut = waypoint %+v, active %t, index %d", waypoint, active, navigation.Index)
	}
}

func newAquaticNavigationRuntime() *Runtime {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	return NewRuntime(world)
}

func fillAquaticNavigationWater(world *game.World, minimumX, maximumX, minimumY, maximumY, minimumZ, maximumZ int32) {
	for y := minimumY; y <= maximumY; y++ {
		for x := minimumX; x <= maximumX; x++ {
			for z := minimumZ; z <= maximumZ; z++ {
				world.SetBlock(game.BlockPosition{X: x, Y: y, Z: z}, game.Water)
			}
		}
	}
}
