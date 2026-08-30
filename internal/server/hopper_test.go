package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type hopperPlacementFace struct {
	face   int32
	facing string
}

type hopperBlockedTransferCase struct {
	name    string
	enabled bool
	fill    bool
}

func TestHopperPlacementAndMenuQuickMove(t *testing.T) {
	if protocol.MenuHopper != 16 {
		t.Fatalf("hopper protocol menu type = %d, want 16", protocol.MenuHopper)
	}

	faces := []hopperPlacementFace{
		{face: protocol.BlockFaceUp, facing: "down"},
		{face: protocol.BlockFaceNorth, facing: "south"},
		{face: protocol.BlockFaceSouth, facing: "north"},
		{face: protocol.BlockFaceWest, facing: "east"},
		{face: protocol.BlockFaceEast, facing: "west"},
	}

	for _, test := range faces {
		state, valid := placementStateWithRotation(game.Hopper, game.ItemPlacementHopper, protocol.UseItemOn{Face: test.face}, game.Rotation{})
		if !valid || blockProperty(state, "facing") != test.facing || blockProperty(state, "enabled") != "true" {
			t.Fatalf("hopper placement on face %d = facing %q enabled %q valid %v", test.face, blockProperty(state, "facing"), blockProperty(state, "enabled"), valid)
		}
	}

	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(position, hopperTestState(t, "down", true))

	runtime := NewRuntime(world)

	session, _ := newPlacementTestSession(runtime, position)

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	hopper := mustRuntimeHopper(t, runtime, position)

	openHopperForTest(t, runtime, session, hopper)

	menu := session.activeMenu()
	if menu.backing != hopper || menu.protocolMenuType != protocol.MenuHopper || menu.containerSlots != game.HopperSlotCount || len(menu.slots) != game.HopperSlotCount+36 {
		t.Fatalf("hopper menu = backing %T type %d slots %d/%d", menu.backing, menu.protocolMenuType, menu.containerSlots, len(menu.slots))
	}

	session.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 3}

	err := session.handleContainerClick(protocol.ContainerClick{
		WindowID:    menu.windowID,
		StateID:     menu.stateID,
		Slot:        game.HopperSlotCount,
		MouseButton: 0,
		Mode:        clickModeQuickMove,
		ChangedSlots: []protocol.ChangedSlot{
			{Location: 0, Item: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 3})},
			{Location: game.HopperSlotCount},
		},
	})

	if err != nil {
		t.Fatalf("quick move into hopper: %v", err)
	}

	if !hopperData(t, hopper).Items[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 3}) || !session.Player.Inventory.Main[0].Empty() {
		t.Fatalf("quick move contents = hopper %+v player %+v", hopperData(t, hopper).Items[0], session.Player.Inventory.Main[0])
	}
}

func TestHopperMovesBetweenChestsAndDoubleChest(t *testing.T) {
	t.Run("sucks from above and ejects below", func(t *testing.T) {
		hopperPosition := game.BlockPosition{Y: 70}
		above := game.BlockPosition{Y: 71}
		below := game.BlockPosition{Y: 69}

		world := &game.World{}

		world.SetBlock(hopperPosition, hopperTestState(t, "down", true))
		world.SetBlock(above, singleChestTestState(t))
		world.SetBlock(below, singleChestTestState(t))

		runtime := NewRuntime(world)

		viewer := &Session{}

		runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(hopperPosition)})

		hopper := mustRuntimeHopper(t, runtime, hopperPosition)

		mustChestItems(t, &mustRuntimeChest(t, runtime, above).entity)[0] = game.ItemStack{Item: game.ItemStone, Count: 2}

		runtime.Tick()

		if hopperData(t, hopper).Items[0].Count != 1 || mustChestItems(t, &mustRuntimeChest(t, runtime, above).entity)[0].Count != 1 {
			t.Fatalf("hopper suction = hopper %+v source %+v", hopperData(t, hopper).Items[0], mustChestItems(t, &mustRuntimeChest(t, runtime, above).entity)[0])
		}

		hopperData(t, hopper).TransferCooldown = 1

		runtime.Tick()

		if hopperData(t, hopper).Items[0].Count != 1 || !mustChestItems(t, &mustRuntimeChest(t, runtime, above).entity)[0].Empty() || mustChestItems(t, &mustRuntimeChest(t, runtime, below).entity)[0].Count != 1 {
			t.Fatalf("hopper ejection/suction = hopper %+v source %+v destination %+v", hopperData(t, hopper).Items[0], mustChestItems(t, &mustRuntimeChest(t, runtime, above).entity)[0], mustChestItems(t, &mustRuntimeChest(t, runtime, below).entity)[0])
		}
	})

	t.Run("ejects into either double chest half", func(t *testing.T) {
		hopperPosition := game.BlockPosition{X: 2, Y: 70}
		left := game.BlockPosition{X: 1, Y: 70}
		right := game.BlockPosition{Y: 70}

		world := &game.World{}

		world.SetBlock(hopperPosition, hopperTestState(t, "west", true))

		world.SetBlock(left, mustBlockState(t, game.Chest,
			game.BlockPropertyValue{Name: "facing", Value: "south"},
			game.BlockPropertyValue{Name: "type", Value: "left"},
		))

		world.SetBlock(right, mustBlockState(t, game.Chest,
			game.BlockPropertyValue{Name: "facing", Value: "south"},
			game.BlockPropertyValue{Name: "type", Value: "right"},
		))

		runtime := NewRuntime(world)

		viewer := &Session{}

		runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(hopperPosition)})

		hopper := mustRuntimeHopper(t, runtime, hopperPosition)

		hopperData(t, hopper).Items[0] = game.ItemStack{Item: game.ItemStone, Count: 1}
		hopperData(t, hopper).TransferCooldown = 1

		runtime.Tick()

		backing, valid := runtime.chestBackingAt(left)
		if !valid || len(backing.chests) != 2 {
			t.Fatal("double chest was not available to hopper automation")
		}

		count := mustChestItems(t, &mustRuntimeChest(t, runtime, left).entity)[0].Count + mustChestItems(t, &mustRuntimeChest(t, runtime, right).entity)[0].Count
		if count != 1 || !hopperData(t, hopper).Items[0].Empty() {
			t.Fatalf("double chest transfer = count %d hopper %+v", count, hopperData(t, hopper).Items[0])
		}
	})
}

