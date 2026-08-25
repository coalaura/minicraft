package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestSelectedHotbarSlotTracking(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.handleSetHeldItem(protocol.SetHeldItem{Slot: 8})

	if session.snapshotPlayer().SelectedHotbarSlot != 8 {
		t.Fatalf("selected hotbar slot = %d, want 8", session.snapshotPlayer().SelectedHotbarSlot)
	}

	session.handleSetHeldItem(protocol.SetHeldItem{Slot: 9})

	if session.snapshotPlayer().SelectedHotbarSlot != 8 {
		t.Fatalf("invalid update changed selected hotbar slot to %d", session.snapshotPlayer().SelectedHotbarSlot)
	}
}

func TestCreativeSlotUpdatesTrackHotbarAndOffhand(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.GameMode = game.GameModeCreative

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 40,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemStone), ItemCount: 64},
	})

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 45,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemDirt), ItemCount: 32},
	})

	player := session.snapshotPlayer()
	if !player.Inventory.Hotbar[4].Equal(game.ItemStack{Item: game.ItemStone, Count: 64}) {
		t.Fatalf("hotbar stack = %+v", player.Inventory.Hotbar[4])
	}

	if !player.Inventory.Offhand.Equal(game.ItemStack{Item: game.ItemDirt, Count: 32}) {
		t.Fatalf("offhand stack = %+v", player.Inventory.Offhand)
	}

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{Slot: 40})

	if !session.snapshotPlayer().Inventory.Hotbar[4].Empty() {
		t.Fatalf("cleared hotbar stack = %+v", session.snapshotPlayer().Inventory.Hotbar[4])
	}
}

func TestCreativeSlotUpdatesRejectInvalidOrNonCreativeChanges(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.GameMode = game.GameModeSurvival

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 36,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemStone), ItemCount: 1},
	})

	if !session.snapshotPlayer().Inventory.Hotbar[0].Empty() {
		t.Fatal("survival player changed creative slot")
	}

	session.Player.GameMode = game.GameModeCreative

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 36,
		Item: protocol.UntrustedSlot{ItemID: int32(game.MaxItemID) + 1, ItemCount: 1},
	})

	if !session.snapshotPlayer().Inventory.Hotbar[0].Empty() {
		t.Fatal("invalid item changed creative slot")
	}

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 36,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemStone), ItemCount: 65},
	})

	if !session.snapshotPlayer().Inventory.Hotbar[0].Empty() {
		t.Fatal("oversized stack changed creative slot")
	}
}

func TestHeldEquipmentUpdatesVisiblePlayers(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	actor.Player.GameMode = game.GameModeCreative

	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, actor)

	observerConnection.reset()

	actor.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 36,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemStone), ItemCount: 64},
	})

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityEquipmentID})
	assertEquipmentUpdate(t, observerConnection.packets(t)[0], actor.Player.EntityID, protocol.EquipmentSlotMainHand, game.ItemStone, 64)

	observerConnection.reset()

	actor.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 37,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemDirt), ItemCount: 32},
	})

	assertPacketIDs(t, observerConnection.packetIDs(t), nil)

	actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityEquipmentID})
	assertEquipmentUpdate(t, observerConnection.packets(t)[0], actor.Player.EntityID, protocol.EquipmentSlotMainHand, game.ItemDirt, 32)
}

