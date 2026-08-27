package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type barrelTestGenerator struct {
	position game.BlockPosition
	entity   game.BlockEntity
}

type barrelFacingTestCase struct {
	name     string
	rotation game.Rotation
	want     string
}

func (g barrelTestGenerator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	if position == g.position {
		return mustBlockStateForBarrelTest(game.Barrel, "north", false)
	}

	return game.Air
}

func (g barrelTestGenerator) GenerateBlockEntities(_ int64, chunk game.ChunkPosition) game.ChunkBlockEntities {
	if chunk != (game.ChunkPosition{X: g.position.X >> 4, Z: g.position.Z >> 4}) {
		return nil
	}

	return game.ChunkBlockEntities{
		{X: g.position.X & 15, Y: g.position.Y, Z: g.position.Z & 15}: g.entity,
	}
}

func TestActiveBarrelRealizationIsPassive(t *testing.T) {
	position := game.BlockPosition{X: 1, Y: 70, Z: 1}

	entity := game.NewBlockEntity(game.BlockEntityTypeBarrel)

	entityItems := mustBarrelItems(t, &entity)

	entityItems[0] = game.ItemStack{Item: game.ItemStone, Count: 3}

	world := &game.World{Generator: barrelTestGenerator{position: position, entity: entity}}

	runtime := NewRuntime(world)

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	barrel, active := runtime.runtimeBarrelAt(position)
	if !active || !barrel.entity.Equal(entity) {
		t.Fatalf("active barrel = %+v, %v; want %+v, true", barrel, active, entity)
	}

	if overrides := world.BlockEntityOverrideCount(); overrides != 0 {
		t.Fatalf("block entity overrides after realization = %d, want 0", overrides)
	}

	runtime.Tick()

	if !barrel.entity.Equal(entity) || world.BlockAt(position) != mustBlockStateForBarrelTest(game.Barrel, "north", false) {
		t.Fatal("passive active barrel changed while ticking")
	}

	if overrides := world.BlockEntityOverrideCount(); overrides != 0 {
		t.Fatalf("block entity overrides after passive tick = %d, want 0", overrides)
	}
}

func TestBarrelOpenAllocatesWindowIDsThroughVanillaCycle(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime, session, connection := newBarrelTestRuntime(t, position)

	for expected := int32(1); expected <= 101; expected++ {
		connection.reset()

		openBarrelForTest(t, runtime, session, position)

		want := expected
		if want == 101 {
			want = 1
		}

		assertBarrelOpenScreen(t, connection.packets(t)[0], want)

		runtime.closeMenu(session, false)
	}
}

func TestBarrelViewersShareContentsWithIndependentStateIDs(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime, first, firstConnection := newBarrelTestRuntime(t, position)

	second, secondConnection := newBarrelTestSession(t, runtime, position, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Second")

	barrel, active := runtime.runtimeBarrelAt(position)
	if !active {
		t.Fatal("barrel is not active")
	}

	barrelItems := mustBarrelItems(t, &barrel.entity)

	barrelItems[0] = game.ItemStack{Item: game.ItemStone, Count: 4}

	openBarrelForTest(t, runtime, first, position)
	openBarrelForTest(t, runtime, second, position)

	firstConnection.reset()
	secondConnection.reset()

	err := first.handleContainerClick(protocol.ContainerClick{
		WindowID:    first.activeMenu().windowID,
		StateID:     first.activeMenu().stateID,
		Slot:        0,
		MouseButton: 0,
		Mode:        clickModePickup,
		ChangedSlots: []protocol.ChangedSlot{{
			Location: 0,
		}},
		CursorItem: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 4}),
	})

	if err != nil {
		t.Fatalf("first viewer pickup: %v", err)
	}

	if !barrelItems[0].Empty() || !first.activeMenu().carried.Equal(game.ItemStack{Item: game.ItemStone, Count: 4}) {
		t.Fatalf("shared barrel contents after pickup = %+v", barrelItems[0])
	}

	if first.activeMenu().stateID != 1 || second.activeMenu().stateID != 1 {
		t.Fatalf("viewer state ids = %d, %d; want 1, 1", first.activeMenu().stateID, second.activeMenu().stateID)
	}

	assertPacketIDs(t, firstConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
	assertPacketIDs(t, secondConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
	assertMenuSnapshotHeader(t, secondConnection.packets(t)[0], second.activeMenu().windowID, 1, 63)
}

func TestPlayerOnlyBarrelMenuClickDoesNotSynchronizeOtherViewers(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime, first, firstConnection := newBarrelTestRuntime(t, position)

	second, secondConnection := newBarrelTestSession(t, runtime, position, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Second")

	first.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemStone, Count: 4}

	openBarrelForTest(t, runtime, first, position)
	openBarrelForTest(t, runtime, second, position)

	firstConnection.reset()
	secondConnection.reset()

	err := first.handleContainerClick(protocol.ContainerClick{
		WindowID:    first.activeMenu().windowID,
		StateID:     first.activeMenu().stateID,
		Slot:        27,
		MouseButton: 0,
		Mode:        clickModePickup,
		ChangedSlots: []protocol.ChangedSlot{{
			Location: 27,
		}},
		CursorItem: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 4}),
	})

	if err != nil {
		t.Fatalf("player-only barrel click: %v", err)
	}

	if first.activeMenu().stateID != 1 || second.activeMenu().stateID != 0 {
		t.Fatalf("viewer state ids = %d, %d; want 1, 0", first.activeMenu().stateID, second.activeMenu().stateID)
	}

	assertPacketIDs(t, firstConnection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
	assertPacketIDs(t, secondConnection.packetIDs(t), nil)
}

