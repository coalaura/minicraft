package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type structuralConnectionTestCase struct {
	name         string
	first        game.Block
	second       game.Block
	property     string
	connected    string
	disconnected string
}

type authoritativeDoorMutationTestCase struct {
	name    string
	changes []game.BlockChange
}

type structuralSupportTestCase struct {
	name    string
	support game.Block
	block   game.Block
}

func TestSlabPlacementAndMerging(t *testing.T) {
	for name, cursorY := range map[string]float32{"bottom": 0.25, "top": 0.75} {
		t.Run(name, func(t *testing.T) {
			clicked := game.BlockPosition{Y: 70}
			target := game.BlockPosition{X: 1, Y: 70}

			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			actor, _ := newPlacementTestSession(runtime, clicked)

			actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakSlab, Count: 64}

			joinTestSession(t, runtime, actor)

			interaction := testUseItemOn(clicked, protocol.BlockFaceEast, protocol.MainHand, 1)

			interaction.CursorY = cursorY

			err := actor.handleUseItemOn(interaction)
			if err != nil {
				t.Fatalf("place slab: %v", err)
			}

			assertBlockProperty(t, world.BlockAt(target), "type", name)
		})
	}

	clicked := game.BlockPosition{Y: 70}

	world := &game.World{}

	oakBottom := mustBlockState(t, game.OakSlab, game.BlockPropertyValue{Name: "type", Value: "bottom"})

	world.SetBlock(clicked, oakBottom)

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, clicked)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakSlab, Count: 64}

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 2))
	if err != nil {
		t.Fatalf("merge slab: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(clicked), "type", "double")
	assertBlockProperty(t, world.BlockAt(clicked), "waterlogged", "false")

	if _, merge := slabMerge(game.StoneSlab, oakBottom, protocol.BlockFaceUp, 0.5); merge {
		t.Fatal("incompatible slab types merged")
	}
}

func TestStairPlacementAndNeighborShape(t *testing.T) {
	clicked := game.BlockPosition{Y: 70}
	target := game.BlockPosition{X: 1, Y: 70}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, clicked)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakStairs, Count: 64}
	actor.Player.Rotation.Yaw = 90

	joinTestSession(t, runtime, actor)

	interaction := testUseItemOn(clicked, protocol.BlockFaceEast, protocol.MainHand, 3)

	interaction.CursorY = 0.75

	err := actor.handleUseItemOn(interaction)
	if err != nil {
		t.Fatalf("place stairs: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(target), "facing", "west")
	assertBlockProperty(t, world.BlockAt(target), "half", "top")

	center := game.BlockPosition{X: 4, Y: 70}
	front := game.BlockPosition{X: 4, Y: 70, Z: 1}

	south := mustBlockState(t, game.OakStairs,
		game.BlockPropertyValue{Name: "facing", Value: "south"},
		game.BlockPropertyValue{Name: "half", Value: "bottom"},
	)

	east := mustBlockState(t, game.OakStairs,
		game.BlockPropertyValue{Name: "facing", Value: "east"},
		game.BlockPropertyValue{Name: "half", Value: "bottom"},
	)

	world.SetBlock(center, south)

	actor.Player.Position = blockMutationTestPlayerPosition(center)

	markPlacementChunksLoaded(actor, center, front)

	result, err := runtime.MutateBlocks(actor, BlockMutationPlace, []game.BlockChange{{Position: front, Replacement: east}})
	if err != nil || !result.Changed {
		t.Fatalf("place neighboring stair: result=%+v err=%v", result, err)
	}

	assertBlockProperty(t, world.BlockAt(center), "shape", "outer_left")

	_, err = runtime.MutateBlock(actor, BlockMutationBreak, front, game.Air)
	if err != nil {
		t.Fatalf("break neighboring stair: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(center), "shape", "straight")
}

func TestDoorPlacementInteractionBreakingAndSynchronization(t *testing.T) {
	clicked := game.BlockPosition{Y: 69}
	lower := game.BlockPosition{Y: 70}
	upper := game.BlockPosition{Y: 71}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	runtime := NewRuntime(world)

	actor, actorConnection := newPlacementTestSession(runtime, clicked)
	observer, observerConnection := newPlacementTestSession(runtime, clicked)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakDoor, Count: 1}

	markPlacementChunksLoaded(actor, clicked, lower, upper)
	markPlacementChunksLoaded(observer, clicked, lower, upper)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 4))
	if err != nil {
		t.Fatalf("place door: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(lower), "half", "lower")
	assertBlockProperty(t, world.BlockAt(upper), "half", "upper")
	assertBlockProperty(t, world.BlockAt(lower), "facing", "south")

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
	assertSoundEvent(t, observerConnection.packets(t)[2], game.SoundBlockWoodPlace)

	actorConnection.reset()
	observerConnection.reset()

	err = actor.handleUseItemOn(testUseItemOn(lower, protocol.BlockFaceNorth, protocol.MainHand, 5))
	if err != nil {
		t.Fatalf("open door: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(lower), "open", "true")
	assertBlockProperty(t, world.BlockAt(upper), "open", "true")

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
	assertSoundEvent(t, observerConnection.packets(t)[2], game.SoundBlockWoodenDoorOpen)

	actorConnection.reset()
	observerConnection.reset()

	err = actor.handleUseItemOn(testUseItemOn(lower, protocol.BlockFaceNorth, protocol.MainHand, 6))
	if err != nil {
		t.Fatalf("close door: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(lower), "open", "false")
	assertSoundEvent(t, observerConnection.packets(t)[2], game.SoundBlockWoodenDoorClose)

	actorConnection.reset()
	observerConnection.reset()

	originalUpperState, err := protocolBlockState(world.BlockAt(upper))
	if err != nil {
		t.Fatalf("encode upper door state: %v", err)
	}

	err = actor.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionStartDestroyBlock, Position: upper, Sequence: 7})
	if err != nil {
		t.Fatalf("break door: %v", err)
	}

	if world.BlockAt(lower) != game.Air || world.BlockAt(upper) != game.Air {
		t.Fatalf("door halves after break = %d, %d", world.BlockAt(lower), world.BlockAt(upper))
	}

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockUpdateID, protocol.ClientboundLevelEventID})
	assertLevelEvent(t, observerConnection.packets(t)[2], protocol.LevelEventBlockBreak, upper, originalUpperState, false)
}

func TestDoorPlacementFailsAtomicallyWhenUpperPositionIsOccupied(t *testing.T) {
	clicked := game.BlockPosition{Y: 69}
	lower := game.BlockPosition{Y: 70}
	upper := game.BlockPosition{Y: 71}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	world.SetBlock(upper, game.Stone)

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, clicked)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakDoor, Count: 1}

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 7))
	if err != nil {
		t.Fatalf("place blocked door: %v", err)
	}

	if world.BlockAt(lower) != game.Air || world.BlockAt(upper) != game.Stone {
		t.Fatalf("blocked door changed world: lower=%d upper=%d", world.BlockAt(lower), world.BlockAt(upper))
	}
}

