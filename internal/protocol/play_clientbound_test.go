package protocol

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type testPacketEncoder interface {
	Encode(*PacketWriter)
}

type packetIDTest struct {
	actual   int32
	expected int32
}

type clientboundItemStackFixture struct {
	stack    game.ItemStack
	expected []byte
}

func TestMovementPacketIDsProtocol774(t *testing.T) {
	packetIDs := map[string]packetIDTest{
		"container set content":        {actual: ClientboundContainerSetContentID, expected: 0x12},
		"container set data":           {actual: ClientboundContainerSetDataID, expected: 0x13},
		"container set slot":           {actual: ClientboundContainerSetSlotID, expected: 0x14},
		"change difficulty":            {actual: ClientboundChangeDifficultyID, expected: 0x0A},
		"damage event":                 {actual: ClientboundDamageEventID, expected: 0x19},
		"synchronize entity position":  {actual: ClientboundSynchronizeEntityPositionID, expected: 0x23},
		"update entity position":       {actual: ClientboundUpdateEntityPositionID, expected: 0x33},
		"update position and rotation": {actual: ClientboundUpdateEntityPositionRotationID, expected: 0x34},
		"update entity rotation":       {actual: ClientboundUpdateEntityRotationID, expected: 0x36},
		"set head rotation":            {actual: ClientboundSetHeadRotationID, expected: 0x51},
		"set entity motion":            {actual: ClientboundSetEntityMotionID, expected: 0x63},
		"entity equipment":             {actual: ClientboundEntityEquipmentID, expected: 0x64},
		"take item entity":             {actual: ClientboundTakeItemEntityID, expected: 0x7A},
		"update recipes":               {actual: ClientboundUpdateRecipesID, expected: 0x83},
		"combat kill":                  {actual: ClientboundCombatKillID, expected: 0x42},
		"respawn":                      {actual: ClientboundRespawnID, expected: 0x50},
		"set health":                   {actual: ClientboundSetHealthID, expected: 0x66},
	}

	for name, packetID := range packetIDs {
		t.Run(name, func(t *testing.T) {
			if packetID.actual != packetID.expected {
				t.Fatalf("packet id = %#x, want %#x", packetID.actual, packetID.expected)
			}
		})
	}
}

func TestContainerInventoryPacketsEncode(t *testing.T) {
	stack := game.ItemStack{
		Item:  game.ItemStone,
		Count: 2,
		Components: []game.ItemComponent{{
			Type: 1,
			Data: []byte{0x10},
		}},
		RemovedComponents: []int32{8},
	}

	assertPacketEncoding(t, ContainerSetContent{
		WindowID:    0,
		StateID:     300,
		Items:       []game.ItemStack{{}, stack},
		CarriedItem: game.ItemStack{Item: game.ItemDirt, Count: 1},
	}, []byte{
		0x00, 0xAC, 0x02, 0x02,
		0x00,
		0x02, byte(game.ItemStone), 0x01, 0x01, 0x01, 0x10, 0x08,
		0x01, byte(game.ItemDirt), 0x00, 0x00,
	})

	assertPacketEncoding(t, ContainerSetSlot{
		WindowID: 0,
		StateID:  1,
		Slot:     45,
		Item:     stack,
	}, []byte{
		0x00, 0x01, 0x00, 0x2D,
		0x02, byte(game.ItemStone), 0x01, 0x01, 0x01, 0x10, 0x08,
	})

	assertPacketEncoding(t, ContainerSetData{
		ContainerID: 3,
		ID:          2,
		Value:       300,
	}, []byte{0x03, 0x00, 0x02, 0x01, 0x2C})
}

func TestSetHealthEncode(t *testing.T) {
	assertPacketEncoding(t, SetHealth{Health: 20, Food: 20, Saturation: 5}, []byte{
		0x41, 0xA0, 0x00, 0x00,
		0x14,
		0x40, 0xA0, 0x00, 0x00,
	})
}

func TestChangeDifficultyEncode(t *testing.T) {
	assertPacketEncoding(t, ChangeDifficulty{Difficulty: 3, Locked: true}, []byte{0x03, 0x01})
}

func TestDamageEventEncode(t *testing.T) {
	assertPacketEncoding(t, DamageEvent{
		EntityID:          300,
		DamageType:        5,
		CauseEntityID:     300,
		DirectEntityID:    301,
		HasSourcePosition: true,
		SourcePositionX:   1.5,
		SourcePositionY:   -2.25,
		SourcePositionZ:   3,
	}, decodeTestHex(t, "ac0205ac02ad02013ff8000000000000c0020000000000004008000000000000"))
}

