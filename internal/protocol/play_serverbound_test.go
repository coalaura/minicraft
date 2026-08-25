package protocol

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestDecodeClientInformation(t *testing.T) {
	data := []byte{
		0x05, 'e', 'n', '_', 'u', 's',
		0xF4,
		0x02,
		0x01,
		0x7F,
		0x01,
		0x00,
		0x01,
		0x02,
	}

	information, err := DecodeClientInformation(data)
	if err != nil {
		t.Fatalf("decode client information: %v", err)
	}

	if information.Locale != "en_us" {
		t.Fatalf("locale = %q, want en_us", information.Locale)
	}

	if information.ViewDistance != -12 {
		t.Fatalf("view distance = %d, want -12", information.ViewDistance)
	}

	if information.ChatMode != 2 {
		t.Fatalf("chat mode = %d, want 2", information.ChatMode)
	}

	if !information.ChatColors {
		t.Fatal("chat colors = false, want true")
	}

	if information.SkinParts != 0x7F {
		t.Fatalf("skin parts = 0x%02X, want 0x7F", information.SkinParts)
	}

	if information.MainHand != 1 {
		t.Fatalf("main hand = %d, want 1", information.MainHand)
	}

	if information.TextFiltering {
		t.Fatal("text filtering = true, want false")
	}

	if !information.ServerListing {
		t.Fatal("server listing = false, want true")
	}

	if information.ParticleStatus != 2 {
		t.Fatalf("particle status = %d, want 2", information.ParticleStatus)
	}
}

func TestDecodeClientInformationRejectsTruncatedPacket(t *testing.T) {
	_, err := DecodeClientInformation([]byte{0x05, 'e', 'n'})
	if err == nil {
		t.Fatal("decode truncated client information succeeded")
	}
}

func TestDecodeChatMessageFullyConsumesProtocol774Packet(t *testing.T) {
	data := testChatMessageData("Hello, world!", true, 300)

	message, err := DecodeChatMessage(data)
	if err != nil {
		t.Fatalf("decode chat message: %v", err)
	}

	if message.Message != "Hello, world!" || message.Timestamp != 1234 || message.Salt != 5678 || !message.HasSignature || message.Offset != 300 {
		t.Fatalf("chat message = %+v", message)
	}

	if message.Acknowledged != [3]byte{0xAA, 0xBB, 0xCC} || message.Checksum != 0xDD {
		t.Fatalf("chat acknowledgement = %x checksum = %x", message.Acknowledged, message.Checksum)
	}

	if message.Signature[0] != 0 || message.Signature[255] != 255 {
		t.Fatalf("chat signature = %x...%x", message.Signature[0], message.Signature[255])
	}
}