func TestIronDoorBreaksAsTwoBlockStructureWithoutBecomingInteractable(t *testing.T) {
	lowerPosition := game.BlockPosition{Y: 70}
	upperPosition := game.BlockPosition{Y: 71}

	lower, valid := game.IronDoor.WithProperties(game.BlockPropertyValue{Name: "half", Value: "lower"})
	if !valid {
		t.Fatal("resolve lower iron door state")
	}

	upper, valid := game.IronDoor.WithProperties(game.BlockPropertyValue{Name: "half", Value: "upper"})
	if !valid {
		t.Fatal("resolve upper iron door state")
	}

	for name, brokenPosition := range map[string]game.BlockPosition{"lower": lowerPosition, "upper": upperPosition} {
		t.Run(name, func(t *testing.T) {
			world := &game.World{}

			world.SetBlock(lowerPosition, lower)
			world.SetBlock(upperPosition, upper)

			runtime := NewRuntime(world)

			actor, _ := newPlacementTestSession(runtime, lowerPosition)

			markPlacementChunksLoaded(actor, lowerPosition, upperPosition)

			joinTestSession(t, runtime, actor)

			handled, _, _, err := runtime.InteractBlock(actor, lowerPosition)
			if err != nil {
				t.Fatalf("interact with iron door: %v", err)
			}

			if handled || world.BlockAt(lowerPosition) != lower || world.BlockAt(upperPosition) != upper {
				t.Fatal("iron door became manually interactable")
			}

			result, err := runtime.MutateBlock(actor, BlockMutationBreak, brokenPosition, game.Air)
			if err != nil {
				t.Fatalf("break iron door: %v", err)
			}

			if !result.Allowed || !result.Changed || world.BlockAt(lowerPosition) != game.Air || world.BlockAt(upperPosition) != game.Air {
				t.Fatalf("iron door halves after break = %d, %d; result = %+v", world.BlockAt(lowerPosition), world.BlockAt(upperPosition), result)
			}
		})
	}
}

