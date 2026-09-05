package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type wearableUseTestCase struct {
	name      string
	item      game.Item
	hand      int32
	armorSlot int
}

func TestUseItemEquipsSwappableWearablesFromEitherHand(t *testing.T) {
	tests := []wearableUseTestCase{
		{name: "helmet", item: game.ItemDiamondHelmet, hand: protocol.MainHand, armorSlot: 0},
		{name: "chestplate", item: game.ItemIronChestplate, hand: protocol.MainHand, armorSlot: 1},
		{name: "leggings", item: game.ItemCopperLeggings, hand: protocol.MainHand, armorSlot: 2},
		{name: "boots", item: game.ItemLeatherBoots, hand: protocol.OffHand, armorSlot: 3},
		{name: "elytra", item: game.ItemElytra, hand: protocol.OffHand, armorSlot: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

			runtime := NewRuntime(world)

			actor, connection := newBlockMutationTestSession(runtime, commandTestBobUUID, "Actor", game.GameModeSurvival)

			if test.hand == protocol.MainHand {
				actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: test.item, Count: 1}
			} else {
				actor.Player.Inventory.Offhand = game.ItemStack{Item: test.item, Count: 1}
			}

			joinTestSession(t, runtime, actor)

			connection.reset()

			err := actor.handleUseItem(protocol.UseItem{Hand: test.hand, Sequence: 71})
			if err != nil {
				t.Fatalf("use wearable: %v", err)
			}

			player := actor.snapshotPlayer()
			equipped := player.Inventory.Armor[test.armorSlot]

			if equipped.Item != test.item || equipped.Count != 1 {
				t.Fatalf("equipped stack = %+v, want one %v", equipped, test.item)
			}

			held, valid := heldItemFromPlayer(player, test.hand)
			if !valid || !held.Empty() {
				t.Fatalf("held stack after equip = %+v, valid %t; want empty", held, valid)
			}

			assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID, protocol.ClientboundBlockChangedAckID})
		})
	}
}

func TestUseItemDoesNotQuickEquipUnswappableHeadwear(t *testing.T) {
	items := []game.Item{game.ItemCarvedPumpkin, game.ItemPlayerHead, game.ItemCreeperHead, game.ItemZombieHead, game.ItemSkeletonSkull, game.ItemWitherSkeletonSkull, game.ItemDragonHead, game.ItemPiglinHead}

	for _, item := range items {
		world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

		runtime := NewRuntime(world)

		actor, connection := newBlockMutationTestSession(runtime, commandTestBobUUID, "Actor", game.GameModeSurvival)

		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: item, Count: 1}

		joinTestSession(t, runtime, actor)

		connection.reset()

		err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 75})
		if err != nil {
			t.Fatalf("use unswappable item %d: %v", item, err)
		}

		player := actor.snapshotPlayer()
		if player.Inventory.Hotbar[0].Item != item || !player.Inventory.Armor[0].Empty() {
			t.Fatalf("unswappable item %d changed inventory = %+v", item, player.Inventory)
		}

		assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundBlockChangedAckID})
	}
}

func TestUseItemSwapsOccupiedArmorAndSynchronizesEquipment(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, actorConnection := newBlockMutationTestSession(runtime, commandTestBobUUID, "Actor", game.GameModeSurvival)
	observer, observerConnection := newBlockMutationTestSession(runtime, commandTestAliceUUID, "Observer", game.GameModeSurvival)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemDiamondHelmet, Count: 1}
	actor.Player.Inventory.Armor[0] = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 72})
	if err != nil {
		t.Fatalf("swap helmet: %v", err)
	}

	player := actor.snapshotPlayer()
	held := player.Inventory.Hotbar[0]
	equipped := player.Inventory.Armor[0]

	if held.Item != game.ItemIronHelmet || held.Count != 1 {
		t.Fatalf("held stack = %+v, want iron helmet", held)
	}

	if equipped.Item != game.ItemDiamondHelmet || equipped.Count != 1 {
		t.Fatalf("equipped stack = %+v, want diamond helmet", equipped)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID, protocol.ClientboundBlockChangedAckID})
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityEquipmentID})

	packet := observerConnection.packets(t)[0]
	reader := protocol.NewPacketReader(packet.Data)

	entityID := reader.VarInt()
	if entityID != player.EntityID {
		t.Fatalf("equipment entity id = %d, want %d", entityID, player.EntityID)
	}

	mainHandSlot := reader.Byte()
	if mainHandSlot != protocol.EquipmentSlotMainHand|0x80 {
		t.Fatalf("first equipment slot = %d, want continued main hand", mainHandSlot)
	}

	mainHand := readSimpleItemStack(t, reader)
	if mainHand.Item != game.ItemIronHelmet || mainHand.Count != 1 {
		t.Fatalf("synchronized main hand = %+v, want iron helmet", mainHand)
	}

	headSlot := reader.Byte()
	if headSlot != protocol.EquipmentSlotHead {
		t.Fatalf("second equipment slot = %d, want head", headSlot)
	}

	head := readSimpleItemStack(t, reader)
	if head.Item != game.ItemDiamondHelmet || head.Count != 1 {
		t.Fatalf("synchronized head = %+v, want diamond helmet", head)
	}

	err = reader.Err()
	if err != nil {
		t.Fatalf("decode equipment update: %v", err)
	}
}

