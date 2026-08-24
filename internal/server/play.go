package server

import (
	"context"
	"errors"
	"io"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func (s *Session) handlePlay(ctx context.Context) error {
	s.Log.Printf("[play] %s - entering play state\n", s.Conn.RemoteAddr())

	s.Runtime.AssignEntityID(s)
	defer s.Runtime.LeaveSession(s)

	playCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go s.keepAliveLoop(playCtx)

	s.startChunkStream(playCtx)

	err := s.sendInitialPlayState()
	if err != nil {
		return err
	}

	player := s.snapshotPlayer()

	s.Log.Printf("[play] player %s joined the world\n", player.Name)

	for {
		packet, err := s.Conn.ReadPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.Log.Printf("[play] client disconnected\n")

				return nil
			}

			s.Log.Warnf("[play] failed to read packet: %v\n", err)

			return nil
		}

		err = s.handlePlayPacket(packet)
		if err != nil {
			return err
		}
	}
}

func (s *Session) handlePlayPacket(packet *protocol.Packet) error {
	switch packet.ID {
	case protocol.ServerboundConfirmTeleportID:
		teleport, err := protocol.DecodeConfirmTeleport(packet.Data)
		if err != nil {
			return err
		}

		s.handleConfirmTeleport(teleport)
	case protocol.ServerboundChunkBatchReceivedID:
		batch, err := protocol.DecodeChunkBatchReceived(packet.Data)
		if err != nil {
			return err
		}

		s.handleChunkBatchReceived(batch)
	case protocol.ServerboundClientTickEndID:
		// End of client tick; nothing to do for now.
	case protocol.ServerboundPlayClientInformationID:
		information, err := protocol.DecodeClientInformation(packet.Data)
		if err != nil {
			return err
		}

		s.Runtime.UpdateSkinParts(s, information.SkinParts)

		s.Log.Printf("[play] received client information\n")
	case protocol.ServerboundPlayKeepAliveID:
		keepAlive, err := protocol.DecodePlayKeepAliveResponse(packet.Data)
		if err != nil {
			return err
		}

		s.handleKeepAlive(keepAlive)
	case protocol.ServerboundMovePlayerPositionID:
		move, err := protocol.DecodeMovePlayerPosition(packet.Data)
		if err != nil {
			return err
		}

		return s.handleMovePlayerPosition(move)
	case protocol.ServerboundMovePlayerPositionRotationID:
		move, err := protocol.DecodeMovePlayerPositionRotation(packet.Data)
		if err != nil {
			return err
		}

		return s.handleMovePlayerPositionRotation(move)
	case protocol.ServerboundMovePlayerRotationID:
		move, err := protocol.DecodeMovePlayerRotation(packet.Data)
		if err != nil {
			return err
		}

		s.handleMovePlayerRotation(move)
	case protocol.ServerboundMovePlayerStatusID:
		move, err := protocol.DecodeMovePlayerStatus(packet.Data)
		if err != nil {
			return err
		}

		s.handleMovePlayerStatus(move)
	case protocol.ServerboundPlayerActionID:
		action, err := protocol.DecodePlayerAction(packet.Data)
		if err != nil {
			return err
		}

		return s.handlePlayerAction(action)
	case protocol.ServerboundPlayerCommandID:
		command, err := protocol.DecodePlayerCommand(packet.Data)
		if err != nil {
			return err
		}

		s.handlePlayerCommand(command)
	case protocol.ServerboundPlayerInputID:
		input, err := protocol.DecodePlayerInput(packet.Data)
		if err != nil {
			return err
		}

		s.handlePlayerInput(input)
	case protocol.ServerboundPlayerLoadedID:
		s.handlePlayerLoaded()
	case protocol.ServerboundSetHeldItemID:
		selection, err := protocol.DecodeSetHeldItem(packet.Data)
		if err != nil {
			return err
		}

		s.handleSetHeldItem(selection)
	case protocol.ServerboundSetCreativeModeSlotID:
		update, err := protocol.DecodeSetCreativeModeSlot(packet.Data)
		if err != nil {
			return err
		}

		s.handleSetCreativeModeSlot(update)
	case protocol.ServerboundSwingArmID:
		swing, err := protocol.DecodeSwingArm(packet.Data)
		if err != nil {
			return err
		}

		s.handleSwingArm(swing)
	case protocol.ServerboundUseItemOnID:
		interaction, err := protocol.DecodeUseItemOn(packet.Data)
		if err != nil {
			return err
		}

		return s.handleUseItemOn(interaction)
	default:
		s.Log.Printf("[play] unhandled packet id: 0x%02X\n", packet.ID)
	}

	return nil
}

func (s *Session) sendInitialPlayState() error {
	err := s.sendPlayLogin()
	if err != nil {
		return err
	}

	err = s.sendInitialChunks()
	if err != nil {
		return err
	}

	err = s.sendPlayerPosition()
	if err != nil {
		return err
	}

	return s.Runtime.JoinSession(s)
}

