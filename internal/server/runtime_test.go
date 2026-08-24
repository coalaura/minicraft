package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type recordingConnection struct {
	mu     sync.Mutex
	input  bytes.Buffer
	buffer bytes.Buffer
}

func pointerTo[T any](value T) *T {
	return &value
}

func (c *recordingConnection) Read(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.input.Read(data)
}

func (c *recordingConnection) queuePacket(t *testing.T, packet protocol.Packet) {
	t.Helper()

	temporary := &recordingConnection{}

	err := protocol.NewConnection(temporary, nil).WritePacket(packet)
	if err != nil {
		t.Fatalf("encode queued packet: %v", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	_, err = c.input.Write(temporary.buffer.Bytes())
	if err != nil {
		t.Fatalf("queue packet: %v", err)
	}
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

func TestRuntimeVisibilityTransitionsAcrossRenderDistance(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	cfg := &config.Config{Server: config.ServerConfig{RenderDistance: pointerTo(int32(1))}}

	bobConnection := &recordingConnection{}

	bob := &Session{
		Conn:    protocol.NewConnection(bobConnection, nil),
		Config:  cfg,
		Runtime: runtime,
		Player: &game.Player{
			UUID: "00010203-0405-0607-0809-0a0b0c0d0e0f",
			Name: "Bob",
		},
	}

	aliceConnection := &recordingConnection{}

	alice := &Session{
		Conn:    protocol.NewConnection(aliceConnection, nil),
		Config:  cfg,
		Runtime: runtime,
		Player: &game.Player{
			UUID:     "10111213-1415-1617-1819-1a1b1c1d1e1f",
			Name:     "Laura",
			Position: game.Position{X: 32},
		},
	}

	for _, session := range []*Session{bob, alice} {
		runtime.AssignEntityID(session)

		err := runtime.JoinSession(session)
		if err != nil {
			t.Fatalf("join %s: %v", session.Player.Name, err)
		}
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundPlayerInfoUpdateID,
		protocol.ClientboundEntityMetadataID,
	})

	assertPacketIDs(t, aliceConnection.packetIDs(t), []int32{
		protocol.ClientboundPlayerInfoUpdateID,
		protocol.ClientboundEntityMetadataID,
	})

	bobConnection.reset()
	aliceConnection.reset()

	runtime.updatePlayerMovement(alice, func(player *game.Player) {
		player.Position.X = 16
	})

	appearance := []int32{
		protocol.ClientboundPlayerInfoUpdateID,
		protocol.ClientboundAddEntityID,
		protocol.ClientboundEntityMetadataID,
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), appearance)
	assertPacketIDs(t, aliceConnection.packetIDs(t), appearance)

	bobConnection.reset()
	aliceConnection.reset()

	runtime.updatePlayerMovement(alice, func(player *game.Player) {
		player.Position.X = 32
	})

	removal := []int32{
		protocol.ClientboundRemoveEntitiesID,
		protocol.ClientboundPlayerInfoRemoveID,
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), removal)
	assertPacketIDs(t, aliceConnection.packetIDs(t), removal)

	bobConnection.reset()

	runtime.BroadcastPlayerAnimation(alice, protocol.EntityAnimationSwingMainHand)

	if packets := bobConnection.packets(t); len(packets) != 0 {
		t.Fatalf("invisible animation packets = %v, want none", bobConnection.packetIDs(t))
	}
}

func TestRuntimePlayerSlotReservationIsAtomic(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	const attempts = 32

	var (
		waitGroup sync.WaitGroup
		results   = make(chan bool, attempts)
	)

	for range attempts {
		waitGroup.Go(func() {
			results <- runtime.ReservePlayerSlot(1)
		})
	}

	waitGroup.Wait()
	close(results)

	var accepted int

	for result := range results {
		if result {
			accepted++
		}
	}

	if accepted != 1 {
		t.Fatalf("accepted reservations = %d, want 1", accepted)
	}

	runtime.ReleasePlayerSlot()

	if !runtime.ReservePlayerSlot(1) {
		t.Fatal("released slot was not reusable")
	}
}

func TestStatusResponseReportsActivePlayers(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	playerConnection := &recordingConnection{}

	player := &Session{
		Conn:    protocol.NewConnection(playerConnection, nil),
		Runtime: runtime,
		Player: &game.Player{
			UUID: "00010203-0405-0607-0809-0a0b0c0d0e0f",
			Name: "Bob",
		},
	}

	runtime.AssignEntityID(player)

	err := runtime.JoinSession(player)
	if err != nil {
		t.Fatalf("join player: %v", err)
	}

	statusConnection := &recordingConnection{}
	status := &Session{
		Conn:    protocol.NewConnection(statusConnection, nil),
		Config:  &config.Config{Server: config.ServerConfig{MaxPlayers: pointerTo(8)}},
		Runtime: runtime,
	}

	err = status.sendStatusResponse()
	if err != nil {
		t.Fatalf("send status: %v", err)
	}

	packets := statusConnection.packets(t)

	reader := protocol.NewPacketReader(packets[0].Data)

	responseJSON := reader.String(32767)

	if err = reader.Err(); err != nil {
		t.Fatalf("decode status packet: %v", err)
	}

	var response struct {
		Players struct {
			Online int `json:"online"`
			Max    int `json:"max"`
		} `json:"players"`
	}

	err = json.Unmarshal([]byte(responseJSON), &response)
	if err != nil {
		t.Fatalf("decode status json: %v", err)
	}

	if response.Players.Online != 1 || response.Players.Max != 8 {
		t.Fatalf("status players = %+v, want online 1 max 8", response.Players)
	}
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
