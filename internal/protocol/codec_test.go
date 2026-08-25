package protocol

import (
	"bytes"
	"testing"
)

func TestReadStringValidatesUTF8AndCharacterLength(t *testing.T) {
	tests := map[string][]byte{
		"invalid utf-8":       {0x01, 0xFF},
		"too many characters": {0x02, 'a', 'b'},
		"negative length":     {0xFF, 0xFF, 0xFF, 0xFF, 0x0F},
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			reader := ByteReader{Reader: bytes.NewReader(data)}

			_, err := ReadString(&reader, 1)
			if err == nil {
				t.Fatal("invalid string decoded")
			}
		})
	}
}

func TestReadStringAcceptsCharacterLimit(t *testing.T) {
	reader := ByteReader{Reader: bytes.NewReader([]byte{0x04, 0xE7, 0x8C, 0xAB, 'a'})}

	value, err := ReadString(&reader, 2)
	if err != nil {
		t.Fatalf("read string: %v", err)
	}

	if value != "猫a" {
		t.Fatalf("value = %q, want %q", value, "猫a")
	}
}