func TestUseItemCreativeWearableSemantics(t *testing.T) {
	t.Run("creative copies wearable", func(t *testing.T) {
		world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

		runtime := NewRuntime(world)

		actor, _ := newBlockMutationTestSession(runtime, commandTestBobUUID, "Actor", game.GameModeCreative)

		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemNetheriteBoots, Count: 1}

		joinTestSession(t, runtime, actor)

		err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 74})
		if err != nil {
			t.Fatalf("equip creative boots: %v", err)
		}

		player := actor.snapshotPlayer()

		if player.Inventory.Hotbar[0].Item != game.ItemNetheriteBoots || player.Inventory.Hotbar[0].Count != 1 {
			t.Fatalf("creative held stack = %+v, want unchanged boots", player.Inventory.Hotbar[0])
		}

		if player.Inventory.Armor[3].Item != game.ItemNetheriteBoots || player.Inventory.Armor[3].Count != 1 {
			t.Fatalf("creative equipped stack = %+v, want copied boots", player.Inventory.Armor[3])
		}
	})
}

func TestUseItemCannotReplaceBoundEquipmentOutsideCreative(t *testing.T) {
	modes := []game.GameMode{game.GameModeSurvival, game.GameModeAdventure}

	for _, mode := range modes {
		world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

		runtime := NewRuntime(world)

		actor, connection := newBlockMutationTestSession(runtime, commandTestBobUUID, "Actor", mode)

		bound := game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

		bound.SetEnchantment(game.EnchantmentBindingCurse, 1)

		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemDiamondHelmet, Count: 1}
		actor.Player.Inventory.Armor[0] = bound

		joinTestSession(t, runtime, actor)

		connection.reset()

		err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 76})
		if err != nil {
			t.Fatalf("replace bound helmet in mode %d: %v", mode, err)
		}

		player := actor.snapshotPlayer()
		if player.Inventory.Hotbar[0].Item != game.ItemDiamondHelmet || !player.Inventory.Armor[0].Equal(bound) {
			t.Fatalf("mode %d replaced bound equipment = %+v", mode, player.Inventory)
		}

		assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundBlockChangedAckID})
	}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, commandTestBobUUID, "Actor", game.GameModeCreative)

	bound := game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

	bound.SetEnchantment(game.EnchantmentBindingCurse, 1)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemDiamondHelmet, Count: 1}
	actor.Player.Inventory.Armor[0] = bound

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 77})
	if err != nil {
		t.Fatalf("creative replace bound helmet: %v", err)
	}

	player := actor.snapshotPlayer()
	if player.Inventory.Hotbar[0].Item != game.ItemDiamondHelmet || player.Inventory.Armor[0].Item != game.ItemDiamondHelmet {
		t.Fatalf("creative bound equipment replacement = %+v", player.Inventory)
	}
}

func TestUseItemBroadcastsGeneratedEquipSound(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, actorConnection := newBlockMutationTestSession(runtime, commandTestBobUUID, "Actor", game.GameModeSurvival)
	observer, observerConnection := newBlockMutationTestSession(runtime, commandTestAliceUUID, "Observer", game.GameModeSurvival)

	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemDiamondHelmet, Count: 1}

	position := toBlockPosition(actor.Player.Position)

	markChunkLoaded(actor, position)
	markChunkLoaded(observer, position)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 78})
	if err != nil {
		t.Fatalf("equip helmet: %v", err)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID, protocol.ClientboundSoundID, protocol.ClientboundBlockChangedAckID})
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityEquipmentID, protocol.ClientboundSoundID})
	assertPlayerEquipSound(t, actorConnection.packets(t)[1], game.SoundEvent("minecraft:item.armor.equip_diamond"))
	assertPlayerEquipSound(t, observerConnection.packets(t)[1], game.SoundEvent("minecraft:item.armor.equip_diamond"))
}

func assertPlayerEquipSound(t *testing.T, packet protocol.Packet, event game.SoundEvent) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)
	if reader.VarInt() != 0 {
		t.Fatal("equip sound did not use a direct holder")
	}

	actualEvent := reader.String(32767)

	if reader.Bool() {
		reader.Float()
	}

	source := reader.VarInt()

	reader.Int()
	reader.Int()
	reader.Int()
	reader.Float()
	reader.Float()
	reader.Long()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode equip sound: %v", err)
	}

	if actualEvent != string(event) || source != protocol.SoundSourcePlayer {
		t.Fatalf("equip sound = event %q source %d, want event %q source %d", actualEvent, source, event, protocol.SoundSourcePlayer)
	}
}