func TestCombatKillEncode(t *testing.T) {
	assertPacketEncoding(t, CombatKill{PlayerID: 300, Message: game.LiteralText("bye")}, []byte{
		0xAC, 0x02,
		0x0A, 0x08, 0x00, 0x04, 't', 'e', 'x', 't', 0x00, 0x03, 'b', 'y', 'e', 0x00,
	})
}

func TestRespawnEncode(t *testing.T) {
	assertPacketEncoding(t, Respawn{
		Spawn: SpawnInfo{
			DimensionType:    2,
			Dimension:        "minecraft:overworld",
			Seed:             3,
			GameMode:         1,
			PreviousGameMode: 0xFF,
			Flat:             true,
			PortalCooldown:   300,
			SeaLevel:         63,
		},
		DataToKeep: 3,
	}, []byte{
		0x02, 0x13, 'm', 'i', 'n', 'e', 'c', 'r', 'a', 'f', 't', ':', 'o', 'v', 'e', 'r', 'w', 'o', 'r', 'l', 'd',
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03,
		0x01, 0xFF, 0x00, 0x01, 0x00, 0xAC, 0x02, 0x3F, 0x03,
	})
}

func TestContainerSetSlotEncodesItemStackReferenceFixtures(t *testing.T) {
	fixtures := map[string]clientboundItemStackFixture{
		"plain diamond pickaxe": {
			stack: game.ItemStack{Item: game.ItemDiamondPickaxe, Count: 1},
			expected: []byte{
				0x00, 0x01, 0x00, 0x2D,
				0x01, 0xAA, 0x07, 0x00, 0x00,
			},
		},
		"damaged diamond pickaxe": {
			stack: game.ItemStack{
				Item:  game.ItemDiamondPickaxe,
				Count: 1,
				Components: []game.ItemComponent{
					{Type: game.ItemComponentDamage, Data: []byte{0xAC, 0x02}},
				},
			},
			expected: []byte{
				0x00, 0x01, 0x00, 0x2D,
				0x01, 0xAA, 0x07, 0x01, 0x00, 0x03, 0xAC, 0x02,
			},
		},
		"one enchantment": {
			stack: game.ItemStack{
				Item:  game.ItemDiamondPickaxe,
				Count: 1,
				Components: []game.ItemComponent{
					{Type: game.ItemComponentEnchantments, Data: []byte{0x01, 0x14, 0x05}},
				},
			},
			expected: []byte{
				0x00, 0x01, 0x00, 0x2D,
				0x01, 0xAA, 0x07, 0x01, 0x00, 0x0D, 0x01, 0x14, 0x05,
			},
		},
		"multiple enchantments": {
			stack: game.ItemStack{
				Item:  game.ItemDiamondPickaxe,
				Count: 1,
				Components: []game.ItemComponent{
					{Type: game.ItemComponentEnchantments, Data: []byte{0x02, 0x14, 0x05, 0x17, 0x03}},
				},
			},
			expected: []byte{
				0x00, 0x01, 0x00, 0x2D,
				0x01, 0xAA, 0x07, 0x01, 0x00, 0x0D, 0x02, 0x14, 0x05, 0x17, 0x03,
			},
		},
		"added and removed components": {
			stack: game.ItemStack{
				Item:              game.ItemDiamondPickaxe,
				Count:             1,
				Components:        []game.ItemComponent{{Type: game.ItemComponentDamage, Data: []byte{0xAC, 0x02}}},
				RemovedComponents: []int32{1},
			},
			expected: []byte{
				0x00, 0x01, 0x00, 0x2D,
				0x01, 0xAA, 0x07, 0x01, 0x01, 0x03, 0xAC, 0x02, 0x01,
			},
		},
	}

	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			packet := ContainerSetSlot{WindowID: 0, StateID: 1, Slot: 45, Item: fixture.stack}

			var writer PacketWriter

			packet.Encode(&writer)

			err := writer.Err()
			if err != nil {
				t.Fatalf("encode container slot: %v", err)
			}

			if !bytes.Equal(writer.Buffer.Bytes(), fixture.expected) {
				t.Fatalf("encoded container slot = %x, want %x", writer.Buffer.Bytes(), fixture.expected)
			}
		})
	}
}

