package protocol

import "testing"

func TestLevelChunkBlockEntityEncoding(t *testing.T) {
	entity := LevelChunkBlockEntity{
		X:    3,
		Z:    12,
		Y:    -64,
		Type: 300,
		Data: []byte{0x0A, 0x00},
	}

	var writer PacketWriter

	entity.Encode(&writer)

	err := writer.Err()
	if err != nil {
		t.Fatalf("encode block entity: %v", err)
	}

	expected := []byte{0x3C, 0xFF, 0xC0, 0xAC, 0x02, 0x0A, 0x00}
	if writer.Buffer.String() != string(expected) {
		t.Fatalf("block entity = %x, want %x", writer.Buffer.Bytes(), expected)
	}
}

func TestLevelChunkBlockEntityWithoutDataEncodesEndTag(t *testing.T) {
	entity := LevelChunkBlockEntity{X: 15, Z: 1, Y: 2, Type: 3}

	var writer PacketWriter

	entity.Encode(&writer)

	err := writer.Err()
	if err != nil {
		t.Fatalf("encode block entity: %v", err)
	}

	expected := []byte{0xF1, 0x00, 0x02, 0x03, 0x00}
	if writer.Buffer.String() != string(expected) {
		t.Fatalf("block entity = %x, want %x", writer.Buffer.Bytes(), expected)
	}
}
