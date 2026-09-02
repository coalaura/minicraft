package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type hoeMaterialTestCase struct {
	name string
	item game.Item
}

type hoeTillingTestCase struct {
	name      string
	block     game.Block
	face      int32
	above     game.Block
	position  game.BlockPosition
	want      game.Block
	wantTill  bool
	wantRoots bool
}

type cropPlacementTestCase struct {
	name  string
	item  game.Item
	block game.Block
}

type cropLootTestCase struct {
	name     string
	block    game.Block
	age      string
	expected map[game.Item]int32
}

func TestHoeMetadataAndTillingReplacements(t *testing.T) {
	hoes := []hoeMaterialTestCase{
		{name: "wooden", item: game.ItemWoodenHoe},
		{name: "copper", item: game.ItemCopperHoe},
		{name: "stone", item: game.ItemStoneHoe},
		{name: "golden", item: game.ItemGoldenHoe},
		{name: "iron", item: game.ItemIronHoe},
		{name: "diamond", item: game.ItemDiamondHoe},
		{name: "netherite", item: game.ItemNetheriteHoe},
	}

	for _, test := range hoes {
		t.Run(test.name, func(t *testing.T) {
			if test.item.OnBlockBehavior() != game.ItemOnBlockBehaviorHoe {
				t.Fatalf("on-block behavior = %d, want hoe", test.item.OnBlockBehavior())
			}

			definition, valid := test.item.Definition()
			if !valid || definition.MaxDurability <= 0 {
				t.Fatalf("hoe definition = %+v, valid %t", definition, valid)
			}
		})
	}

	tests := []hoeTillingTestCase{
		{name: "grass block", block: game.GrassBlock, face: protocol.BlockFaceUp, want: game.Farmland, wantTill: true},
		{name: "dirt", block: game.Dirt, face: protocol.BlockFaceUp, want: game.Farmland, wantTill: true},
		{name: "dirt path", block: game.DirtPath, face: protocol.BlockFaceUp, want: game.Farmland, wantTill: true},
		{name: "coarse dirt", block: game.CoarseDirt, face: protocol.BlockFaceUp, want: game.Dirt, wantTill: true},
		{name: "rooted dirt", block: game.RootedDirt, face: protocol.BlockFaceDown, above: game.Stone, want: game.Dirt, wantTill: true, wantRoots: true},
		{name: "downward face", block: game.Dirt, face: protocol.BlockFaceDown, want: game.Air},
		{name: "blocked above", block: game.Dirt, face: protocol.BlockFaceUp, above: game.Stone, want: game.Farmland},
		{name: "maximum height", block: game.Dirt, face: protocol.BlockFaceUp, position: game.BlockPosition{Y: math.MaxInt32}, want: game.Air},
		{name: "non-tillable", block: game.Stone, face: protocol.BlockFaceUp, want: game.Air},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position := test.position
			above := position

			above.Y++

			blockAt := func(candidate game.BlockPosition) game.Block {
				if candidate == above {
					return test.above
				}

				return game.Air
			}

			interaction := testUseItemOn(position, test.face, protocol.MainHand, 1)

			replacement, tillable, roots := hoeTillingReplacement(blockAt, interaction, test.block)
			if replacement != test.want || tillable != test.wantTill || roots != test.wantRoots {
				t.Fatalf("replacement = %d, tillable %t, roots %t; want %d, %t, %t", replacement, tillable, roots, test.want, test.wantTill, test.wantRoots)
			}
		})
	}
}

