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
	connectionReadTimeout             = 30 * time.Second
	shutdownWriteTimeout              = 2 * time.Second
	maximumRetainedPacketWriterBuffer = 64 * 1024
)

type Session struct {
	Conn    *protocol.Connection
	Config  *config.Config
	Log     Logger
	Runtime *Runtime

	Player          *game.Player
	offlineProfiles *offlineProfileResolver
	playerMx        sync.RWMutex
	writeMx         sync.Mutex
	packetWriter    protocol.PacketWriter
	chatMx          sync.Mutex
	chatState       *sessionChatState
	inventoryMenu   *menu
	containerMenu   *menu
	nextWindowID    int32
	protocolState   int32
	shuttingDown    bool
	mining          miningState

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
	entityTrackMu         sync.Mutex
	trackedEntities       map[int32]struct{}

	nextTeleportID int32
	chunksPerTick  float32
}

type playerView struct {
	EntityID int32
	UUID     string
	Name     string

	Position game.Position
	Rotation game.Rotation
	Velocity game.Velocity

	GameMode game.GameMode
	Pose     game.PlayerPose

	Health             float32
	MaxHealth          float32
	Absorption         float32
	FoodLevel          int32
	Saturation         float32
	AirSupply          int32
	RemainingFireTicks int32
	SkinParts          byte

	Dead               bool
	DeathEntityRemoved bool
	OnGround           bool
	Sneaking           bool
	Sprinting          bool
	Swimming           bool
}

type playerInventoryJournal struct {
	before  [game.PlayerInventorySlots]game.ItemStack
	touched uint64
}

type playerInventoryMutation struct {
	entityID           int32
	gameMode           game.GameMode
	selectedHotbarSlot int
	journal            playerInventoryJournal
}

func (view playerView) collisionBox() game.AABB {
	player := game.Player{Position: view.Position, Pose: view.Pose}

	return player.CollisionBox()
}

func (view playerView) eyePosition() game.Position {
	player := game.Player{Position: view.Position, Pose: view.Pose}

	return player.EyePosition()
}

func (view playerView) withinBlockInteractionRange(position game.BlockPosition, buffer float64) bool {
	player := game.Player{Position: view.Position, Pose: view.Pose}

	return player.IsWithinBlockInteractionRange(position, buffer)
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

	return s.Player.Clone()
}

func (s *Session) playerView() playerView {
	s.playerMx.RLock()
	defer s.playerMx.RUnlock()

	player := s.Player

	return playerView{
		EntityID:           player.EntityID,
		UUID:               player.UUID,
		Name:               player.Name,
		Position:           player.Position,
		Rotation:           player.Rotation,
		Velocity:           player.Velocity,
		GameMode:           player.GameMode,
		Pose:               player.Pose,
		Health:             player.Health,
		MaxHealth:          player.MaxHealth,
		Absorption:         player.Absorption,
		FoodLevel:          player.FoodLevel,
		Saturation:         player.Saturation,
		AirSupply:          player.AirSupply,
		RemainingFireTicks: player.RemainingFireTicks,
		SkinParts:          player.SkinParts,
		Dead:               player.Dead,
		DeathEntityRemoved: player.DeathEntityRemoved,
		OnGround:           player.OnGround,
		Sneaking:           player.Sneaking,
		Sprinting:          player.Sprinting,
		Swimming:           player.Swimming,
	}
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

	return s.Player.Clone(), changed
}

func (s *Session) mutatePlayer(update func(*game.Player) bool) bool {
	s.playerMx.Lock()
	defer s.playerMx.Unlock()

	return update(s.Player)
}

func (s *Session) setCreativeInventorySlot(mutation *playerInventoryMutation, slot int, stack game.ItemStack) bool {
	s.playerMx.Lock()
	defer s.playerMx.Unlock()

	mutation.initialize(s.Player)

	if s.Player.GameMode != game.GameModeCreative {
		return false
	}

	return mutation.journal.set(&s.Player.Inventory, slot, stack)
}

func (s *Session) insertPickedUpItem(mutation *playerInventoryMutation, stack game.ItemStack) (game.ItemStack, bool) {
	s.playerMx.Lock()
	defer s.playerMx.Unlock()

	mutation.initialize(s.Player)

	remaining := stack
	moveItemEntityIntoPlayerInventory(&s.Player.Inventory, &remaining, &mutation.journal)

	return remaining, !remaining.Equal(stack)
}

func (mutation *playerInventoryMutation) initialize(player *game.Player) {
	*mutation = playerInventoryMutation{
		entityID:           player.EntityID,
		gameMode:           player.GameMode,
		selectedHotbarSlot: player.SelectedHotbarSlot,
	}
}

func (journal *playerInventoryJournal) set(inventory *game.PlayerInventory, slot int, stack game.ItemStack) bool {
	target := inventory.Slot(slot)
	if target == nil || target.Equal(stack) {
		return false
	}

	bit := uint64(1) << slot
	if journal.touched&bit == 0 {
		journal.before[slot] = *target
		journal.touched |= bit
	}

	*target = stack

	return true
}

func (journal playerInventoryJournal) touchedSlot(slot int) bool {
	return slot >= 0 && slot < game.PlayerInventorySlots && journal.touched&(uint64(1)<<slot) != 0
}

func NewSession(conn *protocol.Connection, cfg *config.Config, runtime *Runtime, log Logger) *Session {
	return &Session{
		Conn:            conn,
		Config:          cfg,
		Log:             log,
		Runtime:         runtime,
		offlineProfiles: defaultOfflineProfileResolver,
	}
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
