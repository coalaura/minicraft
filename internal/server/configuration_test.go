package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestConfigurationBrand(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	err := session.sendConfigurationBrand()
	if err != nil {
		t.Fatalf("send configuration brand: %v", err)
	}

	packets := connection.packets(t)
	if len(packets) != 1 {
		t.Fatalf("configuration brand packets = %d, want 1", len(packets))
	}

	packet := packets[0]
	if packet.ID != protocol.ClientboundConfigurationPluginMessageID {
		t.Fatalf("configuration brand packet ID = 0x%02x, want 0x%02x", packet.ID, protocol.ClientboundConfigurationPluginMessageID)
	}

	rd := protocol.NewPacketReader(packet.Data)

	channel := rd.String(256)
	brand := rd.String(32767)

	err = rd.Err()
	if err != nil {
		t.Fatalf("decode configuration brand: %v", err)
	}

	if channel != "minecraft:brand" || brand != "minicraft" {
		t.Fatalf("configuration brand = %q %q, want %q %q", channel, brand, "minecraft:brand", "minicraft")
	}
}
