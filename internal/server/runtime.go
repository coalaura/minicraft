package server

import (
	"sync"

	"github.com/coalaura/minicraft/internal/game"
)

type Runtime struct {
	World *game.World

	mu           sync.RWMutex
	nextEntityID int32
	sessions     map[*Session]*game.Player
}

func NewRuntime(world *game.World) *Runtime {
	return &Runtime{
		World:    world,
		sessions: make(map[*Session]*game.Player),
	}
}

func (r *Runtime) RegisterSession(session *Session) int32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	player, ok := r.sessions[session]
	if ok {
		return player.EntityID
	}

	r.nextEntityID++
	session.Player.EntityID = r.nextEntityID

	r.sessions[session] = session.Player

	return session.Player.EntityID
}

func (r *Runtime) RemoveSession(session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessions, session)
}

func (r *Runtime) PlayerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.sessions)
}
