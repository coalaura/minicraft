package server

import (
	"context"
	"errors"
	"io"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	DefaultViewDistance       = 10
	DefaultSimulationDistance = 10
)

func (s *Session) handlePlay(ctx context.Context) error {
	s.Log.Printf("[play] %s - entering play state\n", s.Conn.RemoteAddr())

	s.Runtime.AssignEntityID(s)
	defer s.Runtime.LeaveSession(s)

	playCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go s.keepAliveLoop(playCtx)

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

		s.handleMovePlayerPosition(move)
	case protocol.ServerboundMovePlayerPositionRotationID:
		move, err := protocol.DecodeMovePlayerPositionRotation(packet.Data)
		if err != nil {
			return err
		}

		s.handleMovePlayerPositionRotation(move)
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
	case protocol.ServerboundPlayerLoadedID:
		s.handlePlayerLoaded()
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
		MaxPlayers:         int32(s.Config.MaxPlayers),
		ViewDistance:       DefaultViewDistance,
		SimulationDistance: DefaultSimulationDistance,

		Spawn: protocol.SpawnInfo{
			DimensionType:    0,
			Dimension:        s.Runtime.World.Name,
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

	return s.Conn.WritePacket(protocol.Packet{
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

	err = s.Conn.WritePacket(protocol.Packet{
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
	s.chunksPerTick = batch.ChunksPerTick
}

func (s *Session) handleMovePlayerPosition(move protocol.MovePlayerPosition) {
	s.playerMx.Lock()
	s.Player.Position = game.Position{X: move.X, Y: move.Y, Z: move.Z}
	s.Player.OnGround = move.OnGround
	s.playerMx.Unlock()

	err := s.updatePlayerChunk()
	if err != nil {
		s.Log.Warnf("[play] failed to update center chunk: %v\n", err)
	}
}

func (s *Session) handleMovePlayerPositionRotation(move protocol.MovePlayerPositionRotation) {
	s.playerMx.Lock()
	s.Player.Position = game.Position{X: move.X, Y: move.Y, Z: move.Z}
	s.Player.Rotation = game.Rotation{Yaw: move.Yaw, Pitch: move.Pitch}
	s.Player.OnGround = move.OnGround
	s.playerMx.Unlock()

	err := s.updatePlayerChunk()
	if err != nil {
		s.Log.Warnf("[play] failed to update center chunk: %v\n", err)
	}
}

func (s *Session) handleMovePlayerRotation(move protocol.MovePlayerRotation) {
	s.playerMx.Lock()
	defer s.playerMx.Unlock()

	s.Player.Rotation = game.Rotation{Yaw: move.Yaw, Pitch: move.Pitch}
	s.Player.OnGround = move.OnGround
}

func (s *Session) handleMovePlayerStatus(move protocol.MovePlayerStatus) {
	s.playerMx.Lock()
	defer s.playerMx.Unlock()

	s.Player.OnGround = move.OnGround
}

func (s *Session) handlePlayerLoaded() {
	s.Log.Printf("[play] player loaded\n")
}
