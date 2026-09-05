package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type equipmentSlotTestCase struct {
	name string
	item game.Item
	slot int
}

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
	session, connection := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

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

	assertPacketIDs(t, connection.packetIDs(t), []int32{
		protocol.ClientboundContainerSetSlotID,
		protocol.ClientboundContainerSetSlotID,
		protocol.ClientboundContainerSetSlotID,
	})

	packets := connection.packets(t)

	reader := protocol.NewPacketReader(packets[2].Data)

	windowID := reader.VarInt()
	if windowID != playerInventoryWindowID {
		t.Fatalf("creative slot window id = %d, want %d", windowID, playerInventoryWindowID)
	}

	stateID := reader.VarInt()
	if stateID != 3 {
		t.Fatalf("creative slot state id = %d, want 3", stateID)
	}

	slot := reader.Short()
	if slot != 40 {
		t.Fatalf("creative slot = %d, want 40", slot)
	}

	count := reader.VarInt()
	if count != 0 {
		t.Fatalf("creative slot item count = %d, want 0", count)
	}

	err := reader.Done("creative slot synchronization")
	if err != nil {
		t.Fatalf("decode creative slot synchronization: %v", err)
	}
}

func TestCreativeSlotUpdatesPreserveGameplayComponents(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")
	session.Player.GameMode = game.GameModeCreative

	stack := game.ItemStack{Item: game.ItemDiamondPickaxe, Count: 1}
	stack.SetDamage(16)
	stack.SetEnchantment(game.EnchantmentUnbreaking, 3)

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 36,
		Item: protocol.UntrustedSlot{
			ItemID:     int32(stack.Item),
			ItemCount:  stack.Count,
			Components: stack.Clone().Components,
		},
	})

	actual := session.snapshotPlayer().Inventory.Hotbar[0]
	if !actual.Equal(stack) {
		t.Fatalf("creative gameplay component stack = %+v, want %+v", actual, stack)
	}
}