func TestContainerOpenClosePacketsEncode(t *testing.T) {
	if ClientboundOpenScreenID != 0x39 {
		t.Fatalf("open screen packet id = %#x, want 0x39", ClientboundOpenScreenID)
	}

	if ClientboundCloseContainerID != 0x11 {
		t.Fatalf("close container packet id = %#x, want 0x11", ClientboundCloseContainerID)
	}

	assertPacketEncoding(t, OpenScreen{
		ContainerID: 300,
		MenuType:    6,
		Title:       game.TextComponent{Text: "Chest"},
	}, []byte{
		0xAC, 0x02, 0x06,
		0x0A, 0x08, 0x00, 0x04, 't', 'e', 'x', 't', 0x00, 0x05, 'C', 'h', 'e', 's', 't', 0x00,
	})

	assertPacketEncoding(t, CloseContainer{ContainerID: 300}, []byte{0xAC, 0x02})
}

func TestUpdateRecipesEncodesRecipePropertySets(t *testing.T) {
	var writer PacketWriter

	UpdateRecipes{PropertySets: []RecipePropertySet{
		{Name: "minecraft:furnace_input", Items: []game.Item{game.ItemPotato, game.ItemRawIron}},
	}}.Encode(&writer)

	err := writer.Err()
	if err != nil {
		t.Fatalf("encode update recipes: %v", err)
	}

	reader := NewPacketReader(writer.Buffer.Bytes())

	if reader.VarInt() != 1 {
		t.Fatal("recipe property set count is not one")
	}

	if reader.String(32767) != "minecraft:furnace_input" {
		t.Fatal("recipe property set name is incorrect")
	}

	if reader.VarInt() != 2 || reader.VarInt() != int32(game.ItemPotato) || reader.VarInt() != int32(game.ItemRawIron) {
		t.Fatal("recipe property set items are incorrect")
	}

	if reader.VarInt() != 0 {
		t.Fatal("stonecutter recipe set is not empty")
	}

	err = reader.Err()
	if err != nil {
		t.Fatalf("decode update recipes: %v", err)
	}
}

func TestChunkPacketProtocol774(t *testing.T) {
	if ClientboundForgetLevelChunkID != 0x25 {
		t.Fatalf("forget level chunk packet id = %#x, want 0x25", ClientboundForgetLevelChunkID)
	}

	forget := ForgetLevelChunk{X: 0x01020304, Z: -2}

	assertPacketEncoding(t, forget, []byte{0xFF, 0xFF, 0xFF, 0xFE, 0x01, 0x02, 0x03, 0x04})
}

func TestBlockUpdateEncode(t *testing.T) {
	update := BlockUpdate{
		Position: game.BlockPosition{X: 1, Y: 2, Z: 3},
		State:    300,
	}

	expected := []byte{0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x30, 0x02, 0xAC, 0x02}
	assertPacketEncoding(t, update, expected)
}

func TestBlockEventEncode(t *testing.T) {
	if ClientboundBlockEventID != 0x07 {
		t.Fatalf("block event packet id = %#x, want 0x07", ClientboundBlockEventID)
	}

	event := BlockEvent{
		Position: game.BlockPosition{X: 1, Y: 2, Z: 3},
		Event:    1,
		Param:    2,
		Block:    468,
	}

	assertPacketEncoding(t, event, []byte{
		0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x30, 0x02,
		0x01,
		0x02,
		0xD4, 0x03,
	})
}

func TestSectionBlocksUpdateEncode(t *testing.T) {
	if ClientboundSectionBlocksUpdateID != 0x52 {
		t.Fatalf("section blocks update packet id = %#x, want 0x52", ClientboundSectionBlocksUpdateID)
	}

	update := SectionBlocksUpdate{
		SectionX: -1,
		SectionY: -2,
		SectionZ: 3,
		Records: []SectionBlockUpdateRecord{
			{X: 1, Y: 2, Z: 3, State: 4},
			{X: 15, Y: 14, Z: 13, State: 300},
		},
	}

	assertPacketEncoding(t, update, []byte{
		0xFF, 0xFF, 0xFC, 0x00, 0x00, 0x3F, 0xFF, 0xFE,
		0x02,
		0xB2, 0x82, 0x01,
		0xDE, 0x9F, 0x4B,
	})
}

func TestBlockChangedAckEncode(t *testing.T) {
	assertPacketEncoding(t, BlockChangedAck{Sequence: 300}, []byte{0xAC, 0x02})
}

