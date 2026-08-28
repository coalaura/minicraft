package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type craftingTableValidityTestCase struct {
	name   string
	mutate func(*Runtime, *Session)
}

func TestPlayerCraftingPredictedClicksRecomputeAndConsumeInputs(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemOakLog, Count: 2}

	craftingClick(t, session, protocol.ContainerClick{Slot: 9, MouseButton: 0, Mode: clickModePickup})
	craftingClick(t, session, protocol.ContainerClick{Slot: 1, MouseButton: 0, Mode: clickModePickup})

	menu := session.activeMenu()
	if !menu.slots[1].stack.Equal(game.ItemStack{Item: game.ItemOakLog, Count: 2}) || !menu.slots[0].stack.Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 4}) {
		t.Fatalf("player crafting placement = input %+v result %+v", *menu.slots[1].stack, *menu.slots[0].stack)
	}

	craftingClick(t, session, protocol.ContainerClick{Slot: 0, MouseButton: 0, Mode: clickModePickup})

	if !menu.slots[1].stack.Equal(game.ItemStack{Item: game.ItemOakLog, Count: 1}) || !menu.slots[0].stack.Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 4}) || !menu.carried.Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 4}) {
		t.Fatalf("player crafting pickup = input %+v result %+v carried %+v", *menu.slots[1].stack, *menu.slots[0].stack, menu.carried)
	}
}

func TestPlayerCraftingResultClickRespectsCursorCapacity(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	menu := session.activeMenu()

	session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemOakLog, Count: 2}

	menu.carried = game.ItemStack{Item: game.ItemOakPlanks, Count: 63}

	refreshCraftingMenu(menu)

	craftingClick(t, session, protocol.ContainerClick{Slot: 0, MouseButton: 0, Mode: clickModePickup})

	if menu.carried.Count != 64 || !menu.slots[1].stack.Equal(game.ItemStack{Item: game.ItemOakLog, Count: 1}) || !menu.slots[0].stack.Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 4}) {
		t.Fatalf("capacity-limited crafting pickup = input %+v result %+v carried %+v", *menu.slots[1].stack, *menu.slots[0].stack, menu.carried)
	}
}

func TestPlayerCraftingShiftClickRepeatsUntilInputsAreConsumed(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	menu := session.activeMenu()

	session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemOakLog, Count: 3}

	refreshCraftingMenu(menu)

	craftingClick(t, session, protocol.ContainerClick{Slot: 0, MouseButton: 0, Mode: clickModeQuickMove})

	if !menu.slots[1].stack.Empty() || !menu.slots[0].stack.Empty() || !session.Player.Inventory.Main[0].Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 12}) {
		t.Fatalf("shift crafted inventory = input %+v result %+v main %+v", *menu.slots[1].stack, *menu.slots[0].stack, session.Player.Inventory.Main[0])
	}
}

func TestPlayerCraftingShiftClickDropsPartialResultWithoutDuplicating(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	menu := session.activeMenu()

	fillVisiblePlayerInventory(&session.Player.Inventory, game.ItemStone)

	session.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemOakPlanks, Count: 63}
	session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemOakLog, Count: 2}

	refreshCraftingMenu(menu)

	craftingClick(t, session, protocol.ContainerClick{Slot: 0, MouseButton: 0, Mode: clickModeQuickMove})

	item := onlyRuntimeItemEntity(t, runtime)
	if !session.Player.Inventory.Main[0].Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 64}) || !item.Stack.Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 3}) {
		t.Fatalf("partial shift craft = inventory %+v dropped %+v", session.Player.Inventory.Main[0], item.Stack)
	}

	if !menu.slots[1].stack.Equal(game.ItemStack{Item: game.ItemOakLog, Count: 1}) || !menu.slots[0].stack.Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 4}) {
		t.Fatalf("partial shift craft consumed wrong inputs: input %+v result %+v", *menu.slots[1].stack, *menu.slots[0].stack)
	}
}

