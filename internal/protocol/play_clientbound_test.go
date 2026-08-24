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

func TestMovementPacketIDsProtocol774(t *testing.T) {
	packetIDs := map[string]packetIDTest{
		"synchronize entity position":  {actual: ClientboundSynchronizeEntityPositionID, expected: 0x23},
		"update entity position":       {actual: ClientboundUpdateEntityPositionID, expected: 0x33},
		"update position and rotation": {actual: ClientboundUpdateEntityPositionRotationID, expected: 0x34},
		"update entity rotation":       {actual: ClientboundUpdateEntityRotationID, expected: 0x36},
		"set head rotation":            {actual: ClientboundSetHeadRotationID, expected: 0x51},
	}

	for name, packetID := range packetIDs {
		t.Run(name, func(t *testing.T) {
			if packetID.actual != packetID.expected {
				t.Fatalf("packet id = %#x, want %#x", packetID.actual, packetID.expected)
			}
		})
	}
}

func TestChunkPacketProtocol774(t *testing.T) {
	if ClientboundForgetLevelChunkID != 0x25 {
		t.Fatalf("forget level chunk packet id = %#x, want 0x25", ClientboundForgetLevelChunkID)
	}

	forget := ForgetLevelChunk{X: 0x01020304, Z: -2}

	assertPacketEncoding(t, forget, []byte{0xFF, 0xFF, 0xFF, 0xFE, 0x01, 0x02, 0x03, 0x04})
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

func TestEntityAnimationEncode(t *testing.T) {
	animation := EntityAnimation{EntityID: 300, Animation: EntityAnimationSwingOffHand}

	assertPacketEncoding(t, animation, []byte{0xAC, 0x02, 0x03})
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
