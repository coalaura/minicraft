package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
	"github.com/coalaura/plain"
)

func TestRuntimeTicksSharedWorldTime(t *testing.T) {
	world := game.NewOverworld(nil)

	runtime := NewRuntime(world)

	world.SetTime(18000, false)

	state := runtime.TickWorld()
	if state.Age != 1 || state.DayTime != 18000 || state.DayCycle {
		t.Fatalf("fixed world tick = %+v", state)
	}

	world.SetTime(6000, true)

	state = runtime.TickWorld()
	if state.Age != 1 || state.DayTime != 6001 || !state.DayCycle {
		t.Fatalf("cycling world tick = %+v", state)
	}
}

func TestInitialPlayStateSendsWorldTimeBeforeChunks(t *testing.T) {
	world := game.NewOverworld(nil)

	world.SetTime(18000, false)

	runtime := NewRuntime(world)

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	configuredDistance := int32(config.MinRenderDistance)

	session.Config = &config.Config{Server: config.ServerConfig{MaxPlayers: new(1), RenderDistance: &configuredDistance}}
	session.Log = plain.New()

	runtime.AssignEntityID(session)

	err := session.sendInitialPlayState()
	if err != nil {
		t.Fatalf("send initial play state: %v", err)
	}

	packets := connection.packets(t)
	if len(packets) < 3 || packets[0].ID != protocol.ClientboundPlayLoginID || packets[1].ID != protocol.ClientboundUpdateTimeID {
		t.Fatalf("initial packet IDs = %v", connection.packetIDs(t))
	}

	reader := protocol.NewPacketReader(packets[1].Data)

	age := reader.Long()
	dayTime := reader.Long()
	cycling := reader.Bool()

	if age != 0 || dayTime != 18000 || cycling {
		t.Fatalf("initial time = age %d time %d cycling %v", age, dayTime, cycling)
	}
}