func TestDroppingHeldItemsUpdatesCreativeStateAndEquipment(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, actorConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	actor.Player.GameMode = game.GameModeCreative
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakLog, Count: 2}

	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, actor)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionDropItem, Sequence: 201})
	if err != nil {
		t.Fatalf("drop held item: %v", err)
	}

	if stack := actor.snapshotPlayer().Inventory.Hotbar[0]; !stack.Equal(game.ItemStack{Item: game.ItemOakLog, Count: 1}) {
		t.Fatalf("stack after dropping one = %+v", stack)
	}

	assertEquipmentUpdate(t, observerConnection.packets(t)[0], actor.Player.EntityID, protocol.EquipmentSlotMainHand, game.ItemOakLog, 1)
	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID, protocol.ClientboundBlockChangedAckID})
	assertBlockChangedAck(t, actorConnection.packets(t)[1], 201)

	actorConnection.reset()
	observerConnection.reset()

	err = actor.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionDropItem, Sequence: 202})
	if err != nil {
		t.Fatalf("drop last held item: %v", err)
	}

	if stack := actor.snapshotPlayer().Inventory.Hotbar[0]; !stack.Empty() {
		t.Fatalf("stack after dropping last item = %+v", stack)
	}

	assertEmptyEquipmentUpdate(t, observerConnection.packets(t)[0], actor.Player.EntityID, protocol.EquipmentSlotMainHand)
	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID, protocol.ClientboundBlockChangedAckID})
	assertBlockChangedAck(t, actorConnection.packets(t)[1], 202)

	actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})
	actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 0})

	if stack := actor.snapshotPlayer().Inventory.Hotbar[0]; !stack.Empty() {
		t.Fatalf("switching restored dropped stack = %+v", stack)
	}
}

func TestDroppingAllHeldItemsClearsCreativeStack(t *testing.T) {
	runtime := NewRuntime(&game.World{})
	actor, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")
	actor.Player.GameMode = game.GameModeCreative
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 64}

	actor.handleDropHeldItem(true)

	if stack := actor.snapshotPlayer().Inventory.Hotbar[0]; !stack.Empty() {
		t.Fatalf("stack after dropping all = %+v", stack)
	}
}

func TestJoiningPlayerReceivesVisibleEquipment(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 64}
	actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemDirt, Count: 32}

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	for _, packet := range observerConnection.packets(t) {
		if packet.ID != protocol.ClientboundEntityEquipmentID {
			continue
		}

		reader := protocol.NewPacketReader(packet.Data)
		if entityID := reader.VarInt(); entityID != actor.Player.EntityID {
			t.Fatalf("equipment entity id = %d, want %d", entityID, actor.Player.EntityID)
		}

		assertEquipmentEntry(t, reader, protocol.EquipmentSlotMainHand|0x80, game.ItemStone, 64)
		assertEquipmentEntry(t, reader, protocol.EquipmentSlotOffHand, game.ItemDirt, 32)

		err := reader.Err()
		if err != nil {
			t.Fatalf("decode visible equipment: %v", err)
		}

		return
	}

	t.Fatal("joining player did not receive visible equipment")
}

func TestContainerClickCommitsPredictedPickupAndSynchronizes(t *testing.T) {
	session, connection := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 10}
	session.Player.Inventory.StateID = 4

	err := session.handleContainerClick(protocol.ContainerClick{
		WindowID:    playerInventoryWindowID,
		StateID:     4,
		Slot:        9,
		MouseButton: 0,
		Mode:        clickModePickup,
		ChangedSlots: []protocol.ChangedSlot{{
			Location: 9,
		}},
		CursorItem: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 10}),
	})

	if err != nil {
		t.Fatalf("handle pickup click: %v", err)
	}

	player := session.snapshotPlayer()
	if !player.Inventory.Main[0].Empty() || !player.Inventory.Carried.Equal(game.ItemStack{Item: game.ItemStone, Count: 10}) || player.Inventory.StateID != 5 {
		t.Fatalf("inventory after pickup = %+v", player.Inventory)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
	assertInventoryContentHeader(t, connection.packets(t)[0], 5, game.PlayerInventorySlots)
}

