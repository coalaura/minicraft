package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type furnaceTestCase struct {
	name       string
	block      game.Block
	entityType game.BlockEntityType
	input      game.Item
	result     game.Item
	cookTime   int32
	burnTime   int32
}

type furnacePlacementTestCase struct {
	name       string
	block      game.Block
	entityType game.BlockEntityType
	menuType   int32
}

type furnaceViewerTestCase struct {
	session    *Session
	connection *recordingConnection
}

type furnaceQuickMoveTestCase struct {
	name       string
	block      game.Block
	entityType game.BlockEntityType
	input      game.Item
}

func TestFurnaceFamilyIgnitesAndCompletesRecipes(t *testing.T) {
	tests := []furnaceTestCase{
		{name: "furnace", block: game.Furnace, entityType: game.BlockEntityTypeFurnace, input: game.ItemPotato, result: game.ItemBakedPotato, cookTime: 200, burnTime: 1600},
		{name: "smoker", block: game.Smoker, entityType: game.BlockEntityTypeSmoker, input: game.ItemPotato, result: game.ItemBakedPotato, cookTime: 100, burnTime: 800},
		{name: "blast furnace", block: game.BlastFurnace, entityType: game.BlockEntityTypeBlastFurnace, input: game.ItemRawIron, result: game.ItemIronIngot, cookTime: 100, burnTime: 800},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, session, furnace, data, position := newFurnaceTestRuntime(t, test.block, test.entityType)

			data.Items[furnaceInputSlot] = game.ItemStack{Item: test.input, Count: 1}
			data.Items[furnaceFuelSlot] = game.ItemStack{Item: game.ItemCoal, Count: 1}

			data.CookingTotalTime = 1

			runtime.Tick()

			if data.LitTimeRemaining != test.burnTime || data.LitTotalTime != test.burnTime {
				t.Fatalf("burn state = %d/%d, want %d", data.LitTimeRemaining, data.LitTotalTime, test.burnTime)
			}

			if !data.Items[furnaceFuelSlot].Empty() || data.Items[furnaceResultSlot].Item != test.result || data.CookingTotalTime != test.cookTime {
				t.Fatalf("furnace contents/state = %+v, total %d", data.Items, data.CookingTotalTime)
			}

			if blockProperty(runtime.World.BlockAt(position), "lit") != "true" {
				t.Fatal("furnace did not transition to lit")
			}

			runtime.setSessionActiveChunks(session, nil)

			persisted, present := runtime.World.BlockEntityAt(position)
			if !present {
				t.Fatal("furnace state was not persisted")
			}

			persistedData := persisted.Data.(*game.FurnaceBlockEntityData)
			remaining := persistedData.LitTimeRemaining

			runtime.Tick()

			paused, _ := runtime.World.BlockEntityAt(position)
			if paused.Data.(*game.FurnaceBlockEntityData).LitTimeRemaining != remaining {
				t.Fatal("inactive furnace continued ticking")
			}

			runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

			runtime.Tick()

			resumed, _ := runtime.World.BlockEntityAt(position)
			if resumed.Data.(*game.FurnaceBlockEntityData).LitTimeRemaining != remaining-1 {
				t.Fatal("reactivated furnace did not resume")
			}

			_ = furnace
		})
	}
}

