package server

import (
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
	"github.com/coalaura/plain"
)

func TestOfflinePlayerUsesDefaultGameMode(t *testing.T) {
	session := &Session{
		Config:  &config.Config{Server: config.ServerConfig{DefaultGameMode: "adventure"}},
		Runtime: NewRuntime(&game.World{Spawn: game.Position{X: 1.5, Y: 80, Z: -2.5}}),
	}

	err := session.handleOfflineLogin(protocol.LoginStart{Name: "Laura"})
	if err != nil {
		t.Fatalf("offline login: %v", err)
	}

	if session.Player.GameMode != game.GameModeAdventure {
		t.Fatalf("game mode = %d, want adventure", session.Player.GameMode)
	}

	if session.Player.Position != session.Runtime.World.Spawn {
		t.Fatalf("position = %+v, want spawn %+v", session.Player.Position, session.Runtime.World.Spawn)
	}
}

func TestFullServerRejectsLogin(t *testing.T) {
	runtime := NewRuntime(&game.World{})
	if !runtime.ReservePlayerSlot(1) {
		t.Fatal("reserve initial slot")
	}

	connection := &recordingConnection{}

	var writer protocol.PacketWriter

	writer.String("Laura")
	if err := writer.Err(); err != nil {
		t.Fatalf("encode login start: %v", err)
	}

	connection.queuePacket(t, protocol.Packet{
		ID:   protocol.ServerboundLoginStartID,
		Data: writer.Buffer.Bytes(),
	})

	session := &Session{
		Conn: protocol.NewConnection(connection, nil),
		Log:  plain.New(),
		Config: &config.Config{Server: config.ServerConfig{
			MaxPlayers:      pointerTo(1),
			DefaultGameMode: "creative",
		}},
		Runtime: runtime,
	}

	err := session.handleLogin(t.Context())
	if err == nil || !strings.Contains(err.Error(), "Server is full") {
		t.Fatalf("login error = %v, want server full", err)
	}

	packets := connection.packets(t)
	if len(packets) != 1 || packets[0].ID != protocol.ClientboundLoginDisconnectID {
		t.Fatalf("disconnect packets = %v", connection.packetIDs(t))
	}

	reader := protocol.NewPacketReader(packets[0].Data)

	reason := reader.String(32767)

	err = reader.Err()
	if err != nil {
		t.Fatalf("decode disconnect: %v", err)
	}

	if !strings.Contains(reason, "Server is full") {
		t.Fatalf("disconnect reason = %q", reason)
	}
}

func TestPlayLoginAdvertisesConfiguredWorldAndDistance(t *testing.T) {
	connection := &recordingConnection{}

	session := &Session{
		Conn: protocol.NewConnection(connection, nil),
		Config: &config.Config{Server: config.ServerConfig{
			MaxPlayers:     pointerTo(8),
			RenderDistance: pointerTo(int32(14)),
		}},
		Runtime: NewRuntime(&game.World{
			Name:     "minecraft:overworld",
			Seed:     -1234,
			SeaLevel: 64,
		}),
		Player: &game.Player{EntityID: 7, GameMode: game.GameModeSpectator},
	}

	err := session.sendPlayLogin()
	if err != nil {
		t.Fatalf("send play login: %v", err)
	}

	packets := connection.packets(t)

	reader := protocol.NewPacketReader(packets[0].Data)

	reader.Int()
	reader.Bool()

	worldCount := reader.VarInt()

	for range worldCount {
		reader.String(32767)
	}

	maxPlayers := reader.VarInt()
	viewDistance := reader.VarInt()
	simulationDistance := reader.VarInt()

	reader.Bool()
	reader.Bool()
	reader.Bool()
	reader.VarInt()
	reader.String(32767)

	seed := reader.Long()
	gameMode := reader.Byte()

	err = reader.Err()
	if err != nil {
		t.Fatalf("decode play login: %v", err)
	}

	if maxPlayers != 8 || viewDistance != 14 || simulationDistance != 14 {
		t.Fatalf("login policy = max %d view %d simulation %d", maxPlayers, viewDistance, simulationDistance)
	}

	if seed != -1234 || gameMode != byte(game.GameModeSpectator) {
		t.Fatalf("login world = seed %d game mode %d", seed, gameMode)
	}
}
