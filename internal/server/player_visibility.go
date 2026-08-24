package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type PacketEncoder interface {
	Encode(*protocol.PacketWriter)
}

func (s *Session) sendPlayerInfo(players []game.Player) error {
	entries := make([]protocol.PlayerInfo, 0, len(players))

	for _, player := range players {
		entries = append(entries, protocol.PlayerInfo{
			UUID:       player.UUID,
			Name:       player.Name,
			Properties: player.Properties,
			GameMode:   int32(player.GameMode),
			Listed:     true,
		})
	}

	update := protocol.PlayerInfoUpdate{
		Actions: protocol.PlayerInfoActionAddPlayer | protocol.PlayerInfoActionUpdateGameMode | protocol.PlayerInfoActionUpdateListed,
		Players: entries,
	}

	return s.writePacket(protocol.ClientboundPlayerInfoUpdateID, update)
}

func (s *Session) sendPlayerEntity(player game.Player) error {
	entity := protocol.AddEntity{
		EntityID: player.EntityID,
		UUID:     player.UUID,
		Type:     protocol.PlayerEntityType,

		X: player.Position.X,
		Y: player.Position.Y,
		Z: player.Position.Z,

		VelocityX: protocolVelocity(player.Velocity.X),
		VelocityY: protocolVelocity(player.Velocity.Y),
		VelocityZ: protocolVelocity(player.Velocity.Z),

		Pitch:   protocolAngle(player.Rotation.Pitch),
		Yaw:     protocolAngle(player.Rotation.Yaw),
		HeadYaw: protocolAngle(player.Rotation.Yaw),
	}

	err := s.writePacket(protocol.ClientboundAddEntityID, entity)
	if err != nil {
		return err
	}

	return s.sendPlayerMetadata(player)
}

func (s *Session) sendPlayerMetadata(player game.Player) error {
	metadata := protocol.EntityMetadataSkinParts{
		EntityID:  player.EntityID,
		SkinParts: player.SkinParts,
	}

	return s.writePacket(protocol.ClientboundEntityMetadataID, metadata)
}

func (s *Session) sendPlayerRemoval(player game.Player) error {
	entities := protocol.RemoveEntities{EntityIDs: []int32{player.EntityID}}

	err := s.writePacket(protocol.ClientboundRemoveEntitiesID, entities)
	if err != nil {
		return err
	}

	info := protocol.PlayerInfoRemove{UUIDs: []string{player.UUID}}

	return s.writePacket(protocol.ClientboundPlayerInfoRemoveID, info)
}

func (s *Session) writePacket(packetID int32, encoder PacketEncoder) error {
	var wr protocol.PacketWriter

	encoder.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.Conn.WritePacket(protocol.Packet{
		ID:   packetID,
		Data: wr.Buffer.Bytes(),
	})
}

func protocolVelocity(velocity float64) int16 {
	return int16(math.Round(max(-3.9, min(3.9, velocity)) * 8000))
}

func protocolAngle(angle float32) byte {
	return byte(int32(angle * 256 / 360))
}