func TestAuthoritativeMutationsRemoveMatchingDoorHalves(t *testing.T) {
	lowerPosition := game.BlockPosition{Y: 70}
	upperPosition := game.BlockPosition{Y: 71}

	lower := mustBlockState(t, game.OakDoor, game.BlockPropertyValue{Name: "half", Value: "lower"})
	upper := mustBlockState(t, game.OakDoor, game.BlockPropertyValue{Name: "half", Value: "upper"})

	tests := []authoritativeDoorMutationTestCase{
		{name: "replace lower", changes: []game.BlockChange{{Position: lowerPosition, Replacement: game.Stone}}},
		{name: "replace upper", changes: []game.BlockChange{{Position: upperPosition, Replacement: game.Stone}}},
		{name: "replace both", changes: []game.BlockChange{{Position: lowerPosition, Replacement: game.Stone}, {Position: upperPosition, Replacement: game.Stone}}},
		{name: "replace lower while fill includes upper", changes: []game.BlockChange{{Position: lowerPosition, Replacement: game.Air}, {Position: upperPosition, Replacement: upper}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{}

			world.SetBlock(lowerPosition, lower)
			world.SetBlock(upperPosition, upper)

			runtime := NewRuntime(world)

			result, err := runtime.MutateWorldBlocks(test.changes)
			if err != nil {
				t.Fatalf("mutate world blocks: %v", err)
			}

			if !result.Changed {
				t.Fatalf("authoritative door mutation did not change world: %+v", result)
			}

			if len(test.changes) == 1 {
				if world.BlockAt(test.changes[0].Position) != game.Stone {
					t.Fatalf("changed door position %+v = %d, want stone", test.changes[0].Position, world.BlockAt(test.changes[0].Position))
				}

				otherPosition := lowerPosition
				if test.changes[0].Position == lowerPosition {
					otherPosition = upperPosition
				}

				if world.BlockAt(otherPosition) != game.Air {
					t.Fatalf("matching door half at %+v = %d, want air", otherPosition, world.BlockAt(otherPosition))
				}
			} else if test.name == "replace both" {
				if world.BlockAt(lowerPosition) != game.Stone || world.BlockAt(upperPosition) != game.Stone {
					t.Fatalf("replaced door halves = %d, %d, want stone", world.BlockAt(lowerPosition), world.BlockAt(upperPosition))
				}
			} else if world.BlockAt(lowerPosition) != game.Air || world.BlockAt(upperPosition) != game.Air {
				t.Fatalf("fill-intersected door halves = %d, %d, want air", world.BlockAt(lowerPosition), world.BlockAt(upperPosition))
			}
		})
	}
}

