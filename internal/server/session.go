package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	connectionReadTimeout = 30 * time.Second
	shutdownWriteTimeout  = 2 * time.Second
)

type Session struct {
	Conn    *protocol.Connection
	Config  *config.Config
	Log     Logger
	Runtime *Runtime

	Player           *game.Player
	playerMx         sync.RWMutex
	writeMx          sync.Mutex
	chatMx           sync.Mutex
	chatState        *sessionChatState
	inventoryMenu    *menu
	containerMenu    *menu
	nextWindowID     int32
	preservedCarried []game.ItemStack
	protocolState    int32
	shuttingDown     bool

	chunkMx               sync.Mutex
	centerChunk           LoadedChunk
	hasChunkCenter        bool
	loadedChunks          map[LoadedChunk]struct{}
	queuedChunks          []LoadedChunk
	chunkRevision         uint64
	chunkQueueReady       bool
	chunkBatchAwaiting    bool
	chunkBatchSentAt      time.Time
	chunkFeedbackTimedOut bool
	chunkStreamNotify     chan struct{}
	chunkStreamStarted    bool
	runtimeChunksReleased bool

	nextTeleportID int32
	chunksPerTick  float32
}

func (s *Session) activeMenu() *menu {
	if s.containerMenu == nil {
		s.returnToInventoryMenu()
	}

	return s.containerMenu
}

func (s *Session) returnToInventoryMenu() {
	if s.inventoryMenu == nil {
		s.inventoryMenu = newPlayerInventoryMenu(&s.Player.Inventory)
	}

	if s.containerMenu != nil {
		s.containerMenu.resetDrag()
	}

	s.containerMenu = s.inventoryMenu
}

func (s *Session) allocateWindowID() int32 {
	s.nextWindowID++
	if s.nextWindowID > 100 {
		s.nextWindowID = 1
	}

	return s.nextWindowID
}

func NewSession(conn *protocol.Connection, cfg *config.Config, runtime *Runtime, log Logger) *Session {
	return &Session{
		Conn:    conn,
		Config:  cfg,
		Log:     log,
		Runtime: runtime,
	}
}

func (s *Session) Run(ctx context.Context) error {
	if !s.Runtime.registerConnectedSession(s) {
		return s.Conn.Close()
	}

	defer s.Runtime.unregisterConnectedSession(s)

	return s.handleHandshake(ctx)
}

func (s *Session) setProtocolState(state int32) {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	if s.shuttingDown {
		return
	}

	s.protocolState = state
}

func (s *Session) disconnectForShutdown(reason string) error {
	deadlineErr := s.Conn.SetWriteDeadline(time.Now().Add(shutdownWriteTimeout))

	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.shuttingDown = true

	state := s.protocolState

	var (
		disconnectErr error
		packet        protocol.Packet
	)

	switch state {
	case protocol.StateLogin:
		message, marshalErr := json.Marshal(map[string]string{"text": reason})
		if marshalErr != nil {
			disconnectErr = marshalErr

			break
		}

		var writer protocol.PacketWriter

		writer.String(string(message))

		disconnectErr = writer.Err()
		if disconnectErr == nil {
			packet = protocol.Packet{
				ID:   protocol.ClientboundLoginDisconnectID,
				Data: writer.Buffer.Bytes(),
			}
		}
	case protocol.StateConfiguration:
		packet, disconnectErr = encodeDisconnectPacket(protocol.ClientboundConfigurationDisconnectID, reason)
	case protocol.StatePlay:
		packet, disconnectErr = encodeDisconnectPacket(protocol.ClientboundPlayDisconnectID, reason)
	}

	if disconnectErr == nil && packet.Data != nil {
		disconnectErr = s.Conn.WritePacket(packet)
	}

	closeErr := s.Conn.Close()

	return errors.Join(deadlineErr, disconnectErr, closeErr)
}

func (s *Session) enableEncryption(secret []byte) error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	if s.shuttingDown {
		return net.ErrClosed
	}

	return s.Conn.EnableEncryption(secret)
}

func (s *Session) sendSetCompression(threshold int) error {
	var writer protocol.PacketWriter

	protocol.SetCompression{Threshold: int32(threshold)}.Encode(&writer)

	err := writer.Err()
	if err != nil {
		return err
	}

	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	if s.shuttingDown {
		return net.ErrClosed
	}

	err = s.Conn.WritePacket(protocol.Packet{
		ID:   protocol.ClientboundSetCompressionID,
		Data: writer.Buffer.Bytes(),
	})

	if err != nil {
		return err
	}

	s.Conn.SetCompression(threshold)

	return nil
}

func encodeDisconnectPacket(packetID int32, reason string) (protocol.Packet, error) {
	var writer protocol.PacketWriter

	protocol.PlayDisconnect{Reason: reason}.Encode(&writer)

	err := writer.Err()
	if err != nil {
		return protocol.Packet{}, err
	}

	return protocol.Packet{ID: packetID, Data: writer.Buffer.Bytes()}, nil
}

func (s *Session) renderDistance() int32 {
	if s.Config == nil {
		return config.DefaultRenderDistance
	}

	return s.Config.RenderDistance()
}

func (s *Session) nextTeleport() int32 {
	s.nextTeleportID++

	return s.nextTeleportID
}

func (s *Session) writeRawPacket(packet protocol.Packet) error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	if s.shuttingDown {
		return net.ErrClosed
	}

	return s.Conn.WritePacket(packet)
}

func (s *Session) readPacket() (*protocol.Packet, error) {
	err := s.Conn.SetReadDeadline(time.Now().Add(connectionReadTimeout))
	if err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}

	return s.Conn.ReadPacket()
}

func (s *Session) snapshotPlayer() game.Player {
	s.playerMx.RLock()
	defer s.playerMx.RUnlock()

	player := *s.Player

	player.Properties = append([]game.ProfileProperty(nil), s.Player.Properties...)
	player.Inventory = s.Player.Inventory.Clone()

	return player
}

func (s *Session) setSkinParts(skinParts byte) (game.Player, bool) {
	return s.updatePlayerState(func(player *game.Player) bool {
		if player.SkinParts == skinParts {
			return false
		}

		player.SkinParts = skinParts

		return true
	})
}

func (s *Session) updatePlayerState(update func(*game.Player) bool) (game.Player, bool) {
	s.playerMx.Lock()
	defer s.playerMx.Unlock()

	changed := update(s.Player)

	return *s.Player, changed
}
