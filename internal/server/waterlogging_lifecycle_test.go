package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestPlacementIntoSourceWaterWaterlogsBlock(t *testing.T) {
	clicked := game.BlockPosition{Y: 69}
	target := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(clicked, game.Stone)
	world.SetBlock(target, game.Water)

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, clicked)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakStairs, Count: 1}

	markPlacementChunksLoaded(actor, clicked, target)

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 1))
	if err != nil {
		t.Fatalf("place stairs into water: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(target), "waterlogged", "true")
}

func TestMiningWaterloggedBlockLeavesSourceWater(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	stairs := mustBlockState(t, game.OakStairs, game.BlockPropertyValue{Name: "waterlogged", Value: "true"})

	world := &game.World{}

	world.SetBlock(position, stairs)

	runtime := NewRuntime(world)

	actor, _ := newMiningTestSession(t, runtime, position, game.GameModeCreative, game.ItemAir)

	startMining(t, actor, position, 1)

	if world.BlockAt(position) != game.Water {
		t.Fatalf("block after mining waterlogged stairs = %d, want water", world.BlockAt(position))
	}
}

func TestSupportLossFromWaterloggedBlockLeavesSourceWater(t *testing.T) {
	support := game.BlockPosition{Y: 69}
	position := game.BlockPosition{Y: 70}

	candle := mustBlockState(t, game.Candle, game.BlockPropertyValue{Name: "waterlogged", Value: "true"})

	world := &game.World{}

	world.SetBlock(support, game.Stone)
	world.SetBlock(position, candle)

	runtime := NewRuntime(world)

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: support, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("remove support: result=%+v err=%v", result, err)
	}

	if world.BlockAt(position) != game.Water {
		t.Fatalf("block after support loss = %d, want water", world.BlockAt(position))
	}
}

func TestStrictAuthoritativeReplacementDoesNotRestoreWater(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	stairs := mustBlockState(t, game.OakStairs, game.BlockPropertyValue{Name: "waterlogged", Value: "true"})

	world := &game.World{}

	world.SetBlock(position, stairs)

	runtime := NewRuntime(world)

	result, err := runtime.MutateWorldBlocksStrict([]game.BlockChange{{Position: position, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("strict replacement: result=%+v err=%v", result, err)
	}

	if world.BlockAt(position) != game.Air {
		t.Fatalf("block after strict replacement = %d, want air", world.BlockAt(position))
	}
}