func TestSoundPacketProtocol774(t *testing.T) {
	if ClientboundSoundID != 0x73 {
		t.Fatalf("sound packet id = %#x, want 0x73", ClientboundSoundID)
	}

	assertPacketEncoding(t, Sound{
		Event:  SoundEventHolder{RegistryID: 1418},
		Source: SoundSourceBlock,
		X:      -1.25,
		Y:      70.5,
		Z:      3.875,
		Volume: 1,
		Pitch:  0.8,
		Seed:   0x0102030405060708,
	}, []byte{
		0x8B, 0x0B,
		0x04,
		0xFF, 0xFF, 0xFF, 0xF6,
		0x00, 0x00, 0x02, 0x34,
		0x00, 0x00, 0x00, 0x1F,
		0x3F, 0x80, 0x00, 0x00,
		0x3F, 0x4C, 0xCC, 0xCD,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	})

	assertPacketEncoding(t, Sound{
		Event:  SoundEventHolder{Name: "minecraft:block.stone.place"},
		Source: SoundSourceBlock,
		X:      -1.25,
		Y:      70.5,
		Z:      3.875,
		Volume: 1,
		Pitch:  0.8,
		Seed:   0x0102030405060708,
	}, []byte{
		0x00,
		0x1B, 'm', 'i', 'n', 'e', 'c', 'r', 'a', 'f', 't', ':', 'b', 'l', 'o', 'c', 'k', '.', 's', 't', 'o', 'n', 'e', '.', 'p', 'l', 'a', 'c', 'e',
		0x00,
		0x04,
		0xFF, 0xFF, 0xFF, 0xF6,
		0x00, 0x00, 0x02, 0x34,
		0x00, 0x00, 0x00, 0x1F,
		0x3F, 0x80, 0x00, 0x00,
		0x3F, 0x4C, 0xCC, 0xCD,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	})
}

func TestChatAndLevelEventPacketsProtocol774(t *testing.T) {
	if ClientboundSystemChatID != 0x77 || ClientboundLevelEventID != 0x2D || ServerboundChatMessageID != 0x08 || ServerboundChatAckID != 0x05 || ServerboundChatSessionUpdateID != 0x09 || ClientboundPlayerChatID != 0x3F || ClientboundPlayDisconnectID != 0x20 {
		t.Fatalf("chat/event packet ids = serverbound %#x system %#x level %#x", ServerboundChatMessageID, ClientboundSystemChatID, ClientboundLevelEventID)
	}

	event := LevelEvent{
		Event:    LevelEventBlockBreak,
		Position: game.BlockPosition{X: 1, Y: 2, Z: 3},
		Data:     300,
	}

	assertPacketEncoding(t, event, []byte{
		0x00, 0x00, 0x07, 0xD1,
		0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x30, 0x02,
		0x00, 0x00, 0x01, 0x2C,
		0x00,
	})

	assertPacketEncoding(t, SystemChat{Content: game.LiteralText("hello")}, []byte{
		0x0a,
		0x08, 0x00, 0x04, 't', 'e', 'x', 't', 0x00, 0x05, 'h', 'e', 'l', 'l', 'o',
		0x00,
		0x00,
	})
}