func TestCreativeSlotUpdatesPreservePotionContents(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.GameMode = game.GameModeCreative

	component := game.ItemComponent{Type: game.ItemComponentPotionContents, Data: []byte{0x01, 0x0A, 0x00, 0x00, 0x00}}
	stack := game.ItemStack{Item: game.ItemPotion, Count: 1, Components: []game.ItemComponent{component}}

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 38,
		Item: protocol.UntrustedSlot{
			ItemID:     int32(stack.Item),
			ItemCount:  stack.Count,
			Components: stack.Clone().Components,
		},
	})

	actual := session.snapshotPlayer().Inventory.Hotbar[2]
	if !actual.Equal(stack) {
		t.Fatalf("creative potion component stack = %+v, want %+v", actual, stack)
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

func TestCreativePickBlockPlacesItemInSelectedHotbarSlot(t *testing.T) {
	world := &game.World{}

	session, connection := newMovementTestSession(NewRuntime(world), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")

	position := game.BlockPosition{X: 4, Y: 70, Z: -3}

	session.Player.GameMode = game.GameModeCreative
	session.Player.SelectedHotbarSlot = 2
	session.activeMenu().stateID = 7
	session.loadedChunks = map[LoadedChunk]struct{}{blockLoadedChunk(position): {}}

	world.SetBlock(position, game.Stone)

	session.handlePickItemFromBlock(protocol.PickItemFromBlock{Position: position, IncludeData: true})

	player := session.snapshotPlayer()
	if !player.Inventory.Hotbar[2].Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) {
		t.Fatalf("picked stack = %+v", player.Inventory.Hotbar[2])
	}

	if player.SelectedHotbarSlot != 2 || session.activeMenu().stateID != 8 {
		t.Fatalf("player after pick = %+v", player)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
	assertInventoryContentHeader(t, connection.packets(t)[0], 8, game.PlayerInventorySlots)
}

func TestCreativePickBlockUsesNextEmptyHotbarSlotWithWraparound(t *testing.T) {
	world := &game.World{}

	session, connection := newMovementTestSession(NewRuntime(world), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")

	position := game.BlockPosition{X: 4, Y: 70, Z: -3}

	session.Player.GameMode = game.GameModeCreative
	session.Player.SelectedHotbarSlot = 7
	session.Player.Inventory.Hotbar[7] = game.ItemStack{Item: game.ItemDirt, Count: 64}
	session.Player.Inventory.Hotbar[8] = game.ItemStack{Item: game.ItemDirt, Count: 64}
	session.loadedChunks = map[LoadedChunk]struct{}{blockLoadedChunk(position): {}}

	world.SetBlock(position, game.Stone)

	session.handlePickItemFromBlock(protocol.PickItemFromBlock{Position: position})

	player := session.snapshotPlayer()
	if player.SelectedHotbarSlot != 0 {
		t.Fatalf("selected hotbar slot = %d, want 0", player.SelectedHotbarSlot)
	}

	if !player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) {
		t.Fatalf("wrapped picked stack = %+v", player.Inventory.Hotbar[0])
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{
		protocol.ClientboundSetHeldSlotID,
		protocol.ClientboundContainerSetContentID,
	})
}

func TestCreativePickBlockReplacesSelectedSlotWhenHotbarIsFull(t *testing.T) {
	world := &game.World{}

	session, connection := newMovementTestSession(NewRuntime(world), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")

	position := game.BlockPosition{X: 4, Y: 70, Z: -3}

	session.Player.GameMode = game.GameModeCreative
	session.Player.SelectedHotbarSlot = 4
	session.loadedChunks = map[LoadedChunk]struct{}{blockLoadedChunk(position): {}}

	for slot := range session.Player.Inventory.Hotbar {
		session.Player.Inventory.Hotbar[slot] = game.ItemStack{Item: game.ItemDirt, Count: 64}
	}

	world.SetBlock(position, game.Stone)

	session.handlePickItemFromBlock(protocol.PickItemFromBlock{Position: position})

	player := session.snapshotPlayer()
	if player.SelectedHotbarSlot != 4 {
		t.Fatalf("selected hotbar slot = %d, want 4", player.SelectedHotbarSlot)
	}

	if !player.Inventory.Hotbar[4].Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) {
		t.Fatalf("replacement picked stack = %+v", player.Inventory.Hotbar[4])
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
}

func TestCreativePickBlockSelectsExistingHotbarStack(t *testing.T) {
	world := &game.World{}

	session, connection := newMovementTestSession(NewRuntime(world), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")

	position := game.BlockPosition{X: 4, Y: 70, Z: -3}

	session.Player.GameMode = game.GameModeCreative
	session.activeMenu().stateID = 7
	session.Player.Inventory.Hotbar[5] = game.ItemStack{Item: game.ItemStone, Count: 3}
	session.loadedChunks = map[LoadedChunk]struct{}{blockLoadedChunk(position): {}}

	world.SetBlock(position, game.Stone)

	session.handlePickItemFromBlock(protocol.PickItemFromBlock{Position: position})

	player := session.snapshotPlayer()
	if player.SelectedHotbarSlot != 5 {
		t.Fatalf("selected hotbar slot = %d, want 5", player.SelectedHotbarSlot)
	}

	if !player.Inventory.Hotbar[5].Equal(game.ItemStack{Item: game.ItemStone, Count: 3}) || session.activeMenu().stateID != 7 {
		t.Fatalf("inventory changed while selecting existing stack: %+v", player.Inventory)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundSetHeldSlotID})

	reader := protocol.NewPacketReader(connection.packets(t)[0].Data)

	slot := reader.VarInt()
	if slot != 5 {
		t.Fatalf("client selected slot = %d, want 5", slot)
	}

	err := reader.Done("set held slot")
	if err != nil {
		t.Fatalf("decode selected slot: %v", err)
	}
}

func TestPickBlockRejectsSurvivalAndUnloadedBlocks(t *testing.T) {
	world := &game.World{}

	session, connection := newMovementTestSession(NewRuntime(world), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")

	position := game.BlockPosition{X: 4, Y: 70, Z: -3}

	world.SetBlock(position, game.Stone)

	session.loadedChunks = map[LoadedChunk]struct{}{blockLoadedChunk(position): {}}
	session.Player.GameMode = game.GameModeSurvival

	session.handlePickItemFromBlock(protocol.PickItemFromBlock{Position: position})

	if !session.snapshotPlayer().Inventory.Hotbar[0].Empty() {
		t.Fatal("survival pick changed inventory")
	}

	session.Player.GameMode = game.GameModeCreative
	session.loadedChunks = nil

	session.handlePickItemFromBlock(protocol.PickItemFromBlock{Position: position})

	if !session.snapshotPlayer().Inventory.Hotbar[0].Empty() {
		t.Fatal("unloaded pick changed inventory")
	}

	assertPacketIDs(t, connection.packetIDs(t), nil)
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

	stack := actor.snapshotPlayer().Inventory.Hotbar[0]
	if !stack.Equal(game.ItemStack{Item: game.ItemOakLog, Count: 1}) {
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

	stack = actor.snapshotPlayer().Inventory.Hotbar[0]
	if !stack.Empty() {
		t.Fatalf("stack after dropping last item = %+v", stack)
	}

	assertEmptyEquipmentUpdate(t, observerConnection.packets(t)[0], actor.Player.EntityID, protocol.EquipmentSlotMainHand)
	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID, protocol.ClientboundBlockChangedAckID})
	assertBlockChangedAck(t, actorConnection.packets(t)[1], 202)

	actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})
	actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 0})

	stack = actor.snapshotPlayer().Inventory.Hotbar[0]
	if !stack.Empty() {
		t.Fatalf("switching restored dropped stack = %+v", stack)
	}
}

