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
		Slot: creativeOffhandSlot,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemDirt), ItemCount: 32},
	})

	player := session.snapshotPlayer()
	if player.Hotbar[4] != (game.ItemStack{Item: game.ItemStone, Count: 64}) {
		t.Fatalf("hotbar stack = %+v", player.Hotbar[4])
	}

	if player.Offhand != (game.ItemStack{Item: game.ItemDirt, Count: 32}) {
		t.Fatalf("offhand stack = %+v", player.Offhand)
	}

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{Slot: 40})

	if !session.snapshotPlayer().Hotbar[4].Empty() {
		t.Fatalf("cleared hotbar stack = %+v", session.snapshotPlayer().Hotbar[4])
	}
}

func TestCreativeSlotUpdatesRejectInvalidOrNonCreativeChanges(t *testing.T) {
	session, _ := newMovementTestSession(NewRuntime(&game.World{}), "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.GameMode = game.GameModeSurvival

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 36,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemStone), ItemCount: 1},
	})

	if !session.snapshotPlayer().Hotbar[0].Empty() {
		t.Fatal("survival player changed creative slot")
	}

	session.Player.GameMode = game.GameModeCreative

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 36,
		Item: protocol.UntrustedSlot{ItemID: int32(game.MaxItemID) + 1, ItemCount: 1},
	})

	if !session.snapshotPlayer().Hotbar[0].Empty() {
		t.Fatal("invalid item changed creative slot")
	}

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: 36,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemStone), ItemCount: 65},
	})

	if !session.snapshotPlayer().Hotbar[0].Empty() {
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
	actor.Player.Hotbar[0] = game.ItemStack{Item: game.ItemOakLog, Count: 2}

	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, actor)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionDropItem, Sequence: 201})
	if err != nil {
		t.Fatalf("drop held item: %v", err)
	}

	if stack := actor.snapshotPlayer().Hotbar[0]; stack != (game.ItemStack{Item: game.ItemOakLog, Count: 1}) {
		t.Fatalf("stack after dropping one = %+v", stack)
	}

	assertEquipmentUpdate(t, observerConnection.packets(t)[0], actor.Player.EntityID, protocol.EquipmentSlotMainHand, game.ItemOakLog, 1)
	assertBlockChangedAck(t, actorConnection.packets(t)[0], 201)

	actorConnection.reset()
	observerConnection.reset()

	err = actor.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionDropItem, Sequence: 202})
	if err != nil {
		t.Fatalf("drop last held item: %v", err)
	}

	if stack := actor.snapshotPlayer().Hotbar[0]; !stack.Empty() {
		t.Fatalf("stack after dropping last item = %+v", stack)
	}

	assertEmptyEquipmentUpdate(t, observerConnection.packets(t)[0], actor.Player.EntityID, protocol.EquipmentSlotMainHand)
	assertBlockChangedAck(t, actorConnection.packets(t)[0], 202)

	actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})
	actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 0})

	if stack := actor.snapshotPlayer().Hotbar[0]; !stack.Empty() {
		t.Fatalf("switching restored dropped stack = %+v", stack)
	}
}

func TestDroppingAllHeldItemsClearsCreativeStack(t *testing.T) {
	runtime := NewRuntime(&game.World{})
	actor, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")
	actor.Player.GameMode = game.GameModeCreative
	actor.Player.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 64}

	actor.handleDropHeldItem(true)

	if stack := actor.snapshotPlayer().Hotbar[0]; !stack.Empty() {
		t.Fatalf("stack after dropping all = %+v", stack)
	}
}

func TestJoiningPlayerReceivesVisibleEquipment(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Builder")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	actor.Player.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 64}
	actor.Player.Offhand = game.ItemStack{Item: game.ItemDirt, Count: 32}

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