func TestPlayerCraftingResultExtractionClickModes(t *testing.T) {
	t.Run("number key", func(t *testing.T) {
		session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		menu := session.activeMenu()

		session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemOakLog, Count: 1}

		refreshCraftingMenu(menu)

		craftingClick(t, session, protocol.ContainerClick{Slot: 0, MouseButton: 2, Mode: clickModeSwap})

		if !session.Player.Inventory.Hotbar[2].Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 4}) || !menu.slots[1].stack.Empty() || !menu.slots[0].stack.Empty() {
			t.Fatalf("number-key craft = hotbar %+v input %+v result %+v", session.Player.Inventory.Hotbar[2], *menu.slots[1].stack, *menu.slots[0].stack)
		}
	})

	t.Run("throw all repeats", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		menu := session.activeMenu()

		session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemOakLog, Count: 2}

		refreshCraftingMenu(menu)

		craftingClick(t, session, protocol.ContainerClick{Slot: 0, MouseButton: 1, Mode: clickModeThrow})

		var dropped int32

		for _, entity := range runtime.snapshotRuntimeEntities() {
			item, valid := entity.(*runtimeItemEntity)
			if valid && item.Stack.Item == game.ItemOakPlanks {
				dropped += item.Stack.Count
			}
		}

		if dropped != 8 || !menu.slots[1].stack.Empty() || !menu.slots[0].stack.Empty() {
			t.Fatalf("throw-all craft = dropped %d input %+v result %+v", dropped, *menu.slots[1].stack, *menu.slots[0].stack)
		}
	})

	t.Run("pickup all excludes result", func(t *testing.T) {
		session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		menu := session.activeMenu()

		session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemOakLog, Count: 1}

		menu.carried = game.ItemStack{Item: game.ItemOakPlanks, Count: 1}

		refreshCraftingMenu(menu)

		craftingClick(t, session, protocol.ContainerClick{Slot: 9, MouseButton: 0, Mode: clickModePickupAll})

		if menu.carried.Count != 1 || !menu.slots[0].stack.Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 4}) || !menu.slots[1].stack.Equal(game.ItemStack{Item: game.ItemOakLog, Count: 1}) {
			t.Fatalf("pickup-all craft = carried %+v result %+v input %+v", menu.carried, *menu.slots[0].stack, *menu.slots[1].stack)
		}
	})
}

func TestCraftingRemaindersUseInventoryThenDropWithoutDuplication(t *testing.T) {
	t.Run("placed in player inventory", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		menu := session.activeMenu()

		session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemHoneyBottle, Count: 2}

		refreshCraftingMenu(menu)

		craftingClick(t, session, protocol.ContainerClick{Slot: 0, MouseButton: 0, Mode: clickModePickup})

		if !menu.slots[1].stack.Equal(game.ItemStack{Item: game.ItemHoneyBottle, Count: 1}) || !session.Player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemGlassBottle, Count: 1}) || !menu.carried.Equal(game.ItemStack{Item: game.ItemSugar, Count: 3}) || len(runtime.snapshotRuntimeEntities()) != 0 {
			t.Fatalf("remainder crafting = input %+v inventory %+v carried %+v drops %d", *menu.slots[1].stack, session.Player.Inventory.Hotbar[0], menu.carried, len(runtime.snapshotRuntimeEntities()))
		}
	})

	t.Run("drops when player inventory is full", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		menu := session.activeMenu()

		fillVisiblePlayerInventory(&session.Player.Inventory, game.ItemStone)

		session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemHoneyBottle, Count: 2}

		refreshCraftingMenu(menu)

		craftingClick(t, session, protocol.ContainerClick{Slot: 0, MouseButton: 0, Mode: clickModePickup})

		item := onlyRuntimeItemEntity(t, runtime)
		if !menu.slots[1].stack.Equal(game.ItemStack{Item: game.ItemHoneyBottle, Count: 1}) || !menu.carried.Equal(game.ItemStack{Item: game.ItemSugar, Count: 3}) || !item.Stack.Equal(game.ItemStack{Item: game.ItemGlassBottle, Count: 1}) || len(runtime.snapshotRuntimeEntities()) != 1 {
			t.Fatalf("overflow remainder crafting = input %+v carried %+v dropped %+v", *menu.slots[1].stack, menu.carried, item.Stack)
		}
	})
}

