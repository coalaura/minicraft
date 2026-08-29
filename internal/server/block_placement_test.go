package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type placementTestGenerator struct {
	clicked game.BlockPosition
}

type placementFaceTestCase struct {
	face   int32
	target game.BlockPosition
}

type axisBlockPlacementTestCase struct {
	face int32
	axis int
}

type replaceabilityPlacementTestCase struct {
	clicked game.Block
	target  game.BlockPosition
}

func (g placementTestGenerator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	if position == g.clicked {
		return game.Stone
	}

	return game.Air
}

func TestCreativeBlockPlacementOnEachFace(t *testing.T) {
	tests := map[string]placementFaceTestCase{
		"down":  {face: protocol.BlockFaceDown, target: game.BlockPosition{Y: 69}},
		"up":    {face: protocol.BlockFaceUp, target: game.BlockPosition{Y: 71}},
		"north": {face: protocol.BlockFaceNorth, target: game.BlockPosition{Y: 70, Z: -1}},
		"south": {face: protocol.BlockFaceSouth, target: game.BlockPosition{Y: 70, Z: 1}},
		"west":  {face: protocol.BlockFaceWest, target: game.BlockPosition{X: -1, Y: 70}},
		"east":  {face: protocol.BlockFaceEast, target: game.BlockPosition{X: 1, Y: 70}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			clicked := game.BlockPosition{Y: 70}

			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			actor, connection := newPlacementTestSession(runtime, clicked)

			markPlacementChunksLoaded(actor, clicked, test.target)

			joinTestSession(t, runtime, actor)

			connection.reset()

			err := actor.handleUseItemOn(testUseItemOn(clicked, test.face, protocol.MainHand, 100))
			if err != nil {
				t.Fatalf("handle use item on: %v", err)
			}

			block := world.BlockAt(test.target)
			if block != game.Stone {
				t.Fatalf("placed block = %d, want stone", block)
			}

			assertPacketIDs(t, connection.packetIDs(t), []int32{
				protocol.ClientboundBlockUpdateID,
				protocol.ClientboundBlockChangedAckID,
			})

			assertBlockUpdate(t, connection.packets(t)[0], test.target, protocol.StoneBlockState)
			assertBlockChangedAck(t, connection.packets(t)[1], 100)
		})
	}
}

func TestSurvivalPlacementConsumesActualHand(t *testing.T) {
	tests := map[string]int32{
		"main hand": protocol.MainHand,
		"offhand":   protocol.OffHand,
	}

	for name, hand := range tests {
		t.Run(name, func(t *testing.T) {
			clicked := game.BlockPosition{Y: 70}
			target := game.BlockPosition{Y: 71}

			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			actor, _ := newPlacementTestSession(runtime, clicked)

			actor.Player.GameMode = game.GameModeSurvival
			actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 3}
			actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemStone, Count: 4}

			markPlacementChunksLoaded(actor, clicked, target)

			joinTestSession(t, runtime, actor)

			err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, hand, 200))
			if err != nil {
				t.Fatalf("handle use item on: %v", err)
			}

			player := actor.snapshotPlayer()

			wantMain := int32(3)
			wantOffhand := int32(4)

			if hand == protocol.MainHand {
				wantMain--
			} else {
				wantOffhand--
			}

			if player.Inventory.Hotbar[0].Count != wantMain || player.Inventory.Offhand.Count != wantOffhand {
				t.Fatalf("held counts = main %d, offhand %d; want %d, %d", player.Inventory.Hotbar[0].Count, player.Inventory.Offhand.Count, wantMain, wantOffhand)
			}
		})
	}
}

func TestFailedPlacementDoesNotConsumeSurvivalItem(t *testing.T) {
	clicked := game.BlockPosition{Y: 70}
	target := game.BlockPosition{Y: 71}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	world.SetBlock(target, game.Dirt)

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, clicked)

	actor.Player.GameMode = game.GameModeSurvival
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 3}

	markPlacementChunksLoaded(actor, clicked, target)

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 201))
	if err != nil {
		t.Fatalf("handle use item on: %v", err)
	}

	count := actor.snapshotPlayer().Inventory.Hotbar[0].Count
	if count != 3 {
		t.Fatalf("stack after failed placement = %d, want 3", count)
	}
}

func TestSuccessfulPlacementSynchronizesLoadedPlayers(t *testing.T) {
	clicked := game.BlockPosition{X: 15, Y: 70}
	target := game.BlockPosition{X: 16, Y: 70}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	runtime := NewRuntime(world)

	actor, actorConnection := newPlacementTestSession(runtime, clicked)
	observer, observerConnection := newPlacementTestSession(runtime, clicked)
	unloaded, unloadedConnection := newPlacementTestSession(runtime, clicked)

	markPlacementChunksLoaded(actor, clicked, target)
	markPlacementChunksLoaded(observer, target)

	unloaded.loadedChunks = nil

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, unloaded)

	actorConnection.reset()
	observerConnection.reset()
	unloadedConnection.reset()

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceEast, protocol.MainHand, 101))
	if err != nil {
		t.Fatalf("handle use item on: %v", err)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockChangedAckID})
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
	assertPacketIDs(t, unloadedConnection.packetIDs(t), nil)
	assertBlockUpdate(t, observerConnection.packets(t)[0], target, protocol.StoneBlockState)
	assertSoundEvent(t, observerConnection.packets(t)[1], game.SoundBlockStonePlace)
}