func TestCommandPacketsEncodeProtocol774(t *testing.T) {
	if ClientboundCommandSuggestionsID != 0x0F || ClientboundDeclareCommandsID != 0x10 {
		t.Fatalf("command packet ids = %#x, %#x", ClientboundCommandSuggestionsID, ClientboundDeclareCommandsID)
	}

	tree := DeclareCommands{
		Nodes: []CommandNode{
			{Type: CommandNodeRoot, Children: []int32{1}},
			{Type: CommandNodeLiteral, Children: []int32{2}, Name: "give"},
			{
				Type:           CommandNodeArgument,
				Executable:     true,
				Name:           "count",
				Parser:         CommandParserInteger,
				Properties:     CommandIntegerProperties{HasMin: true, HasMax: true, Min: 1, Max: 64},
				SuggestionType: "minecraft:ask_server",
			},
		},
		RootIndex: 0,
	}

	assertPacketEncoding(t, tree, []byte{
		0x03,
		0x00, 0x01, 0x01,
		0x01, 0x01, 0x02, 0x04, 'g', 'i', 'v', 'e',
		0x16, 0x00, 0x05, 'c', 'o', 'u', 'n', 't', 0x03, 0x03, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x40,
		0x14, 'm', 'i', 'n', 'e', 'c', 'r', 'a', 'f', 't', ':', 'a', 's', 'k', '_', 's', 'e', 'r', 'v', 'e', 'r',
		0x00,
	})

	assertPacketEncoding(t, CommandSuggestions{
		TransactionID: 300,
		Start:         1,
		Length:        2,
		Matches: []CommandSuggestion{
			{Text: "give item"},
			{Text: ""},
		},
	}, []byte{0xAC, 0x02, 0x01, 0x02, 0x02, 0x09, 'g', 'i', 'v', 'e', ' ', 'i', 't', 'e', 'm', 0x00, 0x00, 0x00})

	assertPacketEncoding(t, DeclareCommands{
		Nodes: []CommandNode{
			{Type: CommandNodeRoot, Children: []int32{1, 2}},
			{Type: CommandNodeArgument, Name: "selector", Parser: CommandParserResourceSelector, Properties: CommandResourceProperties{Registry: "minecraft:loot_table"}},
			{Type: CommandNodeArgument, Name: "id", Parser: CommandParserUUID},
		},
		RootIndex: 0,
	}, []byte{
		0x03,
		0x00, 0x02, 0x01, 0x02,
		0x02, 0x00, 0x08, 's', 'e', 'l', 'e', 'c', 't', 'o', 'r', 0x30, 0x14, 'm', 'i', 'n', 'e', 'c', 'r', 'a', 'f', 't', ':', 'l', 'o', 'o', 't', '_', 't', 'a', 'b', 'l', 'e',
		0x02, 0x00, 0x02, 'i', 'd', 0x38,
		0x00,
	})
}

func TestEntityEventEncode(t *testing.T) {
	if ClientboundEntityEventID != 0x22 {
		t.Fatalf("entity event packet ID = %#x", ClientboundEntityEventID)
	}

	assertPacketEncoding(t, EntityEvent{EntityID: 1234, Event: 26}, []byte{
		0x00, 0x00, 0x04, 0xD2,
		0x1A,
	})
}

func TestCommandPacketEncodersRejectInvalidValues(t *testing.T) {
	invalidEncoders := map[string]testPacketEncoder{
		"empty tree":                 DeclareCommands{},
		"invalid parser":             DeclareCommands{Nodes: []CommandNode{{Type: CommandNodeRoot}, {Type: CommandNodeArgument, Name: "target", Parser: CommandParserInteger}}, RootIndex: 0},
		"invalid string parser type": DeclareCommands{Nodes: []CommandNode{{Type: CommandNodeRoot}, {Type: CommandNodeArgument, Name: "target", Parser: CommandParserString, Properties: CommandStringProperties{Type: 3}}}, RootIndex: 0},
		"literal suggestions":        DeclareCommands{Nodes: []CommandNode{{Type: CommandNodeRoot}, {Type: CommandNodeLiteral, Name: "help", SuggestionType: "minecraft:ask_server"}}, RootIndex: 0},
		"negative range":             CommandSuggestions{Start: -1},
	}

	for name, encoder := range invalidEncoders {
		t.Run(name, func(t *testing.T) {
			var writer PacketWriter

			encoder.Encode(&writer)

			err := writer.Err()
			if err == nil {
				t.Fatal("invalid command packet encoded")
			}
		})
	}
}

func TestPlayerChatEncode(t *testing.T) {
	chat := PlayerChat{
		GlobalIndex:        1,
		SenderUUID:         "00010203-0405-0607-0809-0a0b0c0d0e0f",
		SenderIndex:        2,
		PlainMessage:       "hello",
		Timestamp:          3,
		Salt:               4,
		PreviousSignatures: []PreviousChatSignature{{ID: 7}},
		FilterType:         2,
		FilterMask:         []int64{5},
		Type:               ChatTypesHolder{ID: 1},
		NetworkName:        "Laura",
	}

	assertPacketEncoding(t, chat, []byte{
		0x01,
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F,
		0x02, 0x00, 0x05, 'h', 'e', 'l', 'l', 'o',
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04,
		0x01, 0x07, 0x00, 0x02, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
		0x01, 0x08, 0x00, 0x05, 'L', 'a', 'u', 'r', 'a', 0x00,
	})
}

func TestPlayerChatTypeIsPresentInConfigurationRegistry(t *testing.T) {
	for _, registry := range ConfigurationRegistries {
		if registry.ID != "minecraft:chat_type" {
			continue
		}

		entryID, ok := registry.EntryID("minecraft:chat")
		if !ok || entryID != 0 {
			t.Fatalf("minecraft:chat entry = %d, %t; want 0, true", entryID, ok)
		}

		return
	}

	t.Fatal("minecraft:chat_type registry is missing")
}

