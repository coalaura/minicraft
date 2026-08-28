package server

import (
	"slices"
	"strconv"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type buttonPlacementTestCase struct {
	name   string
	face   int32
	value  string
	facing string
}

type chainPlacementTestCase struct {
	name string
	face int32
	axis string
}

type stackingPlacementTestCase struct {
	name     string
	item     game.Item
	property string
	maximum  int
}

func TestButtonPlacementStateAndSupport(t *testing.T) {
	tests := []buttonPlacementTestCase{
		{name: "wall", face: protocol.BlockFaceEast, value: "wall", facing: "east"},
		{name: "floor", face: protocol.BlockFaceUp, value: "floor", facing: "south"},
		{name: "ceiling", face: protocol.BlockFaceDown, value: "ceiling", facing: "south"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			interaction := testUseItemOn(game.BlockPosition{}, test.face, protocol.MainHand, 1)

			state, valid := placementState(game.StoneButton, game.ItemPlacementButton, interaction, 0)
			if !valid {
				t.Fatal("button placement state is invalid")
			}

			assertBlockProperty(t, state, "face", test.value)
			assertBlockProperty(t, state, "facing", test.facing)
			assertBlockProperty(t, state, "powered", "false")

			target, _ := placementTarget(interaction.Position, interaction.Face)
			if !validPlacementSupport(func(position game.BlockPosition) game.Block {
				if position == interaction.Position {
					return game.Stone
				}

				return game.Air
			}, target, state, game.ItemPlacementButton) {
				t.Fatal("button rejected its clicked support")
			}

			if validPlacementSupport(func(game.BlockPosition) game.Block { return game.Air }, target, state, game.ItemPlacementButton) {
				t.Fatal("button accepted an unsupported surface")
			}
		})
	}
}

func TestPlacementFamilyInitialStates(t *testing.T) {
	leaves, valid := placementState(game.OakLeaves, game.ItemPlacementLeaves, testUseItemOn(game.BlockPosition{}, protocol.BlockFaceUp, protocol.MainHand, 1), 0)
	if !valid {
		t.Fatal("leaves placement state is invalid")
	}

	assertBlockProperty(t, leaves, "distance", "7")
	assertBlockProperty(t, leaves, "persistent", "true")
	assertBlockProperty(t, leaves, "waterlogged", "false")

	plate, valid := placementState(game.StonePressurePlate, game.ItemPlacementPressurePlate, protocol.UseItemOn{}, 0)
	if !valid {
		t.Fatal("pressure plate placement state is invalid")
	}

	assertBlockProperty(t, plate, "powered", "false")

	grassBlock, valid := placementState(game.GrassBlock, game.ItemPlacementDefault, protocol.UseItemOn{}, 0)
	if !valid {
		t.Fatal("grass block placement state is invalid")
	}

	assertBlockProperty(t, grassBlock, "snowy", "false")

	copperGrate, valid := placementState(game.CopperGrate, game.ItemPlacementDefault, protocol.UseItemOn{}, 0)
	if !valid {
		t.Fatal("copper grate placement state is invalid")
	}

	assertBlockProperty(t, copperGrate, "waterlogged", "false")

	for _, test := range []chainPlacementTestCase{
		{name: "x", face: protocol.BlockFaceEast, axis: "x"},
		{name: "y", face: protocol.BlockFaceUp, axis: "y"},
		{name: "z", face: protocol.BlockFaceSouth, axis: "z"},
	} {
		t.Run("chain_"+test.name, func(t *testing.T) {
			chain, stateValid := placementState(game.IronChain, game.ItemPlacementChain, protocol.UseItemOn{Face: test.face}, 0)
			if !stateValid {
				t.Fatal("chain placement state is invalid")
			}

			assertBlockProperty(t, chain, "axis", test.axis)
			assertBlockProperty(t, chain, "waterlogged", "false")
		})
	}
}

