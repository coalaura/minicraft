package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type chestBlockTestCase struct {
	name  string
	block game.Block
	item  game.Item
}

type chestFacingTestCase struct {
	name   string
	yaw    float32
	facing string
	left   game.BlockPosition
	right  game.BlockPosition
}

func TestChestPlacementUsesPlayerFacing(t *testing.T) {
	tests := []chestFacingTestCase{
		{name: "south", yaw: 0, facing: "south"},
		{name: "west", yaw: 90, facing: "west"},
		{name: "north", yaw: 180, facing: "north"},
		{name: "east", yaw: 270, facing: "east"},
	}

	for _, chest := range chestBlocksForTest() {
		for _, test := range tests {
			t.Run(chest.name+"_"+test.name, func(t *testing.T) {
				target := game.BlockPosition{Y: 70}
				world := &game.World{}

				world.SetBlock(game.BlockPosition{Y: 69}, game.Stone)

				runtime := NewRuntime(world)

				actor := newChestPlacementSession(t, runtime, target)

				actor.Player.Rotation.Yaw = test.yaw

				placeChestForTest(t, runtime, actor, game.BlockPosition{Y: 69}, chest.item)

				placed := world.BlockAt(target)
				if !sameBlockType(placed, chest.block) {
					t.Fatalf("placed block = %d, want %d", placed, chest.block)
				}

				assertBlockProperty(t, placed, "facing", test.facing)
				assertBlockProperty(t, placed, "type", "single")
				assertBlockProperty(t, placed, "waterlogged", "false")
			})
		}
	}
}

func TestChestPlacementPairsAllOrientations(t *testing.T) {
	tests := []chestFacingTestCase{
		{name: "south", yaw: 0, facing: "south", left: game.BlockPosition{Y: 70}, right: game.BlockPosition{X: -1, Y: 70}},
		{name: "west", yaw: 90, facing: "west", left: game.BlockPosition{Y: 70}, right: game.BlockPosition{Y: 70, Z: -1}},
		{name: "north", yaw: 180, facing: "north", left: game.BlockPosition{Y: 70}, right: game.BlockPosition{X: 1, Y: 70}},
		{name: "east", yaw: 270, facing: "east", left: game.BlockPosition{Y: 70}, right: game.BlockPosition{Y: 70, Z: 1}},
	}

	for _, chest := range chestBlocksForTest() {
		for _, test := range tests {
			t.Run(chest.name+"_"+test.name, func(t *testing.T) {
				support := game.BlockPosition{Y: 69}

				partner := mustBlockState(t, chest.block,
					game.BlockPropertyValue{Name: "facing", Value: test.facing},
					game.BlockPropertyValue{Name: "type", Value: "single"},
				)

				world := &game.World{}

				world.SetBlock(support, game.Stone)
				world.SetBlock(test.right, partner)

				runtime := NewRuntime(world)

				actor := newChestPlacementSession(t, runtime, test.left, test.right)

				actor.Player.Rotation.Yaw = test.yaw

				placeChestForTest(t, runtime, actor, support, chest.item)

				assertBlockProperty(t, world.BlockAt(test.left), "facing", test.facing)
				assertBlockProperty(t, world.BlockAt(test.left), "type", "left")
				assertBlockProperty(t, world.BlockAt(test.right), "facing", test.facing)
				assertBlockProperty(t, world.BlockAt(test.right), "type", "right")
			})
		}
	}
}