func TestDroppingAllHeldItemsClearsCreativeStack(t *testing.T) {
	runtime := NewRuntime(&game.World{})
	actor, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")
	actor.Player.GameMode = game.GameModeCreative
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 64}

	actor.handleDropHeldItem(true)

	stack := actor.snapshotPlayer().Inventory.Hotbar[0]
	if !stack.Empty() {
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

		entityID := reader.VarInt()
		if entityID != actor.Player.EntityID {
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
	session.activeMenu().stateID = 4

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
	if !player.Inventory.Main[0].Empty() || !session.activeMenu().carried.Equal(game.ItemStack{Item: game.ItemStone, Count: 10}) || session.activeMenu().stateID != 5 {
		t.Fatalf("inventory after pickup = %+v", player.Inventory)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
	assertInventoryContentHeader(t, connection.packets(t)[0], 5, game.PlayerInventorySlots)
}

func TestContainerClickRejectsStaleAndInconsistentPredictions(t *testing.T) {
	session, connection := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 8}
	session.activeMenu().stateID = 7

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
			if !player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 8}) || !session.activeMenu().carried.Empty() || session.activeMenu().stateID != 7 {
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

		candidate := newPlayerInventoryMenu(&inventory).candidate()

		if !applyPickup(candidate, game.GameModeSurvival, 9, 1) || candidate.slots[9].Count != 4 || candidate.carried.Count != 5 {
			t.Fatalf("candidate after split = %+v", candidate)
		}

		if !applyPickup(candidate, game.GameModeSurvival, 36, 1) || candidate.slots[36].Count != 1 || candidate.carried.Count != 4 {
			t.Fatalf("candidate after place one = %+v", candidate)
		}
	})

	t.Run("shift click equips armor", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

		playerMenu := newPlayerInventoryMenu(&inventory)
		candidate := playerMenu.candidate()

		if !applyQuickMove(candidate, game.GameModeSurvival, 9, 0) || !candidate.slots[9].Empty() || candidate.slots[5].Item != game.ItemIronHelmet {
			t.Fatalf("candidate after armor quick move = %+v", candidate)
		}
	})

	t.Run("hotbar and offhand swaps", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 2}
		inventory.Hotbar[3] = game.ItemStack{Item: game.ItemDirt, Count: 3}

		candidate := newPlayerInventoryMenu(&inventory).candidate()

		if !applySwap(candidate, game.GameModeSurvival, 9, 3) || candidate.slots[9].Item != game.ItemDirt || candidate.slots[39].Item != game.ItemStone {
			t.Fatalf("candidate after hotbar swap = %+v", candidate)
		}

		if !applySwap(candidate, game.GameModeSurvival, 9, 40) || candidate.slots[9].Item != game.ItemAir || candidate.slots[45].Item != game.ItemDirt {
			t.Fatalf("candidate after offhand swap = %+v", candidate)
		}
	})

	t.Run("throw and double click collection", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 3}

		candidate := newPlayerInventoryMenu(&inventory).candidate()

		if !applyThrow(candidate, game.GameModeSurvival, 9, 0) || candidate.slots[9].Count != 2 {
			t.Fatalf("candidate after throw one = %+v", candidate)
		}

		candidate.carried = game.ItemStack{Item: game.ItemStone, Count: 60}
		candidate.slots[36] = game.ItemStack{Item: game.ItemStone, Count: 8}

		if !applyPickupAll(candidate, game.GameModeSurvival, 10, 0) || candidate.carried.Count != 64 || !candidate.slots[9].Empty() || candidate.slots[36].Count != 6 {
			t.Fatalf("candidate after pickup all = %+v", candidate)
		}
	})

	t.Run("creative clone", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 1}

		candidate := newPlayerInventoryMenu(&inventory).candidate()

		if !applyClone(candidate, game.GameModeCreative, 9, 2) || candidate.carried.Count != 64 {
			t.Fatalf("candidate after clone = %+v", candidate)
		}

		candidate.carried = game.ItemStack{}

		if !applyClone(candidate, game.GameModeCreative, 9, 0) || candidate.carried.Count != 64 {
			t.Fatalf("candidate after keybound clone = %+v", candidate)
		}

		if applyClone(candidate, game.GameModeSurvival, 9, 2) {
			t.Fatal("survival clone was accepted")
		}
	})
}