func TestSnowAndCandleStacking(t *testing.T) {
	tests := []stackingPlacementTestCase{
		{name: "snow", item: game.ItemSnow, property: "layers", maximum: 8},
		{name: "candle", item: game.ItemCandle, property: "candles", maximum: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			support := game.BlockPosition{Y: 69}
			position := game.BlockPosition{Y: 70}

			world := &game.World{}

			world.SetBlock(support, game.Stone)

			runtime := NewRuntime(world)

			actor, actorConnection := newPlacementTestSession(runtime, support)
			observer, observerConnection := newPlacementTestSession(runtime, support)

			actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: test.item, Count: 64}

			markPlacementChunksLoaded(actor, support, position)
			markPlacementChunksLoaded(observer, support, position)

			joinTestSession(t, runtime, actor)
			joinTestSession(t, runtime, observer)

			err := actor.handleUseItemOn(testUseItemOn(support, protocol.BlockFaceUp, protocol.MainHand, 1))
			if err != nil {
				t.Fatalf("initial placement: %v", err)
			}

			assertBlockProperty(t, world.BlockAt(position), test.property, "1")

			actorConnection.reset()
			observerConnection.reset()

			err = actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 2))
			if err != nil {
				t.Fatalf("stack 2: %v", err)
			}

			assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockChangedAckID})
			assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
			assertSoundEvent(t, observerConnection.packets(t)[1], world.BlockAt(position).SoundType().Place)

			for count := 3; count <= test.maximum; count++ {
				err = actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, int32(count)))
				if err != nil {
					t.Fatalf("stack %d: %v", count, err)
				}
			}

			assertBlockProperty(t, world.BlockAt(position), test.property, blockPropertyValue(test.maximum))

			err = actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 20))
			if err != nil {
				t.Fatalf("maximum stack placement: %v", err)
			}

			assertBlockProperty(t, world.BlockAt(position), test.property, blockPropertyValue(test.maximum))
		})
	}
}

func TestSurvivalCandleStackingConsumesAndUpdatesState(t *testing.T) {
	support := game.BlockPosition{Y: 69}
	position := game.BlockPosition{Y: 70}
	world := &game.World{}

	world.SetBlock(support, game.Stone)

	runtime := NewRuntime(world)
	actor, connection := newPlacementTestSession(runtime, support)
	actor.Player.GameMode = game.GameModeSurvival
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemCandle, Count: 3}

	markPlacementChunksLoaded(actor, support, position)
	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(support, protocol.BlockFaceUp, protocol.MainHand, 1))
	if err != nil {
		t.Fatalf("initial candle placement: %v", err)
	}

	err = actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 2))
	if err != nil {
		t.Fatalf("second candle placement: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(position), "candles", "2")

	if count := actor.snapshotPlayer().Inventory.Hotbar[0].Count; count != 1 {
		t.Fatalf("candle count = %d, want 1", count)
	}

	connection.reset()
	actor.Player.Sneaking = true

	err = actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 3))
	if err != nil {
		t.Fatalf("rejected candle stack: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(position), "candles", "2")

	if count := actor.snapshotPlayer().Inventory.Hotbar[0].Count; count != 1 {
		t.Fatalf("candle count after rejected stack = %d, want 1", count)
	}

	packetIDs := connection.packetIDs(t)
	if !slices.Contains(packetIDs, protocol.ClientboundContainerSetContentID) {
		t.Fatalf("rejected stack packets = %v, want inventory correction", packetIDs)
	}
}

func TestButtonInteractionPrecedesPlacementAndPlaysSound(t *testing.T) {
	support := game.BlockPosition{Y: 69}
	position := game.BlockPosition{Y: 70}

	button := mustBlockState(t, game.OakButton,
		game.BlockPropertyValue{Name: "face", Value: "floor"},
		game.BlockPropertyValue{Name: "facing", Value: "south"},
		game.BlockPropertyValue{Name: "powered", Value: "false"},
	)

	world := &game.World{}

	world.SetBlock(support, game.Stone)
	world.SetBlock(position, button)

	runtime := NewRuntime(world)

	actor, actorConnection := newPlacementTestSession(runtime, position)
	observer, observerConnection := newPlacementTestSession(runtime, position)

	markPlacementChunksLoaded(actor, support, position, game.BlockPosition{Y: 71})
	markPlacementChunksLoaded(observer, support, position, game.BlockPosition{Y: 71})

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 30))
	if err != nil {
		t.Fatalf("press button: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(position), "powered", "true")
	if world.BlockAt(game.BlockPosition{Y: 71}) != game.Air {
		t.Fatal("button interaction also placed held block")
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockChangedAckID})
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
	assertSoundEvent(t, observerConnection.packets(t)[1], game.SoundBlockWoodenButtonClickOn)
}

func TestCandleConvertsCake(t *testing.T) {
	support := game.BlockPosition{Y: 69}
	cakePosition := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(support, game.Stone)
	world.SetBlock(cakePosition, game.Cake)

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, cakePosition)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemCandle, Count: 1}

	markPlacementChunksLoaded(actor, support, cakePosition)

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(cakePosition, protocol.BlockFaceUp, protocol.MainHand, 1))
	if err != nil {
		t.Fatalf("place candle on cake: %v", err)
	}

	definition, valid := world.BlockAt(cakePosition).Definition()
	if !valid || definition.Name != "candle_cake" {
		t.Fatalf("cake replacement = %q, want candle_cake", definition.Name)
	}

	assertBlockProperty(t, world.BlockAt(cakePosition), "lit", "false")
}