func TestPlacementUsesVanillaReplaceabilityTarget(t *testing.T) {
	tests := map[string]replaceabilityPlacementTestCase{
		"short grass is replaced":   {clicked: game.ShortGrass, target: game.BlockPosition{Y: 70}},
		"vine from tag is replaced": {clicked: game.Vine, target: game.BlockPosition{Y: 70}},
		"flower targets adjacent":   {clicked: game.Dandelion, target: game.BlockPosition{Y: 71}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			clicked := game.BlockPosition{Y: 70}

			world := &game.World{}

			world.SetBlock(game.BlockPosition{Y: 69}, game.Dirt)
			world.SetBlock(clicked, test.clicked)

			runtime := NewRuntime(world)

			actor, _ := newPlacementTestSession(runtime, clicked)

			markPlacementChunksLoaded(actor, clicked, test.target)

			joinTestSession(t, runtime, actor)

			err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 110))
			if err != nil {
				t.Fatalf("place stone: %v", err)
			}

			block := world.BlockAt(test.target)
			if block != game.Stone {
				t.Fatalf("placement target %+v = %d, want stone", test.target, block)
			}
		})
	}
}

func TestDeniedPlacementResynchronizesAndAcknowledges(t *testing.T) {
	tests := map[string]func(*Runtime){
		"configuration": func(runtime *Runtime) { runtime.AllowBlockPlacing = false },
		"policy":        func(runtime *Runtime) { runtime.BlockMutationPolicy = denyBlockMutationPolicy{} },
	}

	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			clicked := game.BlockPosition{Y: 70}
			target := game.BlockPosition{Y: 71}

			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			configure(runtime)

			actor, actorConnection := newPlacementTestSession(runtime, clicked)
			observer, observerConnection := newPlacementTestSession(runtime, clicked)

			joinTestSession(t, runtime, actor)
			joinTestSession(t, runtime, observer)

			actorConnection.reset()
			observerConnection.reset()

			err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 102))
			if err != nil {
				t.Fatalf("handle use item on: %v", err)
			}

			block := world.BlockAt(target)
			if block != game.Air {
				t.Fatalf("denied placement changed block to %d", block)
			}

			assertPacketIDs(t, actorConnection.packetIDs(t), []int32{
				protocol.ClientboundBlockUpdateID,
				protocol.ClientboundBlockChangedAckID,
				protocol.ClientboundContainerSetContentID,
			})
			assertPacketIDs(t, observerConnection.packetIDs(t), nil)
			assertBlockUpdate(t, actorConnection.packets(t)[0], target, protocol.AirBlockState)
			assertBlockChangedAck(t, actorConnection.packets(t)[1], 102)
		})
	}
}

func TestOffhandPlacementUsesOffhandItem(t *testing.T) {
	clicked := game.BlockPosition{Y: 70}
	target := game.BlockPosition{Y: 71}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	runtime := NewRuntime(world)

	actor, connection := newPlacementTestSession(runtime, clicked)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakDoor, Count: 1}
	actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemDirt, Count: 64}

	joinTestSession(t, runtime, actor)

	connection.reset()

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.OffHand, 103))
	if err != nil {
		t.Fatalf("handle use item on: %v", err)
	}

	block := world.BlockAt(target)
	if block != game.Dirt {
		t.Fatalf("offhand placed block = %d, want dirt", block)
	}
}

func TestMainHandPlacementUsesSelectedHotbarItem(t *testing.T) {
	clicked := game.BlockPosition{Y: 70}
	target := game.BlockPosition{Y: 71}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, clicked)

	actor.Player.Inventory.Hotbar[1] = game.ItemStack{Item: game.ItemDirt, Count: 64}

	joinTestSession(t, runtime, actor)

	actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 108))
	if err != nil {
		t.Fatalf("handle use item on: %v", err)
	}

	block := world.BlockAt(target)
	if block != game.Dirt {
		t.Fatalf("selected hotbar item placed block = %d, want dirt", block)
	}
}

