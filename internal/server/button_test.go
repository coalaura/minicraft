package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type buttonTestCase struct {
	name         string
	button       game.Block
	delay        int
	pressSound   game.SoundEvent
	releaseSound game.SoundEvent
}

type staleButtonTickTestCase struct {
	name        string
	replacement game.Block
}

func TestButtonPressAndScheduledRelease(t *testing.T) {
	tests := []buttonTestCase{
		{
			name:         "stone",
			button:       game.StoneButton,
			delay:        stoneButtonPressTicks,
			pressSound:   game.SoundBlockStoneButtonClickOn,
			releaseSound: game.SoundBlockStoneButtonClickOff,
		},
		{
			name:         "wooden",
			button:       game.OakButton,
			delay:        woodenButtonPressTicks,
			pressSound:   game.SoundBlockWoodenButtonClickOn,
			releaseSound: game.SoundBlockWoodenButtonClickOff,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, actor, _, actorConnection, observerConnection, position := newButtonTestRuntime(t, test.button)

			handled, result, _, err := runtime.InteractBlock(actor, position)
			if err != nil {
				t.Fatalf("press button: %v", err)
			}

			if !handled || !result.Allowed || !result.Changed {
				t.Fatalf("press result = handled %v, %+v", handled, result)
			}

			assertBlockProperty(t, runtime.World.BlockAt(position), "powered", "true")
			assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID})
			assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
			assertSoundEvent(t, observerConnection.packets(t)[1], test.pressSound)

			definition, _ := test.button.Definition()

			key := scheduledBlockTickKey{position: position, typeID: definition.ID}
			if _, scheduled := runtime.scheduledBlockTicks.pending[key]; !scheduled {
				t.Fatal("press did not schedule a block tick")
			}

			actorConnection.reset()
			observerConnection.reset()

			handled, result, _, err = runtime.InteractBlock(actor, position)
			if err != nil {
				t.Fatalf("repeat press: %v", err)
			}

			if !handled || !result.Allowed || result.Changed {
				t.Fatalf("repeat press result = handled %v, %+v", handled, result)
			}

			if len(runtime.scheduledBlockTicks.pending) != 1 {
				t.Fatalf("pending ticks after repeat press = %d, want 1", len(runtime.scheduledBlockTicks.pending))
			}

			assertPacketIDs(t, actorConnection.packetIDs(t), nil)
			assertPacketIDs(t, observerConnection.packetIDs(t), nil)

			tickRuntime(runtime, test.delay-1)

			assertBlockProperty(t, runtime.World.BlockAt(position), "powered", "true")
			assertPacketIDs(t, observerConnection.packetIDs(t), nil)

			runtime.Tick()

			assertBlockProperty(t, runtime.World.BlockAt(position), "powered", "false")
			assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
			assertSoundEvent(t, observerConnection.packets(t)[1], test.releaseSound)
		})
	}
}

func TestScheduledButtonTickBecomesStale(t *testing.T) {
	tests := []staleButtonTickTestCase{
		{name: "removed", replacement: game.Air},
		{name: "replaced", replacement: game.Stone},
		{name: "different button type", replacement: game.OakButton},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, actor, _, _, observerConnection, position := newButtonTestRuntime(t, game.StoneButton)

			_, result, _, err := runtime.InteractBlock(actor, position)
			if err != nil || !result.Changed {
				t.Fatalf("press button = %+v, %v", result, err)
			}

			runtime.World.SetBlock(position, test.replacement)
			observerConnection.reset()

			tickRuntime(runtime, stoneButtonPressTicks)

			if runtime.World.BlockAt(position) != test.replacement {
				t.Fatalf("stale tick changed block to %d, want %d", runtime.World.BlockAt(position), test.replacement)
			}

			assertPacketIDs(t, observerConnection.packetIDs(t), nil)
		})
	}
}