func TestDecodeChatAck(t *testing.T) {
	acknowledgement, err := DecodeChatAck([]byte{0xAC, 0x02})
	if err != nil {
		t.Fatalf("decode chat acknowledgement: %v", err)
	}

	if acknowledgement.MessageCount != 300 {
		t.Fatalf("chat acknowledgement = %+v", acknowledgement)
	}

	for _, data := range [][]byte{{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, {0x00, 0x00}} {
		_, err := DecodeChatAck(data)
		if err == nil {
			t.Fatalf("invalid chat acknowledgement %x decoded", data)
		}
	}
}

func TestDecodeChatSessionUpdate(t *testing.T) {
	var writer PacketWriter

	writer.UUID("00010203-0405-0607-0809-0a0b0c0d0e0f")
	writer.Long(1234)
	writer.Bytes([]byte{1, 2})
	writer.Bytes([]byte{3, 4, 5})

	update, err := DecodeChatSessionUpdate(writer.Buffer.Bytes())
	if err != nil {
		t.Fatalf("decode chat session update: %v", err)
	}

	if update.SessionUUID != "00010203-0405-0607-0809-0a0b0c0d0e0f" || update.ExpiresAt != 1234 || string(update.PublicKey) != "\x01\x02" || string(update.CertificateSignature) != "\x03\x04\x05" {
		t.Fatalf("chat session update = %+v", update)
	}

	writer.Reset()
	writer.UUID("00010203-0405-0607-0809-0a0b0c0d0e0f")
	writer.Long(0)
	writer.VarInt(maxChatSessionBytes + 1)

	_, err = DecodeChatSessionUpdate(writer.Buffer.Bytes())
	if err == nil {
		t.Fatal("oversized chat public key decoded")
	}

	writer.Reset()
	writer.UUID("00010203-0405-0607-0809-0a0b0c0d0e0f")
	writer.Long(0)
	writer.VarInt(-1)

	_, err = DecodeChatSessionUpdate(writer.Buffer.Bytes())
	if err == nil {
		t.Fatal("negative chat public key length decoded")
	}

	writer.Reset()
	writer.UUID("00010203-0405-0607-0809-0a0b0c0d0e0f")
	writer.Long(0)
	writer.Bytes(nil)
	writer.Bytes(nil)
	writer.Byte(0)

	_, err = DecodeChatSessionUpdate(writer.Buffer.Bytes())
	if err == nil {
		t.Fatal("chat session update with trailing data decoded")
	}
}

func TestDecodeChatMessageRejectsMalformedAndInvalidMessages(t *testing.T) {
	tests := map[string][]byte{
		"empty":           testChatMessageData("", false, 0),
		"too long":        testChatMessageData(string(make([]byte, 257)), false, 0),
		"control":         testChatMessageData("hello\nworld", false, 0),
		"formatting code": testChatMessageData("hello \u00a7cworld", false, 0),
		"negative offset": testChatMessageData("hello", false, -1),
		"trailing data":   append(testChatMessageData("hello", false, 0), 0x00),
		"truncated":       testChatMessageData("hello", true, 0)[:20],
	}

	invalidUTF8 := testChatMessageData("hello", false, 0)

	invalidUTF8[1] = 0xFF

	tests["invalid utf-8"] = invalidUTF8

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeChatMessage(data)
			if err == nil {
				t.Fatal("invalid chat message decoded without an error")
			}
		})
	}
}

func TestDecodeMovementFlags(t *testing.T) {
	flags := MovementFlagHorizontalCollision

	var positionWriter PacketWriter

	positionWriter.Double(1)
	positionWriter.Double(2)
	positionWriter.Double(3)
	positionWriter.Byte(byte(flags))

	position, err := DecodeMovePlayerPosition(positionWriter.Buffer.Bytes())
	if err != nil {
		t.Fatalf("decode position: %v", err)
	}

	assertMovementFlags(t, position.Flags, false, true)

	var positionRotationWriter PacketWriter

	positionRotationWriter.Double(1)
	positionRotationWriter.Double(2)
	positionRotationWriter.Double(3)
	positionRotationWriter.Float(4)
	positionRotationWriter.Float(5)
	positionRotationWriter.Byte(byte(MovementFlagOnGround | MovementFlagHorizontalCollision))

	positionRotation, err := DecodeMovePlayerPositionRotation(positionRotationWriter.Buffer.Bytes())
	if err != nil {
		t.Fatalf("decode position rotation: %v", err)
	}

	assertMovementFlags(t, positionRotation.Flags, true, true)

	var rotationWriter PacketWriter

	rotationWriter.Float(4)
	rotationWriter.Float(5)
	rotationWriter.Byte(byte(MovementFlagOnGround))

	rotation, err := DecodeMovePlayerRotation(rotationWriter.Buffer.Bytes())
	if err != nil {
		t.Fatalf("decode rotation: %v", err)
	}

	assertMovementFlags(t, rotation.Flags, true, false)

	status, err := DecodeMovePlayerStatus([]byte{byte(MovementFlagHorizontalCollision)})
	if err != nil {
		t.Fatalf("decode status: %v", err)
	}

	assertMovementFlags(t, status.Flags, false, true)
}