func TestBoundArmorCannotBeRemovedOutsideCreative(t *testing.T) {
	boundHelmet := game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

	boundHelmet.SetEnchantment(game.EnchantmentBindingCurse, 1)

	if canRemoveFromArmorSlot(game.GameModeSurvival, 5, boundHelmet) {
		t.Fatal("survival can remove bound armor")
	}

	if canRemoveFromArmorSlot(game.GameModeAdventure, 5, boundHelmet) {
		t.Fatal("adventure can remove bound armor")
	}

	if !canRemoveFromArmorSlot(game.GameModeCreative, 5, boundHelmet) {
		t.Fatal("creative cannot remove bound armor")
	}

	t.Run("survival pickup", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Armor[0] = boundHelmet

		candidate := newPlayerInventoryMenu(&inventory).candidate()
		if !applyPickup(candidate, game.GameModeSurvival, 5, 0) || !candidate.slots[5].Equal(boundHelmet) || !candidate.carried.Empty() {
			t.Fatalf("survival pickup changed bound armor = %+v", candidate)
		}
	})

	t.Run("survival shift click", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Armor[0] = boundHelmet

		candidate := newPlayerInventoryMenu(&inventory).candidate()
		if !applyQuickMove(candidate, game.GameModeSurvival, 5, 0) || !candidate.slots[5].Equal(boundHelmet) {
			t.Fatalf("survival shift click changed bound armor = %+v", candidate)
		}
	})

	t.Run("survival hotbar swap and throw", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Armor[0] = boundHelmet
		inventory.Hotbar[0] = game.ItemStack{Item: game.ItemDiamondHelmet, Count: 1}

		candidate := newPlayerInventoryMenu(&inventory).candidate()
		if !applySwap(candidate, game.GameModeSurvival, 5, 0) || !candidate.slots[5].Equal(boundHelmet) || candidate.slots[36].Item != game.ItemDiamondHelmet {
			t.Fatalf("survival swap changed bound armor = %+v", candidate)
		}

		if !applyThrow(candidate, game.GameModeSurvival, 5, 1) || !candidate.slots[5].Equal(boundHelmet) || len(candidate.dropped) != 0 {
			t.Fatalf("survival throw changed bound armor = %+v", candidate)
		}
	})

	t.Run("creative pickup and shift click", func(t *testing.T) {
		var inventory game.PlayerInventory

		inventory.Armor[0] = boundHelmet

		candidate := newPlayerInventoryMenu(&inventory).candidate()
		if !applyPickup(candidate, game.GameModeCreative, 5, 0) || !candidate.slots[5].Empty() || !candidate.carried.Equal(boundHelmet) {
			t.Fatalf("creative pickup did not remove bound armor = %+v", candidate)
		}

		candidate = newPlayerInventoryMenu(&inventory).candidate()
		if !applyQuickMove(candidate, game.GameModeCreative, 5, 0) || !candidate.slots[5].Empty() || candidate.slots[9].Item != game.ItemIronHelmet {
			t.Fatalf("creative shift click did not remove bound armor = %+v", candidate)
		}
	})
}

