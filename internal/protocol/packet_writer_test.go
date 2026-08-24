package protocol

import (
	"bytes"
	"testing"
)

type lowPrecisionVectorTest struct {
	name     string
	x        float64
	y        float64
	z        float64
	expected []byte
}

func TestPacketWriterLowPrecisionVector(t *testing.T) {
	tests := []lowPrecisionVectorTest{
		{
			name:     "zero",
			expected: []byte{0x00},
		},
		{
			name:     "single byte scale",
			x:        0.25,
			y:        -0.5,
			z:        1,
			expected: []byte{0xF9, 0x7F, 0xFF, 0xFC, 0x80, 0x02},
		},
		{
			name:     "continued scale",
			x:        4,
			y:        -2,
			z:        0.5,
			expected: []byte{0xF4, 0xFF, 0x8F, 0xFE, 0x80, 0x03, 0x01},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var wr PacketWriter

			wr.LowPrecisionVector(test.x, test.y, test.z)

			err := wr.Err()
			if err != nil {
				t.Fatalf("encode low-precision vector: %v", err)
			}

			if !bytes.Equal(wr.Buffer.Bytes(), test.expected) {
				t.Fatalf("encoded vector = %x, want %x", wr.Buffer.Bytes(), test.expected)
			}
		})
	}
}