func TestBarrelShiftClickMatchesVanillaReversePlayerRouting(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime, session, connection := newBarrelTestRuntime(t, position)

	barrel, active := runtime.runtimeBarrelAt(position)
	if !active {
		t.Fatal("barrel is not active")
	}

	barrelItems := mustBarrelItems(t, &barrel.entity)

	barrelItems[0] = game.ItemStack{Item: game.ItemStone, Count: 5}

	openBarrelForTest(t, runtime, session, position)
	connection.reset()

	currentMenu := session.activeMenu()
	err := session.handleContainerClick(protocol.ContainerClick{
		WindowID:    currentMenu.windowID,
		StateID:     currentMenu.stateID,
		Slot:        0,
		MouseButton: 0,
		Mode:        clickModeQuickMove,
		ChangedSlots: []protocol.ChangedSlot{
			{Location: 0},
			{Location: 62, Item: hashedStack(game.ItemStack{Item: game.ItemStone, Count: 5})},
		},
	})
	if err != nil {
		t.Fatalf("shift-click barrel slot: %v", err)
	}

	player := session.snapshotPlayer()
	if !barrelItems[0].Empty() || !player.Inventory.Hotbar[8].Equal(game.ItemStack{Item: game.ItemStone, Count: 5}) {
		t.Fatalf("shift-click result = barrel %+v, hotbar %+v", barrelItems[0], player.Inventory.Hotbar[8])
	}

	if currentMenu.stateID != 1 {
		t.Fatalf("menu state id = %d, want 1", currentMenu.stateID)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundContainerSetContentID})
}

