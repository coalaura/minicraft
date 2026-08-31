package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type bucketReplacementTestCase struct {
	item     game.Item
	fluid    game.Block
	wantDrop int32
}

func TestUseItemBucketPicksUpOnlyVisibleSourceFluid(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	position := game.BlockPosition{X: 0, Y: 70, Z: 2}

	world.SetBlock(position, game.Water)

	actor.Player.Position = game.Position{X: 0.5, Y: 68.38, Z: 0.5}
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemBucket, Count: 1}

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	connection.reset()

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 31, Yaw: 0, Pitch: 0})
	if err != nil {
		t.Fatalf("use bucket: %v", err)
	}

	block := world.BlockAt(position)
	if block != game.Air {
		t.Fatalf("source fluid block = %d, want air", block)
	}

	held := actor.snapshotPlayer().Inventory.Hotbar[0]
	if held.Item != game.ItemWaterBucket || held.Count != 1 {
		t.Fatalf("held stack = %+v, want water bucket", held)
	}

	assertBlockChangedAck(t, connection.packets(t)[len(connection.packets(t))-1], 31)
}

func TestUseItemBucketDoesNotPickUpFluidBehindSolidBlock(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	wall := game.BlockPosition{X: 0, Y: 70, Z: 1}
	water := game.BlockPosition{X: 0, Y: 70, Z: 2}

	world.SetBlock(wall, game.Stone)
	world.SetBlock(water, game.Water)

	actor.Player.Position = game.Position{X: 0.5, Y: 68.38, Z: 0.5}
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemBucket, Count: 1}

	actor.loadedChunks = map[LoadedChunk]struct{}{
		blockLoadedChunk(wall):  {},
		blockLoadedChunk(water): {},
	}

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 32, Yaw: 0, Pitch: 0})
	if err != nil {
		t.Fatalf("use bucket through wall: %v", err)
	}

	block := world.BlockAt(water)
	if block != game.Water {
		t.Fatalf("fluid behind wall = %d, want water", block)
	}
}

func TestBucketRaycastUsesFluidModeForHeldBucket(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	nearWater := game.BlockPosition{X: 0, Y: 70, Z: 2}
	farWater := game.BlockPosition{X: 0, Y: 70, Z: 3}
	outlinedBlock := game.BlockPosition{X: 0, Y: 70, Z: 4}

	world.SetBlocks([]game.BlockChange{
		{Position: nearWater, Replacement: game.Water},
		{Position: farWater, Replacement: game.Water},
		{Position: outlinedBlock, Replacement: game.Stone},
	})

	player := game.Player{
		Position: game.Position{X: 0.5, Y: 68.38, Z: 0.5},
		Rotation: game.Rotation{},
	}

	fillHit, found := runtime.raycastItemGrid(player, itemUseRange, bucketFillTarget)
	if !found || fillHit.position != nearWater {
		t.Fatalf("empty bucket hit = %+v, found %t, want near source", fillHit, found)
	}

	placementHit, found := runtime.raycastItemGrid(player, itemUseRange, bucketPlacementTarget)
	if !found || placementHit.position != outlinedBlock {
		t.Fatalf("filled bucket hit = %+v, found %t, want outlined block through water", placementHit, found)
	}
}

