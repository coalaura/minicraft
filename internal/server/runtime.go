package server

import (
	"errors"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type Runtime struct {
	World               *game.World
	BlockMutationPolicy BlockMutationPolicy
	AllowBlockBreaking  bool
	AllowBlockPlacing   bool
	ChatEnabled         bool
	ChatFormat          string
	ChatJoinMessage     string
	ChatLeaveMessage    string

	// Keep each profile/entity transition ordered as one lifecycle event.
	lifecycleMu               sync.Mutex
	chatMu                    sync.Mutex
	certificateVerifier       ChatCertificateVerifier
	nextChatIndex             int32
	clockMu                   sync.RWMutex
	nowFunc                   func() time.Time
	worldMutationMu           sync.Mutex
	commandRandomMu           sync.Mutex
	commandRandom             func(int) int
	commands                  *commandRegistry
	blockMutationDeliveryTail chan struct{}
	activeChunksMu            sync.RWMutex
	activeChunks              map[LoadedChunk]*activeChunkReference
	sessionActiveChunks       map[*Session]map[LoadedChunk]struct{}
	mu                        sync.RWMutex
	nextEntityID              int32
	reserved                  int
	sessions                  map[*Session]*game.Player
	connectedSessions         map[*Session]struct{}
	shuttingDown              bool
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
	initialDelivery := make(chan struct{})

	close(initialDelivery)

	runtime := &Runtime{
		World:                     world,
		AllowBlockBreaking:        true,
		AllowBlockPlacing:         true,
		blockMutationDeliveryTail: initialDelivery,
		commandRandom:             rand.IntN,
		activeChunks:              make(map[LoadedChunk]*activeChunkReference),
		sessionActiveChunks:       make(map[*Session]map[LoadedChunk]struct{}),
		sessions:                  make(map[*Session]*game.Player),
		connectedSessions:         make(map[*Session]struct{}),
	}

	runtime.commands = newCommandRegistry(runtime)

	return runtime
}

func (r *Runtime) registerConnectedSession(session *Session) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.shuttingDown {
		return false
	}

	r.connectedSessions[session] = struct{}{}

	return true
}

func (r *Runtime) unregisterConnectedSession(session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.connectedSessions, session)
}

func (r *Runtime) DisconnectAll(reason string) error {
	r.mu.Lock()
	if r.shuttingDown {
		r.mu.Unlock()

		return nil
	}

	r.shuttingDown = true

	sessions := make([]*Session, 0, len(r.connectedSessions))

	for session := range r.connectedSessions {
		sessions = append(sessions, session)
	}

	r.mu.Unlock()

	disconnectErrors := make(chan error, len(sessions))

	var disconnects sync.WaitGroup

	for _, session := range sessions {
		disconnects.Go(func() {
			err := session.disconnectForShutdown(reason)
			if err != nil {
				disconnectErrors <- err
			}
		})
	}

	disconnects.Wait()
	close(disconnectErrors)

	errorsBySession := make([]error, 0, len(disconnectErrors))

	for err := range disconnectErrors {
		errorsBySession = append(errorsBySession, err)
	}

	return errors.Join(errorsBySession...)
}

func (r *Runtime) now() time.Time {
	r.clockMu.RLock()
	defer r.clockMu.RUnlock()

	if r.nowFunc != nil {
		return r.nowFunc()
	}

	return time.Now()
}

func (r *Runtime) SetChatClock(now func() time.Time) {
	r.clockMu.Lock()
	defer r.clockMu.Unlock()

	r.nowFunc = now
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
	r.worldMutationMu.Lock()
	defer r.worldMutationMu.Unlock()

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	session.updatePlayerState(func(player *game.Player) bool {
		player.Pose = r.calculatedPlayerPose(*player)

		return true
	})

	existing := r.snapshotSessions()

	player := session.snapshotPlayer()

	players := make([]playerInfoSnapshot, 0, len(existing)+1)
	visible := make([]*Session, 0, len(existing))

	players = append(players, session.playerInfoSnapshot())

	for _, other := range existing {
		otherPlayer := other.snapshotPlayer()

		players = append(players, other.playerInfoSnapshot())

		if playersVisible(player, otherPlayer, session.renderDistance()) {
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
		err = other.sendPlayerInfo([]playerInfoSnapshot{session.playerInfoSnapshot()})
		if err != nil {
			other.Log.Warnf("[play] failed to announce player info: %v\n", err)
		}

		if !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
			continue
		}

		err = other.sendPlayerEntity(player)
		if err != nil {
			other.Log.Warnf("[play] failed to announce player entity: %v\n", err)
		}
	}

	if r.ChatJoinMessage != "" {
		r.broadcastSystemMessageLocked(formatChatMessage(r.ChatJoinMessage, player.Name, ""))
	}

	return nil
}

