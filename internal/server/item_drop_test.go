package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type heldItemDropTestCase struct {
	name      string
	dropAll   bool
	remaining int32
	dropped   int32
}

func TestDroppingHeldItemSpawnsRequestedStack(t *testing.T) {
	tests := []heldItemDropTestCase{
		{name: "Q drops one", remaining: 4, dropped: 1},
		{name: "Ctrl Q drops all", dropAll: true, dropped: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

			session.Player.Position = game.Position{X: 2, Y: 64, Z: 3}
			session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 5}

			runtime.entityRandom = func() float32 {
				return 0
			}

			session.handleDropHeldItem(test.dropAll)

			if count := session.snapshotPlayer().Inventory.Hotbar[0].Count; count != test.remaining {
				t.Fatalf("remaining count = %d, want %d", count, test.remaining)
			}

			item := onlyRuntimeItemEntity(t, runtime)
			if item.Stack.Count != test.dropped || item.Stack.Item != game.ItemStone || item.PickupDelay != 40 {
				t.Fatalf("dropped item = %+v, delay %d", item.Stack, item.PickupDelay)
			}

			expectedPosition := session.Player.EyePosition()

			expectedPosition.Y -= 0.3

			if item.State.Position != expectedPosition {
				t.Fatalf("drop position = %+v, want %+v", item.State.Position, expectedPosition)
			}

			if math.Abs(item.Velocity.Z-0.3) > 1e-9 || math.Abs(item.Velocity.Y-0.1) > 1e-9 || math.Abs(item.Velocity.X) > 1e-9 {
				t.Fatalf("directional drop velocity = %+v", item.Velocity)
			}
		})
	}
}

func TestMenuThrowSpawnsOnlyAfterAcceptedPrediction(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 3}

	err := session.handleContainerClick(protocol.ContainerClick{
		WindowID:    playerInventoryWindowID,
		Slot:        9,
		MouseButton: 0,
		Mode:        clickModeThrow,
		ChangedSlots: []protocol.ChangedSlot{{
			Location: 9,
			Item:     hashedStack(game.ItemStack{Item: game.ItemStone, Count: 1}),
		}},
	})

	if err != nil {
		t.Fatalf("reject inconsistent throw: %v", err)
	}

	if len(runtime.snapshotRuntimeEntities()) != 0 || session.Player.Inventory.Main[0].Count != 3 {
		t.Fatal("rejected throw mutated inventory or spawned an entity")
	}

	err = session.handleContainerClick(protocol.ContainerClick{
		WindowID:    playerInventoryWindowID,
		Slot:        9,
		MouseButton: 0,
		Mode:        clickModeThrow,
		ChangedSlots: []protocol.ChangedSlot{{
			Location: 9,
			Item:     hashedStack(game.ItemStack{Item: game.ItemStone, Count: 2}),
		}},
	})

	if err != nil {
		t.Fatalf("accept predicted throw: %v", err)
	}

	if session.Player.Inventory.Main[0].Count != 2 || onlyRuntimeItemEntity(t, runtime).Stack.Count != 1 {
		t.Fatal("accepted throw did not commit exactly one dropped item")
	}
}

func TestOutsideCursorDropSpawnsSplitStack(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.activeMenu().carried = game.ItemStack{Item: game.ItemDirt, Count: 3}

	err := session.handleContainerClick(protocol.ContainerClick{
		WindowID:    playerInventoryWindowID,
		Slot:        outsideInventorySlot,
		MouseButton: 1,
		Mode:        clickModePickup,
		CursorItem:  hashedStack(game.ItemStack{Item: game.ItemDirt, Count: 2}),
	})

	if err != nil {
		t.Fatalf("outside cursor drop: %v", err)
	}

	if session.activeMenu().carried.Count != 2 || onlyRuntimeItemEntity(t, runtime).Stack.Count != 1 {
		t.Fatal("secondary outside click did not drop one carried item")
	}
}

func TestCreativeDropSlotUsesRandomScatter(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.GameMode = game.GameModeCreative

	runtime.entityRandom = func() float32 {
		return 0.25
	}

	session.handleSetCreativeModeSlot(protocol.SetCreativeModeSlot{
		Slot: -1,
		Item: protocol.UntrustedSlot{ItemID: int32(game.ItemStone), ItemCount: 4},
	})

	item := onlyRuntimeItemEntity(t, runtime)
	if !item.Stack.Equal(game.ItemStack{Item: game.ItemStone, Count: 4}) || item.PickupDelay != 40 {
		t.Fatalf("creative dropped item = %+v, delay %d", item.Stack, item.PickupDelay)
	}

	if math.Abs(item.Velocity.X+0.125) > 1e-9 || math.Abs(item.Velocity.Y-0.2) > 1e-9 || math.Abs(item.Velocity.Z) > 1e-9 {
		t.Fatalf("creative scatter velocity = %+v", item.Velocity)
	}
}

func TestClosingMenuReturnsCarriedStackBeforeDropping(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	container := make([]game.ItemStack, 9)

	containerMenu := newGenericContainerMenu(1, 1, container, &session.Player.Inventory)

	containerMenu.carried = game.ItemStack{Item: game.ItemStone, Count: 3}

	session.containerMenu = containerMenu

	connection.reset()

	runtime.closeMenu(session, false)

	if !session.Player.Inventory.Main[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 3}) || len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatal("menu close did not return carried stack to inventory")
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
}

func onlyRuntimeItemEntity(t *testing.T, runtime *Runtime) *runtimeItemEntity {
	t.Helper()

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 1 {
		t.Fatalf("runtime entities = %d, want 1", len(entities))
	}

	item, valid := entities[0].(*runtimeItemEntity)
	if !valid {
		t.Fatalf("runtime entity = %T, want item", entities[0])
	}

	return item
}
