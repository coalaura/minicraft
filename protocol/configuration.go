package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/coalaura/minicraft/config"
)

func HandleConfiguration(ctx context.Context, c *MCConnection, cfg *config.Config, uuid, name string) {
	c.Print("configuration", "processing configuration")

	var (
		sentKnownPacks bool
		sentFinish     bool
	)

	for {
		packet, err := c.ReadPacket()
		if err != nil {
			if err == io.EOF {
				c.Print("configuration", "client disconnected")

				return
			}

			HandleConfigurationError(c, "failed to read packet", err)

			return
		}

		switch packet.ID {
		case SB_CustomPayload:
			handleConfigCustomPayload(c, packet)

			if !sentKnownPacks {
				err = sendConfigKnownPacks(c)
				if err != nil {
					HandleConfigurationError(c, "failed to send known packs", err)

					return
				}

				sentKnownPacks = true
			}

		case SB_ClientInformation:
			c.Print("configuration", "received client information")

			if !sentKnownPacks {
				err = sendConfigKnownPacks(c)
				if err != nil {
					HandleConfigurationError(c, "failed to send known packs", err)

					return
				}

				sentKnownPacks = true
			}

		case SB_KnownPacks:
			c.Print("configuration", "received known packs response")

			if !sentFinish {
				err = sendConfigRegistryData(c, "minecraft:dimension_type", []string{"minecraft:overworld"})
				if err != nil {
					HandleConfigurationError(c, "failed to send registry data", err)

					return
				}

				err = sendConfigRegistryData(c, "minecraft:worldgen/biome", []string{"minecraft:plains"})
				if err != nil {
					HandleConfigurationError(c, "failed to send registry data", err)

					return
				}

				c.Print("configuration", "sent registry data")

				err = c.WritePacket(Packet{ID: CB_FinishConfiguration, Data: []byte{}})
				if err != nil {
					HandleConfigurationError(c, "failed to send finish configuration", err)

					return
				}

				c.Print("configuration", "sent finish configuration")

				sentFinish = true
			}

		case SB_AcknowledgeFinishConfig:
			c.Print("configuration", "client acknowledged finish config, switching to play")

			HandlePlay(ctx, c, cfg, uuid, name)

			return

		case SB_KeepAliveConfig:
			// Keep-alive response; nothing to do.

		default:
			c.Warn("configuration", "unhandled packet id: 0x%02X", packet.ID)

			return
		}
	}
}

func handleConfigCustomPayload(c *MCConnection, p *Packet) {
	channel, err := ReadStringBytes(p.Data)
	if err != nil {
		c.Warn("configuration", "failed to read custom payload channel: %v", err)

		return
	}

	c.Print("configuration", "custom payload channel: %s (data len=%d)", channel, len(p.Data))
}

func sendConfigKnownPacks(c *MCConnection) error {
	var b bytes.Buffer

	// Prefixed array of known packs: [count][namespace, id, version]*
	// The vanilla client requires the minecraft:core pack for a normal login sequence.
	err := WriteVarInt(&b, 1)
	if err != nil {
		return err
	}

	err = WriteString(&b, "minecraft")
	if err != nil {
		return err
	}

	err = WriteString(&b, "core")
	if err != nil {
		return err
	}

	err = WriteString(&b, "1.21.11")
	if err != nil {
		return err
	}

	return c.WritePacket(Packet{ID: CB_KnownPacks, Data: b.Bytes()})
}

func sendConfigRegistryData(c *MCConnection, registryID string, entryIDs []string) error {
	var b bytes.Buffer

	err := WriteString(&b, registryID)
	if err != nil {
		return err
	}

	err = WriteVarInt(&b, int32(len(entryIDs)))
	if err != nil {
		return err
	}

	for _, entryID := range entryIDs {
		err = WriteString(&b, entryID)
		if err != nil {
			return err
		}

		// Prefixed Optional NBT: 0 = omitted, client sources data from the
		// selected known pack (minecraft:core).
		err = b.WriteByte(0)
		if err != nil {
			return err
		}
	}

	return c.WritePacket(Packet{ID: CB_RegistryData, Data: b.Bytes()})
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