func TestCreativeCloneAndMiddleDragWorkInBarrelMenu(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime, session, connection := newBarrelTestRuntime(t, position)

	barrel, active := runtime.runtimeBarrelAt(position)
	if !active {
		t.Fatal("barrel is not active")
	}

	barrelItems := mustBarrelItems(t, &barrel.entity)

	barrelItems[0] = game.ItemStack{Item: game.ItemStone, Count: 1}

	session.Player.GameMode = game.GameModeCreative

	openBarrelForTest(t, runtime, session, position)
	connection.reset()

	currentMenu := session.activeMenu()
	fullStack := game.ItemStack{Item: game.ItemStone, Count: 64}

	clicks := []protocol.ContainerClick{
		{
			WindowID:    currentMenu.windowID,
			StateID:     0,
			Slot:        0,
			MouseButton: 2,
			Mode:        clickModeClone,
			CursorItem:  hashedStack(fullStack),
		},
		{
			WindowID:    currentMenu.windowID,
			StateID:     1,
			Slot:        outsideInventorySlot,
			MouseButton: 8,
			Mode:        clickModeQuickCraft,
			CursorItem:  hashedStack(fullStack),
		},
		{
			WindowID:    currentMenu.windowID,
			StateID:     1,
			Slot:        1,
			MouseButton: 9,
			Mode:        clickModeQuickCraft,
			CursorItem:  hashedStack(fullStack),
		},
		{
			WindowID:    currentMenu.windowID,
			StateID:     1,
			Slot:        2,
			MouseButton: 9,
			Mode:        clickModeQuickCraft,
			CursorItem:  hashedStack(fullStack),
		},
		{
			WindowID:    currentMenu.windowID,
			StateID:     1,
			Slot:        outsideInventorySlot,
			MouseButton: 10,
			Mode:        clickModeQuickCraft,
			ChangedSlots: []protocol.ChangedSlot{
				{Location: 1, Item: hashedStack(fullStack)},
				{Location: 2, Item: hashedStack(fullStack)},
			},
			CursorItem: protocol.HashedSlot{},
		},
	}

	for index, click := range clicks {
		err := session.handleContainerClick(click)
		if err != nil {
			t.Fatalf("creative barrel click %d: %v", index, err)
		}
	}

	if !barrelItems[1].Equal(fullStack) || !barrelItems[2].Equal(fullStack) || !currentMenu.carried.Empty() {
		t.Fatalf("creative drag result = slots %+v, %+v, carried %+v", barrelItems[1], barrelItems[2], currentMenu.carried)
	}

	if currentMenu.stateID != 2 {
		t.Fatalf("menu state id = %d, want 2", currentMenu.stateID)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{
		protocol.ClientboundContainerSetContentID,
		protocol.ClientboundContainerSetContentID,
	})
}

func TestBarrelFirstAndLastViewerToggleStateAndSound(t *testing.T) {
	position := game.BlockPosition{X: 4, Y: 70, Z: -3}

	runtime, first, firstConnection := newBarrelTestRuntime(t, position)

	second, secondConnection := newBarrelTestSession(t, runtime, position, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Second")

	firstConnection.reset()
	secondConnection.reset()

	openBarrelForTest(t, runtime, first, position)

	assertBlockProperty(t, runtime.World.BlockAt(position), "open", "true")
	assertBarrelSound(t, firstConnection.packets(t)[3], game.SoundBlockBarrelOpen, position, "north")
	assertBarrelSound(t, secondConnection.packets(t)[1], game.SoundBlockBarrelOpen, position, "north")

	firstConnection.reset()
	secondConnection.reset()

	openBarrelForTest(t, runtime, second, position)

	assertPacketIDs(t, firstConnection.packetIDs(t), nil)
	assertPacketIDs(t, secondConnection.packetIDs(t), []int32{protocol.ClientboundOpenScreenID, protocol.ClientboundContainerSetContentID})

	runtime.closeMenu(first, false)

	assertBlockProperty(t, runtime.World.BlockAt(position), "open", "true")

	firstConnection.reset()
	secondConnection.reset()

	runtime.closeMenu(second, false)

	assertBlockProperty(t, runtime.World.BlockAt(position), "open", "false")
	assertBarrelSound(t, firstConnection.packets(t)[1], game.SoundBlockBarrelClose, position, "north")
	assertBarrelSound(t, secondConnection.packets(t)[1], game.SoundBlockBarrelClose, position, "north")
}

func TestBarrelMenuClosesWhenPlayerMovesOutOfRange(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime, session, connection := newBarrelTestRuntime(t, position)

	openBarrelForTest(t, runtime, session, position)

	connection.reset()

	session.Player.Position.X = 100

	runtime.tickOpenMenus()

	if session.activeMenu() != session.inventoryMenu {
		t.Fatal("distant player retained barrel menu")
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundCloseContainerID, protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
}

func TestBarrelValidityUsesStrictEyeToBlockBoundsDistance(t *testing.T) {
	position := game.BlockPosition{}

	player := game.Player{Position: game.Position{X: 11, Y: 0.5 - 1.62, Z: 0.5}}

	if containerWithinRange(player, position) {
		t.Fatal("barrel menu remained valid at the exact maximum distance")
	}

	player.Position.X -= 0.0001
	if !containerWithinRange(player, position) {
		t.Fatal("barrel menu was invalid just inside the maximum distance")
	}
}

func TestBarrelMenuClosesWhenBarrelIsRemoved(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime, viewer, viewerConnection := newBarrelTestRuntime(t, position)

	breaker, _ := newBarrelTestSession(t, runtime, position, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Breaker")

	openBarrelForTest(t, runtime, viewer, position)

	viewerConnection.reset()

	err := breaker.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionStartDestroyBlock, Position: position, Face: protocol.BlockFaceUp, Sequence: 2})
	if err != nil {
		t.Fatalf("break barrel: %v", err)
	}

	if sessionHasPacket(viewerConnection.packets(t), protocol.ClientboundCloseContainerID) == false {
		t.Fatal("removing barrel did not close viewer menu")
	}

	if viewer.activeMenu() != viewer.inventoryMenu {
		t.Fatal("removed barrel retained viewer menu")
	}
}

func TestBarrelPlacementUsesVerticalFacing(t *testing.T) {
	tests := []barrelFacingTestCase{
		{name: "looking up", rotation: game.Rotation{Pitch: -90}, want: "down"},
		{name: "looking down", rotation: game.Rotation{Pitch: 90}, want: "up"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, valid := placementStateWithRotation(game.Barrel, game.ItemPlacementDirectionalFacing, protocol.UseItemOn{}, test.rotation)
			if !valid {
				t.Fatal("barrel placement state is invalid")
			}

			assertBlockProperty(t, state, "facing", test.want)
			assertBlockProperty(t, state, "open", "false")
		})
	}
}