func TestContainerClickRejectsStaleAndInconsistentPredictions(t *testing.T) {
	session, connection := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 8}
	session.Player.Inventory.StateID = 7

	tests := map[string]protocol.ContainerClick{
		"stale state": {
			WindowID: playerInventoryWindowID,
			StateID:  6,
			Slot:     36,
			Mode:     clickModePickup,
		},
		"wrong cursor": {
			WindowID: playerInventoryWindowID,
			StateID:  7,
			Slot:     36,
			Mode:     clickModePickup,
			ChangedSlots: []protocol.ChangedSlot{{
				Location: 36,
			}},
			CursorItem: hashedStack(game.ItemStack{Item: game.ItemDirt, Count: 8}),
		},
		"duplicate changed slot": {
			WindowID: playerInventoryWindowID,
			StateID:  7,
			Slot:     36,
			Mode:     clickModePickup,
			ChangedSlots: []protocol.ChangedSlot{
				{Location: 36},
				{Location: 36},
			},
			CursorItem: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 8}),
		},
	}

	for name, click := range tests {
		t.Run(name, func(t *testing.T) {
			connection.reset()

			err := session.handleContainerClick(click)
			if err != nil {
				t.Fatalf("handle invalid click: %v", err)
			}

			player := session.snapshotPlayer()
			if !player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 8}) || !player.Inventory.Carried.Empty() || player.Inventory.StateID != 7 {
				t.Fatalf("invalid click changed inventory = %+v", player.Inventory)
			}

			assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
			assertInventoryContentHeader(t, connection.packets(t)[0], 7, game.PlayerInventorySlots)
		})
	}
}

func TestPlayerInventoryStandardOperations(t *testing.T) {
	t.Run("right click splits and places one", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 9}

		if !applyPickup(&inventory, 9, 1) || inventory.Main[0].Count != 4 || inventory.Carried.Count != 5 {
			t.Fatalf("inventory after split = %+v", inventory)
		}

		if !applyPickup(&inventory, 36, 1) || inventory.Hotbar[0].Count != 1 || inventory.Carried.Count != 4 {
			t.Fatalf("inventory after place one = %+v", inventory)
		}
	})

	t.Run("shift click equips armor", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

		if !applyQuickMove(&inventory, 9, 0) || !inventory.Main[0].Empty() || inventory.Armor[0].Item != game.ItemIronHelmet {
			t.Fatalf("inventory after armor quick move = %+v", inventory)
		}
	})

	t.Run("hotbar and offhand swaps", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 2}
		inventory.Hotbar[3] = game.ItemStack{Item: game.ItemDirt, Count: 3}

		if !applySwap(&inventory, 9, 3) || inventory.Main[0].Item != game.ItemDirt || inventory.Hotbar[3].Item != game.ItemStone {
			t.Fatalf("inventory after hotbar swap = %+v", inventory)
		}

		if !applySwap(&inventory, 9, 40) || inventory.Main[0].Item != game.ItemAir || inventory.Offhand.Item != game.ItemDirt {
			t.Fatalf("inventory after offhand swap = %+v", inventory)
		}
	})

	t.Run("throw and double click collection", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 3}

		if !applyThrow(&inventory, 9, 0) || inventory.Main[0].Count != 2 {
			t.Fatalf("inventory after throw one = %+v", inventory)
		}

		inventory.Carried = game.ItemStack{Item: game.ItemStone, Count: 60}
		inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 8}

		if !applyPickupAll(&inventory, 9, 0) || inventory.Carried.Count != 64 || !inventory.Main[0].Empty() || inventory.Hotbar[0].Count != 6 {
			t.Fatalf("inventory after pickup all = %+v", inventory)
		}
	})

	t.Run("creative clone", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 1}

		if !applyClone(&inventory, game.GameModeCreative, 9, 2) || inventory.Carried.Count != 64 {
			t.Fatalf("inventory after clone = %+v", inventory)
		}

		if applyClone(&inventory, game.GameModeSurvival, 9, 2) {
			t.Fatal("survival clone was accepted")
		}
	})
}