func TestAuthoritativeBulkMutationRecalculatesStructuralNeighbors(t *testing.T) {
	world := &game.World{}

	runtime := NewRuntime(world)

	changes := make([]game.BlockChange, 0, 9)

	for blockX := int32(0); blockX < 9; blockX++ {
		changes = append(changes, game.BlockChange{
			Position:    game.BlockPosition{X: blockX, Y: 70},
			Replacement: game.OakFence,
		})
	}

	result, err := runtime.MutateWorldBlocks(changes)
	if err != nil || !result.Changed {
		t.Fatalf("place fence row: result=%+v err=%v", result, err)
	}

	assertBlockProperty(t, world.BlockAt(game.BlockPosition{X: 4, Y: 70}), "west", "true")
	assertBlockProperty(t, world.BlockAt(game.BlockPosition{X: 4, Y: 70}), "east", "true")

	removals := []game.BlockChange{
		{Position: game.BlockPosition{X: 3, Y: 70}, Replacement: game.Air},
		{Position: game.BlockPosition{X: 4, Y: 70}, Replacement: game.Air},
		{Position: game.BlockPosition{X: 5, Y: 70}, Replacement: game.Air},
	}

	result, err = runtime.MutateWorldBlocks(removals)
	if err != nil || !result.Changed {
		t.Fatalf("remove fence row segment: result=%+v err=%v", result, err)
	}

	assertBlockProperty(t, world.BlockAt(game.BlockPosition{X: 2, Y: 70}), "east", "false")
	assertBlockProperty(t, world.BlockAt(game.BlockPosition{X: 6, Y: 70}), "west", "false")
}

func TestTrapdoorAndFenceGateInteraction(t *testing.T) {
	for name, item := range map[string]game.Item{"trapdoor": game.ItemOakTrapdoor, "fence_gate": game.ItemOakFenceGate} {
		t.Run(name, func(t *testing.T) {
			clicked := game.BlockPosition{Y: 70}
			target := game.BlockPosition{X: 1, Y: 70}

			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			actor, connection := newPlacementTestSession(runtime, clicked)

			actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: item, Count: 1}

			joinTestSession(t, runtime, actor)

			interaction := testUseItemOn(clicked, protocol.BlockFaceEast, protocol.MainHand, 8)

			interaction.CursorY = 0.75

			err := actor.handleUseItemOn(interaction)
			if err != nil {
				t.Fatalf("place: %v", err)
			}

			if name == "trapdoor" {
				assertBlockProperty(t, world.BlockAt(target), "half", "top")
				assertBlockProperty(t, world.BlockAt(target), "facing", "east")
			}

			connection.reset()

			err = actor.handleUseItemOn(testUseItemOn(target, protocol.BlockFaceUp, protocol.MainHand, 9))
			if err != nil {
				t.Fatalf("interact: %v", err)
			}

			assertBlockProperty(t, world.BlockAt(target), "open", "true")

			openSound := game.SoundBlockFenceGateOpen
			closeSound := game.SoundBlockFenceGateClose

			if name == "trapdoor" {
				openSound = game.SoundBlockWoodenTrapdoorOpen
				closeSound = game.SoundBlockWoodenTrapdoorClose
			}

			assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID, protocol.ClientboundBlockChangedAckID})
			assertSoundEvent(t, connection.packets(t)[1], openSound)

			connection.reset()

			err = actor.handleUseItemOn(testUseItemOn(target, protocol.BlockFaceUp, protocol.MainHand, 10))
			if err != nil {
				t.Fatalf("close: %v", err)
			}

			assertBlockProperty(t, world.BlockAt(target), "open", "false")
			assertSoundEvent(t, connection.packets(t)[1], closeSound)
		})
	}
}