func (r *Runtime) LeaveSession(session *Session) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.releaseSessionActiveChunks(session)

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
		if playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
			err := other.sendPlayerRemoval(player)
			if err != nil {
				other.Log.Warnf("[play] failed to remove player entity: %v\n", err)
			}
		}

		err := other.sendPlayerInfoRemoval(player)
		if err != nil {
			other.Log.Warnf("[play] failed to remove player info: %v\n", err)
		}
	}

	if r.ChatLeaveMessage != "" {
		r.broadcastSystemMessageLocked(formatChatMessage(r.ChatLeaveMessage, player.Name, ""))
	}
}

func (r *Runtime) BroadcastPlayerChat(session *Session, message string) {
	r.chatMu.Lock()
	defer r.chatMu.Unlock()

	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	if !active || !r.ChatEnabled {
		return
	}

	player := session.snapshotPlayer()

	logAcceptedChat(session, player.Name, message)

	formatted := formatChatMessage(r.ChatFormat, player.Name, message)

	r.broadcastSystemMessageLocked(formatted)
}

func (r *Runtime) BroadcastVerifiedPlayerChat(session *Session, verified verifiedPlayerChat) {
	r.chatMu.Lock()
	defer r.chatMu.Unlock()

	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	if !active || !r.ChatEnabled {
		return
	}

	player := session.snapshotPlayer()

	logAcceptedChat(session, player.Name, verified.message.Message)

	if r.ChatFormat != config.DefaultChatFormat {
		formatted := formatChatMessage(r.ChatFormat, player.Name, verified.message.Message)

		r.broadcastSystemMessageLocked(formatted)

		return
	}

	globalIndex := r.nextChatIndex
	r.nextChatIndex++

	for _, recipient := range r.snapshotSessions() {
		err := recipient.sendVerifiedPlayerChat(globalIndex, player.UUID, player.Name, verified)
		if err != nil {
			recipient.Log.Warnf("[play] failed to send signed player chat: %v\n", err)
		}
	}
}

func (r *Runtime) BroadcastChatSession(session *Session) {
	r.chatMu.Lock()
	defer r.chatMu.Unlock()

	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	if !active {
		return
	}

	player := session.snapshotPlayer()

	entry := protocol.PlayerInfo{
		UUID:        player.UUID,
		ChatSession: session.chatSessionSnapshot(),
	}

	update := protocol.PlayerInfoUpdate{
		Actions: protocol.PlayerInfoActionInitializeChat,
		Players: []protocol.PlayerInfo{entry},
	}

	for _, recipient := range r.snapshotSessions() {
		err := recipient.writePacket(protocol.ClientboundPlayerInfoUpdateID, update)
		if err != nil {
			recipient.Log.Warnf("[play] failed to update player chat session: %v\n", err)
		}
	}
}

func (r *Runtime) BroadcastSystemMessage(message string) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.broadcastSystemMessageLocked(message)
}

func (r *Runtime) broadcastSystemMessageLocked(message string) {
	for _, session := range r.snapshotSessions() {
		err := session.sendSystemMessage(message)
		if err != nil {
			session.Log.Warnf("[play] failed to send system message: %v\n", err)
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
	r.worldMutationMu.Lock()
	defer r.worldMutationMu.Unlock()

	r.updatePlayerMetadata(session, func(player *game.Player) bool {
		previousSneaking := player.Sneaking
		previousPose := player.Pose

		player.Sneaking = sneaking
		player.Pose = r.calculatedPlayerPose(*player)

		if previousSneaking == player.Sneaking && previousPose == player.Pose {
			return false
		}

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
	r.worldMutationMu.Lock()
	defer r.worldMutationMu.Unlock()

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	session.playerMx.Lock()
	previous := *session.Player

	update(session.Player)

	session.Player.Pose = r.calculatedPlayerPose(*session.Player)

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
			err := other.sendPlayerEntity(current)
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

			if previous.Pose != current.Pose {
				err = other.sendPlayerMetadata(current)
				if err != nil {
					other.Log.Warnf("[play] failed to update player pose: %v\n", err)
				}
			}
		}

		wasVisibleToPlayer := playersVisible(previous, otherPlayer, session.renderDistance())
		isVisibleToPlayer := playersVisible(current, otherPlayer, session.renderDistance())

		switch {
		case !wasVisibleToPlayer && isVisibleToPlayer:
			err := session.sendPlayerEntity(otherPlayer)
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

func formatChatMessage(format, player, message string) string {
	formatted := strings.ReplaceAll(format, "{player}", player)

	return strings.ReplaceAll(formatted, "{message}", message)
}

func logAcceptedChat(session *Session, player, message string) {
	if session.Log != nil {
		session.Log.Printf("[chat] <%s> %s\n", player, message)
	}
}
