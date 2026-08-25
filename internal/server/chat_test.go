package server

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestChatFormatPlaceholderReplacement(t *testing.T) {
	formatted := formatChatMessage("{player}: {message} ({player})", "Laura", "hello {player}")

	if formatted != "Laura: hello {player} (Laura)" {
		t.Fatalf("formatted chat = %q", formatted)
	}
}

func TestPlayerChatBroadcastsGlobally(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.ChatEnabled = true
	runtime.ChatFormat = "[{player}] {message}"

	sender, senderConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Laura")
	recipient, recipientConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Bob")

	logger := &chatTestLogger{}

	sender.Log = logger

	recipient.Player.Position.X = float64((config.DefaultRenderDistance + 2) * ChunkWidth)

	joinTestSession(t, runtime, sender)
	joinTestSession(t, runtime, recipient)

	senderConnection.reset()
	recipientConnection.reset()

	err := sender.handlePlayPacket(&protocol.Packet{ID: protocol.ServerboundChatMessageID, Data: serverChatPacketData("hello")})
	if err != nil {
		t.Fatalf("handle chat packet: %v", err)
	}

	assertSystemMessages(t, senderConnection, "[Laura] hello")
	assertSystemMessages(t, recipientConnection, "[Laura] hello")

	if prints := logger.chatPrints(); len(prints) != 1 || prints[0] != "[chat] <Laura> hello\n" {
		t.Fatalf("chat prints = %q", prints)
	}
}

func TestDisabledPlayerChatIsIgnored(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.ChatFormat = config.DefaultChatFormat

	sender, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Laura")

	joinTestSession(t, runtime, sender)

	connection.reset()

	runtime.BroadcastPlayerChat(sender, "hello")

	assertSystemMessages(t, connection)
}

func TestJoinAndLeaveMessagesBroadcastGlobally(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.ChatJoinMessage = "Welcome {player}"
	runtime.ChatLeaveMessage = "Goodbye {player}"

	observer, observerConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Observer")
	joining, joiningConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Laura")

	joining.Player.Position.X = float64((config.DefaultRenderDistance + 2) * ChunkWidth)

	joinTestSession(t, runtime, observer)

	observerConnection.reset()

	joinTestSession(t, runtime, joining)

	assertSystemMessages(t, observerConnection, "Welcome Laura")
	assertSystemMessages(t, joiningConnection, "Welcome Laura")

	observerConnection.reset()
	joiningConnection.reset()

	runtime.LeaveSession(joining)

	assertSystemMessages(t, observerConnection, "Goodbye Laura")
	assertSystemMessages(t, joiningConnection)
}

func TestEmptyLifecycleMessagesDisableBroadcasts(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	observer, observerConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Observer")
	joining, _ := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Laura")

	joinTestSession(t, runtime, observer)

	observerConnection.reset()

	joinTestSession(t, runtime, joining)

	runtime.LeaveSession(joining)

	assertSystemMessages(t, observerConnection)
}

func TestInactiveSessionsDoNotProduceLifecycleMessages(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.ChatJoinMessage = "{player} joined"
	runtime.ChatLeaveMessage = "{player} left"

	observer, observerConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Observer")

	joinTestSession(t, runtime, observer)

	observerConnection.reset()

	inactive, inactiveConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Inactive")

	inactiveConnection.writeErr = errors.New("write failed")

	runtime.AssignEntityID(inactive)

	err := runtime.JoinSession(inactive)
	if err == nil {
		t.Fatal("failed session joined without an error")
	}

	runtime.LeaveSession(inactive)

	assertSystemMessages(t, observerConnection)
}

func assertSystemMessages(t *testing.T, connection *recordingConnection, expected ...string) {
	t.Helper()

	var actual []string

	for _, packet := range connection.packets(t) {
		if packet.ID != protocol.ClientboundSystemChatID {
			continue
		}

		actual = append(actual, decodeSystemMessage(t, packet.Data))
	}

	if len(actual) != len(expected) {
		t.Fatalf("system messages = %q, want %q", actual, expected)
	}

	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("system messages = %q, want %q", actual, expected)
		}
	}
}

func decodeSystemMessage(t *testing.T, data []byte) string {
	t.Helper()

	if len(data) < 4 || data[0] != 8 {
		t.Fatalf("invalid system chat payload %x", data)
	}

	length := int(binary.BigEndian.Uint16(data[1:3]))
	if len(data) != length+4 || data[len(data)-1] != 0 {
		t.Fatalf("invalid system chat payload %x", data)
	}

	return string(data[3 : 3+length])
}

func serverChatPacketData(message string) []byte {
	var writer protocol.PacketWriter

	writer.String(message)
	writer.Long(1)
	writer.Long(2)
	writer.Bool(false)
	writer.VarInt(0)
	writer.Byte(0)
	writer.Byte(0)
	writer.Byte(0)
	writer.Byte(0)

	return writer.Buffer.Bytes()
}