func TestButtonSupportLossLeavesPendingTickSafe(t *testing.T) {
	runtime, actor, _, _, observerConnection, position := newButtonTestRuntime(t, game.StoneButton)

	_, result, _, err := runtime.InteractBlock(actor, position)
	if err != nil || !result.Changed {
		t.Fatalf("press button = %+v, %v", result, err)
	}

	support := position

	support.Y--

	_, err = runtime.MutateWorldBlocks([]game.BlockChange{{Position: support, Replacement: game.Air}})
	if err != nil {
		t.Fatalf("remove support: %v", err)
	}

	if runtime.World.BlockAt(position) != game.Air {
		t.Fatalf("unsupported button = %d, want air", runtime.World.BlockAt(position))
	}

	if len(runtime.scheduledBlockTicks.pending) != 1 {
		t.Fatalf("pending ticks after support loss = %d, want stale tick retained", len(runtime.scheduledBlockTicks.pending))
	}

	runtime.Tick()

	if len(runtime.scheduledBlockTicks.pending) != 1 {
		t.Fatalf("pending ticks before deadline = %d, want 1", len(runtime.scheduledBlockTicks.pending))
	}

	observerConnection.reset()

	tickRuntime(runtime, stoneButtonPressTicks-1)

	if runtime.World.BlockAt(position) != game.Air {
		t.Fatalf("stale support-loss tick restored block %d", runtime.World.BlockAt(position))
	}

	for _, packetID := range observerConnection.packetIDs(t) {
		if packetID == protocol.ClientboundBlockUpdateID || packetID == protocol.ClientboundSoundID {
			t.Fatalf("stale support-loss tick sent packet %d", packetID)
		}
	}
}

func TestButtonAcceptsCenterSupport(t *testing.T) {
	position := game.BlockPosition{Y: 70}
	support := game.BlockPosition{Y: 69}

	button := mustBlockState(t, game.StoneButton,
		game.BlockPropertyValue{Name: "face", Value: "floor"},
		game.BlockPropertyValue{Name: "facing", Value: "north"},
		game.BlockPropertyValue{Name: "powered", Value: "false"},
	)

	blockAt := func(candidate game.BlockPosition) game.Block {
		if candidate == support {
			return game.CobblestoneWall
		}

		return game.Air
	}

	if game.CobblestoneWall.FaceSturdy(game.BlockFaceUp) {
		t.Fatal("center-support fixture unexpectedly has a full upper face")
	}

	if !game.CobblestoneWall.SupportsCenter(game.BlockFaceUp) {
		t.Fatal("center-support fixture does not cover the upper face center")
	}

	if !buttonSupported(blockAt, position, button) {
		t.Fatal("button rejected center-only support")
	}
}

func TestButtonScheduledTickPausesWithInactiveChunk(t *testing.T) {
	runtime, actor, _, _, observerConnection, position := newButtonTestRuntime(t, game.OakButton)

	_, result, _, err := runtime.InteractBlock(actor, position)
	if err != nil || !result.Changed {
		t.Fatalf("press button = %+v, %v", result, err)
	}

	runtime.setSessionActiveChunks(actor, nil)

	observerConnection.reset()

	tickRuntime(runtime, woodenButtonPressTicks*3)

	assertBlockProperty(t, runtime.World.BlockAt(position), "powered", "true")

	runtime.setSessionActiveChunks(actor, []LoadedChunk{blockLoadedChunk(position)})

	tickRuntime(runtime, woodenButtonPressTicks-1)

	assertBlockProperty(t, runtime.World.BlockAt(position), "powered", "true")
	assertPacketIDs(t, observerConnection.packetIDs(t), nil)

	runtime.Tick()

	assertBlockProperty(t, runtime.World.BlockAt(position), "powered", "false")
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
	assertSoundEvent(t, observerConnection.packets(t)[1], game.SoundBlockWoodenButtonClickOff)
}

func newButtonTestRuntime(t *testing.T, base game.Block) (*Runtime, *Session, *Session, *recordingConnection, *recordingConnection, game.BlockPosition) {
	t.Helper()

	support := game.BlockPosition{Y: 69}
	position := game.BlockPosition{Y: 70}

	button := mustBlockState(t, base,
		game.BlockPropertyValue{Name: "face", Value: "floor"},
		game.BlockPropertyValue{Name: "facing", Value: "north"},
		game.BlockPropertyValue{Name: "powered", Value: "false"},
	)

	world := &game.World{}

	world.SetBlock(support, game.Stone)
	world.SetBlock(position, button)

	runtime := NewRuntime(world)
	actor, actorConnection := newPlacementTestSession(runtime, position)
	observer, observerConnection := newPlacementTestSession(runtime, position)

	markPlacementChunksLoaded(actor, support, position)
	markPlacementChunksLoaded(observer, support, position)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	runtime.setSessionActiveChunks(actor, []LoadedChunk{blockLoadedChunk(position)})

	actorConnection.reset()
	observerConnection.reset()

	return runtime, actor, observer, actorConnection, observerConnection, position
}

func tickRuntime(runtime *Runtime, count int) {
	for range count {
		runtime.Tick()
	}
}