func (s *Session) sendPlayLogin() error {
	var wr protocol.PacketWriter

	player := s.snapshotPlayer()

	login := protocol.PlayLogin{
		EntityID: player.EntityID,
		Worlds: []string{
			s.Runtime.World.Name,
		},
		MaxPlayers:         int32(s.Config.MaxPlayers()),
		ViewDistance:       s.renderDistance(),
		SimulationDistance: s.renderDistance(),

		Spawn: protocol.SpawnInfo{
			DimensionType:    0,
			Dimension:        s.Runtime.World.Name,
			Seed:             s.Runtime.World.Seed,
			GameMode:         byte(player.GameMode),
			PreviousGameMode: 0xFF,
			SeaLevel:         s.Runtime.World.SeaLevel,
		},

		EnforcesSecureChat: true,
	}

	login.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundPlayLoginID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendPlayerPosition() error {
	var wr protocol.PacketWriter

	player := s.snapshotPlayer()

	position := protocol.PlayerPosition{
		TeleportID: s.nextTeleport(),

		X: player.Position.X,
		Y: player.Position.Y,
		Z: player.Position.Z,

		Yaw:   player.Rotation.Yaw,
		Pitch: player.Rotation.Pitch,
	}

	position.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	err = s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundPlayerPositionID,
		Data: wr.Buffer.Bytes(),
	})

	if err != nil {
		return err
	}

	s.Log.Printf("[play] sent position and look\n")

	return nil
}

func (s *Session) handleConfirmTeleport(teleport protocol.ConfirmTeleport) {
	s.Log.Printf("[play] confirmed teleport (id=%d)\n", teleport.TeleportID)
}

func (s *Session) handleChunkBatchReceived(batch protocol.ChunkBatchReceived) {
	s.chunkMx.Lock()
	s.chunksPerTick = batch.ChunksPerTick

	s.chunkBatchAwaiting = false
	s.chunkFeedbackTimedOut = false

	s.ensureChunkStreamLocked()

	notify := s.chunkStreamNotify
	s.chunkMx.Unlock()

	notifyChunkStream(notify)
}

func (s *Session) handleMovePlayerPosition(move protocol.MovePlayerPosition) error {
	s.Runtime.updatePlayerMovement(s, func(player *game.Player) {
		player.Position = game.Position{X: move.X, Y: move.Y, Z: move.Z}
		player.OnGround = move.Flags.OnGround()
	})

	err := s.updatePlayerChunks()
	if err != nil {
		s.Log.Warnf("[play] failed to stream chunks: %v\n", err)
	}

	return err
}

func (s *Session) handleMovePlayerPositionRotation(move protocol.MovePlayerPositionRotation) error {
	s.Runtime.updatePlayerMovement(s, func(player *game.Player) {
		player.Position = game.Position{X: move.X, Y: move.Y, Z: move.Z}
		player.Rotation = game.Rotation{Yaw: move.Yaw, Pitch: move.Pitch}
		player.OnGround = move.Flags.OnGround()
	})

	err := s.updatePlayerChunks()
	if err != nil {
		s.Log.Warnf("[play] failed to stream chunks: %v\n", err)
	}

	return err
}

func (s *Session) handleMovePlayerRotation(move protocol.MovePlayerRotation) {
	s.Runtime.updatePlayerMovement(s, func(player *game.Player) {
		player.Rotation = game.Rotation{Yaw: move.Yaw, Pitch: move.Pitch}
		player.OnGround = move.Flags.OnGround()
	})
}

func (s *Session) handleMovePlayerStatus(move protocol.MovePlayerStatus) {
	s.Runtime.updatePlayerMovement(s, func(player *game.Player) {
		player.OnGround = move.Flags.OnGround()
	})
}

func (s *Session) handlePlayerCommand(command protocol.PlayerCommand) {
	switch command.Action {
	case protocol.PlayerCommandStartSprinting:
		s.Runtime.UpdateSprinting(s, true)
	case protocol.PlayerCommandStopSprinting:
		s.Runtime.UpdateSprinting(s, false)
	}
}

func (s *Session) handlePlayerAction(action protocol.PlayerAction) error {
	switch action.Status {
	case protocol.PlayerActionStartDestroyBlock, protocol.PlayerActionStopDestroyBlock:
		result, err := s.Runtime.MutateBlock(s, BlockMutationBreak, action.Position, game.Air)
		if err != nil {
			return err
		}

		if !result.Allowed || !result.Changed {
			state, stateErr := protocolBlockState(result.Block)
			if stateErr != nil {
				return stateErr
			}

			err = s.sendBlockUpdate(action.Position, state)
			if err != nil {
				return err
			}
		}
	case protocol.PlayerActionAbortDestroyBlock:
	case protocol.PlayerActionDropAllItems:
		s.handleDropHeldItem(true)
	case protocol.PlayerActionDropItem:
		s.handleDropHeldItem(false)
	default:
	}

	return s.sendBlockChangedAck(action.Sequence)
}

func (s *Session) sendBlockChangedAck(sequence int32) error {
	var wr protocol.PacketWriter

	acknowledgement := protocol.BlockChangedAck{Sequence: sequence}
	acknowledgement.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundBlockChangedAckID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) handlePlayerInput(input protocol.PlayerInput) {
	s.Runtime.UpdateSneaking(s, input.Flags&protocol.PlayerInputSneak != 0)
}

func (s *Session) handleSwingArm(swing protocol.SwingArm) {
	animation := byte(protocol.EntityAnimationSwingMainHand)

	switch swing.Hand {
	case protocol.MainHand:
	case protocol.OffHand:
		animation = protocol.EntityAnimationSwingOffHand
	default:
		return
	}

	s.Runtime.BroadcastPlayerAnimation(s, animation)
}

func (s *Session) handlePlayerLoaded() {
	s.Log.Printf("[play] player loaded\n")
}
