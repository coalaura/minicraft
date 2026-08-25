package server

import (
	"context"
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
	"github.com/coalaura/plain"
)

func TestGameTickRate(t *testing.T) {
	if gameTickInterval != time.Second/20 {
		t.Fatalf("game tick interval = %s, want 20 TPS", gameTickInterval)
	}
}

func TestGameLoopStopsWhenCancelled(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})

	go func() {
		runtime.RunGameLoop(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("game loop did not stop after cancellation")
	}
}

func TestRuntimeTicksSharedWorldTime(t *testing.T) {
	world := game.NewOverworld(nil)

	runtime := NewRuntime(world)

	world.SetTime(18000, false)

	state := runtime.Tick()
	if state.Age != 1 || state.DayTime != 18000 || state.DayCycle {
		t.Fatalf("fixed world tick = %+v", state)
	}

	world.SetTime(6000, true)

	state = runtime.Tick()
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