func TestFurnaceFuelRemainderCapacityCooldownAndWetSponge(t *testing.T) {
	t.Run("lava bucket remainder", func(t *testing.T) {
		runtime, _, _, data, _ := newFurnaceTestRuntime(t, game.Furnace, game.BlockEntityTypeFurnace)

		data.Items[furnaceInputSlot] = game.ItemStack{Item: game.ItemPotato, Count: 1}
		data.Items[furnaceFuelSlot] = game.ItemStack{Item: game.ItemLavaBucket, Count: 1}

		data.CookingTotalTime = 200

		runtime.Tick()

		if data.Items[furnaceFuelSlot].Item != game.ItemBucket || data.LitTimeRemaining != 20000 {
			t.Fatalf("lava ignition = fuel %+v, duration %d", data.Items[furnaceFuelSlot], data.LitTimeRemaining)
		}
	})

	t.Run("full result blocks ignition", func(t *testing.T) {
		runtime, _, _, data, _ := newFurnaceTestRuntime(t, game.Furnace, game.BlockEntityTypeFurnace)

		data.Items[furnaceInputSlot] = game.ItemStack{Item: game.ItemPotato, Count: 1}
		data.Items[furnaceFuelSlot] = game.ItemStack{Item: game.ItemCoal, Count: 1}
		data.Items[furnaceResultSlot] = game.ItemStack{Item: game.ItemBakedPotato, Count: 64}

		runtime.Tick()

		if data.LitTimeRemaining != 0 || data.Items[furnaceFuelSlot].Count != 1 {
			t.Fatalf("blocked furnace ignited: %+v", data)
		}
	})

	t.Run("unlit progress cools by two", func(t *testing.T) {
		runtime, _, _, data, _ := newFurnaceTestRuntime(t, game.Furnace, game.BlockEntityTypeFurnace)

		data.CookingProgress = 9
		data.CookingTotalTime = 200

		runtime.Tick()

		if data.CookingProgress != 7 {
			t.Fatalf("cooled progress = %d, want 7", data.CookingProgress)
		}
	})

	t.Run("wet sponge fills bucket", func(t *testing.T) {
		runtime, _, _, data, _ := newFurnaceTestRuntime(t, game.Furnace, game.BlockEntityTypeFurnace)

		data.Items[furnaceInputSlot] = game.ItemStack{Item: game.ItemWetSponge, Count: 1}
		data.Items[furnaceFuelSlot] = game.ItemStack{Item: game.ItemBucket, Count: 1}

		data.LitTimeRemaining = 2
		data.LitTotalTime = 1600
		data.CookingProgress = 199
		data.CookingTotalTime = 200

		runtime.Tick()

		if data.Items[furnaceFuelSlot].Item != game.ItemWaterBucket || data.Items[furnaceResultSlot].Item != game.ItemSponge {
			t.Fatalf("wet sponge result = %+v", data.Items)
		}
	})
}

func TestFurnaceLitTurnsOffAndRecipeTypesRemainRestricted(t *testing.T) {
	runtime, _, _, data, position := newFurnaceTestRuntime(t, game.BlastFurnace, game.BlockEntityTypeBlastFurnace)

	data.LitTimeRemaining = 1
	data.LitTotalTime = 800

	runtime.Tick()

	if blockProperty(runtime.World.BlockAt(position), "lit") != "false" {
		t.Fatal("expired furnace remained lit")
	}

	data.Items[furnaceInputSlot] = game.ItemStack{Item: game.ItemPotato, Count: 1}
	data.Items[furnaceFuelSlot] = game.ItemStack{Item: game.ItemCoal, Count: 1}

	runtime.Tick()

	if data.LitTimeRemaining != 0 || data.Items[furnaceFuelSlot].Count != 1 {
		t.Fatal("blast furnace accepted smoking-only input")
	}
}

func TestFurnaceFamilyPlacementAndInteraction(t *testing.T) {
	tests := []furnacePlacementTestCase{
		{name: "furnace", block: game.Furnace, entityType: game.BlockEntityTypeFurnace, menuType: protocol.MenuFurnace},
		{name: "smoker", block: game.Smoker, entityType: game.BlockEntityTypeSmoker, menuType: protocol.MenuSmoker},
		{name: "blast furnace", block: game.BlastFurnace, entityType: game.BlockEntityTypeBlastFurnace, menuType: protocol.MenuBlastFurnace},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, valid := placementStateWithRotation(test.block, game.ItemPlacementFurnace, protocol.UseItemOn{}, game.Rotation{Yaw: 90})
			if !valid {
				t.Fatal("placement state is invalid")
			}

			if blockProperty(state, "facing") != "east" || blockProperty(state, "lit") != "false" {
				t.Fatalf("placement properties = facing %q, lit %q", blockProperty(state, "facing"), blockProperty(state, "lit"))
			}

			position := game.BlockPosition{Y: 70}

			world := &game.World{}

			world.SetBlock(position, state)

			runtime := NewRuntime(world)

			session, connection := newPlacementTestSession(runtime, position)

			runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

			err := session.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 9))
			if err != nil {
				t.Fatalf("open furnace: %v", err)
			}

			current := session.activeMenu()
			if current.backing == nil || current.protocolMenuType != test.menuType {
				t.Fatalf("opened menu = backing %T, type %d", current.backing, current.protocolMenuType)
			}

			got := countPacketID(connection.packets(t), protocol.ClientboundContainerSetDataID)
			if got != 4 {
				t.Fatalf("initial furnace data packets = %d, want 4", got)
			}
		})
	}
}

