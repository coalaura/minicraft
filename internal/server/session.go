package server

import (
	"context"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type Session struct {
	Conn    *protocol.Connection
	Config  *config.Config
	Log     Logger
	Runtime *Runtime

	Player *game.Player

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

func (s *Session) nextTeleport() int32 {
	s.nextTeleportID++

	return s.nextTeleportID
}