func TestPlayerChatEncodesLiteralPreviousSignature(t *testing.T) {
	chat := PlayerChat{
		SenderUUID: "00010203-0405-0607-0809-0a0b0c0d0e0f",
		PreviousSignatures: []PreviousChatSignature{{
			Signature: [chatMessageSignatureLength]byte{0xAA},
		}},
		Type: ChatTypesHolder{ID: 1},
	}

	chat.PreviousSignatures[0].Signature[255] = 0xDD

	var writer PacketWriter

	chat.Encode(&writer)

	err := writer.Err()
	if err != nil {
		t.Fatalf("encode player chat: %v", err)
	}

	reader := NewPacketReader(writer.Buffer.Bytes())

	reader.VarInt()
	reader.UUID()
	reader.VarInt()
	reader.Bool()
	reader.String(256)
	reader.Long()
	reader.Long()

	if reader.VarInt() != 1 || reader.VarInt() != 0 || reader.Byte() != 0xAA {
		t.Fatal("literal previous signature prefix was not encoded")
	}

	for range chatMessageSignatureLength - 2 {
		reader.Byte()
	}

	if reader.Byte() != 0xDD {
		t.Fatal("literal previous signature suffix was not encoded")
	}
}

func TestPlayerInfoInitializeChatEncode(t *testing.T) {
	update := PlayerInfoUpdate{
		Actions: PlayerInfoActionInitializeChat,
		Players: []PlayerInfo{{
			UUID: "00010203-0405-0607-0809-0a0b0c0d0e0f",
			ChatSession: &ChatSession{
				UUID:                 "10111213-1415-1617-1819-1a1b1c1d1e1f",
				ExpiresAt:            1,
				PublicKey:            []byte{2},
				CertificateSignature: []byte{3},
			},
		}},
	}

	assertPacketEncoding(t, update, []byte{
		0x02, 0x01,
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F,
		0x01,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x01, 0x02, 0x01, 0x03,
	})
}

func TestPlayDisconnectEncode(t *testing.T) {
	assertPacketEncoding(t, PlayDisconnect{Reason: "bye"}, []byte{0x08, 0x00, 0x03, 'b', 'y', 'e'})
}

func TestSystemChatModifiedUTF8Encode(t *testing.T) {
	assertPacketEncoding(t, SystemChat{Content: game.LiteralText("A\x00😀")}, []byte{
		0x0a,
		0x08, 0x00, 0x04, 't', 'e', 'x', 't', 0x00, 0x09,
		'A', 0xC0, 0x80,
		0xED, 0xA0, 0xBD,
		0xED, 0xB8, 0x80,
		0x00,
		0x00,
	})
}

func TestPlayerInfoUpdateEncode(t *testing.T) {
	update := PlayerInfoUpdate{
		Actions: PlayerInfoActionAddPlayer | PlayerInfoActionUpdateGameMode | PlayerInfoActionUpdateListed,
		Players: []PlayerInfo{
			{
				UUID: "00010203-0405-0607-0809-0a0b0c0d0e0f",
				Name: "Laura",
				Properties: []game.ProfileProperty{
					{Name: "textures", Value: "skin", Signature: "signature"},
				},
				GameMode: 1,
				Listed:   true,
			},
		},
	}

	var wr PacketWriter

	update.Encode(&wr)

	err := wr.Err()
	if err != nil {
		t.Fatalf("encode player info update: %v", err)
	}

	expected := []byte{
		0x0D, 0x01,
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F,
		0x05, 'L', 'a', 'u', 'r', 'a',
		0x01,
		0x08, 't', 'e', 'x', 't', 'u', 'r', 'e', 's',
		0x04, 's', 'k', 'i', 'n',
		0x01, 0x09, 's', 'i', 'g', 'n', 'a', 't', 'u', 'r', 'e',
		0x01, 0x01,
	}

	if !bytes.Equal(wr.Buffer.Bytes(), expected) {
		t.Fatalf("encoded player info update = %x, want %x", wr.Buffer.Bytes(), expected)
	}
}