func TestCraftingRejectsStalePredictionWithoutMutation(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	menu := session.activeMenu()

	session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemOakLog, Count: 1}

	refreshCraftingMenu(menu)

	connection.reset()

	err := session.handleContainerClick(protocol.ContainerClick{WindowID: menu.windowID, StateID: menu.stateID - 1, Slot: 0, MouseButton: 0, Mode: clickModePickup})
	if err != nil {
		t.Fatalf("handle stale crafting click: %v", err)
	}

	if !menu.slots[1].stack.Equal(game.ItemStack{Item: game.ItemOakLog, Count: 1}) || !menu.slots[0].stack.Equal(game.ItemStack{Item: game.ItemOakPlanks, Count: 4}) || !menu.carried.Empty() || len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatalf("stale crafting click mutated input %+v result %+v carried %+v drops %d", *menu.slots[1].stack, *menu.slots[0].stack, menu.carried, len(runtime.snapshotRuntimeEntities()))
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
}

func TestCraftingTableMenuLayoutRecipesAndValidity(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	t.Run("opens a 3x3 crafting menu", func(t *testing.T) {
		runtime, session, connection := newCraftingTableTestRuntime(t, position)

		openCraftingTableForTest(t, runtime, session, position)

		menu := session.activeMenu()
		if menu.protocolMenuType != protocol.MenuCrafting || menu.containerSlots != 10 || len(menu.slots) != 46 {
			t.Fatalf("crafting table layout = type %d container %d slots %d", menu.protocolMenuType, menu.containerSlots, len(menu.slots))
		}

		assertCraftingTableOpenScreen(t, connection.packets(t)[0], menu.windowID)
		assertMenuSnapshotHeader(t, connection.packets(t)[1], menu.windowID, 0, 46)
	})

	t.Run("supports 3x3 recipes that the player grid rejects", func(t *testing.T) {
		runtime, session, _ := newCraftingTableTestRuntime(t, position)

		openCraftingTableForTest(t, runtime, session, position)

		menu := session.activeMenu()

		menu.slots[1].stack.Item, menu.slots[1].stack.Count = game.ItemIronIngot, 1
		menu.slots[2].stack.Item, menu.slots[2].stack.Count = game.ItemIronIngot, 1
		menu.slots[3].stack.Item, menu.slots[3].stack.Count = game.ItemIronIngot, 1
		menu.slots[5].stack.Item, menu.slots[5].stack.Count = game.ItemStick, 1
		menu.slots[8].stack.Item, menu.slots[8].stack.Count = game.ItemStick, 1

		refreshCraftingMenu(menu)

		if !menu.slots[0].stack.Equal(game.ItemStack{Item: game.ItemIronPickaxe, Count: 1}) {
			t.Fatalf("crafting table result = %+v", *menu.slots[0].stack)
		}

		session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemIronIngot, Count: 1}
		session.Player.Inventory.Crafting[1] = game.ItemStack{Item: game.ItemIronIngot, Count: 1}
		session.Player.Inventory.Crafting[2] = game.ItemStack{Item: game.ItemIronIngot, Count: 1}
		session.Player.Inventory.Crafting[3] = game.ItemStack{Item: game.ItemStick, Count: 1}

		refreshCraftingMenu(session.inventoryMenu)

		if !session.inventoryMenu.slots[0].stack.Empty() {
			t.Fatalf("player 2x2 grid produced 3x3 result %+v", *session.inventoryMenu.slots[0].stack)
		}
	})

	t.Run("returns inputs on close and drops overflow", func(t *testing.T) {
		runtime, session, _ := newCraftingTableTestRuntime(t, position)

		openCraftingTableForTest(t, runtime, session, position)

		menu := session.activeMenu()

		*menu.slots[1].stack = game.ItemStack{Item: game.ItemDirt, Count: 2}

		runtime.closeMenu(session, false)

		if countInventoryItem(session.Player.Inventory, game.ItemDirt) != 2 || len(runtime.snapshotRuntimeEntities()) != 0 {
			t.Fatalf("closed crafting table = dirt %d drops %d", countInventoryItem(session.Player.Inventory, game.ItemDirt), len(runtime.snapshotRuntimeEntities()))
		}

		openCraftingTableForTest(t, runtime, session, position)

		menu = session.activeMenu()

		fillVisiblePlayerInventory(&session.Player.Inventory, game.ItemStone)

		*menu.slots[1].stack = game.ItemStack{Item: game.ItemDirt, Count: 2}

		runtime.closeMenu(session, false)

		item := onlyRuntimeItemEntity(t, runtime)
		if !item.Stack.Equal(game.ItemStack{Item: game.ItemDirt, Count: 2}) || len(runtime.snapshotRuntimeEntities()) != 1 {
			t.Fatalf("overflow close drop = %+v", item.Stack)
		}
	})

	t.Run("closes when replaced or out of range", func(t *testing.T) {
		tests := []craftingTableValidityTestCase{
			{name: "replacement", mutate: func(runtime *Runtime, _ *Session) { runtime.World.SetBlock(position, game.Stone) }},
			{name: "range", mutate: func(_ *Runtime, session *Session) { session.Player.Position.X = 100 }},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runtime, session, connection := newCraftingTableTestRuntime(t, position)

				openCraftingTableForTest(t, runtime, session, position)

				connection.reset()

				test.mutate(runtime, session)

				runtime.tickOpenMenus()

				if session.activeMenu() != session.inventoryMenu {
					t.Fatal("invalid crafting table retained its menu")
				}

				assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundCloseContainerID})
			})
		}
	})
}