func TestQuickCraftRequiresStartAndCountsFullSlots(t *testing.T) {
	var inventory game.PlayerInventory

	inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 64}

	playerMenu := newPlayerInventoryMenu(&inventory)

	playerMenu.carried = game.ItemStack{Item: game.ItemStone, Count: 8}

	candidate := playerMenu.candidate()

	if applyQuickCraft(candidate, game.GameModeSurvival, 37, 1) {
		t.Fatal("quick craft slot was accepted without a start")
	}

	if !applyQuickCraft(candidate, game.GameModeSurvival, outsideInventorySlot, 0) {
		t.Fatal("quick craft start was rejected")
	}

	if !applyQuickCraft(candidate, game.GameModeSurvival, 36, 1) || !applyQuickCraft(candidate, game.GameModeSurvival, 37, 1) {
		t.Fatal("quick craft slot selection was rejected")
	}

	if len(playerMenu.drag.slots) != 2 || playerMenu.drag.slots[0] != 36 || playerMenu.drag.slots[1] != 37 {
		t.Fatalf("quick craft slots = %v, want [36 37]", playerMenu.drag.slots)
	}

	if !applyQuickCraft(candidate, game.GameModeSurvival, outsideInventorySlot, 2) || candidate.slots[37].Count != 4 || candidate.carried.Count != 4 {
		t.Fatalf("candidate after quick craft = %+v", candidate)
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

	session.activeMenu().stateID = 12
	session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemStone, Count: 1}
	session.Player.Inventory.Main[26] = game.ItemStack{Item: game.ItemDirt, Count: 2}
	session.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemOakLog, Count: 3}
	session.activeMenu().carried = game.ItemStack{Item: game.ItemStone, Count: 4}

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
	if nextMenuStateID(32766) != 32767 || nextMenuStateID(32767) != 0 {
		t.Fatalf("inventory state rollover = %d, %d", nextMenuStateID(32766), nextMenuStateID(32767))
	}
}

func TestWindowZeroUsesGenericMenuState(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	playerMenu := session.activeMenu()

	if playerMenu.windowID != playerInventoryWindowID || session.inventoryMenu != playerMenu || session.containerMenu != playerMenu {
		t.Fatalf("player menu lifecycle = inventory %p, active %p, menu %p", session.inventoryMenu, session.containerMenu, playerMenu)
	}

	playerMenu.stateID = 9
	playerMenu.carried = game.ItemStack{Item: game.ItemStone, Count: 3}
	playerMenu.slots[36].stack.Count = 2

	if session.Player.Inventory.Hotbar[0].Count != 2 {
		t.Fatal("generic player menu slot is not backed by player inventory storage")
	}

	clone := session.Player.Inventory.Clone()
	if clone.Hotbar[0].Count != 2 || playerMenu.stateID != 9 || playerMenu.carried.Count != 3 {
		t.Fatal("menu state was not independent from cloned player storage")
	}
}

func TestGenericMenuSlotRestrictions(t *testing.T) {
	var inventory game.PlayerInventory

	playerMenu := newPlayerInventoryMenu(&inventory)

	playerMenu.carried = game.ItemStack{Item: game.ItemStone, Count: 2}

	candidate := playerMenu.candidate()

	if !applyPickup(candidate, game.GameModeSurvival, 0, 0) || !candidate.slots[0].Empty() || candidate.carried.Count != 2 {
		t.Fatal("result slot accepted a carried stack")
	}

	if !applyPickup(candidate, game.GameModeSurvival, 5, 0) || !candidate.slots[5].Empty() || candidate.carried.Item != game.ItemStone {
		t.Fatal("armor slot accepted an invalid item")
	}

	candidate.carried = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

	if !applyPickup(candidate, game.GameModeSurvival, 5, 0) || candidate.slots[5].Item != game.ItemIronHelmet || !candidate.carried.Empty() {
		t.Fatal("armor slot rejected matching armor")
	}
}