func TestSneakingPlacesAgainstBarrelInsteadOfOpeningIt(t *testing.T) {
	position := game.BlockPosition{Y: 70}
	target := game.BlockPosition{Y: 71}

	runtime, session, _ := newBarrelTestRuntime(t, position)

	session.Player.Sneaking = true
	session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemBarrel, Count: 1}

	markPlacementChunksLoaded(session, position, target)

	err := session.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 1))
	if err != nil {
		t.Fatalf("place against barrel while sneaking: %v", err)
	}

	if game.BlockEntityTypeForBlock(runtime.World.BlockAt(target)) != game.BlockEntityTypeBarrel {
		t.Fatalf("block placed against barrel = %d, want barrel", runtime.World.BlockAt(target))
	}

	if session.activeMenu() != session.inventoryMenu {
		t.Fatal("sneaking interaction opened barrel menu")
	}
}

func TestUseItemOnOpensBarrelAndAcknowledgesInteraction(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	_, session, connection := newBarrelTestRuntime(t, position)

	err := session.handleUseItemOn(testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 9))
	if err != nil {
		t.Fatalf("open barrel: %v", err)
	}

	if session.activeMenu().backing == nil {
		t.Fatal("barrel interaction did not open a container menu")
	}

	if !sessionHasPacket(connection.packets(t), protocol.ClientboundOpenScreenID) {
		t.Fatal("barrel interaction did not send open screen")
	}

	if !sessionHasPacket(connection.packets(t), protocol.ClientboundBlockChangedAckID) {
		t.Fatal("barrel interaction did not acknowledge the sequence")
	}
}

func TestNormalLightChunkIncludesBarrelBlockEntity(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{Lighting: game.LightingNormal}

	world.SetBlock(position, mustBlockStateForBarrelTest(game.Barrel, "north", false))

	chunk, err := buildNormalLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatalf("build normal-light chunk: %v", err)
	}

	if len(chunk.BlockEntities) != 1 {
		t.Fatalf("normal-light chunk block entities = %d, want 1", len(chunk.BlockEntities))
	}

	entity := chunk.BlockEntities[0]

	definition, _ := game.BlockEntityTypeBarrel.Definition()
	if entity.X != 0 || entity.Y != int16(position.Y) || entity.Z != 0 || entity.Type != definition.ProtocolRegistryID12111 {
		t.Fatalf("normal-light barrel entity = %+v", entity)
	}
}