func TestNormalAndTrappedChestsDoNotPair(t *testing.T) {
	for _, test := range []chestBlockTestCase{
		{name: "normal_next_to_trapped", block: game.TrappedChest, item: game.ItemTrappedChest},
		{name: "trapped_next_to_normal", block: game.Chest, item: game.ItemChest},
	} {
		t.Run(test.name, func(t *testing.T) {
			support := game.BlockPosition{Y: 69}
			target := game.BlockPosition{Y: 70}
			partnerPosition := game.BlockPosition{X: -1, Y: 70}

			partnerBlock := game.Chest
			if test.block == game.Chest {
				partnerBlock = game.TrappedChest
			}

			partner := mustBlockState(t, partnerBlock,
				game.BlockPropertyValue{Name: "facing", Value: "south"},
				game.BlockPropertyValue{Name: "type", Value: "single"},
			)

			world := &game.World{}

			world.SetBlock(support, game.Stone)
			world.SetBlock(partnerPosition, partner)

			runtime := NewRuntime(world)

			actor := newChestPlacementSession(t, runtime, target, partnerPosition)

			placeChestForTest(t, runtime, actor, support, test.item)

			assertBlockProperty(t, world.BlockAt(target), "type", "single")
			assertBlockProperty(t, world.BlockAt(partnerPosition), "type", "single")
		})
	}
}

func TestChestPlacementDoesNotCreateTriple(t *testing.T) {
	for _, chest := range chestBlocksForTest() {
		t.Run(chest.name, func(t *testing.T) {
			support := game.BlockPosition{X: 1, Y: 69}
			left := game.BlockPosition{Y: 70}
			right := game.BlockPosition{X: -1, Y: 70}
			third := game.BlockPosition{X: 1, Y: 70}

			leftBlock := mustBlockState(t, chest.block,
				game.BlockPropertyValue{Name: "facing", Value: "south"},
				game.BlockPropertyValue{Name: "type", Value: "left"},
			)

			rightBlock := mustBlockState(t, chest.block,
				game.BlockPropertyValue{Name: "facing", Value: "south"},
				game.BlockPropertyValue{Name: "type", Value: "right"},
			)

			world := &game.World{}

			world.SetBlock(support, game.Stone)
			world.SetBlock(left, leftBlock)
			world.SetBlock(right, rightBlock)

			runtime := NewRuntime(world)

			actor := newChestPlacementSession(t, runtime, third, left, right)

			placeChestForTest(t, runtime, actor, support, chest.item)

			assertBlockProperty(t, world.BlockAt(left), "type", "left")
			assertBlockProperty(t, world.BlockAt(right), "type", "right")
			assertBlockProperty(t, world.BlockAt(third), "type", "single")
		})
	}
}

func TestSecondaryUseControlsChestPairing(t *testing.T) {
	t.Run("vertical_face_stays_single", func(t *testing.T) {
		support := game.BlockPosition{Y: 69}
		target := game.BlockPosition{Y: 70}
		partner := game.BlockPosition{X: -1, Y: 70}

		world := &game.World{}

		world.SetBlock(support, game.Stone)
		world.SetBlock(partner, mustBlockState(t, game.Chest,
			game.BlockPropertyValue{Name: "facing", Value: "south"},
			game.BlockPropertyValue{Name: "type", Value: "single"},
		))

		runtime := NewRuntime(world)

		actor := newChestPlacementSession(t, runtime, target, partner)

		actor.Player.Sneaking = true

		placeChestForTest(t, runtime, actor, support, game.ItemChest)

		assertBlockProperty(t, world.BlockAt(target), "type", "single")
		assertBlockProperty(t, world.BlockAt(partner), "type", "single")
	})

	t.Run("horizontal_face_explicitly_pairs", func(t *testing.T) {
		partner := game.BlockPosition{Y: 70}
		target := game.BlockPosition{X: 1, Y: 70}

		world := &game.World{}

		world.SetBlock(partner, mustBlockState(t, game.Chest,
			game.BlockPropertyValue{Name: "facing", Value: "south"},
			game.BlockPropertyValue{Name: "type", Value: "single"},
		))

		runtime := NewRuntime(world)

		actor := newChestPlacementSession(t, runtime, partner, target)

		actor.Player.Sneaking = true

		result, _, err := runtime.PlaceItem(actor, testUseItemOn(partner, protocol.BlockFaceEast, protocol.MainHand, 1), game.ItemChest)
		if err != nil || !result.Changed {
			t.Fatalf("secondary-use chest placement: result=%+v err=%v", result, err)
		}

		assertBlockProperty(t, world.BlockAt(target), "facing", "south")
		assertBlockProperty(t, world.BlockAt(target), "type", "left")
		assertBlockProperty(t, world.BlockAt(partner), "type", "right")
	})
}