func TestStructuralConnectionsAndNeighborUpdates(t *testing.T) {
	world := &game.World{}

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, game.BlockPosition{Y: 70})

	actor.Player.Position = game.Position{X: 2, Y: 71, Z: 0}

	markPlacementChunksLoaded(actor, game.BlockPosition{})

	joinTestSession(t, runtime, actor)

	tests := []structuralConnectionTestCase{
		{name: "fence", first: game.OakFence, second: game.SpruceFence, property: "east", connected: "true", disconnected: "false"},
		{name: "pane_and_bars", first: game.GlassPane, second: game.IronBars, property: "east", connected: "true", disconnected: "false"},
		{name: "wall", first: game.CobblestoneWall, second: game.BrickWall, property: "east", connected: "low", disconnected: "none"},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := game.BlockPosition{X: int32(index * 3), Y: 70}
			second := game.BlockPosition{X: first.X + 1, Y: 70}

			result, err := runtime.MutateBlocks(actor, BlockMutationPlace, []game.BlockChange{{Position: first, Replacement: test.first}, {Position: second, Replacement: test.second}})
			if err != nil || !result.Changed {
				t.Fatalf("place connected blocks: result=%+v err=%v", result, err)
			}

			assertBlockProperty(t, world.BlockAt(first), test.property, test.connected)
			assertBlockProperty(t, world.BlockAt(second), "west", test.connected)

			_, err = runtime.MutateBlock(actor, BlockMutationBreak, second, game.Air)
			if err != nil {
				t.Fatalf("break neighbor: %v", err)
			}

			assertBlockProperty(t, world.BlockAt(first), test.property, test.disconnected)
		})
	}
}

func TestFenceGateUpdatesInWallState(t *testing.T) {
	position := game.BlockPosition{Y: 70}
	west := game.BlockPosition{X: -1, Y: 70}

	world := &game.World{}

	world.SetBlock(west, game.CobblestoneWall)

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, position)

	actor.Player.Position = blockMutationTestPlayerPosition(position)

	markPlacementChunksLoaded(actor, position, west)

	joinTestSession(t, runtime, actor)

	result, err := runtime.MutateBlocks(actor, BlockMutationPlace, []game.BlockChange{{Position: position, Replacement: game.OakFenceGate}})
	if err != nil || !result.Changed {
		t.Fatalf("place gate: result=%+v err=%v", result, err)
	}

	assertBlockProperty(t, world.BlockAt(position), "in_wall", "true")

	_, err = runtime.MutateBlock(actor, BlockMutationBreak, west, game.Air)
	if err != nil {
		t.Fatalf("break wall: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(position), "in_wall", "false")
}

func TestDeniedStructuralInteractionResynchronizesWithoutMutation(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	trapdoor := mustBlockState(t, game.OakTrapdoor, game.BlockPropertyValue{Name: "open", Value: "false"})

	world := &game.World{}

	world.SetBlock(position, trapdoor)

	runtime := NewRuntime(world)

	runtime.AllowBlockPlacing = false

	actor, actorConnection := newPlacementTestSession(runtime, position)
	observer, observerConnection := newPlacementTestSession(runtime, position)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 10))
	if err != nil {
		t.Fatalf("denied interaction: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(position), "open", "false")

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockChangedAckID})
	assertPacketIDs(t, observerConnection.packetIDs(t), nil)
}

func TestStructuralNeighborUpdatesDoNotWrapCoordinates(t *testing.T) {
	wrapped := game.BlockPosition{X: math.MinInt32, Y: 70}
	edge := game.BlockPosition{X: math.MaxInt32, Y: 70}

	world := &game.World{}

	world.SetBlock(wrapped, game.OakFence)

	runtime := NewRuntime(world)

	changes := runtime.withStructuralNeighborChanges([]game.BlockChange{{Position: edge, Replacement: game.OakFence}})

	for _, change := range changes {
		if change.Position == wrapped {
			t.Fatalf("edge neighbor update wrapped to %+v", wrapped)
		}
	}
}

func mustBlockState(t *testing.T, block game.Block, values ...game.BlockPropertyValue) game.Block {
	t.Helper()

	state, ok := block.WithProperties(values...)
	if !ok {
		t.Fatalf("resolve block %d with %+v", block, values)
	}

	return state
}

func assertBlockProperty(t *testing.T, block game.Block, name, want string) {
	t.Helper()

	actual, ok := block.Property(name)
	if !ok || actual != want {
		t.Fatalf("block %d property %s = %q, %v; want %q, true", block, name, actual, ok, want)
	}
}
