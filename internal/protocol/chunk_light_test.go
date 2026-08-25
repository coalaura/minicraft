package protocol

import "testing"

func TestUpdateLightEncodingUsesVarIntCoordinatesAndLightPayload(t *testing.T) {
	update := UpdateLight{
		Position:            ChunkPosition{X: -17, Z: 300},
		SkyLightMask:        []int64{2},
		BlockLightMask:      []int64{4},
		EmptySkyLightMask:   []int64{1},
		EmptyBlockLightMask: []int64{8},
		SkyLight:            [][]byte{{0xab, 0xcd}},
		BlockLight:          [][]byte{{0x12}},
	}

	var writer PacketWriter

	update.Encode(&writer)

	err := writer.Err()
	if err != nil {
		t.Fatalf("encode light update: %v", err)
	}

	reader := NewPacketReader(writer.Buffer.Bytes())

	x := reader.VarInt()
	z := reader.VarInt()

	if x != -17 || z != 300 {
		t.Fatalf("chunk coordinates = %d, %d", x, z)
	}

	masks := []struct {
		name     string
		expected int64
	}{
		{name: "sky", expected: 2},
		{name: "block", expected: 4},
		{name: "empty sky", expected: 1},
		{name: "empty block", expected: 8},
	}

	for _, expected := range masks {
		length := reader.VarInt()
		mask := reader.Long()

		if length != 1 || mask != expected.expected {
			t.Fatalf("%s mask = length %d value %d", expected.name, length, mask)
		}
	}

	count := reader.VarInt()
	light := reader.Bytes()

	if count != 1 || len(light) != 2 || light[0] != 0xab || light[1] != 0xcd {
		t.Fatalf("sky light = count %d data %x", count, light)
	}

	count = reader.VarInt()
	light = reader.Bytes()

	if count != 1 || len(light) != 1 || light[0] != 0x12 {
		t.Fatalf("block light = count %d data %x", count, light)
	}

	if err := reader.Err(); err != nil {
		t.Fatalf("decode light update: %v", err)
	}
}

func TestUpdateTimeEncoding(t *testing.T) {
	var writer PacketWriter

	UpdateTime{Age: 42, Time: 18000, TickDayTime: false}.Encode(&writer)

	reader := NewPacketReader(writer.Buffer.Bytes())

	age := reader.Long()
	dayTime := reader.Long()
	cycling := reader.Bool()

	if age != 42 || dayTime != 18000 || cycling {
		t.Fatalf("time update = age %d time %d cycling %v", age, dayTime, cycling)
	}

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode time update: %v", err)
	}
}
