package server

import (
	"context"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type Session struct {
	Conn   *protocol.Connection
	Config *config.Config
	Log    Logger

	Player *game.Player
	World  *game.World

	nextTeleportID int32
	chunksPerTick  float32
}

func NewSession(conn *protocol.Connection, cfg *config.Config, log Logger) *Session {
	return &Session{
		Conn:   conn,
		Config: cfg,
		Log:    log,

		World: game.NewOverworld(),
	}
}

func (s *Session) Run(ctx context.Context) error {
	return s.handleHandshake(ctx)
}

func (s *Session) nextTeleport() int32 {
	s.nextTeleportID++

	return s.nextTeleportID
}
