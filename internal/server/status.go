package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/coalaura/minicraft/internal/protocol"
)

func (s *Session) handleStatus(ctx context.Context) error {
	for {
		packet, err := s.Conn.ReadPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.Log.Printf("[status] %s - disconnected (EOF)\n", s.Conn.RemoteAddr())

				return nil
			}

			return fmt.Errorf("read status packet: %w", err)
		}

		switch packet.ID {
		case protocol.ServerboundStatusRequestID:
			err = s.sendStatusResponse()
			if err != nil {
				return err
			}
		case protocol.ServerboundStatusPingID:
			if len(packet.Data) != 8 {
				return nil
			}

			payload := int64(binary.BigEndian.Uint64(packet.Data))

			err = s.sendStatusPong(payload)
			if err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (s *Session) sendStatusResponse() error {
	info := map[string]any{
		"version": map[string]any{
			"name":     "1.21.11",
			"protocol": protocol.ProtocolVersion,
		},
		"players": map[string]any{
			"max":    s.Config.MaxPlayers(),
			"online": s.Runtime.PlayerCount(),
		},
		"description": map[string]any{
			"text": s.Config.Server.Motd,
		},
		"enforcesSecureChat": s.secureChatEnforced(),
		"previewsChat":       false,
	}

	js, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal status response: %w", err)
	}

	var wr protocol.PacketWriter

	response := protocol.StatusResponse{JSON: string(js)}

	response.Encode(&wr)

	err = wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundStatusResponseID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendStatusPong(payload int64) error {
	var wr protocol.PacketWriter

	pong := protocol.StatusPong{Payload: payload}

	pong.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundStatusPongID,
		Data: wr.Buffer.Bytes(),
	})
}