func TestHopperReceivingCooldownAccountsForTickOrder(t *testing.T) {
	orders := [][]game.BlockPosition{{{X: 0, Y: 70}, {X: 1, Y: 70}}, {{X: 1, Y: 70}, {X: 0, Y: 70}}}

	for _, positions := range orders {
		sourcePosition := positions[0]
		targetPosition := positions[1]

		facing := "east"

		if targetPosition.X < sourcePosition.X {
			facing = "west"
		}

		world := &game.World{}

		world.SetBlock(sourcePosition, hopperTestState(t, facing, true))
		world.SetBlock(targetPosition, hopperTestState(t, "down", true))

		runtime := NewRuntime(world)

		viewer := &Session{}

		runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(sourcePosition)})

		source := mustRuntimeHopper(t, runtime, sourcePosition)
		target := mustRuntimeHopper(t, runtime, targetPosition)

		hopperData(t, source).Items[0] = game.ItemStack{Item: game.ItemStone, Count: 1}
		hopperData(t, source).TransferCooldown = 1

		runtime.Tick()

		if !hopperData(t, source).Items[0].Empty() || hopperData(t, target).Items[0].Count != 1 || hopperData(t, target).TransferCooldown != hopperTransferCooldown-1 {
			t.Fatalf("hopper order transfer = source %+v target %+v cooldown %d", hopperData(t, source).Items[0], hopperData(t, target).Items[0], hopperData(t, target).TransferCooldown)
		}
	}
}

func TestAutomatedContainerFurnaceFaces(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(position, furnaceTestState(t, game.Furnace, false))

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(position)})

	container, present := runtime.automatedContainerAt(position)
	if !present {
		t.Fatal("furnace automated container is absent")
	}

	stone := game.ItemStack{Item: game.ItemStone, Count: 1}
	coal := game.ItemStack{Item: game.ItemCoal, Count: 1}
	waterBucket := game.ItemStack{Item: game.ItemWaterBucket, Count: 1}

	if !container.canInsert(furnaceInputSlot, stone, containerFaceUp) || container.canInsert(furnaceFuelSlot, coal, containerFaceUp) || container.canInsert(furnaceInputSlot, stone, containerFaceNorth) || !container.canInsert(furnaceFuelSlot, coal, containerFaceNorth) || container.canInsert(furnaceResultSlot, stone, containerFaceDown) {
		t.Fatal("furnace insertion faces did not enforce top input, side fuel, and bottom rejection")
	}

	if container.canInsert(furnaceFuelSlot, stone, containerFaceNorth) {
		t.Fatal("side insertion accepted a non-fuel item")
	}

	*container.slots[furnaceFuelSlot] = game.ItemStack{Item: game.ItemWaterBucket, Count: 1}

	if !container.canExtract(furnaceResultSlot, stone, containerFaceDown) || !container.canExtract(furnaceFuelSlot, waterBucket, containerFaceDown) || container.canExtract(furnaceFuelSlot, coal, containerFaceDown) || container.canExtract(furnaceResultSlot, stone, containerFaceNorth) {
		t.Fatal("furnace extraction faces did not enforce bottom result and bucket-only fuel")
	}
}