func TestMultipleFurnaceViewersShareStateWithIndependentMenus(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(position, furnaceTestState(t, game.Furnace, false))

	runtime := NewRuntime(world)

	first, firstConnection := newPlacementTestSession(runtime, position)

	first.Player.UUID = "00010203-0405-0607-0809-0a0b0c0d0e0f"
	first.Player.Name = "First"

	runtime.setSessionActiveChunks(first, []LoadedChunk{blockLoadedChunk(position)})

	joinTestSession(t, runtime, first)

	second, secondConnection := newPlacementTestSession(runtime, position)

	second.Player.UUID = "10111213-1415-1617-1819-1a1b1c1d1e1f"
	second.Player.Name = "Second"

	runtime.setSessionActiveChunks(second, []LoadedChunk{blockLoadedChunk(position)})

	joinTestSession(t, runtime, second)

	furnace := mustRuntimeFurnace(t, runtime, position)

	openFurnaceForTest(t, runtime, first, furnace)
	openFurnaceForTest(t, runtime, second, furnace)

	if first.activeMenu() == second.activeMenu() || first.activeMenu().backing != second.activeMenu().backing {
		t.Fatal("furnace viewers did not receive independent menus over shared backing")
	}

	first.activeMenu().carried = game.ItemStack{Item: game.ItemDirt, Count: 1}

	firstConnection.reset()
	secondConnection.reset()

	data := furnace.entity.Data.(*game.FurnaceBlockEntityData)

	data.Items[furnaceInputSlot] = game.ItemStack{Item: game.ItemPotato, Count: 1}

	data.LitTimeRemaining = 2
	data.LitTotalTime = 1600
	data.CookingProgress = 199
	data.CookingTotalTime = 200

	runtime.Tick()

	furnaceViewers := map[string]furnaceViewerTestCase{
		"first":  {session: first, connection: firstConnection},
		"second": {session: second, connection: secondConnection},
	}

	for name, viewer := range furnaceViewers {
		packets := viewer.connection.packets(t)
		if countPacketID(packets, protocol.ClientboundContainerSetContentID) != 1 || countPacketID(packets, protocol.ClientboundContainerSetDataID) == 0 {
			t.Fatalf("%s viewer packets = %v", name, viewer.connection.packetIDs(t))
		}

		if viewer.session.activeMenu().stateID != 1 {
			t.Fatalf("%s viewer state ID = %d, want 1", name, viewer.session.activeMenu().stateID)
		}
	}

	if !second.activeMenu().carried.Empty() || first.activeMenu().carried.Item != game.ItemDirt {
		t.Fatal("viewer cursors were not independent")
	}
}

func TestFurnaceInputClickImmediatelySynchronizesChangedData(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(position, furnaceTestState(t, game.Furnace, false))

	entity, _ := world.BlockEntityAt(position)

	data := entity.Data.(*game.FurnaceBlockEntityData)

	data.CookingProgress = 40
	data.CookingTotalTime = 100

	world.SetBlockEntity(position, entity)

	runtime := NewRuntime(world)

	first, firstConnection := newPlacementTestSession(runtime, position)

	first.Player.UUID = "00010203-0405-0607-0809-0a0b0c0d0e0f"
	first.Player.Name = "First"

	runtime.setSessionActiveChunks(first, []LoadedChunk{blockLoadedChunk(position)})

	joinTestSession(t, runtime, first)

	second, secondConnection := newPlacementTestSession(runtime, position)

	second.Player.UUID = "10111213-1415-1617-1819-1a1b1c1d1e1f"
	second.Player.Name = "Second"

	runtime.setSessionActiveChunks(second, []LoadedChunk{blockLoadedChunk(position)})

	joinTestSession(t, runtime, second)

	furnace := mustRuntimeFurnace(t, runtime, position)

	openFurnaceForTest(t, runtime, first, furnace)
	openFurnaceForTest(t, runtime, second, furnace)

	first.activeMenu().carried = game.ItemStack{Item: game.ItemPotato, Count: 1}

	firstConnection.reset()
	secondConnection.reset()

	err := first.handleContainerClick(protocol.ContainerClick{
		WindowID:    first.activeMenu().windowID,
		StateID:     first.activeMenu().stateID,
		Slot:        furnaceInputSlot,
		MouseButton: 0,
		Mode:        clickModePickup,
		ChangedSlots: []protocol.ChangedSlot{{
			Location: furnaceInputSlot,
			Item:     hashedStack(game.ItemStack{Item: game.ItemPotato, Count: 1}),
		}},
	})

	if err != nil {
		t.Fatalf("insert furnace input: %v", err)
	}

	currentData := furnace.entity.Data.(*game.FurnaceBlockEntityData)
	if currentData.CookingProgress != 0 || currentData.CookingTotalTime != 200 {
		t.Fatalf("cooking data after input click = progress %d, total %d", currentData.CookingProgress, currentData.CookingTotalTime)
	}

	viewerConnections := map[string]*recordingConnection{"first": firstConnection, "second": secondConnection}

	for name, connection := range viewerConnections {
		packets := connection.packets(t)
		if countPacketID(packets, protocol.ClientboundContainerSetContentID) != 1 || countPacketID(packets, protocol.ClientboundContainerSetDataID) != 2 {
			t.Fatalf("%s viewer packets = %v", name, connection.packetIDs(t))
		}
	}
}

