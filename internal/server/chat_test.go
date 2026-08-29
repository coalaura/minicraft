package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
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

	prints := logger.chatPrints()
	if len(prints) != 1 || prints[0] != "[chat] <Laura> hello\n" {
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

	return renderTextComponent(decodeSystemComponent(t, data))
}

func decodeSystemComponent(t *testing.T, data []byte) game.TextComponent {
	t.Helper()

	if len(data) < 2 || data[0] != 10 || data[len(data)-1] != 0 {
		t.Fatalf("invalid system chat payload %x", data)
	}

	reader := bytes.NewReader(data[1:])

	component := readTextComponent(t, reader)

	actionBar, err := reader.ReadByte()
	if err != nil || actionBar != 0 || reader.Len() != 0 {
		t.Fatalf("invalid system chat trailer %x", data)
	}

	return component
}

func readTextComponent(t *testing.T, reader *bytes.Reader) game.TextComponent {
	t.Helper()

	var component game.TextComponent

	for {
		tag, err := reader.ReadByte()
		if err != nil {
			t.Fatalf("read text component tag: %v", err)
		}

		if tag == 0 {
			return component
		}

		name := readNBTString(t, reader)

		switch tag {
		case 1:
			value, readErr := reader.ReadByte()
			if readErr != nil {
				t.Fatalf("read text component byte: %v", readErr)
			}

			boolean := value != 0

			switch name {
			case "italic":
				component.Style.Italic = &boolean
			case "underlined":
				component.Style.Underlined = &boolean
			default:
				t.Fatalf("unexpected text component byte field %q", name)
			}
		case 8:
			value := readNBTString(t, reader)

			switch name {
			case "text":
				component.Text = value
			case "translate":
				component.Translate = value
			case "color":
				component.Style.Color = game.TextColor(value)
			default:
				t.Fatalf("unexpected text component string field %q", name)
			}
		case 9:
			components := readTextComponentList(t, reader)

			switch name {
			case "with":
				component.Arguments = components
			case "extra":
				component.Siblings = components
			default:
				t.Fatalf("unexpected text component list field %q", name)
			}
		case 10:
			if name != "click_event" {
				t.Fatalf("unexpected text component compound field %q", name)
			}

			component.Style.ClickEvent = readClickEvent(t, reader)
		default:
			t.Fatalf("unexpected text component tag %d for %q", tag, name)
		}
	}
}

func readTextComponentList(t *testing.T, reader *bytes.Reader) []game.TextComponent {
	t.Helper()

	tag, err := reader.ReadByte()
	if err != nil || tag != 10 {
		t.Fatalf("invalid text component list tag %d: %v", tag, err)
	}

	var count int32

	err = binary.Read(reader, binary.BigEndian, &count)
	if err != nil || count < 0 {
		t.Fatalf("invalid text component list length %d: %v", count, err)
	}

	components := make([]game.TextComponent, count)

	for index := range components {
		components[index] = readTextComponent(t, reader)
	}

	return components
}

func readClickEvent(t *testing.T, reader *bytes.Reader) *game.ClickEvent {
	t.Helper()

	event := &game.ClickEvent{}

	for {
		tag, err := reader.ReadByte()
		if err != nil {
			t.Fatalf("read click event tag: %v", err)
		}

		if tag == 0 {
			return event
		}

		name := readNBTString(t, reader)

		if tag != 8 {
			t.Fatalf("unexpected click event tag %d for %q", tag, name)
		}

		value := readNBTString(t, reader)

		switch name {
		case "action":
			event.Action = game.ClickAction(value)
		case "command", "value":
			event.Value = value
		default:
			t.Fatalf("unexpected click event field %q", name)
		}
	}
}

func readNBTString(t *testing.T, reader *bytes.Reader) string {
	t.Helper()

	var length uint16

	err := binary.Read(reader, binary.BigEndian, &length)
	if err != nil {
		t.Fatalf("read nbt string length: %v", err)
	}

	value := make([]byte, length)

	_, err = io.ReadFull(reader, value)
	if err != nil {
		t.Fatalf("read nbt string: %v", err)
	}

	return string(value)
}

func renderTextComponent(component game.TextComponent) string {
	var value string

	if component.Translate != "" {
		arguments := make([]string, len(component.Arguments))

		for index, argument := range component.Arguments {
			arguments[index] = renderTextComponent(argument)
		}

		value = fmt.Sprintf("%s(%s)", component.Translate, strings.Join(arguments, ", "))
	} else {
		value = component.Text
	}

	for _, sibling := range component.Siblings {
		value += renderTextComponent(sibling)
	}

	return value
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
