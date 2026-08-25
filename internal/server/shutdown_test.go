package server

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type shutdownStateTest struct {
	name     string
	state    int32
	packetID int32
}

func TestRuntimeDisconnectAllSendsStateAppropriatePackets(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil, 0))

	reason := "Server shutting down"

	tests := []shutdownStateTest{
		{name: "handshake", state: protocol.StateHandshake, packetID: -1},
		{name: "status", state: protocol.StateStatus, packetID: -1},
		{name: "login", state: protocol.StateLogin, packetID: protocol.ClientboundLoginDisconnectID},
		{name: "configuration", state: protocol.StateConfiguration, packetID: protocol.ClientboundConfigurationDisconnectID},
		{name: "play", state: protocol.StatePlay, packetID: protocol.ClientboundPlayDisconnectID},
	}

	testConnections := make(map[string]*recordingConnection, len(tests))

	for _, test := range tests {
		connection := &recordingConnection{}

		session := NewSession(protocol.NewConnection(connection, nil), nil, runtime, nil)

		session.setProtocolState(test.state)

		if !runtime.registerConnectedSession(session) {
			t.Fatalf("register %s session failed", test.name)
		}

		testConnections[test.name] = connection
	}

	err := runtime.DisconnectAll(reason)
	if err != nil {
		t.Fatalf("disconnect all: %v", err)
	}

	err = runtime.DisconnectAll(reason)
	if err != nil {
		t.Fatalf("repeat disconnect all: %v", err)
	}

	for _, test := range tests {
		connection := testConnections[test.name]

		if !connection.isClosed() {
			t.Errorf("%s connection was not closed", test.name)
		}

		packets := connection.packets(t)

		if test.packetID < 0 {
			if len(packets) != 0 {
				t.Errorf("%s packets = %v; want none", test.name, connection.packetIDs(t))
			}

			continue
		}

		if len(packets) != 1 || packets[0].ID != test.packetID {
			t.Errorf("%s packets = %v; want [%d]", test.name, connection.packetIDs(t), test.packetID)

			continue
		}

		if test.state == protocol.StateLogin {
			assertLoginDisconnectReason(t, packets[0].Data, reason)

			continue
		}

		assertConfigurationOrPlayDisconnectReason(t, packets[0].Data, reason)
	}
}

func TestSessionRunClosesConnectionAfterShutdownStarts(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil, 0))

	err := runtime.DisconnectAll("Server shutting down")
	if err != nil {
		t.Fatalf("start shutdown: %v", err)
	}

	connection := &recordingConnection{}

	session := NewSession(protocol.NewConnection(connection, nil), nil, runtime, nil)

	err = session.Run(t.Context())
	if err != nil {
		t.Fatalf("run session: %v", err)
	}

	if !connection.isClosed() {
		t.Fatal("connection accepted during shutdown was not closed")
	}
}

func assertLoginDisconnectReason(t *testing.T, data []byte, expected string) {
	t.Helper()

	reader := protocol.NewPacketReader(data)

	encodedReason := reader.String(32767)

	err := reader.Done("login disconnect")
	if err != nil {
		t.Fatalf("decode login disconnect: %v", err)
	}

	var reason map[string]string

	err = json.Unmarshal([]byte(encodedReason), &reason)
	if err != nil {
		t.Fatalf("parse login disconnect reason: %v", err)
	}

	if reason["text"] != expected {
		t.Errorf("login disconnect reason = %q; want %q", reason["text"], expected)
	}
}

func assertConfigurationOrPlayDisconnectReason(t *testing.T, data []byte, expected string) {
	t.Helper()

	var writer protocol.PacketWriter

	protocol.PlayDisconnect{Reason: expected}.Encode(&writer)

	err := writer.Err()
	if err != nil {
		t.Fatalf("encode expected disconnect: %v", err)
	}

	if !bytes.Equal(data, writer.Buffer.Bytes()) {
		t.Errorf("disconnect data = %x; want %x", data, writer.Buffer.Bytes())
	}
}