func TestArmorSlotForItemUsesGeneratedEquippableMetadata(t *testing.T) {
	tests := []equipmentSlotTestCase{
		{name: "helmet", item: game.ItemCopperHelmet, slot: 5},
		{name: "chestplate", item: game.ItemNetheriteChestplate, slot: 6},
		{name: "leggings", item: game.ItemChainmailLeggings, slot: 7},
		{name: "boots", item: game.ItemLeatherBoots, slot: 8},
		{name: "carved pumpkin", item: game.ItemCarvedPumpkin, slot: 5},
		{name: "elytra", item: game.ItemElytra, slot: 6},
		{name: "player head", item: game.ItemPlayerHead, slot: 5},
		{name: "creeper head", item: game.ItemCreeperHead, slot: 5},
		{name: "zombie head", item: game.ItemZombieHead, slot: 5},
		{name: "skeleton skull", item: game.ItemSkeletonSkull, slot: 5},
		{name: "wither skeleton skull", item: game.ItemWitherSkeletonSkull, slot: 5},
		{name: "dragon head", item: game.ItemDragonHead, slot: 5},
		{name: "piglin head", item: game.ItemPiglinHead, slot: 5},
		{name: "stone", item: game.ItemStone, slot: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stack := game.ItemStack{Item: test.item, Count: 1}
			slot := armorSlotForItem(stack)

			if slot != test.slot {
				t.Fatalf("equipment slot = %d, want %d", slot, test.slot)
			}
		})
	}
}

func TestQuickMoveEquipsGeneratedHeadwear(t *testing.T) {
	headwear := []game.Item{game.ItemCarvedPumpkin, game.ItemPlayerHead, game.ItemCreeperHead, game.ItemZombieHead, game.ItemSkeletonSkull, game.ItemWitherSkeletonSkull, game.ItemDragonHead, game.ItemPiglinHead}

	for _, item := range headwear {
		var inventory game.PlayerInventory

		inventory.Main[0] = game.ItemStack{Item: item, Count: 1}

		candidate := newPlayerInventoryMenu(&inventory).candidate()
		if !applyQuickMove(candidate, game.GameModeSurvival, 9, 0) || candidate.slots[5].Item != item || !candidate.slots[9].Empty() {
			t.Fatalf("quick move item %d = head %+v source %+v", item, candidate.slots[5], candidate.slots[9])
		}
	}
}

func TestGenericMenuQuickMoveCommitsMultipleBackings(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.activeMenu()

	container := [1]game.ItemStack{{Item: game.ItemStone, Count: 5}}

	combinedMenu := testCombinedMenu(&container[0], &session.Player.Inventory.Hotbar[0])

	session.containerMenu = combinedMenu

	connection.reset()

	err := session.handleContainerClick(protocol.ContainerClick{
		WindowID:    combinedMenu.windowID,
		Slot:        0,
		MouseButton: 0,
		Mode:        clickModeQuickMove,
		ChangedSlots: []protocol.ChangedSlot{
			{Location: 0},
			{Location: 1, Item: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 5})},
		},
	})

	if err != nil {
		t.Fatalf("handle combined menu quick move: %v", err)
	}

	if !container[0].Empty() || session.Player.Inventory.Hotbar[0].Count != 5 || combinedMenu.stateID != 1 {
		t.Fatalf("combined menu commit = container %+v, hotbar %+v, state %d", container[0], session.Player.Inventory.Hotbar[0], combinedMenu.stateID)
	}

	packet := connection.packets(t)[0]

	reader := protocol.NewPacketReader(packet.Data)

	if reader.VarInt() != combinedMenu.windowID || reader.VarInt() != 1 || reader.VarInt() != 2 {
		t.Fatal("combined menu snapshot had the wrong window, state, or slot count")
	}
}

func TestNineByThreeMenuSlotsAndQuickMoveRouting(t *testing.T) {
	var (
		container [27]game.ItemStack
		inventory game.PlayerInventory
	)

	container[0] = game.ItemStack{Item: game.ItemStone, Count: 5}
	inventory.Main[26] = game.ItemStack{Item: game.ItemDirt, Count: 3}
	inventory.Hotbar[8] = game.ItemStack{Item: game.ItemOakLog, Count: 2}

	menu := newGenericContainerMenu(7, 3, container[:], &inventory)

	if len(menu.slots) != 63 || menu.slots[0].stack != &container[0] || menu.slots[26].stack != &container[26] || menu.slots[27].stack != &inventory.Main[0] || menu.slots[53].stack != &inventory.Main[26] || menu.slots[54].stack != &inventory.Hotbar[0] || menu.slots[62].stack != &inventory.Hotbar[8] {
		t.Fatal("nine by three menu slot ordering is incorrect")
	}

	candidate := menu.candidate()
	if !applyQuickMove(candidate, game.GameModeSurvival, 0, 0) || !candidate.slots[0].Empty() || candidate.slots[61].Count != 5 {
		t.Fatalf("container quick move = %+v", candidate)
	}

	candidate = menu.candidate()
	if !applyQuickMove(candidate, game.GameModeSurvival, 53, 0) || !candidate.slots[53].Empty() || candidate.slots[1].Item != game.ItemDirt || candidate.slots[1].Count != 3 {
		t.Fatalf("player quick move = %+v", candidate)
	}
}

