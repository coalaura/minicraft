package server

import (
	"context"
	"sync"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type Session struct {
	Conn    *protocol.Connection
	Config  *config.Config
	Log     Logger
	Runtime *Runtime

	Player   *game.Player
	playerMx sync.RWMutex
	writeMx  sync.Mutex

	chunkMx        sync.Mutex
	centerChunk    LoadedChunk
	hasChunkCenter bool
	loadedChunks   map[LoadedChunk]struct{}

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

func (s *Session) snapshotPlayer() game.Player {
	s.playerMx.RLock()
	defer s.playerMx.RUnlock()

	player := *s.Player

	player.Properties = append([]game.ProfileProperty(nil), s.Player.Properties...)

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
