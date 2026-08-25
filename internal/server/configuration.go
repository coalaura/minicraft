package server

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/coalaura/minicraft/internal/protocol"
)

func (s *Session) handleConfiguration(ctx context.Context) error {
	s.Log.Printf("[configuration] %s - processing configuration\n", s.Conn.RemoteAddr())

	err := s.sendConfigurationBrand()
	if err != nil {
		return fmt.Errorf("send brand: %w", err)
	}

	var (
		sentKnownPacks bool
		sentFinish     bool
	)

	for {
		packet, err := s.readPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.Log.Printf("[configuration] %s - client disconnected\n", s.Conn.RemoteAddr())

				return nil
			}

			return fmt.Errorf("read configuration packet: %w", err)
		}

		switch packet.ID {
		case protocol.ServerboundConfigurationCustomPayloadID:
			err = s.handleConfigurationCustomPayload(packet)
			if err != nil {
				return err
			}

			if !sentKnownPacks {
				err = s.sendKnownPacks()
				if err != nil {
					return fmt.Errorf("send known packs: %w", err)
				}

				sentKnownPacks = true
			}
		case protocol.ServerboundConfigurationClientInformationID:
			information, decodeErr := protocol.DecodeClientInformation(packet.Data)
			if decodeErr != nil {
				return fmt.Errorf("decode client information: %w", decodeErr)
			}

			s.setSkinParts(information.SkinParts)

			s.Log.Printf("[configuration] %s - received client information\n", s.Conn.RemoteAddr())

			if !sentKnownPacks {
				err = s.sendKnownPacks()
				if err != nil {
					return fmt.Errorf("send known packs: %w", err)
				}

				sentKnownPacks = true
			}
		case protocol.ServerboundConfigurationKnownPacksID:
			s.Log.Printf("[configuration] %s - received known packs response\n", s.Conn.RemoteAddr())

			if !sentFinish {
				err = s.sendRegistries()
				if err != nil {
					return fmt.Errorf("send registry data: %w", err)
				}

				err = s.sendRegistryTags()
				if err != nil {
					return fmt.Errorf("send tags: %w", err)
				}

				s.Log.Printf("[configuration] %s - sent registry data and tags\n", s.Conn.RemoteAddr())

				err = s.sendFinishConfiguration()
				if err != nil {
					return fmt.Errorf("send finish configuration: %w", err)
				}

				s.Log.Printf("[configuration] %s - sent finish configuration\n", s.Conn.RemoteAddr())

				sentFinish = true
			}
		case protocol.ServerboundConfigurationFinishAcknowledgedID:
			err = protocol.DecodeEmptyPacket(packet.Data, "configuration finish acknowledged")
			if err != nil {
				return err
			}

			s.Log.Printf("[configuration] %s - client acknowledged finish config, switching to play\n", s.Conn.RemoteAddr())

			return s.handlePlay(ctx)
		case protocol.ServerboundConfigurationKeepAliveID:
			_, err = protocol.DecodePlayKeepAliveResponse(packet.Data)
			if err != nil {
				return err
			}
		default:
			s.Log.Warnf("[configuration] %s - unhandled packet id: 0x%02X\n", s.Conn.RemoteAddr(), packet.ID)

			return nil
		}
	}
}

func (s *Session) sendConfigurationBrand() error {
	var wr protocol.PacketWriter

	wr.String("minecraft:brand")
	wr.String("minicraft")

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundConfigurationPluginMessageID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) handleConfigurationCustomPayload(packet *protocol.Packet) error {
	rd := protocol.NewPacketReader(packet.Data)

	channel := rd.String(256)

	err := rd.Err()
	if err != nil {
		s.Log.Warnf("[configuration] failed to read custom payload channel: %v\n", err)

		return nil
	}

	s.Log.Printf("[configuration] custom payload channel: %s (data len=%d)\n", channel, len(packet.Data))

	return nil
}

func (s *Session) sendKnownPacks() error {
	var wr protocol.PacketWriter

	packs := protocol.KnownPacks{
		Packs: []protocol.KnownPack{
			{
				Namespace: "minecraft",
				ID:        "core",
				Version:   "1.21.11",
			},
		},
	}

	packs.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundConfigurationKnownPacksID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendRegistries() error {
	for _, registry := range protocol.ConfigurationRegistries {
		var wr protocol.PacketWriter

		data := protocol.RegistryData{Registry: registry}

		data.Encode(&wr)

		err := wr.Err()
		if err != nil {
			return err
		}

		err = s.writeRawPacket(protocol.Packet{
			ID:   protocol.ClientboundConfigurationRegistryDataID,
			Data: wr.Buffer.Bytes(),
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Session) sendRegistryTags() error {
	var wr protocol.PacketWriter

	tags := protocol.UpdateTags{Registries: protocol.ConfigurationTags}

	tags.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundConfigurationUpdateTagsID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendFinishConfiguration() error {
	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundConfigurationFinishID,
		Data: []byte{},
	})
}