func TestGenericContainerMenuLayoutsOneThroughSixRows(t *testing.T) {
	for rows := 1; rows <= 6; rows++ {
		container := make([]game.ItemStack, rows*9)

		inventory := game.PlayerInventory{}

		menu := newGenericContainerMenu(7, rows, container, &inventory)
		if menu == nil {
			t.Fatalf("%d-row menu is nil", rows)
		}

		containerSlots := rows * 9
		if len(menu.slots) != containerSlots+36 || menu.containerSlots != containerSlots {
			t.Fatalf("%d-row menu slots = %d, container slots %d", rows, len(menu.slots), menu.containerSlots)
		}

		if menu.protocolMenuType != int32(rows-1) {
			t.Fatalf("%d-row protocol menu type = %d", rows, menu.protocolMenuType)
		}

		if menu.slots[0].stack != &container[0] || menu.slots[containerSlots-1].stack != &container[containerSlots-1] || menu.slots[containerSlots].stack != &inventory.Main[0] || menu.slots[len(menu.slots)-1].stack != &inventory.Hotbar[8] {
			t.Fatalf("%d-row menu slot ordering is incorrect", rows)
		}
	}

	inventory := game.PlayerInventory{}
	if newGenericContainerMenu(1, 0, nil, &inventory) != nil || newGenericContainerMenu(1, 7, make([]game.ItemStack, 63), &inventory) != nil || newGenericContainerMenu(1, 2, make([]game.ItemStack, 17), &inventory) != nil {
		t.Fatal("invalid generic container layout was accepted")
	}
}

func TestClosingMenuDropsCarriedStackWhenInventoryIsFull(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	fullStack := game.ItemStack{Item: game.ItemDirt, Count: 64}

	for slot := 9; slot <= 44; slot++ {
		*session.Player.Inventory.Slot(slot) = fullStack
	}

	container := make([]game.ItemStack, 9)

	containerMenu := newGenericContainerMenu(1, 1, container, &session.Player.Inventory)

	containerMenu.carried = game.ItemStack{Item: game.ItemStone, Count: 1}

	session.containerMenu = containerMenu

	runtime.closeMenu(session, false)

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 1 {
		t.Fatalf("dropped entities = %d, want 1", len(entities))
	}

	item, valid := entities[0].(*runtimeItemEntity)
	if !valid || !item.Stack.Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) {
		t.Fatalf("dropped entity = %#v", entities[0])
	}
}

func TestNineByThreeMenuOffhandSwapIsHiddenAndPredicted(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.activeMenu()

	var container [27]game.ItemStack

	container[0] = game.ItemStack{Item: game.ItemStone, Count: 2}

	session.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemDirt, Count: 3}

	menu := newGenericContainerMenu(7, 3, container[:], &session.Player.Inventory)
	session.containerMenu = menu

	err := session.handleContainerClick(protocol.ContainerClick{
		WindowID:    menu.windowID,
		Slot:        0,
		MouseButton: 40,
		Mode:        clickModeSwap,
		ChangedSlots: []protocol.ChangedSlot{{
			Location: 0,
			Item:     hashedStack(game.ItemStack{Item: game.ItemDirt, Count: 3}),
		}},
	})

	if err != nil {
		t.Fatalf("handle hidden offhand swap: %v", err)
	}

	if !container[0].Equal(game.ItemStack{Item: game.ItemDirt, Count: 3}) || !session.Player.Inventory.Offhand.Equal(game.ItemStack{Item: game.ItemStone, Count: 2}) || menu.stateID != 1 {
		t.Fatalf("hidden offhand swap = container %+v, offhand %+v, state %d", container[0], session.Player.Inventory.Offhand, menu.stateID)
	}

	packet := connection.packets(t)[0]

	reader := protocol.NewPacketReader(packet.Data)

	if reader.VarInt() != menu.windowID || reader.VarInt() != 1 || reader.VarInt() != 63 {
		t.Fatal("hidden offhand swap exposed the offhand as a wire slot")
	}
}