func TestBreakingEitherChestHalfLeavesSingleSurvivor(t *testing.T) {
	for _, chest := range chestBlocksForTest() {
		for _, brokenType := range []string{"left", "right"} {
			t.Run(chest.name+"_"+brokenType, func(t *testing.T) {
				left := game.BlockPosition{Y: 70}
				right := game.BlockPosition{X: -1, Y: 70}

				broken := left
				survivor := right

				if brokenType == "right" {
					broken = right
					survivor = left
				}

				world := &game.World{}

				world.SetBlock(left, mustBlockState(t, chest.block,
					game.BlockPropertyValue{Name: "facing", Value: "south"},
					game.BlockPropertyValue{Name: "type", Value: "left"},
				))

				world.SetBlock(right, mustBlockState(t, chest.block,
					game.BlockPropertyValue{Name: "facing", Value: "south"},
					game.BlockPropertyValue{Name: "type", Value: "right"},
				))

				runtime := NewRuntime(world)

				actor := newChestPlacementSession(t, runtime, broken, survivor)

				result, err := runtime.MutateBlock(actor, BlockMutationBreak, broken, game.Air)
				if err != nil || !result.Changed {
					t.Fatalf("break chest half: result=%+v err=%v", result, err)
				}

				if world.BlockAt(broken) != game.Air {
					t.Fatalf("broken chest at %+v = %d, want air", broken, world.BlockAt(broken))
				}

				assertBlockProperty(t, world.BlockAt(survivor), "type", "single")
			})
		}
	}
}

func TestChestMenusUseExpectedSizesAndDoubleStorageOrder(t *testing.T) {
	single := game.BlockPosition{Y: 70}
	runtime, session, connection := newChestTestRuntime(t, single)

	openChestForTest(t, runtime, session, single)

	assertChestOpenScreen(t, chestPacket(t, connection.packets(t), protocol.ClientboundOpenScreenID), protocol.MenuGeneric9x3)
	assertMenuSnapshotHeader(t, chestPacket(t, connection.packets(t), protocol.ClientboundContainerSetContentID), 1, 0, 63)

	for _, position := range []game.BlockPosition{{Y: 70}, {X: -1, Y: 70}} {
		left := game.BlockPosition{Y: 70}
		right := game.BlockPosition{X: -1, Y: 70}

		runtime, session, connection = newChestTestRuntime(t, left, right)

		backing, valid := runtime.chestBackingAt(position)
		if !valid || backing.chests[0].position != right || backing.chests[1].position != left {
			t.Fatalf("double chest backing before open = %+v; want right then left", backing)
		}

		connection.reset()

		openChestForTest(t, runtime, session, position)

		menu := session.activeMenu()
		if menu.containerSlots != 54 || len(menu.slots) != 90 {
			t.Fatalf("double chest menu = %d container slots and %d total slots; want 54 and 90", menu.containerSlots, len(menu.slots))
		}

		assertChestOpenScreen(t, chestPacket(t, connection.packets(t), protocol.ClientboundOpenScreenID), protocol.MenuGeneric9x6)
		assertMenuSnapshotHeader(t, chestPacket(t, connection.packets(t), protocol.ClientboundContainerSetContentID), menu.windowID, 0, 90)

		backing, valid = runtime.chestBackingAt(position)
		if !valid || backing.chests[0].position != right || backing.chests[1].position != left {
			t.Fatalf("double chest backing order = %+v; want right then left", backing)
		}
	}
}