func TestAddEntityEncode(t *testing.T) {
	entity := AddEntity{
		EntityID: 300,
		UUID:     "00010203-0405-0607-0809-0a0b0c0d0e0f",
		Type:     PlayerEntityType,

		Pitch:   4,
		Yaw:     5,
		HeadYaw: 6,
		Data:    7,
	}

	var wr PacketWriter

	entity.Encode(&wr)

	err := wr.Err()
	if err != nil {
		t.Fatalf("encode add entity: %v", err)
	}

	expected := []byte{
		0xAC, 0x02,
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F,
		0x9B, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00,
		0x04, 0x05, 0x06, 0x07,
	}

	if !bytes.Equal(wr.Buffer.Bytes(), expected) {
		t.Fatalf("encoded add entity = %x, want %x", wr.Buffer.Bytes(), expected)
	}
}

func TestEntityMetadataEncode(t *testing.T) {
	if LivingFlagsMetadataIndex != 8 {
		t.Fatalf("living flags metadata index = %d, want 8", LivingFlagsMetadataIndex)
	}

	if LivingFlagUsingItem != 0x01 || LivingFlagUsingOffhand != 0x02 {
		t.Fatalf("living use flags = %#x/%#x, want 0x1/0x2", LivingFlagUsingItem, LivingFlagUsingOffhand)
	}

	metadata := EntityMetadata{
		EntityID: 300,
		Entries: []EntityMetadataEntry{
			{Index: EntityFlagsMetadataIndex, Type: MetadataTypeByte, Value: MetadataByte(EntityFlagSneaking | EntityFlagSprinting)},
			{Index: EntityPoseMetadataIndex, Type: MetadataTypePose, Value: MetadataVarInt(EntityPoseCrouching)},
			{Index: PlayerSkinPartsMetadataIndex, Type: MetadataTypeByte, Value: MetadataByte(0x7F)},
		},
	}

	var wr PacketWriter

	metadata.Encode(&wr)

	err := wr.Err()
	if err != nil {
		t.Fatalf("encode entity metadata: %v", err)
	}

	expected := []byte{
		0xAC, 0x02,
		0x00, 0x00, 0x0A,
		0x06, 0x14, 0x05,
		0x10, 0x00, 0x7F,
		0xFF,
	}

	if !bytes.Equal(wr.Buffer.Bytes(), expected) {
		t.Fatalf("encoded entity metadata = %x, want %x", wr.Buffer.Bytes(), expected)
	}
}

func TestItemEntityMetadataEncode(t *testing.T) {
	if ItemEntityType != 71 {
		t.Fatalf("item entity type = %d, want 71", ItemEntityType)
	}

	if ItemEntityItemMetadataIndex != 8 {
		t.Fatalf("item entity item metadata index = %d, want 8", ItemEntityItemMetadataIndex)
	}

	metadata := EntityMetadata{
		EntityID: 300,
		Entries: []EntityMetadataEntry{{
			Index: ItemEntityItemMetadataIndex,
			Type:  MetadataTypeItemStack,
			Value: MetadataItemStack{Stack: game.ItemStack{Item: game.ItemStone, Count: 2}},
		}},
	}

	assertPacketEncoding(t, metadata, []byte{
		0xAC, 0x02,
		0x08, 0x07,
		0x02, byte(game.ItemStone), 0x00, 0x00,
		0xFF,
	})
}

func TestEntityAnimationEncode(t *testing.T) {
	animation := EntityAnimation{EntityID: 300, Animation: EntityAnimationSwingOffHand}

	assertPacketEncoding(t, animation, []byte{0xAC, 0x02, 0x03})
}

func TestSetEntityMotionEncode(t *testing.T) {
	motion := SetEntityMotion{
		EntityID:  300,
		VelocityX: 1,
		VelocityY: -2,
		VelocityZ: 3,
	}

	assertPacketEncoding(t, motion, []byte{0xAC, 0x02, 0xA3, 0xAA, 0xFF, 0xFC, 0x55, 0x56})
}

func TestTakeItemEntityEncode(t *testing.T) {
	pickup := TakeItemEntity{ItemEntityID: 300, PlayerEntityID: 301, Amount: 2}

	assertPacketEncoding(t, pickup, []byte{0xAC, 0x02, 0xAD, 0x02, 0x02})
}

func TestEntityEquipmentEncode(t *testing.T) {
	equipment := EntityEquipment{
		EntityID: 300,
		Equipment: []EquipmentEntry{
			{Slot: EquipmentSlotMainHand, Item: game.ItemStack{Item: game.ItemStone, Count: 64}},
			{Slot: EquipmentSlotOffHand, Item: game.ItemStack{Item: game.ItemDirt, Count: 32}},
		},
	}

	assertPacketEncoding(t, equipment, []byte{
		0xAC, 0x02,
		0x80, 0x40, 0x01, 0x00, 0x00,
		0x01, 0x20, 0x1C, 0x00, 0x00,
	})
}

