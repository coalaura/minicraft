package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"

	"github.com/coalaura/minicraft/config"
)

type ServerVersionInfo struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}

type ServerPlayerInfo struct {
	Max    int `json:"max"`
	Online int `json:"online"`
}

type ServerInfo struct {
	Version     ServerVersionInfo `json:"version"`
	Players     ServerPlayerInfo  `json:"players"`
	Description any               `json:"description"`

	EnforcesSecureChat bool `json:"enforcesSecureChat"`
	PreviewsChat       bool `json:"previewsChat"`
}

func HandleConnection(ctx context.Context, conn net.Conn, cfg *config.Config) {
	defer conn.Close()

	c := NewConn(conn)

	c.Print("handshake", "new connection")

	state := StateHandshake

	for {
		packet, err := c.ReadPacket()
		if err != nil {
			log.Warnf("failed to read handshake packet: %v\n", err)

			return
		}

		switch state {
		case StateHandshake:
			if packet.ID != SB_Handshake {
				return
			}

			br := bytes.NewReader(packet.Data)
			rd := ByteReader{br}

			// Handshake: protocol varint, server address (string), port (unsigned short), next state (varint)
			proto, _ := ReadVarInt(&rd)
			addr, _ := ReadString(&rd, 255)

			_ = addr

			io.ReadFull(rd, make([]byte, 2))

			next, _ := ReadVarInt(&rd)

			switch next {
			case StateStatus:
				state = StateStatus
			case StateLogin:
				state = StateLogin

				HandleLogin(ctx, c, cfg, int(proto))

				return
			default:
				return
			}
		case StateStatus:
			switch packet.ID {
			case SB_StatusRequest:
				var info ServerInfo

				info.Version.Name = "1.21.11"
				info.Version.Protocol = ProtocolVersion
				info.Players.Max = cfg.MaxPlayers
				info.Players.Online = 0
				info.Description = map[string]any{"text": cfg.Motd}
				info.EnforcesSecureChat = true
				info.PreviewsChat = false

				js, _ := json.Marshal(info)

				WriteStatusResponse(c, string(js))
			case SB_StatusPing:
				// payload is int64
				if len(packet.Data) != 8 {
					return
				}

				WriteStatusPong(c, packet.Data)
			default:
				return
			}
		default:
			return
		}
	}
}

func ReadString(r *ByteReader, max int) (string, error) {
	ln, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}

	if int(ln) < 0 || int(ln) > max*3+3 {
		return "", errors.New("string too long")
	}

	buf := make([]byte, int(ln))

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return "", err
	}

	return string(buf), nil
}

func WriteStatusResponse(c *MCConnection, jsonStr string) error {
	var buf bytes.Buffer

	err := WriteString(&buf, jsonStr)
	if err != nil {
		return err
	}

	return c.WritePacket(Packet{
		ID:   CB_StatusResponse,
		Data: buf.Bytes(),
	})
}

func WriteStatusPong(c *MCConnection, payload []byte) error {
	return c.WritePacket(Packet{
		ID:   CB_StatusPong,
		Data: payload,
	})
}