func TestDoubleChestQuickMoveAndSwapTransactions(t *testing.T) {
	left := game.BlockPosition{Y: 70}
	right := game.BlockPosition{X: -1, Y: 70}

	runtime, session, connection := newChestTestRuntime(t, left, right)

	openChestForTest(t, runtime, session, left)

	connection.reset()

	rightItems := mustChestItems(t, &mustRuntimeChest(t, runtime, right).entity)
	leftItems := mustChestItems(t, &mustRuntimeChest(t, runtime, left).entity)

	rightItems[0] = game.ItemStack{Item: game.ItemStone, Count: 5}

	menu := session.activeMenu()

	err := session.handleContainerClick(protocol.ContainerClick{
		WindowID:    menu.windowID,
		StateID:     menu.stateID,
		Slot:        0,
		MouseButton: 0,
		Mode:        clickModeQuickMove,
		ChangedSlots: []protocol.ChangedSlot{
			{Location: 0},
			{Location: 89, Item: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 5})},
		},
	})

	if err != nil {
		t.Fatalf("quick-move from double chest: %v", err)
	}

	if !rightItems[0].Empty() || !session.Player.Inventory.Hotbar[8].Equal(game.ItemStack{Item: game.ItemStone, Count: 5}) {
		t.Fatalf("quick-move result = right %+v, hotbar %+v", rightItems[0], session.Player.Inventory.Hotbar[8])
	}

	leftItems[0] = game.ItemStack{Item: game.ItemDirt, Count: 3}

	session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 2}

	err = session.handleContainerClick(protocol.ContainerClick{
		WindowID:    menu.windowID,
		StateID:     menu.stateID,
		Slot:        27,
		MouseButton: 0,
		Mode:        clickModeSwap,
		ChangedSlots: []protocol.ChangedSlot{
			{Location: 27, Item: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 2})},
			{Location: 81, Item: hashedStack(game.ItemStack{Item: game.ItemDirt, Count: 3})},
		},
	})

	if err != nil {
		t.Fatalf("number-key swap in double chest: %v", err)
	}

	if !leftItems[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 2}) || !session.Player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemDirt, Count: 3}) {
		t.Fatalf("number-key swap = left %+v, hotbar %+v", leftItems[0], session.Player.Inventory.Hotbar[0])
	}

	rightItems[1] = game.ItemStack{Item: game.ItemStone, Count: 4}

	session.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemDirt, Count: 1}

	err = session.handleContainerClick(protocol.ContainerClick{
		WindowID:    menu.windowID,
		StateID:     menu.stateID,
		Slot:        1,
		MouseButton: 40,
		Mode:        clickModeSwap,
		ChangedSlots: []protocol.ChangedSlot{{
			Location: 1,
			Item:     hashedStack(game.ItemStack{Item: game.ItemDirt, Count: 1}),
		}},
	})

	if err != nil {
		t.Fatalf("offhand swap in double chest: %v", err)
	}

	if !rightItems[1].Equal(game.ItemStack{Item: game.ItemDirt, Count: 1}) || !session.Player.Inventory.Offhand.Equal(game.ItemStack{Item: game.ItemStone, Count: 4}) {
		t.Fatalf("offhand swap = right %+v, offhand %+v", rightItems[1], session.Player.Inventory.Offhand)
	}
}

func TestChestOpeningIsBlockedAboveEitherHalf(t *testing.T) {
	for _, positions := range [][]game.BlockPosition{{{Y: 70}}, {{Y: 70}, {X: -1, Y: 70}}} {
		for _, blocked := range positions {
			t.Run("blocked", func(t *testing.T) {
				runtime, session, connection := newChestTestRuntime(t, positions...)

				above := blocked

				above.Y++

				runtime.World.SetBlock(above, game.Stone)

				openChestForTest(t, runtime, session, positions[0])

				if session.activeMenu() != session.inventoryMenu {
					t.Fatal("blocked chest opened a menu")
				}

				assertPacketIDs(t, connection.packetIDs(t), nil)
			})
		}
	}
}

