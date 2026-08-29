package server

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const entityPositionScale = 4096

type PacketEncoder interface {
	Encode(*protocol.PacketWriter)
}

type playerInfoSnapshot struct {
	player      game.Player
	chatSession *protocol.ChatSession
}

func (s *Session) playerInfoSnapshot() playerInfoSnapshot {
	return playerInfoSnapshot{
		player:      s.snapshotPlayer(),
		chatSession: s.chatSessionSnapshot(),
	}
}

func (s *Session) sendPlayerInfo(players []playerInfoSnapshot) error {
	entries := make([]protocol.PlayerInfo, 0, len(players))

	for _, snapshot := range players {
		player := snapshot.player

		entries = append(entries, protocol.PlayerInfo{
			UUID:        player.UUID,
			Name:        player.Name,
			Properties:  player.Properties,
			GameMode:    int32(player.GameMode),
			Listed:      true,
			ChatSession: snapshot.chatSession,
		})
	}

	actions := byte(protocol.PlayerInfoActionAddPlayer | protocol.PlayerInfoActionUpdateGameMode | protocol.PlayerInfoActionUpdateListed)

	if s.secureChatEnforced() {
		actions |= protocol.PlayerInfoActionInitializeChat
	}

	update := protocol.PlayerInfoUpdate{
		Actions: actions,
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

	err = s.sendPlayerMetadata(player)
	if err != nil {
		return err
	}

	slots := visibleEquipmentSlots(player)
	if len(slots) == 0 {
		return nil
	}

	return s.sendPlayerEquipment(player, slots...)
}

func (s *Session) sendPlayerEquipment(player game.Player, slots ...byte) error {
	equipment := make([]protocol.EquipmentEntry, 0, len(slots))

	for _, slot := range slots {
		var stack game.ItemStack

		switch slot {
		case protocol.EquipmentSlotMainHand:
			held := player.Inventory.Held(player.SelectedHotbarSlot)

			if held != nil {
				stack = held.Clone()
			}
		case protocol.EquipmentSlotOffHand:
			stack = player.Inventory.Offhand.Clone()
		case protocol.EquipmentSlotFeet:
			stack = player.Inventory.Armor[3].Clone()
		case protocol.EquipmentSlotLegs:
			stack = player.Inventory.Armor[2].Clone()
		case protocol.EquipmentSlotChest:
			stack = player.Inventory.Armor[1].Clone()
		case protocol.EquipmentSlotHead:
			stack = player.Inventory.Armor[0].Clone()
		default:
			continue
		}

		equipment = append(equipment, protocol.EquipmentEntry{Slot: slot, Item: stack})
	}

	if len(equipment) == 0 {
		return nil
	}

	return s.writePacket(protocol.ClientboundEntityEquipmentID, protocol.EntityEquipment{
		EntityID:  player.EntityID,
		Equipment: equipment,
	})
}

func visibleEquipmentSlots(player game.Player) []byte {
	var slots []byte

	held := player.Inventory.Held(player.SelectedHotbarSlot)
	if held != nil && !held.Empty() {
		slots = append(slots, protocol.EquipmentSlotMainHand)
	}

	if !player.Inventory.Offhand.Empty() {
		slots = append(slots, protocol.EquipmentSlotOffHand)
	}

	for index := len(player.Inventory.Armor) - 1; index >= 0; index-- {
		if !player.Inventory.Armor[index].Empty() {
			slots = append(slots, protocol.EquipmentSlotFeet+byte(len(player.Inventory.Armor)-1-index))
		}
	}

	return slots
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

	if player.Swimming {
		flags |= protocol.EntityFlagSwimming
	}

	pose := protocol.EntityPoseStanding

	switch player.Pose {
	case game.PlayerPoseCrouching:
		pose = protocol.EntityPoseCrouching
	case game.PlayerPoseCrawling:
		pose = protocol.EntityPoseSwimming
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

	return s.writePacket(protocol.ClientboundRemoveEntitiesID, entities)
}

func (s *Session) sendPlayerInfoRemoval(player game.Player) error {
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
