package server

import (
	"errors"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestRuntimeEntityIDsSharePlayerNamespace(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	first, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "First")
	second, _ := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Second")

	if id := runtime.AssignEntityID(first); id != 1 {
		t.Fatalf("first player entity id = %d, want 1", id)
	}

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{}, game.Velocity{}, 0)
	if item.State.ID != 2 {
		t.Fatalf("item entity id = %d, want 2", item.State.ID)
	}

	if id := runtime.AssignEntityID(second); id != 3 {
		t.Fatalf("second player entity id = %d, want 3", id)
	}

	if item.State.UUID == "" || item.State.UUID == first.Player.UUID || item.State.UUID == second.Player.UUID {
		t.Fatalf("item UUID = %q", item.State.UUID)
	}
}

func TestRuntimeEntityTrackingFollowsLoadedChunks(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, session)

	connection.reset()

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 3}, game.Position{X: 1, Y: 64, Z: 1}, game.Velocity{}, 40)

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID})

	if !session.tracksRuntimeEntity(item.State.ID) {
		t.Fatal("loaded item entity was not tracked")
	}

	connection.reset()

	session.untrackEntitiesInChunk(LoadedChunk{})

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundRemoveEntitiesID})

	if session.tracksRuntimeEntity(item.State.ID) {
		t.Fatal("unloaded item entity remained tracked")
	}

	connection.reset()

	session.trackEntitiesInChunk(LoadedChunk{})

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID})

	connection.reset()

	runtime.removeRuntimeEntity(item.State.ID)

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundRemoveEntitiesID})

	if len(runtime.snapshotRuntimeEntities()) != 0 || session.tracksRuntimeEntity(item.State.ID) {
		t.Fatal("removed entity remained authoritative or tracked")
	}
}

func TestRuntimeEntityTracksAfterChunkLoads(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemDirt, Count: 2}, game.Position{X: 17, Y: 64}, game.Velocity{}, 40)

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	joinTestSession(t, runtime, session)

	connection.reset()

	session.loadedChunks = map[LoadedChunk]struct{}{{X: 1}: {}}

	session.trackEntitiesInChunk(LoadedChunk{X: 1})

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID})
	if !session.tracksRuntimeEntity(item.State.ID) {
		t.Fatal("entity in newly loaded chunk was not tracked")
	}
}

func TestRuntimeEntityTrackingRetriesAfterWriteFailure(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, session)

	connection.reset()

	connection.writeErr = errors.New("write failed")

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{}, game.Velocity{}, 40)
	if session.tracksRuntimeEntity(item.State.ID) {
		t.Fatal("failed spawn remained marked as tracked")
	}

	retryConnection := &recordingConnection{}

	session.Conn = protocol.NewConnection(retryConnection, nil)

	runtime.reconcileRuntimeEntityTracking(item)

	assertPacketIDs(t, retryConnection.packetIDs(t), []int32{protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID})
}

func TestRuntimeEntityInactiveChunkPausesWithoutDeleting(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{Y: 100}, game.Velocity{}, 40)

	runtime.Tick()

	if item.Age != 0 || item.PickupDelay != 40 || len(runtime.snapshotRuntimeEntities()) != 1 {
		t.Fatalf("inactive entity changed: age %d, delay %d, entities %d", item.Age, item.PickupDelay, len(runtime.snapshotRuntimeEntities()))
	}

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	runtime.Tick()

	if item.Age != 1 || item.PickupDelay != 39 || item.State.Position.Y >= 100 {
		t.Fatalf("activated entity did not tick: age %d, delay %d, position %+v", item.Age, item.PickupDelay, item.State.Position)
	}

	runtime.releaseSessionActiveChunks(viewer)

	runtime.Tick()

	if item.Age != 1 {
		t.Fatalf("deactivated entity age = %d, want 1", item.Age)
	}
}