func TestHopperDoesNotTransferWhenDisabledOrDestinationRejects(t *testing.T) {
	cases := []hopperBlockedTransferCase{
		{name: "disabled", enabled: false},
		{name: "full incompatible destination", enabled: true, fill: true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			hopperPosition := game.BlockPosition{Y: 70}
			destination := game.BlockPosition{Y: 69}

			world := &game.World{}

			world.SetBlock(hopperPosition, hopperTestState(t, "down", test.enabled))
			world.SetBlock(destination, singleChestTestState(t))

			runtime := NewRuntime(world)

			viewer := &Session{}

			runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(hopperPosition)})

			hopper := mustRuntimeHopper(t, runtime, hopperPosition)

			hopperData(t, hopper).Items[0] = game.ItemStack{Item: game.ItemStone, Count: 1}
			hopperData(t, hopper).TransferCooldown = 1

			if test.fill {
				items := mustChestItems(t, &mustRuntimeChest(t, runtime, destination).entity)

				for slot := range items {
					items[slot] = game.ItemStack{Item: game.ItemDirt, Count: 64}
				}
			}

			runtime.Tick()

			if hopperData(t, hopper).Items[0].Count != 1 || hopperData(t, hopper).TransferCooldown != 0 {
				t.Fatalf("blocked transfer changed hopper to %+v with cooldown %d", hopperData(t, hopper).Items[0], hopperData(t, hopper).TransferCooldown)
			}
		})
	}
}

func TestHopperSucksItemEntitiesPartiallyAndFully(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(position, hopperTestState(t, "down", true))

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(position)})

	hopper := mustRuntimeHopper(t, runtime, position)

	data := hopperData(t, hopper)

	data.Items[0] = game.ItemStack{Item: game.ItemStone, Count: 63}

	for slot := 1; slot < len(data.Items); slot++ {
		data.Items[slot] = game.ItemStack{Item: game.ItemDirt, Count: 64}
	}

	partial := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 2}, game.Position{X: 0.5, Y: 71, Z: 0.5}, game.Velocity{}, 0)

	runtime.Tick()

	if data.Items[0].Count != 64 || !partial.Stack.Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) || partial.State.Removed {
		t.Fatalf("partial suction = hopper %+v entity %+v removed %v", data.Items[0], partial.Stack, partial.State.Removed)
	}

	data.Items[1] = game.ItemStack{}
	data.TransferCooldown = 1

	runtime.removeRuntimeEntity(partial.State.ID)

	full := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemGranite, Count: 1}, game.Position{X: 0.5, Y: 71, Z: 0.5}, game.Velocity{}, 0)

	runtime.Tick()

	if data.Items[1].Item != game.ItemGranite || !full.State.Removed {
		t.Fatalf("full suction = hopper %+v entity removed %v", data.Items[1], full.State.Removed)
	}
}

func TestHopperAutomationSynchronizesViewers(t *testing.T) {
	position := game.BlockPosition{Y: 70}
	above := game.BlockPosition{Y: 71}

	world := &game.World{}

	world.SetBlock(position, hopperTestState(t, "down", true))
	world.SetBlock(above, singleChestTestState(t))

	runtime := NewRuntime(world)

	first, firstConnection := newPlacementTestSession(runtime, position)
	second, secondConnection := newPlacementTestSession(runtime, position)

	runtime.setSessionActiveChunks(first, []LoadedChunk{blockLoadedChunk(position)})
	runtime.setSessionActiveChunks(second, []LoadedChunk{blockLoadedChunk(position)})

	hopper := mustRuntimeHopper(t, runtime, position)

	openHopperForTest(t, runtime, first, hopper)
	openHopperForTest(t, runtime, second, hopper)

	firstConnection.reset()
	secondConnection.reset()

	mustChestItems(t, &mustRuntimeChest(t, runtime, above).entity)[0] = game.ItemStack{Item: game.ItemStone, Count: 1}

	runtime.Tick()

	if first.activeMenu().stateID != 1 || second.activeMenu().stateID != 1 || countPacketID(firstConnection.packets(t), protocol.ClientboundContainerSetContentID) != 1 || countPacketID(secondConnection.packets(t), protocol.ClientboundContainerSetContentID) != 1 {
		t.Fatalf("hopper viewer synchronization = states %d/%d packets %v/%v", first.activeMenu().stateID, second.activeMenu().stateID, firstConnection.packetIDs(t), secondConnection.packetIDs(t))
	}
}

