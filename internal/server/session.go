package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const connectionReadTimeout = 30 * time.Second

type Session struct {
	Conn    *protocol.Connection
	Config  *config.Config
	Log     Logger
	Runtime *Runtime

	Player        *game.Player
	playerMx      sync.RWMutex
	writeMx       sync.Mutex
	chatMx        sync.Mutex
	chatState     *sessionChatState
	inventoryDrag inventoryDragState

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

func NewSession(conn *protocol.Connection, cfg *config.Config, runtime *Runtime, log Logger) *Session {
	return &Session{
		Conn:    conn,
		Config:  cfg,
		Log:     log,
		Runtime: runtime,
	}
}

func (s *Session) Run(ctx context.Context) error {
	return s.handleHandshake(ctx)
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