func TestQuickCraftRequiresStartAndIgnoresFullSlots(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	inventory := game.PlayerInventory{Carried: game.ItemStack{Item: game.ItemStone, Count: 8}}
	inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 64}

	if session.applyQuickCraft(&inventory, game.GameModeSurvival, 37, 1) {
		t.Fatal("quick craft slot was accepted without a start")
	}

	if !session.applyQuickCraft(&inventory, game.GameModeSurvival, outsideInventorySlot, 0) {
		t.Fatal("quick craft start was rejected")
	}

	if !session.applyQuickCraft(&inventory, game.GameModeSurvival, 36, 1) || !session.applyQuickCraft(&inventory, game.GameModeSurvival, 37, 1) {
		t.Fatal("quick craft slot selection was rejected")
	}

	if len(session.inventoryDrag.slots) != 1 || session.inventoryDrag.slots[0] != 37 {
		t.Fatalf("quick craft slots = %v, want [37]", session.inventoryDrag.slots)
	}

	if !session.applyQuickCraft(&inventory, game.GameModeSurvival, outsideInventorySlot, 2) || inventory.Hotbar[1].Count != 8 || !inventory.Carried.Empty() {
		t.Fatalf("inventory after quick craft = %+v", inventory)
	}
}

func TestCreativeArmorUpdateBroadcastsVisibleEquipment(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	actor.Player.GameMode = game.GameModeCreative

	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, actor)

	observerConnection.reset()

	actor.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 5,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemIronHelmet), ItemCount: 1},
	})

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityEquipmentID})
	assertEquipmentUpdate(t, observerConnection.packets(t)[0], actor.Player.EntityID, protocol.EquipmentSlotHead, game.ItemIronHelmet, 1)
}

func TestContainerClickArmorChangeBroadcastsVisibleEquipment(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, actorConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	actor.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, actor)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handleContainerClick(protocol.ContainerClick{
		WindowID:    playerInventoryWindowID,
		Slot:        9,
		MouseButton: 0,
		Mode:        clickModeQuickMove,
		ChangedSlots: []protocol.ChangedSlot{
			{Location: 5, Item: hashedStack(game.ItemStack{Item: game.ItemIronHelmet, Count: 1})},
			{Location: 9},
		},
	})

	if err != nil {
		t.Fatalf("handle armor quick move: %v", err)
	}

	player := actor.snapshotPlayer()
	if player.Inventory.Armor[0].Item != game.ItemIronHelmet || !player.Inventory.Main[0].Empty() {
		t.Fatalf("inventory after armor quick move = %+v", player.Inventory)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityEquipmentID})
	assertEquipmentUpdate(t, observerConnection.packets(t)[0], actor.Player.EntityID, protocol.EquipmentSlotHead, game.ItemIronHelmet, 1)
}

func TestPlayerInventorySynchronizationIncludesAllSlotsAndCarriedItem(t *testing.T) {
	session, connection := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.Inventory.StateID = 12
	session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemStone, Count: 1}
	session.Player.Inventory.Main[26] = game.ItemStack{Item: game.ItemDirt, Count: 2}
	session.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemOakLog, Count: 3}
	session.Player.Inventory.Carried = game.ItemStack{Item: game.ItemStone, Count: 4}

	err := session.sendPlayerInventory()
	if err != nil {
		t.Fatalf("send player inventory: %v", err)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})

	packet := connection.packets(t)[0]

	reader := protocol.NewPacketReader(packet.Data)

	if reader.VarInt() != playerInventoryWindowID || reader.VarInt() != 12 || reader.VarInt() != game.PlayerInventorySlots {
		t.Fatal("inventory content header does not match the player inventory")
	}

	items := make([]game.ItemStack, game.PlayerInventorySlots)

	for slot := range items {
		items[slot] = readSimpleItemStack(t, reader)
	}

	carried := readSimpleItemStack(t, reader)
	if items[1].Item != game.ItemStone || items[35].Item != game.ItemDirt || items[45].Item != game.ItemOakLog || !carried.Equal(game.ItemStack{Item: game.ItemStone, Count: 4}) {
		t.Fatalf("synchronized inventory slots = crafting %+v, main %+v, offhand %+v, carried %+v", items[1], items[35], items[45], carried)
	}

	err = reader.Err()
	if err != nil || reader.Len() != 0 {
		t.Fatalf("decode synchronized inventory: %v, trailing bytes = %d", err, reader.Len())
	}
}