func TestPlayerActionPacketIDsProtocol774(t *testing.T) {
	packetIDs := map[string]packetIDTest{
		"block action":   {actual: ServerboundPlayerActionID, expected: 0x28},
		"block ack":      {actual: ClientboundBlockChangedAckID, expected: 0x04},
		"block update":   {actual: ClientboundBlockUpdateID, expected: 0x08},
		"player command": {actual: ServerboundPlayerCommandID, expected: 0x29},
		"player input":   {actual: ServerboundPlayerInputID, expected: 0x2A},
		"player loaded":  {actual: ServerboundPlayerLoadedID, expected: 0x2B},
		"held item":      {actual: ServerboundSetHeldItemID, expected: 0x34},
		"creative slot":  {actual: ServerboundSetCreativeModeSlotID, expected: 0x37},
		"swing arm":      {actual: ServerboundSwingArmID, expected: 0x3C},
		"use item on":    {actual: ServerboundUseItemOnID, expected: 0x3F},
		"animation":      {actual: ClientboundEntityAnimationID, expected: 0x02},
	}

	for name, packetID := range packetIDs {
		t.Run(name, func(t *testing.T) {
			if packetID.actual != packetID.expected {
				t.Fatalf("packet id = %#x, want %#x", packetID.actual, packetID.expected)
			}
		})
	}
}

func TestDecodePlayerAction(t *testing.T) {
	data := []byte{
		PlayerActionStartDestroyBlock,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xDF, 0xFE,
		0xFF,
		0xAC, 0x02,
	}

	action, err := DecodePlayerAction(data)
	if err != nil {
		t.Fatalf("decode player action: %v", err)
	}

	expectedPosition := game.BlockPosition{X: -1, Y: -2, Z: -3}
	if action.Status != PlayerActionStartDestroyBlock || action.Position != expectedPosition || action.Face != -1 || action.Sequence != 300 {
		t.Fatalf("player action = %+v", action)
	}
}

func TestDecodePlayerActionRejectsTruncatedPacket(t *testing.T) {
	data := []byte{
		PlayerActionStartDestroyBlock,
		0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x30, 0x02,
		0x01,
	}

	_, err := DecodePlayerAction(data)
	if err == nil {
		t.Fatal("decode truncated player action succeeded")
	}
}

func TestBlockPositionRoundTrip(t *testing.T) {
	positions := []game.BlockPosition{
		{X: 1, Y: 2, Z: 3},
		{X: -1, Y: -2, Z: -3},
		{X: -33554432, Y: -2048, Z: 33554431},
		{X: 33554431, Y: 2047, Z: -33554432},
	}

	for _, position := range positions {
		var wr PacketWriter

		wr.BlockPosition(position)
		if err := wr.Err(); err != nil {
			t.Fatalf("encode position %+v: %v", position, err)
		}

		rd := NewPacketReader(wr.Buffer.Bytes())

		actualB := rd.BlockPosition()
		if actualB != position {
			t.Errorf("position round trip = %+v, want %+v", actualB, position)
		}

		if err := rd.Err(); err != nil {
			t.Fatalf("decode position %+v: %v", position, err)
		}
	}
}

func TestDecodePlayerCommand(t *testing.T) {
	command, err := DecodePlayerCommand([]byte{0xAC, 0x02, PlayerCommandStartSprinting, 0x00})
	if err != nil {
		t.Fatalf("decode player command: %v", err)
	}

	if command.EntityID != 300 || command.Action != PlayerCommandStartSprinting || command.Data != 0 {
		t.Fatalf("player command = %+v", command)
	}
}

func TestDecodePlayerInput(t *testing.T) {
	input, err := DecodePlayerInput([]byte{PlayerInputSneak | PlayerInputSprint})
	if err != nil {
		t.Fatalf("decode player input: %v", err)
	}

	if input.Flags != PlayerInputSneak|PlayerInputSprint {
		t.Fatalf("player input flags = %#x, want %#x", input.Flags, PlayerInputSneak|PlayerInputSprint)
	}
}

func TestDecodeSwingArm(t *testing.T) {
	swing, err := DecodeSwingArm([]byte{OffHand})
	if err != nil {
		t.Fatalf("decode swing arm: %v", err)
	}

	if swing.Hand != OffHand {
		t.Fatalf("swing hand = %d, want %d", swing.Hand, OffHand)
	}
}

func TestDecodeSetHeldItem(t *testing.T) {
	selection, err := DecodeSetHeldItem([]byte{0x00, 0x08})
	if err != nil {
		t.Fatalf("decode held item: %v", err)
	}

	if selection.Slot != 8 {
		t.Fatalf("held slot = %d, want 8", selection.Slot)
	}
}