func TestEntityEquipmentEncodeEmptyHand(t *testing.T) {
	equipment := EntityEquipment{
		EntityID: 1,
		Equipment: []EquipmentEntry{
			{Slot: EquipmentSlotMainHand},
		},
	}

	assertPacketEncoding(t, equipment, []byte{0x01, 0x00, 0x00})
}

func TestSynchronizeEntityPositionEncode(t *testing.T) {
	position := SynchronizeEntityPosition{
		EntityID:  300,
		X:         1.5,
		Y:         -2.25,
		Z:         3,
		VelocityX: 0.25,
		VelocityY: -0.5,
		VelocityZ: 1,
		Yaw:       90,
		Pitch:     -45,
		OnGround:  true,
	}

	expected := decodeTestHex(t, "ac023ff8000000000000c00200000000000040080000000000003fd0000000000000bfe00000000000003ff000000000000042b40000c234000001")

	assertPacketEncoding(t, position, expected)
}

func TestUpdateEntityPositionEncode(t *testing.T) {
	position := UpdateEntityPosition{
		EntityID: 300,
		DeltaX:   1,
		DeltaY:   -2,
		DeltaZ:   32767,
		OnGround: true,
	}

	assertPacketEncoding(t, position, []byte{0xAC, 0x02, 0x00, 0x01, 0xFF, 0xFE, 0x7F, 0xFF, 0x01})
}

func TestUpdateEntityPositionRotationEncode(t *testing.T) {
	movement := UpdateEntityPositionRotation{
		EntityID: 300,
		DeltaX:   -32768,
		DeltaY:   0,
		DeltaZ:   32767,
		Yaw:      0xFE,
		Pitch:    0x80,
		OnGround: false,
	}

	assertPacketEncoding(t, movement, []byte{0xAC, 0x02, 0x80, 0x00, 0x00, 0x00, 0x7F, 0xFF, 0xFE, 0x80, 0x00})
}

func TestUpdateEntityRotationEncode(t *testing.T) {
	rotation := UpdateEntityRotation{
		EntityID: 300,
		Yaw:      0xFF,
		Pitch:    0x7F,
		OnGround: true,
	}

	assertPacketEncoding(t, rotation, []byte{0xAC, 0x02, 0xFF, 0x7F, 0x01})
}

func TestSetHeadRotationEncode(t *testing.T) {
	head := SetHeadRotation{EntityID: 300, HeadYaw: 0x80}

	assertPacketEncoding(t, head, []byte{0xAC, 0x02, 0x80})
}

func TestRemoveEntitiesEncode(t *testing.T) {
	entities := RemoveEntities{EntityIDs: []int32{1, 300}}

	var wr PacketWriter

	entities.Encode(&wr)

	err := wr.Err()
	if err != nil {
		t.Fatalf("encode remove entities: %v", err)
	}

	expected := []byte{0x02, 0x01, 0xAC, 0x02}

	if !bytes.Equal(wr.Buffer.Bytes(), expected) {
		t.Fatalf("encoded remove entities = %x, want %x", wr.Buffer.Bytes(), expected)
	}
}

func TestPlayerInfoRemoveEncode(t *testing.T) {
	remove := PlayerInfoRemove{
		UUIDs: []string{"00010203-0405-0607-0809-0a0b0c0d0e0f"},
	}

	var wr PacketWriter

	remove.Encode(&wr)

	err := wr.Err()
	if err != nil {
		t.Fatalf("encode player info remove: %v", err)
	}

	expected := []byte{
		0x01,
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F,
	}

	if !bytes.Equal(wr.Buffer.Bytes(), expected) {
		t.Fatalf("encoded player info remove = %x, want %x", wr.Buffer.Bytes(), expected)
	}
}

func assertPacketEncoding(t *testing.T, encoder testPacketEncoder, expected []byte) {
	t.Helper()

	var wr PacketWriter

	encoder.Encode(&wr)

	err := wr.Err()
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}

	if !bytes.Equal(wr.Buffer.Bytes(), expected) {
		t.Fatalf("encoded packet = %x, want %x", wr.Buffer.Bytes(), expected)
	}
}

func decodeTestHex(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("decode test hex: %v", err)
	}

	return decoded
}