func TestHopperPausesAcrossInactiveChunksAndStaleRuntimeCannotTick(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(position, hopperTestState(t, "down", true))

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(position)})

	stale := mustRuntimeHopper(t, runtime, position)

	hopperData(t, stale).TransferCooldown = 3

	runtime.Tick()

	if hopperData(t, stale).TransferCooldown != 2 {
		t.Fatalf("active hopper cooldown = %d, want 2", hopperData(t, stale).TransferCooldown)
	}

	runtime.setSessionActiveChunks(viewer, nil)

	runtime.Tick()

	persisted, _ := runtime.World.BlockEntityAt(position)
	if persisted.Data.(*game.HopperBlockEntityData).TransferCooldown != 2 {
		t.Fatal("inactive hopper continued ticking")
	}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(position)})

	current := mustRuntimeHopper(t, runtime, position)
	if current == stale {
		t.Fatal("hopper runtime was not re-realized after reactivation")
	}

	stale.Tick(runtime, nil)

	if hopperData(t, current).TransferCooldown != 2 {
		t.Fatal("stale hopper runtime mutated the authoritative hopper")
	}

	runtime.Tick()

	if hopperData(t, current).TransferCooldown != 1 {
		t.Fatalf("re-activated hopper cooldown = %d, want 1", hopperData(t, current).TransferCooldown)
	}
}

func TestHopperCrossChunkTransferAndRemovalDropsInventory(t *testing.T) {
	hopperPosition := game.BlockPosition{X: 15, Y: 70}
	destination := game.BlockPosition{X: 16, Y: 70}

	world := &game.World{}

	world.SetBlock(hopperPosition, hopperTestState(t, "east", true))
	world.SetBlock(destination, singleChestTestState(t))

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(hopperPosition)})

	hopper := mustRuntimeHopper(t, runtime, hopperPosition)

	hopperData(t, hopper).Items[0] = game.ItemStack{Item: game.ItemStone, Count: 2}
	hopperData(t, hopper).TransferCooldown = 1

	runtime.Tick()

	if hopperData(t, hopper).Items[0].Count != 2 {
		t.Fatal("hopper transferred into an inactive destination chunk")
	}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(hopperPosition), blockLoadedChunk(destination)})

	runtime.Tick()

	if hopperData(t, hopper).Items[0].Count != 1 || mustChestItems(t, &mustRuntimeChest(t, runtime, destination).entity)[0].Count != 1 {
		t.Fatal("hopper did not safely resume transfer after destination activation")
	}

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: hopperPosition, Replacement: game.Air}})
	if err != nil || !result.Changed || countDroppedItem(runtime, game.ItemStone) != 1 {
		t.Fatalf("hopper removal = result %+v err %v stone drops %d", result, err, countDroppedItem(runtime, game.ItemStone))
	}
}

func hopperTestState(t *testing.T, facing string, enabled bool) game.Block {
	t.Helper()

	return mustBlockState(t, game.Hopper,
		game.BlockPropertyValue{Name: "facing", Value: facing},
		game.BlockPropertyValue{Name: "enabled", Value: boolProperty(enabled)},
	)
}

func singleChestTestState(t *testing.T) game.Block {
	t.Helper()

	return mustBlockState(t, game.Chest,
		game.BlockPropertyValue{Name: "facing", Value: "south"},
		game.BlockPropertyValue{Name: "type", Value: "single"},
	)
}

func mustRuntimeHopper(t *testing.T, runtime *Runtime, position game.BlockPosition) *runtimeHopper {
	t.Helper()

	entity, present := runtime.runtimeBlockEntityAt(position)
	hopper, valid := entity.(*runtimeHopper)

	if !present || !valid {
		t.Fatalf("runtime block entity at %+v = %T, want hopper", position, entity)
	}

	return hopper
}

func hopperData(t *testing.T, hopper *runtimeHopper) *game.HopperBlockEntityData {
	t.Helper()

	data, valid := hopper.entity.Data.(*game.HopperBlockEntityData)
	if !valid {
		t.Fatalf("hopper data = %T", hopper.entity.Data)
	}

	return data
}

func openHopperForTest(t *testing.T, runtime *Runtime, session *Session, hopper *runtimeHopper) {
	t.Helper()

	runtime.worldMutationMu.Lock()
	runtime.lifecycleMu.Lock()

	err := runtime.openHopperLocked(session, hopper)

	runtime.lifecycleMu.Unlock()
	runtime.worldMutationMu.Unlock()

	if err != nil {
		t.Fatalf("open hopper: %v", err)
	}
}
