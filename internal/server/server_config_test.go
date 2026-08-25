package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
	"github.com/coalaura/plain"
)

type secureChatAdvertisementTestCase struct {
	name            string
	onlineMode      bool
	installVerifier bool
	wantSecureChat  bool
}

type statusSecureChatResponse struct {
	EnforcesSecureChat bool `json:"enforcesSecureChat"`
}

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
	writer.UUID("00000000-0000-0000-0000-000000000001")

	err := writer.Err()
	if err != nil {
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
			MaxPlayers:      new(1),
			DefaultGameMode: "creative",
		}},
		Runtime: runtime,
	}

	err = session.handleLogin(t.Context())
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
			MaxPlayers:     new(8),
			RenderDistance: new(int32(14)),
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

	reader.Byte()
	reader.Bool()
	reader.Bool()

	hasDeathLocation := reader.Bool()
	if hasDeathLocation {
		reader.String(32767)
		reader.BlockPosition()
	}

	reader.VarInt()
	reader.VarInt()

	enforcesSecureChat := reader.Bool()

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

	if enforcesSecureChat {
		t.Fatal("play login advertises enforced secure chat")
	}

	if reader.Len() != 0 {
		t.Fatalf("play login has %d unread bytes", reader.Len())
	}
}

func TestSecureChatAdvertisementMatchesModeAndVerifierReadiness(t *testing.T) {
	tests := []secureChatAdvertisementTestCase{
		{name: "offline", onlineMode: false, installVerifier: false, wantSecureChat: false},
		{name: "online ready", onlineMode: true, installVerifier: true, wantSecureChat: true},
		{name: "online uninitialized", onlineMode: true, installVerifier: false, wantSecureChat: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{Name: "minecraft:overworld"})

			if test.installVerifier {
				runtime.SetChatCertificateVerifier(&MinecraftCertificateVerifier{})
			}

			cfg := &config.Config{Server: config.ServerConfig{OnlineMode: test.onlineMode}}

			statusConnection := &recordingConnection{}

			status := &Session{
				Conn:    protocol.NewConnection(statusConnection, nil),
				Config:  cfg,
				Runtime: runtime,
			}

			err := status.sendStatusResponse()
			if err != nil {
				t.Fatalf("send status response: %v", err)
			}

			statusSecureChat := decodeStatusSecureChat(t, statusConnection.packets(t)[0].Data)

			loginConnection := &recordingConnection{}

			login := &Session{
				Conn:    protocol.NewConnection(loginConnection, nil),
				Config:  cfg,
				Runtime: runtime,
				Player:  &game.Player{},
			}

			err = login.sendPlayLogin()
			if err != nil {
				t.Fatalf("send play login: %v", err)
			}

			loginSecureChat := decodePlayLoginSecureChat(t, loginConnection.packets(t)[0].Data)
			if statusSecureChat != test.wantSecureChat || loginSecureChat != test.wantSecureChat {
				t.Fatalf("secure chat status=%t login=%t, want %t", statusSecureChat, loginSecureChat, test.wantSecureChat)
			}
		})
	}
}

func decodeStatusSecureChat(t *testing.T, data []byte) bool {
	t.Helper()

	reader := protocol.NewPacketReader(data)

	responseJSON := reader.String(32767)

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode status packet: %v", err)
	}

	var response statusSecureChatResponse

	err = json.Unmarshal([]byte(responseJSON), &response)
	if err != nil {
		t.Fatalf("decode status JSON: %v", err)
	}

	return response.EnforcesSecureChat
}

func decodePlayLoginSecureChat(t *testing.T, data []byte) bool {
	t.Helper()

	reader := protocol.NewPacketReader(data)

	reader.Int()
	reader.Bool()

	for range reader.VarInt() {
		reader.String(32767)
	}

	reader.VarInt()
	reader.VarInt()
	reader.VarInt()
	reader.Bool()
	reader.Bool()
	reader.Bool()
	reader.VarInt()
	reader.String(32767)
	reader.Long()
	reader.Byte()
	reader.Byte()
	reader.Bool()
	reader.Bool()

	if reader.Bool() {
		reader.String(32767)
		reader.BlockPosition()
	}

	reader.VarInt()
	reader.VarInt()

	secureChat := reader.Bool()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode play login: %v", err)
	}

	if reader.Len() != 0 {
		t.Fatalf("play login has %d unread bytes", reader.Len())
	}

	return secureChat
}
