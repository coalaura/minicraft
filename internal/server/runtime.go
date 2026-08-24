package server

import (
	"sync"

	"github.com/coalaura/minicraft/internal/game"
)

type Runtime struct {
	World *game.World

	// Keep each profile/entity transition ordered as one lifecycle event.
	lifecycleMu  sync.Mutex
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

func (r *Runtime) AssignEntityID(session *Session) int32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	session.playerMx.Lock()
	defer session.playerMx.Unlock()

	if session.Player.EntityID != 0 {
		return session.Player.EntityID
	}

	r.nextEntityID++
	session.Player.EntityID = r.nextEntityID

	return session.Player.EntityID
}

func (r *Runtime) JoinSession(session *Session) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	existing := r.snapshotSessions()

	players := make([]game.Player, 0, len(existing)+1)

	players = append(players, session.snapshotPlayer())

	for _, other := range existing {
		players = append(players, other.snapshotPlayer())
	}

	err := session.sendPlayerInfo(players)
	if err != nil {
		return err
	}

	err = session.sendPlayerMetadata(session.snapshotPlayer())
	if err != nil {
		return err
	}

	for _, other := range existing {
		err = session.sendPlayerEntity(other.snapshotPlayer())
		if err != nil {
			return err
		}
	}

	r.mu.Lock()
	r.sessions[session] = session.Player
	r.mu.Unlock()

	player := session.snapshotPlayer()

	for _, other := range existing {
		err = other.sendPlayerInfo([]game.Player{player})
		if err != nil {
			other.Log.Warnf("[play] failed to announce player info: %v\n", err)

			continue
		}

		err = other.sendPlayerEntity(player)
		if err != nil {
			other.Log.Warnf("[play] failed to announce player entity: %v\n", err)
		}
	}

	return nil
}

func (r *Runtime) LeaveSession(session *Session) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.mu.Lock()
	_, active := r.sessions[session]

	if active {
		delete(r.sessions, session)
	}

	r.mu.Unlock()

	if !active {
		return
	}

	player := session.snapshotPlayer()

	for _, other := range r.snapshotSessions() {
		err := other.sendPlayerRemoval(player)
		if err != nil {
			other.Log.Warnf("[play] failed to remove player: %v\n", err)
		}
	}
}

func (r *Runtime) UpdateSkinParts(session *Session, skinParts byte) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	player, changed := session.setSkinParts(skinParts)
	if !changed {
		return
	}

	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	if !active {
		return
	}

	for _, other := range r.snapshotSessions() {
		err := other.sendPlayerMetadata(player)
		if err != nil {
			other.Log.Warnf("[play] failed to update player skin parts: %v\n", err)
		}
	}
}

func (r *Runtime) UpdateSneaking(session *Session, sneaking bool) {
	r.updatePlayerMetadata(session, func(player *game.Player) bool {
		if player.Sneaking == sneaking {
			return false
		}

		player.Sneaking = sneaking

		return true
	})
}

func (r *Runtime) UpdateSprinting(session *Session, sprinting bool) {
	r.updatePlayerMetadata(session, func(player *game.Player) bool {
		if player.Sprinting == sprinting {
			return false
		}

		player.Sprinting = sprinting

		return true
	})
}

func (r *Runtime) BroadcastPlayerAnimation(session *Session, animation byte) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	if !active {
		return
	}

	player := session.snapshotPlayer()

	for _, other := range r.snapshotSessions() {
		if other == session {
			continue
		}

		err := other.sendPlayerAnimation(player, animation)
		if err != nil {
			other.Log.Warnf("[play] failed to send player animation: %v\n", err)
		}
	}
}

func (r *Runtime) updatePlayerMetadata(session *Session, update func(*game.Player) bool) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	player, changed := session.updatePlayerState(update)
	if !changed {
		return
	}

	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	if !active {
		return
	}

	for _, other := range r.snapshotSessions() {
		if other == session {
			continue
		}

		err := other.sendPlayerMetadata(player)
		if err != nil {
			other.Log.Warnf("[play] failed to update player metadata: %v\n", err)
		}
	}
}

func (r *Runtime) updatePlayerMovement(session *Session, update func(*game.Player)) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	session.playerMx.Lock()
	previous := *session.Player

	update(session.Player)

	current := *session.Player
	session.playerMx.Unlock()

	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	if !active {
		return
	}

	for _, other := range r.snapshotSessions() {
		if other == session {
			continue
		}

		err := other.sendPlayerMovement(previous, current)
		if err != nil {
			other.Log.Warnf("[play] failed to update player movement: %v\n", err)
		}
	}
}

func (r *Runtime) snapshotSessions() []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sessions := make([]*Session, 0, len(r.sessions))

	for session := range r.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

func (r *Runtime) PlayerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.sessions)
}