func TestMenuMappingsDefaultToUnmapped(t *testing.T) {
	var slots [2]game.ItemStack

	slots[0] = game.ItemStack{Item: game.ItemStone, Count: 1}
	slots[1] = game.ItemStack{Item: game.ItemDirt, Count: 1}

	menu := &menu{slots: []menuSlot{{stack: &slots[0]}, {stack: &slots[1]}}}

	candidate := menu.candidate()

	if applySwap(candidate, game.GameModeSurvival, 1, 0) || !candidate.slots[0].Equal(slots[0]) || !candidate.slots[1].Equal(slots[1]) {
		t.Fatal("zero-value hotbar mapping swapped slot zero")
	}

	if menu.exposesPlayerSlots([]int{0}) {
		t.Fatal("zero-value player mapping exposed player slot zero")
	}
}

func TestExternalPlayerMutationSynchronizesActiveCombinedMenu(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	playerMenu := session.activeMenu()

	var container game.ItemStack

	combinedMenu := testCombinedMenu(&container, &session.Player.Inventory.Hotbar[0])

	session.containerMenu = combinedMenu

	connection.reset()

	err := runtime.GiveItem(session, game.ItemStone, 3)
	if err != nil {
		t.Fatalf("give item with combined menu active: %v", err)
	}

	if session.Player.Inventory.Hotbar[0].Count != 3 || combinedMenu.stateID != 1 || playerMenu.stateID != 0 {
		t.Fatalf("external mutation state = hotbar %+v, combined %d, inventory %d", session.Player.Inventory.Hotbar[0], combinedMenu.stateID, playerMenu.stateID)
	}

	packet := connection.packets(t)[0]

	reader := protocol.NewPacketReader(packet.Data)

	if reader.VarInt() != combinedMenu.windowID || reader.VarInt() != 1 || reader.VarInt() != 2 {
		t.Fatal("external mutation did not synchronize the active combined menu")
	}
}

func TestQuickCraftStateDoesNotLeakBetweenMenus(t *testing.T) {
	var (
		firstStorage  game.ItemStack
		secondStorage game.ItemStack
	)

	firstMenu := testCombinedMenu(&firstStorage, &secondStorage)

	firstMenu.carried = game.ItemStack{Item: game.ItemStone, Count: 4}

	firstCandidate := firstMenu.candidate()

	if !applyQuickCraft(firstCandidate, game.GameModeSurvival, outsideInventorySlot, 0) || !firstMenu.drag.active {
		t.Fatal("first menu did not start quick craft")
	}

	secondMenu := testCombinedMenu(&secondStorage, &firstStorage)
	if secondMenu.drag.active || len(secondMenu.drag.slots) != 0 {
		t.Fatal("new menu inherited another menu's quick craft state")
	}
}

func testCombinedMenu(containerSlot, playerHotbarSlot *game.ItemStack) *menu {
	combinedMenu := &menu{
		windowID: 7,
		slots: []menuSlot{
			{stack: containerSlot, limit: 64},
			{stack: playerHotbarSlot, limit: 64, playerSlot: 36, hasPlayerSlot: true},
		},
	}

	combinedMenu.hotbarSlots[0] = 1
	combinedMenu.hasHotbarSlots[0] = true

	combinedMenu.quickMove = func(candidate *menuCandidate, slot int) {
		remaining := candidate.slots[slot].Clone()

		target := 1

		if slot == 1 {
			target = 0
		}

		moveIntoSlots(candidate, &remaining, []int{target})

		candidate.slots[slot] = remaining

		normalizeStack(&candidate.slots[slot])
	}

	return combinedMenu
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

	windowID := reader.VarInt()
	if windowID != playerInventoryWindowID {
		t.Fatalf("inventory window id = %d, want %d", windowID, playerInventoryWindowID)
	}

	actualStateID := reader.VarInt()
	if actualStateID != stateID {
		t.Fatalf("inventory state id = %d, want %d", actualStateID, stateID)
	}

	actualItemCount := reader.VarInt()
	if actualItemCount != itemCount {
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

	added := reader.VarInt()
	removed := reader.VarInt()

	if added != 0 || removed != 0 {
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