func TestIronDoorPlacementAndManualInteraction(t *testing.T) {
	support := game.BlockPosition{Y: 69}
	lower := game.BlockPosition{Y: 70}
	upper := game.BlockPosition{Y: 71}

	world := &game.World{Generator: placementTestGenerator{clicked: support}}

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, support)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemIronDoor, Count: 1}

	markPlacementChunksLoaded(actor, support, lower, upper)

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(support, protocol.BlockFaceUp, protocol.MainHand, 1))
	if err != nil {
		t.Fatalf("place iron door: %v", err)
	}

	assertBlockProperty(t, world.BlockAt(lower), "half", "lower")
	assertBlockProperty(t, world.BlockAt(upper), "half", "upper")

	handled, _, _, err := runtime.InteractBlock(actor, lower)
	if err != nil {
		t.Fatalf("interact with iron door: %v", err)
	}

	if handled {
		t.Fatal("iron door handled manual interaction")
	}

	assertBlockProperty(t, world.BlockAt(lower), "open", "false")

	trapdoor, valid := placementState(game.IronTrapdoor, game.ItemPlacementTrapdoor, testUseItemOn(support, protocol.BlockFaceUp, protocol.MainHand, 2), 0)
	if !valid {
		t.Fatal("iron trapdoor placement state is invalid")
	}

	trapdoorPosition := game.BlockPosition{X: 1, Y: 70}

	world.SetBlock(trapdoorPosition, trapdoor)

	handled, _, _, err = runtime.InteractBlock(actor, trapdoorPosition)
	if err != nil {
		t.Fatalf("interact with iron trapdoor: %v", err)
	}

	if handled {
		t.Fatal("iron trapdoor handled manual interaction")
	}

	assertBlockProperty(t, world.BlockAt(trapdoorPosition), "open", "false")
}

func TestSupportedPlantAndButtonBreakWithSupport(t *testing.T) {
	tests := []structuralSupportTestCase{
		{name: "plant", support: game.Dirt, block: game.ShortGrass},
		{name: "flower", support: game.GrassBlock, block: game.Poppy},
		{name: "fern", support: game.Podzol, block: game.Fern},
		{name: "pressure_plate", support: game.Stone, block: game.StonePressurePlate},
		{name: "button", support: game.Stone, block: mustBlockState(t, game.StoneButton,
			game.BlockPropertyValue{Name: "face", Value: "floor"},
			game.BlockPropertyValue{Name: "facing", Value: "south"},
			game.BlockPropertyValue{Name: "powered", Value: "false"},
		)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			supportPosition := game.BlockPosition{Y: 69}
			blockPosition := game.BlockPosition{Y: 70}

			world := &game.World{}

			world.SetBlock(supportPosition, test.support)
			world.SetBlock(blockPosition, test.block)

			runtime := NewRuntime(world)

			result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: supportPosition, Replacement: game.Air}})
			if err != nil || !result.Changed {
				t.Fatalf("remove support: result=%+v err=%v", result, err)
			}

			if block := world.BlockAt(blockPosition); block != game.Air {
				t.Fatalf("unsupported block = %d, want air", block)
			}
		})
	}
}