func TestChestLidEventsAndSoundsTrackViewers(t *testing.T) {
	left := game.BlockPosition{X: 4, Y: 70, Z: -3}
	right := game.BlockPosition{X: 3, Y: 70, Z: -3}

	runtime, first, firstConnection := newChestTestRuntime(t, left, right)

	second, secondConnection := newChestTestSession(t, runtime, left, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Second")

	firstConnection.reset()
	secondConnection.reset()

	openChestForTest(t, runtime, first, left)

	assertChestEvents(t, firstConnection.packets(t), right, left, 1)
	assertChestSound(t, firstConnection.packets(t)[1], game.SoundBlockChestOpen, right, "east")
	assertChestEvents(t, secondConnection.packets(t), right, left, 1)
	assertChestSound(t, secondConnection.packets(t)[1], game.SoundBlockChestOpen, right, "east")

	firstConnection.reset()
	secondConnection.reset()

	openChestForTest(t, runtime, second, right)

	assertChestEvents(t, firstConnection.packets(t), right, left, 2)
	assertChestEvents(t, secondConnection.packets(t)[:2], right, left, 2)

	firstConnection.reset()
	secondConnection.reset()

	runtime.closeMenu(first, false)

	assertChestEvents(t, firstConnection.packets(t), right, left, 1)
	assertChestEvents(t, secondConnection.packets(t), right, left, 1)

	firstConnection.reset()
	secondConnection.reset()

	runtime.closeMenu(second, false)

	assertChestEvents(t, firstConnection.packets(t), right, left, 0)
	assertChestSoundInPackets(t, firstConnection.packets(t), game.SoundBlockChestClose, right, "east")
	assertChestEvents(t, secondConnection.packets(t), right, left, 0)
	assertChestSoundInPackets(t, secondConnection.packets(t), game.SoundBlockChestClose, right, "east")
}

func TestSingleChestSoundsAndViewerSynchronization(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime, first, firstConnection := newChestTestRuntime(t, position)

	second, secondConnection := newChestTestSession(t, runtime, position, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Second")

	firstConnection.reset()
	secondConnection.reset()

	chest := mustRuntimeChest(t, runtime, position)

	items := mustChestItems(t, &chest.entity)

	items[0] = game.ItemStack{Item: game.ItemStone, Count: 4}

	openChestForTest(t, runtime, first, position)

	assertChestSound(t, firstConnection.packets(t)[1], game.SoundBlockChestOpen, position, "")
	openChestForTest(t, runtime, second, position)

	firstConnection.reset()
	secondConnection.reset()

	err := first.handleContainerClick(protocol.ContainerClick{WindowID: first.activeMenu().windowID, StateID: first.activeMenu().stateID, Slot: 0, Mode: clickModePickup, ChangedSlots: []protocol.ChangedSlot{{Location: 0}}, CursorItem: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 4})})
	if err != nil {
		t.Fatalf("first viewer pickup: %v", err)
	}

	if !items[0].Empty() || first.activeMenu().stateID != 1 || second.activeMenu().stateID != 1 {
		t.Fatalf("shared chest contents or state IDs = %+v, %d, %d", items[0], first.activeMenu().stateID, second.activeMenu().stateID)
	}

	assertPacketIDs(t, firstConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
	assertPacketIDs(t, secondConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})

	firstConnection.reset()
	secondConnection.reset()

	first.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 1}

	err = first.handleContainerClick(protocol.ContainerClick{WindowID: first.activeMenu().windowID, StateID: first.activeMenu().stateID, Slot: 27, Mode: clickModePickup, ChangedSlots: []protocol.ChangedSlot{{Location: 27}}, CursorItem: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 1})})
	if err != nil {
		t.Fatalf("player-only chest click: %v", err)
	}

	if second.activeMenu().stateID != 1 {
		t.Fatalf("other viewer state ID = %d, want 1", second.activeMenu().stateID)
	}

	assertPacketIDs(t, secondConnection.packetIDs(t), nil)

	firstConnection.reset()
	secondConnection.reset()

	runtime.closeMenu(first, false)
	runtime.closeMenu(second, false)

	assertChestSoundInPackets(t, firstConnection.packets(t), game.SoundBlockChestClose, position, "")
}

