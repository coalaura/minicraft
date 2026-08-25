package protocol

import (
	"bytes"
	"compress/zlib"
	"net"
	"testing"
	"time"
)

type packetReadTestConnection struct {
	*bytes.Reader
}

func (c *packetReadTestConnection) Write(data []byte) (int, error) {
	return len(data), nil
}

func (c *packetReadTestConnection) Close() error {
	return nil
}

func (c *packetReadTestConnection) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *packetReadTestConnection) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *packetReadTestConnection) SetDeadline(time.Time) error {
	return nil
}

func (c *packetReadTestConnection) SetReadDeadline(time.Time) error {
	return nil
}

func (c *packetReadTestConnection) SetWriteDeadline(time.Time) error {
	return nil
}

func TestReadPacketRejectsInvalidFrameLengths(t *testing.T) {
	for _, frame := range [][]byte{
		{0x00},
		{0x80, 0x80, 0x80, 0x01},
		{0xFF, 0xFF, 0xFF, 0xFF, 0x0F},
	} {
		connection := newPacketReadTestConnection(frame, 0)

		_, err := connection.ReadPacket()
		if err == nil {
			t.Fatalf("invalid frame %x decoded", frame)
		}
	}
}

func TestReadPacketRejectsInvalidCompressionLengths(t *testing.T) {
	oversizedPayload := encodeTestVarInt(t, maxPacketDataLength+1)
	oversizedFrame := frameTestPayload(t, oversizedPayload)

	connection := newPacketReadTestConnection(oversizedFrame, 10)

	_, err := connection.ReadPacket()
	if err == nil {
		t.Fatal("oversized uncompressed length decoded")
	}

	belowThreshold := compressedTestFrame(t, 5, []byte{0x00, 1, 2, 3, 4})
	connection = newPacketReadTestConnection(belowThreshold, 10)

	_, err = connection.ReadPacket()
	if err == nil {
		t.Fatal("compressed packet below threshold decoded")
	}

	uncompressedAtThreshold := frameTestPayload(t, []byte{0x00, 0x2A, 0x01, 0x02})
	connection = newPacketReadTestConnection(uncompressedAtThreshold, 3)

	_, err = connection.ReadPacket()
	if err == nil {
		t.Fatal("uncompressed packet at threshold decoded")
	}
}

func TestReadPacketRejectsMismatchedDecompressedLengths(t *testing.T) {
	for _, frame := range [][]byte{
		compressedTestFrame(t, 4, []byte{0x00, 1, 2}),
		compressedTestFrame(t, 2, []byte{0x00, 1, 2}),
		frameTestPayload(t, append(encodeTestVarInt(t, 2), 0x01, 0x02, 0x03)),
	} {
		connection := newPacketReadTestConnection(frame, 1)

		_, err := connection.ReadPacket()
		if err == nil {
			t.Fatalf("malformed compressed frame %x decoded", frame)
		}
	}
}

func TestReadPacketAcceptsExactCompressedLength(t *testing.T) {
	frame := compressedTestFrame(t, 3, []byte{0x2A, 1, 2})
	connection := newPacketReadTestConnection(frame, 1)

	packet, err := connection.ReadPacket()
	if err != nil {
		t.Fatalf("read compressed packet: %v", err)
	}

	if packet.ID != 0x2A || !bytes.Equal(packet.Data, []byte{1, 2}) {
		t.Fatalf("packet = %+v", packet)
	}
}

func TestReadPacketRejectsDataAfterCompressedStream(t *testing.T) {
	validFrame := compressedTestFrame(t, 3, []byte{0x2A, 1, 2})

	payloadWithTrailingData := append(append([]byte(nil), validFrame[1:]...), 0x7F)

	frame := frameTestPayload(t, payloadWithTrailingData)

	connection := newPacketReadTestConnection(frame, 1)

	_, err := connection.ReadPacket()
	if err == nil {
		t.Fatal("compressed packet with trailing data decoded")
	}
}

func newPacketReadTestConnection(frame []byte, threshold int) *Connection {
	networkConnection := &packetReadTestConnection{Reader: bytes.NewReader(frame)}

	connection := NewConnection(networkConnection, nil)

	connection.SetCompression(threshold)

	return connection
}

func compressedTestFrame(t *testing.T, declaredLength int32, data []byte) []byte {
	t.Helper()

	var payload bytes.Buffer

	err := WriteVarInt(&payload, declaredLength)
	if err != nil {
		t.Fatalf("encode declared length: %v", err)
	}

	compressor := zlib.NewWriter(&payload)

	_, err = compressor.Write(data)
	if err != nil {
		t.Fatalf("compress packet: %v", err)
	}

	err = compressor.Close()
	if err != nil {
		t.Fatalf("finish compressed packet: %v", err)
	}

	return frameTestPayload(t, payload.Bytes())
}

func frameTestPayload(t *testing.T, payload []byte) []byte {
	t.Helper()

	var frame bytes.Buffer

	err := WriteVarInt(&frame, int32(len(payload)))
	if err != nil {
		t.Fatalf("encode frame length: %v", err)
	}

	frame.Write(payload)

	return frame.Bytes()
}

func encodeTestVarInt(t *testing.T, value int32) []byte {
	t.Helper()

	var encoded bytes.Buffer

	err := WriteVarInt(&encoded, value)
	if err != nil {
		t.Fatalf("encode VarInt: %v", err)
	}

	return encoded.Bytes()
}
