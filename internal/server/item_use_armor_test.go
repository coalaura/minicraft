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

func TestUseItemEquipsWearablesFromEitherHand(t *testing.T) {
	tests := []wearableUseTestCase{
		{name: "helmet", item: game.ItemDiamondHelmet, hand: protocol.MainHand, armorSlot: 0},
		{name: "chestplate", item: game.ItemIronChestplate, hand: protocol.MainHand, armorSlot: 1},
		{name: "leggings", item: game.ItemCopperLeggings, hand: protocol.MainHand, armorSlot: 2},
		{name: "boots", item: game.ItemLeatherBoots, hand: protocol.OffHand, armorSlot: 3},
		{name: "carved pumpkin", item: game.ItemCarvedPumpkin, hand: protocol.MainHand, armorSlot: 0},
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

func TestUseItemStackedAndCreativeWearableSemantics(t *testing.T) {
	t.Run("stacked wearable returns old armor to inventory", func(t *testing.T) {
		world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

		runtime := NewRuntime(world)

		actor, _ := newBlockMutationTestSession(runtime, commandTestBobUUID, "Actor", game.GameModeSurvival)

		actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemCarvedPumpkin, Count: 3}
		actor.Player.Inventory.Armor[0] = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

		joinTestSession(t, runtime, actor)

		err := actor.handleUseItem(protocol.UseItem{Hand: protocol.OffHand, Sequence: 73})
		if err != nil {
			t.Fatalf("equip stacked pumpkin: %v", err)
		}

		player := actor.snapshotPlayer()

		if player.Inventory.Offhand.Item != game.ItemCarvedPumpkin || player.Inventory.Offhand.Count != 2 {
			t.Fatalf("offhand stack = %+v, want two carved pumpkins", player.Inventory.Offhand)
		}

		if player.Inventory.Armor[0].Item != game.ItemCarvedPumpkin || player.Inventory.Armor[0].Count != 1 {
			t.Fatalf("equipped stack = %+v, want one carved pumpkin", player.Inventory.Armor[0])
		}

		if player.Inventory.Hotbar[0].Item != game.ItemIronHelmet || player.Inventory.Hotbar[0].Count != 1 {
			t.Fatalf("returned armor = %+v, want iron helmet", player.Inventory.Hotbar[0])
		}
	})

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
