package server

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/coalaura/minicraft/internal/protocol"
)

type Handshake struct {
	ProtocolVersion int32
	ServerAddress   string
	ServerPort      uint16
	NextState       int32
}

func decodeHandshake(data []byte) (Handshake, error) {
	rd := protocol.NewPacketReader(data)

	handshake := Handshake{
		ProtocolVersion: rd.VarInt(),
		ServerAddress:   rd.String(255),
		ServerPort:      uint16(rd.Short()),
		NextState:       rd.VarInt(),
	}

	err := rd.Done("handshake")
	if err != nil {
		return Handshake{}, err
	}

	return handshake, nil
}

func (s *Session) handleHandshake(ctx context.Context) error {
	s.Log.Printf("[handshake] %s - new connection\n", s.Conn.RemoteAddr())

	packet, err := s.readPacket()
	if err != nil {
		if errors.Is(err, io.EOF) {
			s.Log.Printf("[net] %s - disconnected (EOF)\n", s.Conn.RemoteAddr())

			return nil
		}

		return fmt.Errorf("read handshake packet: %w", err)
	}

	if packet.ID != protocol.ServerboundHandshakeID {
		return fmt.Errorf("expected handshake packet, got id %d", packet.ID)
	}

	handshake, err := decodeHandshake(packet.Data)
	if err != nil {
		return fmt.Errorf("decode handshake: %w", err)
	}

	switch handshake.NextState {
	case protocol.StateStatus:
		s.setProtocolState(protocol.StateStatus)

		return s.handleStatus(ctx)
	case protocol.StateLogin:
		s.setProtocolState(protocol.StateLogin)

		return s.handleLogin(ctx)
	default:
		return fmt.Errorf("invalid next state %d", handshake.NextState)
	}
}
