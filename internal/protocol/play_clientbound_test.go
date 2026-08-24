package protocol

import (
	"bytes"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

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

		VelocityX: 1,
		VelocityY: -2,
		VelocityZ: 3,

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
		0x00, 0x01, 0xFF, 0xFE, 0x00, 0x03,
		0x04, 0x05, 0x06, 0x07,
	}

	if !bytes.Equal(wr.Buffer.Bytes(), expected) {
		t.Fatalf("encoded add entity = %x, want %x", wr.Buffer.Bytes(), expected)
	}
}

func TestEntityMetadataSkinPartsEncode(t *testing.T) {
	metadata := EntityMetadataSkinParts{
		EntityID:  300,
		SkinParts: 0x7F,
	}

	var wr PacketWriter

	metadata.Encode(&wr)

	err := wr.Err()
	if err != nil {
		t.Fatalf("encode entity metadata: %v", err)
	}

	expected := []byte{0xAC, 0x02, 0x10, 0x00, 0x7F, 0xFF}

	if !bytes.Equal(wr.Buffer.Bytes(), expected) {
		t.Fatalf("encoded entity metadata = %x, want %x", wr.Buffer.Bytes(), expected)
	}
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