func TestPlayerBreakEmitsSupportLossEffects(t *testing.T) {
	tests := []structuralSupportTestCase{
		{name: "short grass", support: game.Dirt, block: game.ShortGrass},
		{name: "snow", support: game.Stone, block: game.Snow},
		{name: "carpet", support: game.Stone, block: game.WhiteCarpet},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			supportPosition := game.BlockPosition{Y: 69}
			dependentPosition := game.BlockPosition{Y: 70}

			world := &game.World{}

			world.SetBlock(supportPosition, test.support)
			world.SetBlock(dependentPosition, test.block)

			runtime := NewRuntime(world)

			actor, actorConnection := newPlacementTestSession(runtime, supportPosition)
			observer, observerConnection := newPlacementTestSession(runtime, supportPosition)

			markPlacementChunksLoaded(actor, supportPosition, dependentPosition)
			markPlacementChunksLoaded(observer, supportPosition, dependentPosition)

			joinTestSession(t, runtime, actor)
			joinTestSession(t, runtime, observer)

			actorConnection.reset()
			observerConnection.reset()

			dependentState, err := protocolBlockState(test.block)
			if err != nil {
				t.Fatalf("encode dependent block: %v", err)
			}

			result, err := runtime.MutateBlock(actor, BlockMutationBreak, supportPosition, game.Air)
			if err != nil || !result.Changed {
				t.Fatalf("break support: result=%+v err=%v", result, err)
			}

			if world.BlockAt(dependentPosition) != game.Air {
				t.Fatal("dependent block survived support loss")
			}

			assertPacketIDs(t, actorConnection.packetIDs(t), []int32{
				protocol.ClientboundBlockUpdateID,
				protocol.ClientboundBlockUpdateID,
				protocol.ClientboundLevelEventID,
			})

			assertLevelEvent(t, actorConnection.packets(t)[2], protocol.LevelEventBlockBreak, dependentPosition, dependentState, false)

			assertPacketIDs(t, observerConnection.packetIDs(t), []int32{
				protocol.ClientboundBlockUpdateID,
				protocol.ClientboundBlockUpdateID,
				protocol.ClientboundLevelEventID,
				protocol.ClientboundLevelEventID,
			})

			assertLevelEvent(t, observerConnection.packets(t)[3], protocol.LevelEventBlockBreak, dependentPosition, dependentState, false)
		})
	}
}

func TestStructuralStateRecalculationHasNoBreakEffect(t *testing.T) {
	grassPosition := game.BlockPosition{Y: 69}
	snowPosition := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(grassPosition, game.GrassBlock)

	runtime := NewRuntime(world)

	actor, actorConnection := newPlacementTestSession(runtime, grassPosition)
	observer, observerConnection := newPlacementTestSession(runtime, grassPosition)

	markPlacementChunksLoaded(actor, grassPosition, snowPosition)
	markPlacementChunksLoaded(observer, grassPosition, snowPosition)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	actorConnection.reset()
	observerConnection.reset()

	result, err := runtime.MutateBlock(actor, BlockMutationPlace, snowPosition, game.Snow)
	if err != nil || !result.Changed {
		t.Fatalf("place snow: result=%+v err=%v", result, err)
	}

	assertBlockProperty(t, world.BlockAt(grassPosition), "snowy", "true")
	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockUpdateID})
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
}

func TestPointedDripstoneDirectionAndNeighborThickness(t *testing.T) {
	interaction := testUseItemOn(game.BlockPosition{}, protocol.BlockFaceUp, protocol.MainHand, 1)

	tip, valid := placementState(game.PointedDripstone, game.ItemPlacementPointedDripstone, interaction, 0)
	if !valid {
		t.Fatal("pointed dripstone placement state is invalid")
	}

	assertBlockProperty(t, tip, "vertical_direction", "up")
	assertBlockProperty(t, tip, "thickness", "tip")

	support := game.BlockPosition{}
	first := game.BlockPosition{Y: 1}
	second := game.BlockPosition{Y: 2}

	world := &game.World{}

	world.SetBlock(support, game.DripstoneBlock)
	world.SetBlock(first, tip)

	runtime := NewRuntime(world)

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: second, Replacement: tip}})
	if err != nil || !result.Changed {
		t.Fatalf("extend pointed dripstone: result=%+v err=%v", result, err)
	}

	assertBlockProperty(t, world.BlockAt(first), "thickness", "frustum")
	assertBlockProperty(t, world.BlockAt(second), "thickness", "tip")

	result, err = runtime.MutateWorldBlocks([]game.BlockChange{{Position: second, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("shorten pointed dripstone: result=%+v err=%v", result, err)
	}

	assertBlockProperty(t, world.BlockAt(first), "thickness", "tip")
}

func TestSnowUpdatesSnowyTerrainState(t *testing.T) {
	grassPosition := game.BlockPosition{Y: 69}
	snowPosition := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(grassPosition, game.GrassBlock)

	runtime := NewRuntime(world)

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: snowPosition, Replacement: game.Snow}})
	if err != nil || !result.Changed {
		t.Fatalf("place snow: result=%+v err=%v", result, err)
	}

	assertBlockProperty(t, world.BlockAt(grassPosition), "snowy", "true")

	result, err = runtime.MutateWorldBlocks([]game.BlockChange{{Position: snowPosition, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("remove snow: result=%+v err=%v", result, err)
	}

	assertBlockProperty(t, world.BlockAt(grassPosition), "snowy", "false")
}

func blockPropertyValue(value int) string {
	return strconv.Itoa(value)
}