func TestChestMenuClosesWhenEitherDoubleHalfIsRemoved(t *testing.T) {
	for _, removed := range []game.BlockPosition{{Y: 70}, {X: -1, Y: 70}} {
		t.Run("removed", func(t *testing.T) {
			left := game.BlockPosition{Y: 70}
			right := game.BlockPosition{X: -1, Y: 70}

			runtime, viewer, connection := newChestTestRuntime(t, left, right)

			breaker, _ := newChestTestSession(t, runtime, left, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Breaker")

			markPlacementChunksLoaded(breaker, left, right)

			runtime.setSessionActiveChunks(breaker, chestLoadedChunks(left, right))

			openChestForTest(t, runtime, viewer, left)

			connection.reset()

			err := breaker.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionStartDestroyBlock, Position: removed, Face: protocol.BlockFaceUp, Sequence: 2})
			if err != nil {
				t.Fatalf("break chest half: %v", err)
			}

			if runtime.World.BlockAt(removed) != game.Air {
				t.Fatalf("removed chest half at %+v was not broken", removed)
			}

			if viewer.activeMenu() != viewer.inventoryMenu || !sessionHasPacket(connection.packets(t), protocol.ClientboundCloseContainerID) {
				t.Fatal("removing a double chest half did not close the viewer menu")
			}
		})
	}
}

func chestBlocksForTest() []chestBlockTestCase {
	return []chestBlockTestCase{
		{name: "chest", block: game.Chest, item: game.ItemChest},
		{name: "trapped_chest", block: game.TrappedChest, item: game.ItemTrappedChest},
	}
}

func newChestPlacementSession(t *testing.T, runtime *Runtime, positions ...game.BlockPosition) *Session {
	t.Helper()

	actor, _ := newPlacementTestSession(runtime, positions[0])

	markPlacementChunksLoaded(actor, positions...)

	joinTestSession(t, runtime, actor)

	return actor
}

func placeChestForTest(t *testing.T, runtime *Runtime, actor *Session, support game.BlockPosition, item game.Item) {
	t.Helper()

	result, _, err := runtime.PlaceItem(actor, testUseItemOn(support, protocol.BlockFaceUp, protocol.MainHand, 1), item)
	if err != nil || !result.Changed {
		t.Fatalf("place chest: result=%+v err=%v", result, err)
	}
}

func newChestTestRuntime(t *testing.T, positions ...game.BlockPosition) (*Runtime, *Session, *recordingConnection) {
	t.Helper()

	world := &game.World{}

	for _, position := range positions {
		chestType := "single"
		if len(positions) == 2 {
			chestType = "left"
			if position.X < positions[0].X {
				chestType = "right"
			}
		}

		world.SetBlock(position, mustBlockState(t, game.Chest,
			game.BlockPropertyValue{Name: "facing", Value: "south"},
			game.BlockPropertyValue{Name: "type", Value: chestType},
		))
	}

	runtime := NewRuntime(world)

	session, connection := newChestTestSession(t, runtime, positions[0], "00010203-0405-0607-0809-0a0b0c0d0e0f", "First")

	runtime.setSessionActiveChunks(session, chestLoadedChunks(positions...))

	return runtime, session, connection
}

func chestLoadedChunks(positions ...game.BlockPosition) []LoadedChunk {
	chunks := make([]LoadedChunk, 0, len(positions))
	seen := make(map[LoadedChunk]struct{})

	for _, position := range positions {
		chunk := blockLoadedChunk(position)
		if _, present := seen[chunk]; present {
			continue
		}

		seen[chunk] = struct{}{}
		chunks = append(chunks, chunk)
	}

	return chunks
}

func newChestTestSession(t *testing.T, runtime *Runtime, position game.BlockPosition, uuid, name string) (*Session, *recordingConnection) {
	t.Helper()

	session, connection := newPlacementTestSession(runtime, position)

	session.Player.UUID = uuid
	session.Player.Name = name

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	joinTestSession(t, runtime, session)

	connection.reset()

	return session, connection
}

