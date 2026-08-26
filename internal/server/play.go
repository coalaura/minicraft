package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const MaxPlayerCoordinate = 30_000_000

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
		packet, err := s.readPacket()
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
	case protocol.ServerboundChatCommandID:
		command, err := protocol.DecodeChatCommand(packet.Data)
		if err != nil {
			return err
		}

		return s.Runtime.commands.execute(playerCommandSource{session: s}, command.Command)
	case protocol.ServerboundSignedChatCommandID:
		command, err := protocol.DecodeSignedChatCommand(packet.Data)
		if err != nil {
			if s.secureChatEnforced() {
				return s.disconnectChatViolation(fmt.Sprintf("malformed signed command: %v", err))
			}

			return err
		}

		err = s.handleSignedCommandAcknowledgement(command)
		if err != nil {
			return s.disconnectChatViolation(fmt.Sprintf("invalid signed command acknowledgement: %v", err))
		}

		return s.Runtime.commands.execute(playerCommandSource{session: s}, command.Command)
	case protocol.ServerboundChatMessageID:
		message, err := protocol.DecodeChatMessage(packet.Data)
		if err != nil {
			if s.secureChatEnforced() {
				return s.disconnectChatViolation(fmt.Sprintf("malformed chat message: %v", err))
			}

			return err
		}

		if !s.Runtime.ChatEnabled {
			return nil
		}

		if !s.secureChatEnforced() {
			s.Runtime.BroadcastPlayerChat(s, message.Message)

			return nil
		}

		verified, err := s.verifyPlayerChat(message)
		if err != nil {
			return s.disconnectChatViolation(fmt.Sprintf("chat validation failed: %v", err))
		}

		s.Runtime.BroadcastVerifiedPlayerChat(s, verified)
	case protocol.ServerboundChatAckID:
		acknowledgement, err := protocol.DecodeChatAck(packet.Data)
		if err != nil {
			if s.secureChatEnforced() {
				return s.disconnectChatViolation(fmt.Sprintf("malformed chat acknowledgement: %v", err))
			}

			return err
		}

		err = s.handleChatAck(acknowledgement)
		if err != nil {
			return s.disconnectChatViolation(fmt.Sprintf("invalid chat acknowledgement: %v", err))
		}
	case protocol.ServerboundChatSessionUpdateID:
		update, err := protocol.DecodeChatSessionUpdate(packet.Data)
		if err != nil {
			if s.secureChatEnforced() {
				return s.disconnectChatViolation(fmt.Sprintf("malformed chat session: %v", err))
			}

			return err
		}

		return s.handleChatSessionUpdate(update)
	case protocol.ServerboundChunkBatchReceivedID:
		batch, err := protocol.DecodeChunkBatchReceived(packet.Data)
		if err != nil {
			return err
		}

		s.handleChunkBatchReceived(batch)
	case protocol.ServerboundClientTickEndID:
		return protocol.DecodeEmptyPacket(packet.Data, "client tick end")
	case protocol.ServerboundPlayClientInformationID:
		information, err := protocol.DecodeClientInformation(packet.Data)
		if err != nil {
			return err
		}

		s.Runtime.UpdateSkinParts(s, information.SkinParts)

		s.Log.Printf("[play] received client information\n")
	case protocol.ServerboundCommandSuggestionsID:
		request, err := protocol.DecodeCommandSuggestionRequest(packet.Data)
		if err != nil {
			return err
		}

		suggestions := s.Runtime.commands.suggestions(playerCommandSource{session: s}, request.Text)

		suggestions.TransactionID = request.TransactionID

		return s.writePacket(protocol.ClientboundCommandSuggestionsID, suggestions)
	case protocol.ServerboundContainerClickID:
		click, err := protocol.DecodeContainerClick(packet.Data)
		if err != nil {
			return err
		}

		return s.handleContainerClick(click)
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

		return s.handleMovePlayerRotation(move)
	case protocol.ServerboundMovePlayerStatusID:
		move, err := protocol.DecodeMovePlayerStatus(packet.Data)
		if err != nil {
			return err
		}

		s.handleMovePlayerStatus(move)
	case protocol.ServerboundPickItemFromBlockID:
		pick, err := protocol.DecodePickItemFromBlock(packet.Data)
		if err != nil {
			return err
		}

		s.handlePickItemFromBlock(pick)
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
		err := protocol.DecodeEmptyPacket(packet.Data, "player loaded")
		if err != nil {
			return err
		}

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

	err = s.sendTimeUpdate(s.Runtime.World.Time())
	if err != nil {
		return err
	}

	err = s.writePacket(protocol.ClientboundDeclareCommandsID, s.Runtime.commands.declaration())
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

	err = s.sendPlayerInventory()
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

		EnforcesSecureChat: s.secureChatEnforced(),
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

func (s *Session) sendSystemMessage(content string) error {
	var wr protocol.PacketWriter

	message := protocol.SystemChat{Content: content}
	message.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundSystemChatID,
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
	if !validPlayerPosition(move.X, move.Y, move.Z) {
		return fmt.Errorf("invalid player position")
	}

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
	if !validPlayerPosition(move.X, move.Y, move.Z) || !validPlayerRotation(move.Yaw, move.Pitch) {
		return fmt.Errorf("invalid player position or rotation")
	}

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

func (s *Session) handleMovePlayerRotation(move protocol.MovePlayerRotation) error {
	if !validPlayerRotation(move.Yaw, move.Pitch) {
		return fmt.Errorf("invalid player rotation")
	}

	s.Runtime.updatePlayerMovement(s, func(player *game.Player) {
		player.Rotation = game.Rotation{Yaw: move.Yaw, Pitch: move.Pitch}
		player.OnGround = move.Flags.OnGround()
	})

	return nil
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
			return s.resynchronizeBlocks([]game.BlockPosition{action.Position}, action.Sequence)
		}
	case protocol.PlayerActionAbortDestroyBlock:
	case protocol.PlayerActionDropAllItems:
		s.handleDropHeldItem(true)
	case protocol.PlayerActionDropItem:
		s.handleDropHeldItem(false)
	case protocol.PlayerActionSwapWithOffhand:
		s.handleSwapWithOffhand()
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

func validPlayerPosition(x, y, z float64) bool {
	coordinates := [...]float64{x, y, z}

	for _, coordinate := range coordinates {
		if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) || math.Abs(coordinate) > MaxPlayerCoordinate {
			return false
		}
	}

	return true
}

func validPlayerRotation(yaw, pitch float32) bool {
	return !math.IsNaN(float64(yaw)) && !math.IsInf(float64(yaw), 0) &&
		!math.IsNaN(float64(pitch)) && !math.IsInf(float64(pitch), 0)
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
