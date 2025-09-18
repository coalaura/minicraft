package protocol

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/coalaura/minicraft/config"
)

func HandleConfiguration(ctx context.Context, c *MCConnection, cfg *config.Config, uuid, name string) {
	c.Print("configuration", "processing configuration")

	err := c.WritePacket(Packet{
		ID:   CB_FinishConfiguration,
		Data: []byte{},
	})

	if err != nil {
		HandleConfigurationError(c, "failed to send finish configuration", err)

		return
	}

	for {
		packet, err := c.ReadPacket()
		if err != nil {
			HandleConfigurationError(c, "failed to read packet", err)

			return
		}

		log.Println(packet)

		switch packet.ID {
		case SB_ClientInformation:
			log.Println("[config] received client information")
		case SB_AcknowledgeFinishConfig:
			log.Println("[config] client acknowledged finish config, switching to play")

			HandlePlay(ctx, c, cfg, uuid, name)

			return
		default:
			log.Warnf("[config] unhandled packet id: 0x%02X", packet.ID)

			return
		}
	}
}

func HandleConfigurationError(c *MCConnection, msg string, err error) {
	c.Warn("configuration", "%s: %v", msg, err)

	SendConfigurationDisconnect(c, "Something went wrong")
}

func SendConfigurationDisconnect(c *MCConnection, reason string) {
	js, _ := json.Marshal(map[string]any{
		"text": reason,
	})

	var b bytes.Buffer

	err := WriteString(&b, string(js))
	if err != nil {
		log.Warnf("failed to write configuration disconnect: %v\n", err)
		return
	}

	c.WritePacket(Packet{
		ID:   CB_DisconnectConfig,
		Data: b.Bytes(),
	})
}
