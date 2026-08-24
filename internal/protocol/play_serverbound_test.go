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
		"swing arm":      {actual: ServerboundSwingArmID, expected: 0x3C},
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