func TestBucketUseItemOnPassesBeforeItemUse(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	position := game.BlockPosition{X: 8, Y: 70, Z: 8}

	world.SetBlock(position, game.OakStairs)
	world.SetBlock(game.BlockPosition{X: position.X, Y: position.Y - 1, Z: position.Z}, game.Stone)

	actor.Player.Position = game.Position{X: 8.5, Y: 68.38, Z: 6.5}
	actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemWaterBucket, Count: 1}

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceNorth, protocol.OffHand, 33))
	if err != nil {
		t.Fatalf("pass bucket use on block: %v", err)
	}

	if world.BlockAt(position) != game.OakStairs {
		t.Fatalf("block after use item on = %d, want dry stairs", world.BlockAt(position))
	}

	held := actor.snapshotPlayer().Inventory.Offhand
	if held.Item != game.ItemWaterBucket || held.Count != 1 {
		t.Fatalf("offhand stack after use item on = %+v, want water bucket", held)
	}

	err = actor.handleUseItem(protocol.UseItem{Hand: protocol.OffHand, Sequence: 34, Yaw: 0, Pitch: 0})
	if err != nil {
		t.Fatalf("use water bucket: %v", err)
	}

	block := world.BlockAt(position)
	if !waterlogged(block) {
		t.Fatalf("waterlogged block = %d, want waterlogged stairs", block)
	}

	held = actor.snapshotPlayer().Inventory.Offhand
	if held.Item != game.ItemBucket || held.Count != 1 {
		t.Fatalf("offhand stack = %+v, want bucket", held)
	}

	above := game.BlockPosition{X: position.X, Y: position.Y + 1, Z: position.Z}
	if world.BlockAt(above) != game.Air {
		t.Fatalf("block above waterlogged stairs = %v, want air", world.BlockAt(above))
	}

	if len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatalf("waterlogging spawned %d item entities, want none", len(runtime.snapshotRuntimeEntities()))
	}

	runtime.setSessionActiveChunks(actor, []LoadedChunk{blockLoadedChunk(position)})

	for range waterFluidDelay {
		tickFluidAcceptanceSchedule(runtime)
	}

	if world.BlockAt(above) != game.Air {
		t.Fatalf("block above waterlogged stairs after fluid tick = %v, want air", world.BlockAt(above))
	}

	blocked := game.BlockPosition{X: position.X, Y: position.Y, Z: position.Z - 1}
	if !world.FluidAt(blocked).Empty() {
		t.Fatalf("fluid passed through solid stair face: %+v", world.FluidAt(blocked))
	}

	openFaces := []game.BlockPosition{
		{X: position.X - 1, Y: position.Y, Z: position.Z},
		{X: position.X + 1, Y: position.Y, Z: position.Z},
		{X: position.X, Y: position.Y, Z: position.Z + 1},
	}

	for _, open := range openFaces {
		if world.FluidAt(open).Type() != game.FluidTypeWater {
			t.Fatalf("open stair face at %v did not release water", open)
		}
	}
}

func TestWaterBucketSucceedsInExistingSource(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	position := game.BlockPosition{X: 0, Y: 70, Z: 2}

	world.SetBlock(position, game.Water)
	world.SetBlock(game.BlockPosition{X: position.X, Y: position.Y, Z: position.Z + 1}, game.Stone)

	actor.Player.Position = game.Position{X: 0.5, Y: 68.38, Z: 0.5}
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemWaterBucket, Count: 1}

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceNorth, protocol.MainHand, 37))
	if err != nil {
		t.Fatalf("pass water bucket use on source: %v", err)
	}

	held := actor.snapshotPlayer().Inventory.Hotbar[0]
	if world.BlockAt(position) != game.Water || held.Item != game.ItemWaterBucket || held.Count != 1 {
		t.Fatalf("use item on changed source or held stack: block %v, stack %+v", world.BlockAt(position), held)
	}

	err = actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 38, Yaw: 0, Pitch: 0})
	if err != nil {
		t.Fatalf("empty water bucket into source: %v", err)
	}

	if world.BlockAt(position) != game.Water {
		t.Fatalf("source block = %v, want water", world.BlockAt(position))
	}

	held = actor.snapshotPlayer().Inventory.Hotbar[0]
	if held.Item != game.ItemBucket || held.Count != 1 {
		t.Fatalf("held stack = %+v, want empty bucket", held)
	}

	if len(runtime.runtimeBlockMutations) != 0 {
		t.Fatalf("source placement queued %d block mutations, want none", len(runtime.runtimeBlockMutations))
	}
}

func TestUseItemBucketPicksUpWaterloggedSource(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	position := game.BlockPosition{X: 0, Y: 70, Z: 2}

	stairs, valid := game.OakStairs.WithProperties(game.BlockPropertyValue{Name: "waterlogged", Value: "true"})
	if !valid {
		t.Fatal("waterlog oak stairs")
	}

	world.SetBlock(position, stairs)
	world.SetBlock(game.BlockPosition{X: position.X, Y: position.Y - 1, Z: position.Z}, game.Stone)

	actor.Player.Position = game.Position{X: 0.5, Y: 68.38, Z: 0.5}
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemBucket, Count: 1}

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 34, Yaw: 0, Pitch: 0})
	if err != nil {
		t.Fatalf("pick up waterlogged source: %v", err)
	}

	if waterlogged(world.BlockAt(position)) {
		t.Fatal("stairs remain waterlogged")
	}

	held := actor.snapshotPlayer().Inventory.Hotbar[0]
	if held.Item != game.ItemWaterBucket || held.Count != 1 {
		t.Fatalf("held stack = %+v, want water bucket", held)
	}
}

func TestUseItemBucketDoesNotPickUpFlowingFluid(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	position := game.BlockPosition{X: 0, Y: 70, Z: 2}

	flowing, valid := game.Water.WithProperties(game.BlockPropertyValue{Name: "level", Value: "1"})
	if !valid {
		t.Fatal("make flowing water")
	}

	world.SetBlock(position, flowing)

	actor.Player.Position = game.Position{X: 0.5, Y: 68.38, Z: 0.5}
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemBucket, Count: 1}

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 35, Yaw: 0, Pitch: 0})
	if err != nil {
		t.Fatalf("pick up flowing water: %v", err)
	}

	if world.BlockAt(position) != flowing {
		t.Fatalf("flowing water = %d, want unchanged", world.BlockAt(position))
	}
}

