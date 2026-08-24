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
	reserved     int
	sessions     map[*Session]*game.Player
}

func (r *Runtime) ReservePlayerSlot(maxPlayers int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.reserved >= maxPlayers {
		return false
	}

	r.reserved++

	return true
}

func (r *Runtime) ReleasePlayerSlot() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.reserved > 0 {
		r.reserved--
	}
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

	player := session.snapshotPlayer()

	players := make([]game.Player, 0, len(existing)+1)
	visible := make([]*Session, 0, len(existing))

	players = append(players, player)

	for _, other := range existing {
		otherPlayer := other.snapshotPlayer()
		if playersVisible(player, otherPlayer, session.renderDistance()) {
			players = append(players, otherPlayer)
			visible = append(visible, other)
		}
	}

	err := session.sendPlayerInfo(players)
	if err != nil {
		return err
	}

	err = session.sendPlayerMetadata(session.snapshotPlayer())
	if err != nil {
		return err
	}

	for _, other := range visible {
		err = session.sendPlayerEntity(other.snapshotPlayer())
		if err != nil {
			return err
		}
	}

	r.mu.Lock()
	r.sessions[session] = session.Player
	r.mu.Unlock()

	for _, other := range existing {
		if !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
			continue
		}

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
		if !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
			continue
		}

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
		if other != session && !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
			continue
		}

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

		if !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
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

		if !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
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

		otherPlayer := other.snapshotPlayer()

		wasVisibleToOther := playersVisible(otherPlayer, previous, other.renderDistance())
		isVisibleToOther := playersVisible(otherPlayer, current, other.renderDistance())

		switch {
		case !wasVisibleToOther && isVisibleToOther:
			err := other.sendPlayerAppearance(current)
			if err != nil {
				other.Log.Warnf("[play] failed to show player: %v\n", err)
			}
		case wasVisibleToOther && !isVisibleToOther:
			err := other.sendPlayerRemoval(current)
			if err != nil {
				other.Log.Warnf("[play] failed to hide player: %v\n", err)
			}
		case isVisibleToOther:
			err := other.sendPlayerMovement(previous, current)
			if err != nil {
				other.Log.Warnf("[play] failed to update player movement: %v\n", err)
			}
		}

		wasVisibleToPlayer := playersVisible(previous, otherPlayer, session.renderDistance())
		isVisibleToPlayer := playersVisible(current, otherPlayer, session.renderDistance())

		switch {
		case !wasVisibleToPlayer && isVisibleToPlayer:
			err := session.sendPlayerAppearance(otherPlayer)
			if err != nil {
				session.Log.Warnf("[play] failed to show player: %v\n", err)
			}
		case wasVisibleToPlayer && !isVisibleToPlayer:
			err := session.sendPlayerRemoval(otherPlayer)
			if err != nil {
				session.Log.Warnf("[play] failed to hide player: %v\n", err)
			}
		}
	}
}

func playersVisible(observer, target game.Player, renderDistance int32) bool {
	observerX := int64(chunkCoordinate(observer.Position.X))
	observerZ := int64(chunkCoordinate(observer.Position.Z))

	targetX := int64(chunkCoordinate(target.Position.X))
	targetZ := int64(chunkCoordinate(target.Position.Z))

	distance := int64(renderDistance)

	return abs64(observerX-targetX) <= distance && abs64(observerZ-targetZ) <= distance
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}

	return value
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