func TestDecodeSetHeldItemRejectsTruncatedSlot(t *testing.T) {
	_, err := DecodeSetHeldItem([]byte{0x08})
	if err == nil {
		t.Fatal("decode truncated held item succeeded")
	}
}

func TestDecodeSetCreativeModeSlot(t *testing.T) {
	var writer PacketWriter

	writer.Short(40)
	writer.VarInt(64)
	writer.VarInt(1)
	writer.VarInt(1)
	writer.VarInt(1)
	writer.VarInt(2)
	writer.Bytes([]byte{0xAA, 0xBB})
	writer.VarInt(3)

	update, err := DecodeSetCreativeModeSlot(writer.Buffer.Bytes())
	if err != nil {
		t.Fatalf("decode creative slot: %v", err)
	}

	if update.Slot != 40 || update.Item.ItemID != 1 || update.Item.ItemCount != 64 {
		t.Fatalf("creative slot = %+v", update)
	}
}

func TestDecodeSetCreativeModeSlotRejectsTruncatedComponents(t *testing.T) {
	var writer PacketWriter

	writer.Short(36)
	writer.VarInt(1)
	writer.VarInt(1)
	writer.VarInt(1)
	writer.VarInt(0)

	_, err := DecodeSetCreativeModeSlot(writer.Buffer.Bytes())
	if err == nil {
		t.Fatal("decode truncated creative slot succeeded")
	}
}

func TestDecodeSetCreativeModeSlotRejectsOversizedComponent(t *testing.T) {
	var writer PacketWriter

	writer.Short(36)
	writer.VarInt(1)
	writer.VarInt(1)
	writer.VarInt(1)
	writer.VarInt(0)
	writer.VarInt(2)
	writer.VarInt(1 << 30)

	_, err := DecodeSetCreativeModeSlot(writer.Buffer.Bytes())
	if err == nil {
		t.Fatal("decode oversized creative slot component succeeded")
	}
}

func TestDecodeUseItemOn(t *testing.T) {
	position := game.BlockPosition{X: -1, Y: 70, Z: 3}

	var writer PacketWriter

	writer.VarInt(OffHand)
	writer.BlockPosition(position)
	writer.VarInt(BlockFaceWest)
	writer.Float(0.25)
	writer.Float(0.5)
	writer.Float(0.75)
	writer.Bool(false)
	writer.Bool(true)
	writer.VarInt(300)

	interaction, err := DecodeUseItemOn(writer.Buffer.Bytes())
	if err != nil {
		t.Fatalf("decode use item on: %v", err)
	}

	if interaction.Hand != OffHand || interaction.Position != position || interaction.Face != BlockFaceWest || interaction.CursorX != 0.25 || interaction.CursorY != 0.5 || interaction.CursorZ != 0.75 || interaction.InsideBlock || !interaction.WorldBorderHit || interaction.Sequence != 300 {
		t.Fatalf("use item on = %+v", interaction)
	}
}

func TestDecodeUseItemOnRejectsTruncatedPacket(t *testing.T) {
	_, err := DecodeUseItemOn([]byte{MainHand, 0, 0, 0})
	if err == nil {
		t.Fatal("decode truncated use item on succeeded")
	}
}

func assertMovementFlags(t *testing.T, flags MovementFlags, onGround, horizontalCollision bool) {
	t.Helper()

	if flags.OnGround() != onGround {
		t.Fatalf("on ground = %v, want %v", flags.OnGround(), onGround)
	}

	if flags.HasHorizontalCollision() != horizontalCollision {
		t.Fatalf(
			"horizontal collision = %v, want %v",
			flags.HasHorizontalCollision(),
			horizontalCollision,
		)
	}
}

func testChatMessageData(message string, signed bool, offset int32) []byte {
	var writer PacketWriter

	writer.String(message)
	writer.Long(1234)
	writer.Long(5678)
	writer.Bool(signed)

	if signed {
		for index := range chatMessageSignatureLength {
			writer.Byte(byte(index))
		}
	}

	writer.VarInt(offset)
	writer.Byte(0xAA)
	writer.Byte(0xBB)
	writer.Byte(0xCC)
	writer.Byte(0xDD)

	return writer.Buffer.Bytes()
}
