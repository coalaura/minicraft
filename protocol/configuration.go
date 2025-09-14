package protocol

import (
	"bytes"
	"context"

	"github.com/coalaura/minicraft/config"
)

func HandleConfiguration(ctx context.Context, c *MCConnection, cfg *config.Config, uuid, name string) {
	var buf bytes.Buffer

	err := WriteUUIDString(&buf, uuid)
	if err != nil {
		HandleLoginError(c, "failed to write uuid", err)

		return
	}

	err = WriteString(&buf, name)
	if err != nil {
		HandleLoginError(c, "failed to write username", err)

		return
	}

	// properties (VarInt 0 for none)
	err = WriteVarInt(&buf, 0)
	if err != nil {
		HandleLoginError(c, "failed to write properties", err)

		return
	}

	// has secure profile (false for now, no signed chat)
	buf.WriteByte(0)

	err = c.WritePacket(Packet{
		ID:   CB_Hello,
		Data: buf.Bytes(),
	})

	if err != nil {
		HandleLoginError(c, "failed to write hello packet", err)

		return
	}

	for {
		packet, err := c.ReadPacket()
		if err != nil {
			HandleLoginError(c, "failed to read configuration packet", err)
			return
		}

		switch packet.ID {
		case SB_ClientInformation:
			// Client preferences (ignore for now)
		case SB_AcknowledgeFinishConfig:
			// Client finished config -> go to Play
			HandlePlay(ctx, c, cfg, uuid, name)
			return
		default:
			log.Warnf("[config] unhandled packet id: %x", packet.ID)
		}

		// Server must periodically send FinishConfiguration until client acks
		c.WritePacket(Packet{
			ID:   CB_FinishConfiguration,
			Data: []byte{},
		})
	}
}