func TestWaterBucketReplacesFlowingWaterWithSource(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	target := game.BlockPosition{X: 8, Y: 70, Z: 8}
	support := game.BlockPosition{X: 8, Y: 69, Z: 8}

	flowing, valid := game.Water.WithProperties(game.BlockPropertyValue{Name: "level", Value: "1"})
	if !valid {
		t.Fatal("make flowing water")
	}

	world.SetBlocks([]game.BlockChange{
		{Position: target, Replacement: flowing},
		{Position: support, Replacement: game.Stone},
	})

	actor.Player.Position = blockMutationTestPlayerPosition(target)
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemWaterBucket, Count: 1}

	markChunkLoaded(actor, target)

	joinTestSession(t, runtime, actor)

	used, err := runtime.useBucketOn(actor, testUseItemOn(support, protocol.BlockFaceUp, protocol.MainHand, 36), actor.Player.Inventory.Hotbar[0])
	if err != nil || !used {
		t.Fatalf("place source into flowing water: used %t, err %v", used, err)
	}

	if world.BlockAt(target) != game.Water {
		t.Fatalf("target water = %+v, want source", world.FluidAt(target))
	}

	held := actor.snapshotPlayer().Inventory.Hotbar[0]
	if held.Item != game.ItemBucket || held.Count != 1 {
		t.Fatalf("held stack = %+v, want empty bucket", held)
	}
}

func TestLavaBucketMixesImmediatelyBesideWater(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	target := game.BlockPosition{X: 8, Y: 70, Z: 8}
	support := game.BlockPosition{X: 8, Y: 69, Z: 8}
	water := game.BlockPosition{X: 9, Y: 70, Z: 8}

	world.SetBlocks([]game.BlockChange{
		{Position: support, Replacement: game.Stone},
		{Position: water, Replacement: game.Water},
	})

	actor.Player.Position = blockMutationTestPlayerPosition(target)
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemLavaBucket, Count: 1}

	markChunkLoaded(actor, target)

	joinTestSession(t, runtime, actor)

	used, err := runtime.useBucketOn(actor, testUseItemOn(support, protocol.BlockFaceUp, protocol.MainHand, 37), actor.Player.Inventory.Hotbar[0])
	if err != nil || !used {
		t.Fatalf("place lava beside water: used %t, err %v", used, err)
	}

	if world.BlockAt(target) != game.Obsidian {
		t.Fatalf("lava bucket contact = %v, want obsidian", world.BlockAt(target))
	}

	held := actor.snapshotPlayer().Inventory.Hotbar[0]
	if held.Item != game.ItemBucket || held.Count != 1 {
		t.Fatalf("held stack = %+v, want empty bucket", held)
	}
}

func TestBucketsDropReplacedBlocks(t *testing.T) {
	tests := map[string]bucketReplacementTestCase{
		"water": {item: game.ItemWaterBucket, fluid: game.Water, wantDrop: 1},
		"lava":  {item: game.ItemLavaBucket, fluid: game.Lava, wantDrop: 1},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

			runtime := NewRuntime(world)

			actor, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

			position := game.BlockPosition{X: 0, Y: 70, Z: 0}

			world.SetBlock(position, game.WarpedRoots)

			actor.Player.Position = blockMutationTestPlayerPosition(position)
			actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: test.item, Count: 1}

			markChunkLoaded(actor, position)

			joinTestSession(t, runtime, actor)

			used, err := runtime.useBucketOn(actor, testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 36), actor.Player.Inventory.Hotbar[0])
			if err != nil || !used {
				t.Fatalf("empty bucket: used %t, err %v", used, err)
			}

			if world.BlockAt(position) != test.fluid {
				t.Fatalf("block = %d, want %d", world.BlockAt(position), test.fluid)
			}

			state, err := protocolBlockState(test.fluid)
			if err != nil {
				t.Fatalf("encode bucket fluid: %v", err)
			}

			foundUpdate := false

			for _, packet := range connection.packets(t) {
				if packet.ID != protocol.ClientboundBlockUpdateID {
					continue
				}

				assertBlockUpdate(t, packet, position, state)

				foundUpdate = true

				break
			}

			if !foundUpdate {
				t.Fatal("missing bucket fluid block update")
			}

			drops := countDroppedItem(runtime, game.ItemWarpedRoots)
			if drops != test.wantDrop {
				t.Fatalf("warped roots drops = %d, want %d", drops, test.wantDrop)
			}
		})
	}
}
