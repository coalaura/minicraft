package server

import (
	"io"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type playerMovementPacketTest struct {
	name     string
	current  game.Player
	expected []int32
}

type positionDeltaTest struct {
	name     string
	current  float64
	expected int16
	fits     bool
}

func TestSendPlayerMovementPacketSelection(t *testing.T) {
	connection := &recordingConnection{}

	session := &Session{Conn: protocol.NewConnection(connection, nil)}

	previous := game.Player{EntityID: 42}

	tests := []playerMovementPacketTest{
		{
			name: "relative position",
			current: game.Player{
				EntityID: 42,
				Position: game.Position{X: 1, Y: -2, Z: 7.999},
				OnGround: true,
			},
			expected: []int32{protocol.ClientboundUpdateEntityPositionID},
		},
		{
			name: "relative position and rotation",
			current: game.Player{
				EntityID: 42,
				Position: game.Position{X: 1},
				Rotation: game.Rotation{Yaw: 90, Pitch: 45},
			},
			expected: []int32{
				protocol.ClientboundUpdateEntityPositionRotationID,
				protocol.ClientboundSetHeadRotationID,
			},
		},
		{
			name: "rotation and head yaw",
			current: game.Player{
				EntityID: 42,
				Rotation: game.Rotation{Yaw: -90, Pitch: 20},
				OnGround: true,
			},
			expected: []int32{
				protocol.ClientboundUpdateEntityRotationID,
				protocol.ClientboundSetHeadRotationID,
			},
		},
		{
			name: "pitch only",
			current: game.Player{
				EntityID: 42,
				Rotation: game.Rotation{Pitch: 20},
			},
			expected: []int32{protocol.ClientboundUpdateEntityRotationID},
		},
		{
			name: "position synchronization",
			current: game.Player{
				EntityID: 42,
				Position: game.Position{X: 8},
			},
			expected: []int32{protocol.ClientboundSynchronizeEntityPositionID},
		},
		{
			name: "ground status",
			current: game.Player{
				EntityID: 42,
				OnGround: true,
			},
			expected: []int32{protocol.ClientboundUpdateEntityPositionID},
		},
		{
			name:     "unchanged",
			current:  previous,
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connection.reset()

			err := session.sendPlayerMovement(previous, test.current)
			if err != nil {
				t.Fatalf("send player movement: %v", err)
			}

			assertPacketIDs(t, connection.packetIDs(t), test.expected)
		})
	}
}

func TestProtocolPositionDeltaRange(t *testing.T) {
	tests := []positionDeltaTest{
		{name: "minimum", current: -8, expected: -32768, fits: true},
		{name: "maximum", current: 32767.0 / entityPositionScale, expected: 32767, fits: true},
		{name: "positive overflow", current: 8, fits: false},
		{name: "negative overflow", current: -8.001, fits: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			delta, fits := protocolPositionDelta(0, test.current)
			if delta != test.expected || fits != test.fits {
				t.Fatalf("protocolPositionDelta(0, %v) = (%d, %t), want (%d, %t)", test.current, delta, fits, test.expected, test.fits)
			}
		})
	}
}

func TestRuntimeBroadcastPlayerMovement(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Bob")
	alice, aliceConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Alice")
	charlie, charlieConnection := newMovementTestSession(runtime, "20212223-2425-2627-2829-2a2b2c2d2e2f", "Charlie")

	for _, session := range []*Session{bob, alice, charlie} {
		runtime.AssignEntityID(session)

		err := runtime.JoinSession(session)
		if err != nil {
			t.Fatalf("join %s: %v", session.Player.Name, err)
		}
	}

	bobConnection.reset()
	aliceConnection.reset()
	charlieConnection.reset()

	bob.handleMovePlayerRotation(protocol.MovePlayerRotation{Yaw: 90, Pitch: 30, Flags: protocol.MovementFlagOnGround})

	if packetIDs := bobConnection.packetIDs(t); len(packetIDs) != 0 {
		t.Fatalf("originating player received movement packets: %v", packetIDs)
	}

	expected := []int32{
		protocol.ClientboundUpdateEntityRotationID,
		protocol.ClientboundSetHeadRotationID,
	}

	assertPacketIDs(t, aliceConnection.packetIDs(t), expected)
	assertPacketIDs(t, charlieConnection.packetIDs(t), expected)

	player := bob.snapshotPlayer()
	if player.Rotation.Yaw != 90 || player.Rotation.Pitch != 30 || !player.OnGround {
		t.Fatalf("authoritative player state = %+v", player)
	}
}

func TestHorizontalCollisionFlagDoesNotSetOnGround(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Bob")

	session.handleMovePlayerStatus(protocol.MovePlayerStatus{
		Flags: protocol.MovementFlagHorizontalCollision,
	})

	if session.snapshotPlayer().OnGround {
		t.Fatal("horizontal collision flag set player on ground")
	}

	session.handleMovePlayerStatus(protocol.MovePlayerStatus{
		Flags: protocol.MovementFlagOnGround | protocol.MovementFlagHorizontalCollision,
	})

	if !session.snapshotPlayer().OnGround {
		t.Fatal("on-ground flag did not set player on ground")
	}
}

func TestJoiningPlayerSeesLatestAuthoritativePosition(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Bob")

	runtime.AssignEntityID(bob)

	err := runtime.JoinSession(bob)
	if err != nil {
		t.Fatalf("join Bob: %v", err)
	}

	bob.handleMovePlayerPosition(protocol.MovePlayerPosition{
		X: 12.5, Y: 80.25, Z: -4.75,
		Flags: protocol.MovementFlagOnGround,
	})

	bobConnection.reset()

	alice, aliceConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Alice")

	runtime.AssignEntityID(alice)

	err = runtime.JoinSession(alice)
	if err != nil {
		t.Fatalf("join Alice: %v", err)
	}

	for _, packet := range aliceConnection.packets(t) {
		if packet.ID != protocol.ClientboundAddEntityID {
			continue
		}

		reader := protocol.NewPacketReader(packet.Data)

		entityID := reader.VarInt()
		if entityID != bob.Player.EntityID {
			continue
		}

		_, err = reader.Seek(16, io.SeekCurrent)
		if err != nil {
			t.Fatalf("skip entity UUID: %v", err)
		}

		reader.VarInt()

		position := game.Position{X: reader.Double(), Y: reader.Double(), Z: reader.Double()}

		if err = reader.Err(); err != nil {
			t.Fatalf("decode add entity: %v", err)
		}

		expected := (game.Position{X: 12.5, Y: 80.25, Z: -4.75})
		if position != expected {
			t.Fatalf("spawn position = %+v, want %+v", position, expected)
		}

		return
	}

	t.Fatal("joining player did not receive Bob's entity")
}

func newMovementTestSession(runtime *Runtime, uuid, name string) (*Session, *recordingConnection) {
	connection := &recordingConnection{}

	session := &Session{
		Conn:    protocol.NewConnection(connection, nil),
		Runtime: runtime,
		Player:  &game.Player{UUID: uuid, Name: name},
	}

	return session, connection
}