func newBarrelTestRuntime(t *testing.T, position game.BlockPosition) (*Runtime, *Session, *recordingConnection) {
	t.Helper()

	world := &game.World{}

	world.SetBlock(position, mustBlockStateForBarrelTest(game.Barrel, "north", false))

	runtime := NewRuntime(world)

	session, connection := newBarrelTestSession(t, runtime, position, "00010203-0405-0607-0809-0a0b0c0d0e0f", "First")

	return runtime, session, connection
}

func newBarrelTestSession(t *testing.T, runtime *Runtime, position game.BlockPosition, uuid, name string) (*Session, *recordingConnection) {
	t.Helper()

	session, connection := newPlacementTestSession(runtime, position)

	session.Player.UUID = uuid
	session.Player.Name = name

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	joinTestSession(t, runtime, session)

	connection.reset()

	return session, connection
}

func openBarrelForTest(t *testing.T, runtime *Runtime, session *Session, position game.BlockPosition) {
	t.Helper()

	barrel, active := runtime.runtimeBarrelAt(position)
	if !active {
		t.Fatal("barrel is not active")
	}

	runtime.worldMutationMu.Lock()
	runtime.lifecycleMu.Lock()

	err := runtime.openBarrelLocked(session, barrel)

	deliveries := runtime.takeRuntimeBlockMutationsLocked()

	runtime.lifecycleMu.Unlock()
	runtime.worldMutationMu.Unlock()

	runtime.completeRuntimeBlockMutations(deliveries)

	if err != nil {
		t.Fatalf("open barrel: %v", err)
	}
}

func mustBlockStateForBarrelTest(block game.Block, facing string, open bool) game.Block {
	state, valid := block.WithProperties(
		game.BlockPropertyValue{Name: "facing", Value: facing},
		game.BlockPropertyValue{Name: "open", Value: boolProperty(open)},
	)
	if !valid {
		panic("invalid barrel test block state")
	}

	return state
}

func assertBarrelOpenScreen(t *testing.T, packet protocol.Packet, wantWindowID int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	windowID := reader.VarInt()
	menuType := reader.VarInt()

	if windowID != wantWindowID || menuType != protocol.MenuGeneric9x3 {
		t.Fatalf("barrel open screen = window %d type %d; want window %d type %d", windowID, menuType, wantWindowID, protocol.MenuGeneric9x3)
	}
}

func mustBarrelItems(t *testing.T, entity *game.BlockEntity) []game.ItemStack {
	t.Helper()

	items, inventory := entity.Inventory()
	if !inventory || len(items) != game.BarrelSlotCount {
		t.Fatal("barrel entity does not expose 27 inventory slots")
	}

	return items
}

func assertMenuSnapshotHeader(t *testing.T, packet protocol.Packet, wantWindowID, wantStateID, wantSlots int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	windowID := reader.VarInt()
	stateID := reader.VarInt()
	slots := reader.VarInt()

	if windowID != wantWindowID || stateID != wantStateID || slots != wantSlots {
		t.Fatalf("menu snapshot = window %d state %d slots %d; want window %d state %d slots %d", windowID, stateID, slots, wantWindowID, wantStateID, wantSlots)
	}
}

func assertBarrelSound(t *testing.T, packet protocol.Packet, event game.SoundEvent, position game.BlockPosition, facing string) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	reader.VarInt()

	actualEvent := reader.String(32767)

	if reader.Bool() {
		reader.Float()
	}

	reader.VarInt()

	actualX := reader.Int()
	actualY := reader.Int()
	actualZ := reader.Int()

	directionX, directionY, directionZ := barrelFacingOffset(facing)

	wantX := int32((float64(position.X) + 0.5 + directionX*0.5) * 8)
	wantY := int32((float64(position.Y) + 0.5 + directionY*0.5) * 8)
	wantZ := int32((float64(position.Z) + 0.5 + directionZ*0.5) * 8)

	if actualEvent != string(event) || actualX != wantX || actualY != wantY || actualZ != wantZ {
		t.Fatalf("barrel sound = event %q coordinates %d, %d, %d; want event %q coordinates %d, %d, %d", actualEvent, actualX, actualY, actualZ, event, wantX, wantY, wantZ)
	}
}

func sessionHasPacket(packets []protocol.Packet, packetID int32) bool {
	for _, packet := range packets {
		if packet.ID == packetID {
			return true
		}
	}

	return false
}