func TestFurnaceMenuRestrictionsQuickMoveAndDataSynchronization(t *testing.T) {
	entity := game.NewBlockEntity(game.BlockEntityTypeFurnace)

	data := entity.Data.(*game.FurnaceBlockEntityData)

	furnace := newRuntimeFurnace(game.BlockPosition{}, entity).(*runtimeFurnace)

	inventory := game.PlayerInventory{}

	current := newFurnaceMenu(3, furnace, data, &inventory)

	candidate := current.candidate()

	if candidate.accepts(furnaceResultSlot, game.ItemStack{Item: game.ItemStone, Count: 1}) {
		t.Fatal("result slot accepted insertion")
	}

	if !candidate.accepts(furnaceFuelSlot, game.ItemStack{Item: game.ItemBucket, Count: 1}) {
		t.Fatal("empty fuel slot rejected bucket exception")
	}

	candidate.slots[furnaceFuelSlot] = game.ItemStack{Item: game.ItemBucket, Count: 1}

	if candidate.accepts(furnaceFuelSlot, game.ItemStack{Item: game.ItemBucket, Count: 1}) {
		t.Fatal("bucket exception allowed stacking buckets")
	}

	candidate.slots[furnaceFuelSlot] = game.ItemStack{}

	candidate.slots[3] = game.ItemStack{Item: game.ItemPotato, Count: 1}

	current.quickMove(candidate, 3)

	if candidate.slots[furnaceInputSlot].Item != game.ItemPotato || !candidate.slots[3].Empty() {
		t.Fatalf("cookable quick move = input %+v, source %+v", candidate.slots[0], candidate.slots[3])
	}

	candidate.slots[4] = game.ItemStack{Item: game.ItemCoal, Count: 1}

	current.quickMove(candidate, 4)

	if candidate.slots[furnaceFuelSlot].Item != game.ItemCoal || !candidate.slots[4].Empty() {
		t.Fatalf("fuel quick move = fuel %+v, source %+v", candidate.slots[1], candidate.slots[4])
	}

	runtime := NewRuntime(&game.World{})

	session, connection := newPlacementTestSession(runtime, game.BlockPosition{})

	current.protocolMenuType = protocol.MenuFurnace
	session.containerMenu = current

	err := session.sendChangedMenuData(current, true)
	if err != nil {
		t.Fatalf("initial data synchronization: %v", err)
	}

	got := countPacketID(connection.packets(t), protocol.ClientboundContainerSetDataID)
	if got != 4 {
		t.Fatalf("initial data packets = %d, want 4", got)
	}

	connection.reset()

	data.CookingProgress = 1

	err = session.sendChangedMenuData(current, false)
	if err != nil {
		t.Fatalf("changed data synchronization: %v", err)
	}

	got = countPacketID(connection.packets(t), protocol.ClientboundContainerSetDataID)
	if got != 1 {
		t.Fatalf("changed data packets = %d, want 1", got)
	}
}

func TestFurnaceFamilyQuickMovesCookableItemsIntoInput(t *testing.T) {
	tests := []furnaceQuickMoveTestCase{
		{name: "furnace", block: game.Furnace, entityType: game.BlockEntityTypeFurnace, input: game.ItemPotato},
		{name: "smoker", block: game.Smoker, entityType: game.BlockEntityTypeSmoker, input: game.ItemPotato},
		{name: "blast furnace", block: game.BlastFurnace, entityType: game.BlockEntityTypeBlastFurnace, input: game.ItemRawIron},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position := game.BlockPosition{Y: 70}

			world := &game.World{}

			world.SetBlock(position, furnaceTestState(t, test.block, false))

			runtime := NewRuntime(world)

			session, _ := newPlacementTestSession(runtime, position)

			session.Player.Inventory.Main[0] = game.ItemStack{Item: test.input, Count: 1}

			runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

			furnace := mustRuntimeFurnace(t, runtime, position)
			if furnace.entity.Type != test.entityType {
				t.Fatalf("block entity type = %v, want %v", furnace.entity.Type, test.entityType)
			}

			openFurnaceForTest(t, runtime, session, furnace)

			current := session.activeMenu()

			err := session.handleContainerClick(protocol.ContainerClick{
				WindowID:    current.windowID,
				StateID:     current.stateID,
				Slot:        3,
				MouseButton: 0,
				Mode:        clickModeQuickMove,
				ChangedSlots: []protocol.ChangedSlot{
					{Location: furnaceInputSlot, Item: hashedStack(game.ItemStack{Item: test.input, Count: 1})},
					{Location: 3},
				},
			})

			if err != nil {
				t.Fatalf("quick move cookable input: %v", err)
			}

			data := furnace.entity.Data.(*game.FurnaceBlockEntityData)
			if data.Items[furnaceInputSlot].Item != test.input || !session.Player.Inventory.Main[0].Empty() {
				t.Fatalf("quick move result = input %+v, player %+v", data.Items[furnaceInputSlot], session.Player.Inventory.Main[0])
			}
		})
	}
}

