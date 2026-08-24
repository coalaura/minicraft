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