func TestHoeUseDamagesSurvivalHandAndPlaysTillSound(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, actorConnection := newBlockMutationTestSession(runtime, commandTestBobUUID, "Farmer", game.GameModeSurvival)
	observer, observerConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e10", "Observer", game.GameModeCreative)

	world.SetBlock(position, game.RootedDirt)

	actor.Player.Position = blockMutationTestPlayerPosition(position)
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemIronHoe, Count: 1}
	actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemDiamondHoe, Count: 1}

	markChunkLoaded(actor, position)
	markChunkLoaded(observer, position)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.OffHand, 1))
	if err != nil {
		t.Fatalf("till rooted dirt: %v", err)
	}

	if world.BlockAt(position) != game.Dirt {
		t.Fatalf("rooted dirt replacement = %d, want dirt", world.BlockAt(position))
	}

	player := actor.snapshotPlayer()
	if player.Inventory.Hotbar[0].Damage() != 0 || player.Inventory.Offhand.Damage() != 1 {
		t.Fatalf("hoe damage = main %d offhand %d, want 0 1", player.Inventory.Hotbar[0].Damage(), player.Inventory.Offhand.Damage())
	}

	if countDroppedItem(runtime, game.ItemHangingRoots) != 1 {
		t.Fatalf("hanging roots drops = %d, want 1", countDroppedItem(runtime, game.ItemHangingRoots))
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundContainerSetContentID, protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID, protocol.ClientboundBlockChangedAckID})
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID, protocol.ClientboundEntityEquipmentID, protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID})
	assertSoundEvent(t, observerConnection.packets(t)[1], game.SoundItemHoeTill)
}

func TestHoeUseCreativeAndUnbreakingDurability(t *testing.T) {
	tests := []hoeMaterialTestCase{
		{name: "creative main hand", item: game.ItemWoodenHoe},
		{name: "creative offhand", item: game.ItemNetheriteHoe},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position := game.BlockPosition{X: int32(index), Y: 70}

			world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

			runtime := NewRuntime(world)

			actor, _ := newBlockMutationTestSession(runtime, commandTestBobUUID, "Farmer", game.GameModeCreative)

			world.SetBlock(position, game.Dirt)

			actor.Player.Position = blockMutationTestPlayerPosition(position)
			actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemWoodenHoe, Count: 1}
			actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemNetheriteHoe, Count: 1}

			var hand int32 = protocol.MainHand

			if index == 1 {
				hand = protocol.OffHand
			}

			markChunkLoaded(actor, position)

			joinTestSession(t, runtime, actor)

			err := actor.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, hand, 1))
			if err != nil {
				t.Fatalf("creative till: %v", err)
			}

			player := actor.snapshotPlayer()
			if player.Inventory.Hotbar[0].Damage() != 0 || player.Inventory.Offhand.Damage() != 0 {
				t.Fatalf("creative hoe damage = main %d offhand %d, want 0 0", player.Inventory.Hotbar[0].Damage(), player.Inventory.Offhand.Damage())
			}
		})
	}

	runtime := NewRuntime(&game.World{})

	session, _ := newBlockMutationTestSession(runtime, commandTestBobUUID, "Farmer", game.GameModeSurvival)

	stack := game.ItemStack{Item: game.ItemIronHoe, Count: 1}

	stack.SetEnchantment(game.EnchantmentUnbreaking, 3)

	session.Player.Inventory.Hotbar[0] = stack

	joinTestSession(t, runtime, session)

	runtime.miningRandomMu.Lock()

	runtime.miningRandom = func(bound int) int {
		if bound != 4 {
			t.Fatalf("unbreaking random bound = %d, want 4", bound)
		}

		return 3
	}

	runtime.miningRandomMu.Unlock()

	before, broke := runtime.damageHeldItem(session, protocol.MainHand, stack, 1)
	if before != nil || broke || session.snapshotPlayer().Inventory.Hotbar[0].Damage() != 0 {
		t.Fatalf("unbreaking-prevented damage = before %v broke %t damage %d", before != nil, broke, session.snapshotPlayer().Inventory.Hotbar[0].Damage())
	}
}