func newCraftingTableTestRuntime(t *testing.T, position game.BlockPosition) (*Runtime, *Session, *recordingConnection) {
	t.Helper()

	world := &game.World{}

	world.SetBlock(position, game.CraftingTable)

	runtime := NewRuntime(world)

	session, connection := newPlacementTestSession(runtime, position)

	joinTestSession(t, runtime, session)

	connection.reset()

	return runtime, session, connection
}

func openCraftingTableForTest(t *testing.T, runtime *Runtime, session *Session, position game.BlockPosition) {
	t.Helper()

	handled, result, _, err := runtime.InteractBlock(session, position)
	if err != nil || !handled || !result.Allowed || session.activeMenu() == session.inventoryMenu {
		t.Fatalf("open crafting table = handled %t result %+v err %v", handled, result, err)
	}
}

func craftingClick(t *testing.T, session *Session, click protocol.ContainerClick) {
	t.Helper()

	menu := session.activeMenu()
	candidate := menu.candidate()

	candidate.selected = session.Player.SelectedHotbarSlot

	if !applyMenuClick(candidate, session.Player.GameMode, click) {
		t.Fatalf("could not construct crafting click prediction: %+v", click)
	}

	click.WindowID = menu.windowID
	click.StateID = menu.stateID
	click.CursorItem = hashedStack(candidate.carried)

	for _, slot := range candidate.changedSlots() {
		click.ChangedSlots = append(click.ChangedSlots, protocol.ChangedSlot{Location: int16(slot), Item: hashedStack(candidate.slots[slot])})
	}

	err := session.handleContainerClick(click)
	if err != nil {
		t.Fatalf("handle crafting click: %v", err)
	}
}

func refreshCraftingMenu(menu *menu) {
	candidate := menu.candidate()

	candidate.deriveSlots()

	menu.commit(candidate)
}

func fillVisiblePlayerInventory(inventory *game.PlayerInventory, item game.Item) {
	for slot := 9; slot <= 44; slot++ {
		*inventory.Slot(slot) = game.ItemStack{Item: item, Count: 64}
	}
}

func countInventoryItem(inventory game.PlayerInventory, item game.Item) int32 {
	var count int32

	for slot := 9; slot <= 45; slot++ {
		stack := inventory.Slot(slot)
		if stack.Item == item {
			count += stack.Count
		}
	}

	return count
}

func assertCraftingTableOpenScreen(t *testing.T, packet protocol.Packet, windowID int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	if actualWindowID, menuType := reader.VarInt(), reader.VarInt(); actualWindowID != windowID || menuType != protocol.MenuCrafting {
		t.Fatalf("crafting table open screen = window %d type %d; want window %d type %d", actualWindowID, menuType, windowID, protocol.MenuCrafting)
	}
}
