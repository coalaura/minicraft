package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestItemRaycastUsesCollisionShapes(t *testing.T) {
	tests := map[string]game.Block{
		"above bottom slab": bottomSlabForItemRaycast(t),
		"outside fence":     game.OakFence,
	}

	for name, blocker := range tests {
		t.Run(name, func(t *testing.T) {
			world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

			runtime := NewRuntime(world)

			blockerPosition := game.BlockPosition{X: 0, Y: 70, Z: 1}
			targetPosition := game.BlockPosition{X: 0, Y: 70, Z: 2}

			world.SetBlock(blockerPosition, blocker)
			world.SetBlock(targetPosition, game.Stone)

			player := game.Player{Position: game.Position{X: 0.1, Y: 69.13, Z: 0.5}}

			if name == "above bottom slab" {
				player.Position.X = 0.5
			}

			hit, found := runtime.raycastItemGrid(player, itemUseRange, bucketPlacementTarget)
			if !found || hit.position != targetPosition {
				t.Fatalf("raycast hit = %+v, found = %t, want %v", hit, found, targetPosition)
			}
		})
	}
}

func TestItemRaycastHitsSourceFluidUntilOccluded(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	wallPosition := game.BlockPosition{X: 0, Y: 70, Z: 1}
	fluidPosition := game.BlockPosition{X: 0, Y: 70, Z: 2}

	world.SetBlock(fluidPosition, game.Water)

	player := game.Player{Position: game.Position{X: 0.5, Y: 69.13, Z: 0.5}}

	hit, found := runtime.raycastItemGrid(player, itemUseRange, bucketFillTarget)
	if !found || hit.position != fluidPosition {
		t.Fatalf("source raycast hit = %+v, found = %t, want %v", hit, found, fluidPosition)
	}

	world.SetBlock(wallPosition, game.Stone)

	hit, found = runtime.raycastItemGrid(player, itemUseRange, bucketFillTarget)
	if !found || hit.position != wallPosition {
		t.Fatalf("occluded raycast hit = %+v, found = %t, want %v", hit, found, wallPosition)
	}
}

func bottomSlabForItemRaycast(t *testing.T) game.Block {
	t.Helper()

	slab, valid := game.StoneSlab.WithProperties(game.BlockPropertyValue{Name: "type", Value: "bottom"})
	if !valid {
		t.Fatal("create bottom stone slab")
	}

	return slab
}