func TestRuntimeEntityCrossesChunksExactlyOncePerTick(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}, {X: 1}})

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 15.9, Y: 100}, game.Velocity{X: 0.2}, 40)

	runtime.Tick()

	if item.Age != 1 || item.TickCount != 1 {
		t.Fatalf("crossing entity ticked %d times with age %d", item.TickCount, item.Age)
	}

	if item.State.Chunk != (LoadedChunk{X: 1}) {
		t.Fatalf("crossing entity chunk = %+v, want x=1", item.State.Chunk)
	}

	first, _ := runtime.ActiveChunk(LoadedChunk{})
	second, _ := runtime.ActiveChunk(LoadedChunk{X: 1})

	if first.EntityCount() != 0 || second.EntityCount() != 1 {
		t.Fatalf("active entity counts = first %d, second %d", first.EntityCount(), second.EntityCount())
	}

	if len(runtime.snapshotEntitiesInChunk(LoadedChunk{})) != 0 || len(runtime.snapshotEntitiesInChunk(LoadedChunk{X: 1})) != 1 {
		t.Fatal("authoritative chunk index did not follow entity movement")
	}
}

func TestRuntimeEntityCrossingUnloadedChunkStopsTracking(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, session)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}, {X: 1}})

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 15.9, Y: 100}, game.Velocity{X: 0.2}, 40)
	if !session.tracksRuntimeEntity(item.State.ID) {
		t.Fatal("entity was not tracked in its initial loaded chunk")
	}

	connection.reset()

	runtime.Tick()

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundRemoveEntitiesID})

	if session.tracksRuntimeEntity(item.State.ID) || item.State.Chunk != (LoadedChunk{X: 1}) {
		t.Fatalf("crossing entity tracking = %v, chunk %+v", session.tracksRuntimeEntity(item.State.ID), item.State.Chunk)
	}
}

func TestItemEntityGravityCollisionAndDespawn(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 1, Z: 0.5}, game.Velocity{}, 40)

	runtime.Tick()

	if !item.OnGround || item.State.Position.Y != 1 || item.Velocity.Y <= 0 {
		t.Fatalf("grounded item state = position %+v, velocity %+v, onGround %v", item.State.Position, item.Velocity, item.OnGround)
	}

	item.Age = itemEntityLifetime - 1

	runtime.Tick()

	if !item.State.Removed || len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatal("expired item entity was not removed")
	}
}

func TestItemEntityPickupDelayAndFullPickup(t *testing.T) {
	runtime, session, connection := newItemPickupTestSession(t)

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 2}, session.Player.Position, game.Velocity{}, 2)

	connection.reset()

	runtime.Tick()

	if !session.snapshotPlayer().Inventory.Hotbar[0].Empty() || item.PickupDelay != 1 {
		t.Fatalf("early pickup state = stack %+v, delay %d", session.snapshotPlayer().Inventory.Hotbar[0], item.PickupDelay)
	}

	runtime.Tick()

	if !session.snapshotPlayer().Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 2}) {
		t.Fatalf("picked stack = %+v", session.snapshotPlayer().Inventory.Hotbar[0])
	}

	if !item.State.Removed || len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatal("fully picked item entity remained")
	}

	ids := connection.packetIDs(t)
	if !containsPacketSequence(ids, []int32{protocol.ClientboundTakeItemEntityID, protocol.ClientboundContainerSetContentID, protocol.ClientboundRemoveEntitiesID}) {
		t.Fatalf("pickup packet ids = %v", ids)
	}
}

func TestItemEntityPartialPickupKeepsRemainder(t *testing.T) {
	runtime, session, connection := newItemPickupTestSession(t)

	for slot := 9; slot <= 44; slot++ {
		*session.Player.Inventory.Slot(slot) = game.ItemStack{Item: game.ItemDirt, Count: 64}
	}

	session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 63}

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 2}, session.Player.Position, game.Velocity{}, 0)

	connection.reset()

	runtime.Tick()

	if session.snapshotPlayer().Inventory.Hotbar[0].Count != 64 || !item.Stack.Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) || item.State.Removed {
		t.Fatalf("partial pickup = inventory %+v, entity %+v, removed %v", session.snapshotPlayer().Inventory.Hotbar[0], item.Stack, item.State.Removed)
	}

	ids := connection.packetIDs(t)
	if !containsPacketSequence(ids, []int32{protocol.ClientboundTakeItemEntityID, protocol.ClientboundContainerSetContentID, protocol.ClientboundEntityMetadataID}) {
		t.Fatalf("partial pickup packet ids = %v", ids)
	}

	for _, packet := range connection.packets(t) {
		if packet.ID != protocol.ClientboundTakeItemEntityID {
			continue
		}

		reader := protocol.NewPacketReader(packet.Data)
		if reader.VarInt() != item.State.ID || reader.VarInt() != session.Player.EntityID || reader.VarInt() != 2 {
			t.Fatal("partial pickup animation did not use vanilla original count")
		}

		return
	}

	t.Fatal("partial pickup sent no take animation")
}

