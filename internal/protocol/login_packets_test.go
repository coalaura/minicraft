package protocol

import "testing"

func TestDecodeLoginStartRequiresUUIDAndConsumesPacket(t *testing.T) {
	var writer PacketWriter

	writer.String("Bob")
	writer.UUID("00010203-0405-0607-0809-0a0b0c0d0e0f")

	start, err := DecodeLoginStart(writer.Buffer.Bytes())
	if err != nil {
		t.Fatalf("decode login start: %v", err)
	}

	if start.Name != "Bob" || start.UUID != "00010203-0405-0607-0809-0a0b0c0d0e0f" {
		t.Fatalf("login start = %+v", start)
	}

	truncated := writer.Buffer.Bytes()[:writer.Len()-1]

	_, err = DecodeLoginStart(truncated)
	if err == nil {
		t.Fatal("truncated login start decoded")
	}

	trailing := append(append([]byte(nil), writer.Buffer.Bytes()...), 0x00)

	_, err = DecodeLoginStart(trailing)
	if err == nil {
		t.Fatal("login start with trailing data decoded")
	}
}