func TestInventoryStateIDWrapsAtVanillaBoundary(t *testing.T) {
	if nextInventoryStateID(32766) != 32767 || nextInventoryStateID(32767) != 0 {
		t.Fatalf("inventory state rollover = %d, %d", nextInventoryStateID(32766), nextInventoryStateID(32767))
	}
}

func hashedStack(stack game.ItemStack) protocol.HashedSlot {
	if stack.Empty() {
		return protocol.HashedSlot{}
	}

	return protocol.HashedSlot{
		Present:           true,
		ItemID:            int32(stack.Item),
		ItemCount:         stack.Count,
		RemovedComponents: append([]int32(nil), stack.RemovedComponents...),
	}
}

func assertInventoryContentHeader(t *testing.T, packet protocol.Packet, stateID, itemCount int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	if windowID := reader.VarInt(); windowID != playerInventoryWindowID {
		t.Fatalf("inventory window id = %d, want %d", windowID, playerInventoryWindowID)
	}

	if actualStateID := reader.VarInt(); actualStateID != stateID {
		t.Fatalf("inventory state id = %d, want %d", actualStateID, stateID)
	}

	if actualItemCount := reader.VarInt(); actualItemCount != itemCount {
		t.Fatalf("inventory item count = %d, want %d", actualItemCount, itemCount)
	}
}

func readSimpleItemStack(t *testing.T, reader *protocol.PacketReader) game.ItemStack {
	t.Helper()

	count := reader.VarInt()
	if count == 0 {
		return game.ItemStack{}
	}

	stack := game.ItemStack{Item: game.Item(reader.VarInt()), Count: count}

	if added, removed := reader.VarInt(), reader.VarInt(); added != 0 || removed != 0 {
		t.Fatalf("simple item stack has %d added and %d removed components", added, removed)
	}

	return stack
}

func assertEquipmentUpdate(t *testing.T, packet protocol.Packet, entityID int32, slot byte, item game.Item, count int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actual := reader.VarInt()
	if actual != entityID {
		t.Fatalf("equipment entity id = %d, want %d", actual, entityID)
	}

	assertEquipmentEntry(t, reader, slot, item, count)

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode equipment update: %v", err)
	}
}

func assertEmptyEquipmentUpdate(t *testing.T, packet protocol.Packet, entityID int32, slot byte) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actual := reader.VarInt()
	if actual != entityID {
		t.Fatalf("equipment entity id = %d, want %d", actual, entityID)
	}

	actualB := reader.Byte()
	if actualB != slot {
		t.Fatalf("equipment slot = %d, want %d", actualB, slot)
	}

	count := reader.VarInt()
	if count != 0 {
		t.Fatalf("equipment item count = %d, want 0", count)
	}

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode empty equipment update: %v", err)
	}
}

func assertEquipmentEntry(t *testing.T, reader *protocol.PacketReader, slot byte, item game.Item, count int32) {
	t.Helper()

	actualB := reader.Byte()
	if actualB != slot {
		t.Fatalf("equipment slot = %d, want %d", actualB, slot)
	}

	actual := reader.VarInt()
	if actual != count {
		t.Fatalf("equipment item count = %d, want %d", actual, count)
	}

	actual = reader.VarInt()
	if actual != int32(item) {
		t.Fatalf("equipment item id = %d, want %d", actual, item)
	}

	added := reader.VarInt()
	removed := reader.VarInt()

	if added != 0 || removed != 0 {
		t.Fatalf("equipment components = (%d added, %d removed), want none", added, removed)
	}

}
