package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const entityPositionScale = 4096

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

		VelocityX: player.Velocity.X,
		VelocityY: player.Velocity.Y,
		VelocityZ: player.Velocity.Z,

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

func (s *Session) sendPlayerAppearance(player game.Player) error {
	err := s.sendPlayerInfo([]game.Player{player})
	if err != nil {
		return err
	}

	return s.sendPlayerEntity(player)
}

func (s *Session) sendPlayerMovement(previous, current game.Player) error {
	deltaX, xRelative := protocolPositionDelta(previous.Position.X, current.Position.X)
	deltaY, yRelative := protocolPositionDelta(previous.Position.Y, current.Position.Y)
	deltaZ, zRelative := protocolPositionDelta(previous.Position.Z, current.Position.Z)

	positionChanged := previous.Position != current.Position

	yaw := protocolAngle(current.Rotation.Yaw)
	pitch := protocolAngle(current.Rotation.Pitch)

	yawChanged := protocolAngle(previous.Rotation.Yaw) != yaw
	rotationChanged := yawChanged || protocolAngle(previous.Rotation.Pitch) != pitch

	onGroundChanged := previous.OnGround != current.OnGround

	if !positionChanged && !rotationChanged && !onGroundChanged {
		return nil
	}

	var err error

	switch {
	case positionChanged && (!xRelative || !yRelative || !zRelative):
		position := protocol.SynchronizeEntityPosition{
			EntityID: current.EntityID,

			X: current.Position.X,
			Y: current.Position.Y,
			Z: current.Position.Z,

			VelocityX: current.Velocity.X,
			VelocityY: current.Velocity.Y,
			VelocityZ: current.Velocity.Z,

			Yaw:   current.Rotation.Yaw,
			Pitch: current.Rotation.Pitch,

			OnGround: current.OnGround,
		}

		err = s.writePacket(protocol.ClientboundSynchronizeEntityPositionID, position)
	case positionChanged && rotationChanged:
		movement := protocol.UpdateEntityPositionRotation{
			EntityID: current.EntityID,
			DeltaX:   deltaX,
			DeltaY:   deltaY,
			DeltaZ:   deltaZ,
			Yaw:      yaw,
			Pitch:    pitch,
			OnGround: current.OnGround,
		}

		err = s.writePacket(protocol.ClientboundUpdateEntityPositionRotationID, movement)
	case positionChanged:
		movement := protocol.UpdateEntityPosition{
			EntityID: current.EntityID,
			DeltaX:   deltaX,
			DeltaY:   deltaY,
			DeltaZ:   deltaZ,
			OnGround: current.OnGround,
		}

		err = s.writePacket(protocol.ClientboundUpdateEntityPositionID, movement)
	case rotationChanged:
		rotation := protocol.UpdateEntityRotation{
			EntityID: current.EntityID,
			Yaw:      yaw,
			Pitch:    pitch,
			OnGround: current.OnGround,
		}

		err = s.writePacket(protocol.ClientboundUpdateEntityRotationID, rotation)
	case onGroundChanged:
		movement := protocol.UpdateEntityPosition{
			EntityID: current.EntityID,
			OnGround: current.OnGround,
		}

		err = s.writePacket(protocol.ClientboundUpdateEntityPositionID, movement)
	}

	if err != nil || !yawChanged {
		return err
	}

	head := protocol.SetHeadRotation{
		EntityID: current.EntityID,
		HeadYaw:  yaw,
	}

	return s.writePacket(protocol.ClientboundSetHeadRotationID, head)
}

func (s *Session) sendPlayerMetadata(player game.Player) error {
	flags := byte(0)

	if player.Sneaking {
		flags |= protocol.EntityFlagSneaking
	}

	if player.Sprinting {
		flags |= protocol.EntityFlagSprinting
	}

	pose := protocol.EntityPoseStanding

	if player.Sneaking {
		pose = protocol.EntityPoseCrouching
	}

	metadata := protocol.EntityMetadata{
		EntityID: player.EntityID,
		Entries: []protocol.EntityMetadataEntry{
			{Index: protocol.EntityFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(flags)},
			{Index: protocol.EntityPoseMetadataIndex, Type: protocol.MetadataTypePose, Value: protocol.MetadataVarInt(pose)},
			{Index: protocol.PlayerSkinPartsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(player.SkinParts)},
		},
	}

	return s.writePacket(protocol.ClientboundEntityMetadataID, metadata)
}

func (s *Session) sendPlayerAnimation(player game.Player, animation byte) error {
	packet := protocol.EntityAnimation{EntityID: player.EntityID, Animation: animation}

	return s.writePacket(protocol.ClientboundEntityAnimationID, packet)
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

	return s.writeRawPacket(protocol.Packet{
		ID:   packetID,
		Data: wr.Buffer.Bytes(),
	})
}

func protocolAngle(angle float32) byte {
	return byte(int32(angle * 256 / 360))
}

func protocolPositionDelta(previous, current float64) (int16, bool) {
	delta := math.Round((current - previous) * entityPositionScale)

	if math.IsNaN(delta) || math.IsInf(delta, 0) || delta < math.MinInt16 || delta > math.MaxInt16 {
		return 0, false
	}

	return int16(delta), true
}
