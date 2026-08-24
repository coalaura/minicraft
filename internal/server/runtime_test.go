package server

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type recordingConnection struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (c *recordingConnection) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (c *recordingConnection) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.buffer.Write(data)
}

func (c *recordingConnection) Close() error {
	return nil
}

func (c *recordingConnection) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *recordingConnection) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *recordingConnection) SetDeadline(time.Time) error {
	return nil
}

func (c *recordingConnection) SetReadDeadline(time.Time) error {
	return nil
}

func (c *recordingConnection) SetWriteDeadline(time.Time) error {
	return nil
}

func (c *recordingConnection) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.buffer.Reset()
}

func (c *recordingConnection) packetIDs(t *testing.T) []int32 {
	t.Helper()

	packets := c.packets(t)
	packetIDs := make([]int32, 0, len(packets))

	for _, packet := range packets {
		packetIDs = append(packetIDs, packet.ID)
	}

	return packetIDs
}

func (c *recordingConnection) packets(t *testing.T) []protocol.Packet {
	t.Helper()

	c.mu.Lock()
	data := append([]byte(nil), c.buffer.Bytes()...)
	c.mu.Unlock()

	reader := bytes.NewReader(data)

	var packets []protocol.Packet

	for reader.Len() > 0 {
		length, err := protocol.ReadVarInt(reader)
		if err != nil {
			t.Fatalf("read packet length: %v", err)
		}

		packetData := make([]byte, length)

		_, err = io.ReadFull(reader, packetData)
		if err != nil {
			t.Fatalf("read packet data: %v", err)
		}

		packetReader := bytes.NewReader(packetData)

		packetID, err := protocol.ReadVarInt(packetReader)
		if err != nil {
			t.Fatalf("read packet id: %v", err)
		}

		payload, err := io.ReadAll(packetReader)
		if err != nil {
			t.Fatalf("read packet payload: %v", err)
		}

		packets = append(packets, protocol.Packet{ID: packetID, Data: payload})
	}

	return packets
}

func TestRuntimePlayerVisibilityLifecycle(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bobConnection := &recordingConnection{}

	bob := &Session{
		Conn:    protocol.NewConnection(bobConnection, nil),
		Runtime: runtime,
		Player: &game.Player{
			UUID: "00010203-0405-0607-0809-0a0b0c0d0e0f",
			Name: "Bob",
		},
	}

	aliceConnection := &recordingConnection{}

	alice := &Session{
		Conn:    protocol.NewConnection(aliceConnection, nil),
		Runtime: runtime,
		Player: &game.Player{
			UUID: "10111213-1415-1617-1819-1a1b1c1d1e1f",
			Name: "Laura",
			Properties: []game.ProfileProperty{
				{Name: "textures", Value: "skin", Signature: "signature"},
			},
		},
	}

	runtime.AssignEntityID(bob)

	err := runtime.JoinSession(bob)
	if err != nil {
		t.Fatalf("join Bob: %v", err)
	}

	bobConnection.reset()
	runtime.AssignEntityID(alice)

	err = runtime.JoinSession(alice)
	if err != nil {
		t.Fatalf("join Laura: %v", err)
	}

	assertPacketIDs(t, aliceConnection.packetIDs(t), []int32{
		protocol.ClientboundPlayerInfoUpdateID,
		protocol.ClientboundEntityMetadataID,
		protocol.ClientboundAddEntityID,
		protocol.ClientboundEntityMetadataID,
	})

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundPlayerInfoUpdateID,
		protocol.ClientboundAddEntityID,
		protocol.ClientboundEntityMetadataID,
	})

	bobConnection.reset()
	aliceConnection.reset()

	runtime.UpdateSkinParts(alice, 0x7F)

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundEntityMetadataID,
	})

	assertPacketIDs(t, aliceConnection.packetIDs(t), []int32{
		protocol.ClientboundEntityMetadataID,
	})

	bobConnection.reset()

	runtime.LeaveSession(alice)

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundRemoveEntitiesID,
		protocol.ClientboundPlayerInfoRemoveID,
	})
}

func assertPacketIDs(t *testing.T, actual, expected []int32) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("packet ids = %v, want %v", actual, expected)
	}

	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("packet ids = %v, want %v", actual, expected)
		}
	}
}