func openChestForTest(t *testing.T, runtime *Runtime, session *Session, position game.BlockPosition) {
	t.Helper()

	chest := mustRuntimeChest(t, runtime, position)

	runtime.worldMutationMu.Lock()

	runtime.lifecycleMu.Lock()
	err := runtime.openChestLocked(session, chest)
	runtime.lifecycleMu.Unlock()

	runtime.worldMutationMu.Unlock()

	if err != nil {
		t.Fatalf("open chest: %v", err)
	}
}

func mustRuntimeChest(t *testing.T, runtime *Runtime, position game.BlockPosition) *runtimeChest {
	t.Helper()

	entity, present := runtime.runtimeBlockEntityAt(position)
	chest, valid := entity.(*runtimeChest)

	if !present || !valid {
		t.Fatal("chest is not active")
	}

	return chest
}

func mustChestItems(t *testing.T, entity *game.BlockEntity) []game.ItemStack {
	t.Helper()

	items, inventory := entity.Inventory()
	if !inventory || len(items) != game.ChestSlotCount {
		t.Fatal("chest entity does not expose 27 inventory slots")
	}

	return items
}

func assertChestOpenScreen(t *testing.T, packet protocol.Packet, wantMenuType int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	reader.VarInt()

	menuType := reader.VarInt()
	if menuType != wantMenuType {
		t.Fatalf("chest menu type = %d, want %d", menuType, wantMenuType)
	}
}

func assertChestEvents(t *testing.T, packets []protocol.Packet, first, second game.BlockPosition, wantOpeners byte) {
	t.Helper()

	positions := make([]game.BlockPosition, 0, 2)

	for _, packet := range packets {
		if packet.ID != protocol.ClientboundBlockEventID {
			continue
		}

		reader := protocol.NewPacketReader(packet.Data)

		position := reader.BlockPosition()

		event := reader.Byte()
		openers := reader.Byte()

		if event != chestOpenersEvent || openers != wantOpeners {
			t.Fatalf("chest block event = event %d openers %d; want event %d openers %d", event, openers, chestOpenersEvent, wantOpeners)
		}

		positions = append(positions, position)
	}

	if len(positions) != 2 || positions[0] != first || positions[1] != second {
		t.Fatalf("chest event positions = %+v; want %+v then %+v", positions, first, second)
	}
}

func chestPacket(t *testing.T, packets []protocol.Packet, packetID int32) protocol.Packet {
	t.Helper()

	for _, packet := range packets {
		if packet.ID == packetID {
			return packet
		}
	}

	t.Fatalf("missing chest packet %#x", packetID)

	return protocol.Packet{}
}

func assertChestSoundInPackets(t *testing.T, packets []protocol.Packet, event game.SoundEvent, position game.BlockPosition, facing string) {
	t.Helper()

	for _, packet := range packets {
		if packet.ID == protocol.ClientboundSoundID {
			assertChestSound(t, packet, event, position, facing)

			return
		}
	}

	t.Fatalf("missing chest sound %q", event)
}

func assertChestSound(t *testing.T, packet protocol.Packet, event game.SoundEvent, position game.BlockPosition, offsetDirection string) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	reader.VarInt()

	actualEvent := reader.String(32767)

	if reader.Bool() {
		reader.Float()
	}

	reader.VarInt()

	actualX := reader.Int()
	actualY := reader.Int()
	actualZ := reader.Int()

	directionX := 0.0
	directionZ := 0.0

	if offsetDirection != "" {
		direction, _ := directionFromName(offsetDirection)
		directionX, directionZ = horizontalSoundOffset(direction)
	}

	wantX := int32((float64(position.X) + 0.5 + directionX*0.5) * 8)
	wantY := int32((float64(position.Y) + 0.5) * 8)
	wantZ := int32((float64(position.Z) + 0.5 + directionZ*0.5) * 8)

	if actualEvent != string(event) || actualX != wantX || actualY != wantY || actualZ != wantZ {
		t.Fatalf("chest sound = event %q coordinates %d, %d, %d; want event %q coordinates %d, %d, %d", actualEvent, actualX, actualY, actualZ, event, wantX, wantY, wantZ)
	}
}