func TestItemEntityTargetAndConcurrentPickupCannotDuplicate(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	first, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "First")
	second, _ := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Second")

	for _, session := range []*Session{first, second} {
		session.Player.Position = game.Position{Y: 64}
		session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

		joinTestSession(t, runtime, session)

		runtime.setSessionActiveChunks(session, []LoadedChunk{{}})
	}

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{Y: 64}, game.Velocity{}, 0)

	item.TargetUUID = second.Player.UUID

	runtime.Tick()

	if !first.snapshotPlayer().Inventory.Hotbar[0].Empty() || second.snapshotPlayer().Inventory.Hotbar[0].Count != 1 {
		t.Fatalf("targeted pickup inventories = first %+v, second %+v", first.snapshotPlayer().Inventory.Hotbar[0], second.snapshotPlayer().Inventory.Hotbar[0])
	}

	if len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatal("picked entity remained after competing player scan")
	}
}

func TestItemPickupSynchronizesOpenBarrelAndChestMenus(t *testing.T) {
	t.Run("barrel", func(t *testing.T) {
		position := game.BlockPosition{Y: 64}

		runtime, session, connection := newBarrelTestRuntime(t, position)

		openBarrelForTest(t, runtime, session, position)

		clearPlayerStorage(session)

		connection.reset()

		item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, session.Player.Position, game.Velocity{}, 0)

		connection.reset()

		runtime.Tick()

		assertOpenContainerPickup(t, session, connection, item, 54)
	})

	t.Run("chest", func(t *testing.T) {
		position := game.BlockPosition{Y: 64}

		runtime, session, connection := newChestTestRuntime(t, position)

		openChestForTest(t, runtime, session, position)

		clearPlayerStorage(session)

		connection.reset()

		item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, session.Player.Position, game.Velocity{}, 0)

		connection.reset()

		runtime.Tick()

		assertOpenContainerPickup(t, session, connection, item, 54)
	})
}

func newItemPickupTestSession(t *testing.T) (*Runtime, *Session, *recordingConnection) {
	t.Helper()

	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.Position = game.Position{Y: 64}
	session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, session)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	return runtime, session, connection
}

func assertOpenContainerPickup(t *testing.T, session *Session, connection *recordingConnection, item *runtimeItemEntity, playerMenuSlot int) {
	t.Helper()

	if item.State.Removed == false || session.Player.Inventory.Hotbar[0].Count != 1 {
		t.Fatalf("open-container pickup state = removed %v, inventory %+v", item.State.Removed, session.Player.Inventory.Hotbar[0])
	}

	for _, packet := range connection.packets(t) {
		if packet.ID != protocol.ClientboundContainerSetContentID {
			continue
		}

		reader := protocol.NewPacketReader(packet.Data)
		if windowID := reader.VarInt(); windowID != session.activeMenu().windowID {
			t.Fatalf("pickup menu window = %d, want %d", windowID, session.activeMenu().windowID)
		}

		reader.VarInt()

		itemCount := int(reader.VarInt())
		if itemCount <= playerMenuSlot {
			t.Fatalf("pickup menu item count = %d", itemCount)
		}

		var picked game.ItemStack

		for slot := range itemCount {
			stack := readSimpleItemStack(t, reader)
			if slot == playerMenuSlot {
				picked = stack
			}
		}

		if !picked.Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) {
			t.Fatalf("pickup menu player slot = %+v", picked)
		}

		return
	}

	t.Fatal("pickup did not synchronize the active container menu")
}

func clearPlayerStorage(session *Session) {
	for slot := 9; slot <= 44; slot++ {
		*session.Player.Inventory.Slot(slot) = game.ItemStack{}
	}
}

func containsPacketSequence(ids, expected []int32) bool {
	next := 0

	for _, id := range ids {
		if id == expected[next] {
			next++
			if next == len(expected) {
				return true
			}
		}
	}

	return false
}
