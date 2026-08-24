package protocol

import "testing"

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

func TestPlayerActionPacketIDsProtocol774(t *testing.T) {
	packetIDs := map[string]packetIDTest{
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