func TestAxisBlockPlacementUsesClickedFace(t *testing.T) {
	tests := map[string]axisBlockPlacementTestCase{
		"x": {face: protocol.BlockFaceEast, axis: 0},
		"y": {face: protocol.BlockFaceUp, axis: 1},
		"z": {face: protocol.BlockFaceSouth, axis: 2},
	}

	definition, ok := game.OakLog.Definition()
	if !ok {
		t.Fatal("oak log has no block definition")
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			clicked := game.BlockPosition{Y: 70}

			target, valid := placementTarget(clicked, test.face)
			if !valid {
				t.Fatal("placement target is invalid")
			}

			want, valid := definition.StateForProperties(test.axis)
			if !valid {
				t.Fatal("oak log axis state is invalid")
			}

			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			actor, _ := newPlacementTestSession(runtime, clicked)

			actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakLog, Count: 64}

			markPlacementChunksLoaded(actor, clicked, target)

			joinTestSession(t, runtime, actor)

			err := actor.handleUseItemOn(testUseItemOn(clicked, test.face, protocol.MainHand, 109))
			if err != nil {
				t.Fatalf("handle use item on: %v", err)
			}

			block := world.BlockAt(target)
			if block != want {
				t.Fatalf("placed oak log state = %d, want %d", block, want)
			}
		})
	}
}

func TestUnsupportedComplexPlacementIsRejected(t *testing.T) {
	clicked := game.BlockPosition{Y: 70}
	target := game.BlockPosition{Y: 71}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	runtime := NewRuntime(world)

	actor, connection := newPlacementTestSession(runtime, clicked)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemWhiteBed, Count: 1}

	joinTestSession(t, runtime, actor)

	connection.reset()

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 104))
	if err != nil {
		t.Fatalf("handle use item on: %v", err)
	}

	block := world.BlockAt(target)
	if block != game.Air {
		t.Fatalf("unsupported placement changed block to %d", block)
	}

	assertBlockUpdate(t, connection.packets(t)[0], target, protocol.AirBlockState)
	assertBlockChangedAck(t, connection.packets(t)[1], 104)
}

func TestPlacementAgainstAirIsRejected(t *testing.T) {
	clicked := game.BlockPosition{Y: 70}
	target := game.BlockPosition{Y: 71}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	world.SetBlock(clicked, game.Air)

	runtime := NewRuntime(world)

	actor, connection := newPlacementTestSession(runtime, clicked)

	joinTestSession(t, runtime, actor)
	connection.reset()

	err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 107))
	if err != nil {
		t.Fatalf("handle use item on: %v", err)
	}

	block := world.BlockAt(target)
	if block != game.Air {
		t.Fatalf("placement against air changed block to %d", block)
	}

	assertBlockUpdate(t, connection.packets(t)[0], target, protocol.AirBlockState)
	assertBlockChangedAck(t, connection.packets(t)[1], 107)
}

func TestOccupiedOrDistantPlacementIsRejected(t *testing.T) {
	t.Run("occupied", func(t *testing.T) {
		clicked := game.BlockPosition{Y: 70}
		target := game.BlockPosition{Y: 71}

		world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

		world.SetBlock(target, game.Dirt)
		runtime := NewRuntime(world)

		actor, connection := newPlacementTestSession(runtime, clicked)

		joinTestSession(t, runtime, actor)

		connection.reset()

		err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 105))
		if err != nil {
			t.Fatalf("handle use item on: %v", err)
		}

		block := world.BlockAt(target)
		if block != game.Dirt {
			t.Fatalf("occupied block changed to %d", block)
		}

		assertBlockUpdate(t, connection.packets(t)[0], target, int32(game.Dirt))
	})

	t.Run("distant", func(t *testing.T) {
		clicked := game.BlockPosition{X: 10, Y: 70}
		target := game.BlockPosition{X: 11, Y: 70}

		world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

		runtime := NewRuntime(world)

		actor, connection := newPlacementTestSession(runtime, clicked)

		actor.Player.Position = game.Position{}

		joinTestSession(t, runtime, actor)

		connection.reset()

		err := actor.handleUseItemOn(testUseItemOn(clicked, protocol.BlockFaceEast, protocol.MainHand, 106))
		if err != nil {
			t.Fatalf("handle use item on: %v", err)
		}

		block := world.BlockAt(target)
		if block != game.Air {
			t.Fatalf("distant placement changed block to %d", block)
		}

		assertBlockChangedAck(t, connection.packets(t)[1], 106)
	})
}

func newPlacementTestSession(runtime *Runtime, clicked game.BlockPosition) (*Session, *recordingConnection) {
	session, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder", game.GameModeCreative)

	session.Player.Position = blockMutationTestPlayerPosition(clicked)

	session.Player.Position.X += 2

	session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 64}

	markPlacementChunksLoaded(session, clicked)

	return session, connection
}

func markPlacementChunksLoaded(session *Session, positions ...game.BlockPosition) {
	session.loadedChunks = make(map[LoadedChunk]struct{}, len(positions))

	for _, position := range positions {
		session.loadedChunks[blockLoadedChunk(position)] = struct{}{}
	}
}

func testUseItemOn(position game.BlockPosition, face, hand, sequence int32) protocol.UseItemOn {
	return protocol.UseItemOn{
		Hand:     hand,
		Position: position,
		Face:     face,
		CursorX:  0.5,
		CursorY:  0.5,
		CursorZ:  0.5,
		Sequence: sequence,
	}
}