func TestCropPlacementMappingsConsumptionAndSupportLoss(t *testing.T) {
	crops := []cropPlacementTestCase{
		{name: "wheat", item: game.ItemWheatSeeds, block: game.Wheat},
		{name: "carrots", item: game.ItemCarrot, block: game.Carrots},
		{name: "potatoes", item: game.ItemPotato, block: game.Potatoes},
		{name: "beetroots", item: game.ItemBeetrootSeeds, block: game.Beetroots},
	}

	for _, test := range crops {
		t.Run(test.name, func(t *testing.T) {
			support := game.BlockPosition{Y: 69}
			crop := game.BlockPosition{Y: 70}

			world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

			runtime := NewRuntime(world)

			actor, _ := newBlockMutationTestSession(runtime, commandTestBobUUID, "Farmer", game.GameModeSurvival)

			world.SetBlock(support, game.Farmland)

			actor.Player.Position = blockMutationTestPlayerPosition(support)
			actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: test.item, Count: 2}

			markPlacementChunksLoaded(actor, support, crop)

			joinTestSession(t, runtime, actor)

			err := actor.handleUseItemOn(testUseItemOn(support, protocol.BlockFaceUp, protocol.MainHand, 1))
			if err != nil {
				t.Fatalf("place crop: %v", err)
			}

			placed := world.BlockAt(crop)
			if !sameBlockType(placed, test.block) || blockProperty(placed, "age") != "0" {
				t.Fatalf("placed crop = %d age %q, want %d age 0", placed, blockProperty(placed, "age"), test.block)
			}

			if actor.snapshotPlayer().Inventory.Hotbar[0].Count != 1 {
				t.Fatalf("survival crop count = %d, want 1", actor.snapshotPlayer().Inventory.Hotbar[0].Count)
			}

			result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: support, Replacement: game.Dirt}})
			if err != nil || !result.Changed || world.BlockAt(crop) != game.Air {
				t.Fatalf("remove farmland support = %+v, %v; crop = %d", result, err, world.BlockAt(crop))
			}
		})
	}
}

func TestCropPlacementRequiresFarmlandAndDoesNotConsumeCreativeItems(t *testing.T) {
	support := game.BlockPosition{Y: 69}
	crop := game.BlockPosition{Y: 70}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, commandTestBobUUID, "Farmer", game.GameModeCreative)

	world.SetBlock(support, game.Dirt)

	actor.Player.Position = blockMutationTestPlayerPosition(support)
	actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemWheatSeeds, Count: 2}

	markPlacementChunksLoaded(actor, support, crop)

	joinTestSession(t, runtime, actor)

	err := actor.handleUseItemOn(testUseItemOn(support, protocol.BlockFaceUp, protocol.OffHand, 1))
	if err != nil {
		t.Fatalf("reject dirt crop placement: %v", err)
	}

	if world.BlockAt(crop) != game.Air || actor.snapshotPlayer().Inventory.Offhand.Count != 2 {
		t.Fatalf("dirt crop placement = %d, count %d; want air and 2", world.BlockAt(crop), actor.snapshotPlayer().Inventory.Offhand.Count)
	}

	world.SetBlock(support, game.Farmland)

	err = actor.handleUseItemOn(testUseItemOn(support, protocol.BlockFaceUp, protocol.OffHand, 2))
	if err != nil {
		t.Fatalf("place creative crop: %v", err)
	}

	if !sameBlockType(world.BlockAt(crop), game.Wheat) || actor.snapshotPlayer().Inventory.Offhand.Count != 2 {
		t.Fatalf("creative crop = %d, count %d; want wheat and 2", world.BlockAt(crop), actor.snapshotPlayer().Inventory.Offhand.Count)
	}
}

func TestCanonicalCropLootRespectsAge(t *testing.T) {
	tests := []cropLootTestCase{
		{name: "immature wheat", block: game.Wheat, age: "0", expected: map[game.Item]int32{game.ItemWheatSeeds: 1}},
		{name: "mature wheat", block: game.Wheat, age: "7", expected: map[game.Item]int32{game.ItemWheat: 1, game.ItemWheatSeeds: 1}},
		{name: "immature carrots", block: game.Carrots, age: "0", expected: map[game.Item]int32{game.ItemCarrot: 1}},
		{name: "mature carrots", block: game.Carrots, age: "7", expected: map[game.Item]int32{game.ItemCarrot: 2}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			runtime.lootRandomFloat = func() float32 {
				return 1
			}

			crop := withBlockProperties(test.block, game.BlockPropertyValue{Name: "age", Value: test.age})

			record := blockMutationRecord{
				change:      game.BlockChange{Position: game.BlockPosition{Y: 70}, Replacement: game.Air},
				previous:    crop,
				lootContext: blockLootPlayer,
			}

			runtime.commitOrdinaryBlockDrops([]blockMutationRecord{record})

			for item, expected := range test.expected {
				actual := countDroppedItem(runtime, item)

				if actual != expected {
					t.Fatalf("item %d count = %d, want %d", item, actual, expected)
				}
			}
		})
	}
}