func TestBreakingFurnaceDropsItsInventory(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(position, furnaceTestState(t, game.Furnace, false))

	entity, _ := world.BlockEntityAt(position)

	data := entity.Data.(*game.FurnaceBlockEntityData)

	data.Items[0] = game.ItemStack{Item: game.ItemPotato, Count: 5}
	data.Items[1] = game.ItemStack{Item: game.ItemCoal, Count: 2}
	data.Items[2] = game.ItemStack{Item: game.ItemBakedPotato, Count: 3}

	world.SetBlockEntity(position, entity)

	runtime := NewRuntime(world)

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("break furnace = %+v, %v", result, err)
	}

	counts := map[game.Item]int32{}

	for _, entity := range runtime.snapshotRuntimeEntities() {
		item := entity.(*runtimeItemEntity)

		counts[item.Stack.Item] += item.Stack.Count

		if item.PickupDelay != 0 {
			t.Fatalf("drop pickup delay = %d, want 0", item.PickupDelay)
		}
	}

	if counts[game.ItemPotato] != 5 || counts[game.ItemCoal] != 2 || counts[game.ItemBakedPotato] != 3 {
		t.Fatalf("dropped contents = %+v", counts)
	}
}

func newFurnaceTestRuntime(t *testing.T, block game.Block, entityType game.BlockEntityType) (*Runtime, *Session, *runtimeFurnace, *game.FurnaceBlockEntityData, game.BlockPosition) {
	t.Helper()

	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(position, furnaceTestState(t, block, false))

	runtime := NewRuntime(world)

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	runtimeEntity, present := runtime.runtimeBlockEntityAt(position)
	if !present {
		t.Fatal("furnace block entity is not active")
	}

	furnace, valid := runtimeEntity.(*runtimeFurnace)
	if !valid || furnace.entity.Type != entityType {
		t.Fatalf("runtime block entity = %T, type %v", runtimeEntity, runtimeEntity.BlockEntityType())
	}

	data := furnace.entity.Data.(*game.FurnaceBlockEntityData)

	return runtime, session, furnace, data, position
}

func furnaceTestState(t *testing.T, block game.Block, lit bool) game.Block {
	t.Helper()

	state, valid := block.WithProperties(
		game.BlockPropertyValue{Name: "facing", Value: "north"},
		game.BlockPropertyValue{Name: "lit", Value: boolProperty(lit)},
	)

	if !valid {
		t.Fatalf("furnace state for %v is invalid", block)
	}

	return state
}

func countPacketID(packets []protocol.Packet, id int32) int {
	count := 0

	for _, packet := range packets {
		if packet.ID == id {
			count++
		}
	}

	return count
}

func mustRuntimeFurnace(t *testing.T, runtime *Runtime, position game.BlockPosition) *runtimeFurnace {
	t.Helper()

	entity, present := runtime.runtimeBlockEntityAt(position)
	if !present {
		t.Fatal("runtime furnace is absent")
	}

	furnace, valid := entity.(*runtimeFurnace)
	if !valid {
		t.Fatalf("runtime block entity = %T, want furnace", entity)
	}

	return furnace
}

func openFurnaceForTest(t *testing.T, runtime *Runtime, session *Session, furnace *runtimeFurnace) {
	t.Helper()

	runtime.worldMutationMu.Lock()
	runtime.lifecycleMu.Lock()

	err := runtime.openFurnaceLocked(session, furnace)

	runtime.lifecycleMu.Unlock()
	runtime.worldMutationMu.Unlock()

	if err != nil {
		t.Fatalf("open furnace: %v", err)
	}
}
