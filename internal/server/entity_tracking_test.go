package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestItemEntityTrackingConfig(t *testing.T) {
	entity := &runtimeItemEntity{}

	configuration := entity.RuntimeEntityTrackingConfig()
	if configuration != (RuntimeEntityTrackingConfig{ClientRangeChunks: 6, UpdateInterval: 20, TrackDeltas: true}) {
		t.Fatalf("item tracking configuration = %+v", configuration)
	}
}

func TestRuntimeEntityPacketsPositionAndGroundState(t *testing.T) {
	configuration := RuntimeEntityTrackingConfig{}

	view := runtimeEntityView{ID: 7, Position: game.Position{X: 0.25}}

	tracker := newRuntimeEntityTracker(runtimeEntityView{ID: view.ID})

	tracker.UpdateTick = 1

	packets := runtimeEntityPackets(view, &tracker, configuration)
	if len(packets) != 1 || packets[0].ID != protocol.ClientboundUpdateEntityPositionID {
		t.Fatalf("relative position packets = %+v", packets)
	}

	position, valid := packets[0].Encoder.(protocol.UpdateEntityPosition)
	if !valid || position.DeltaX != 1024 || position.DeltaY != 0 || position.DeltaZ != 0 {
		t.Fatalf("relative position packet = %+v", packets[0].Encoder)
	}

	view.Position.X = 8.25

	packets = runtimeEntityPackets(view, &tracker, configuration)
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundSynchronizeEntityPositionID)

	view.Position = tracker.PositionBase
	view.OnGround = true

	packets = runtimeEntityPackets(view, &tracker, configuration)
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundSynchronizeEntityPositionID)
}

func TestProtocolPositionDeltaUsesAbsoluteRounding(t *testing.T) {
	validPrevious := 0.49 / entityPositionScale
	validCurrent := 0.5 / entityPositionScale

	delta, valid := protocolPositionDelta(validPrevious, validCurrent)
	if !valid || delta != 1 {
		t.Fatalf("rounded delta = (%d, %t), want (1, true)", delta, valid)
	}

	for _, position := range []float64{
		32767.49 / entityPositionScale,
		-32768.49 / entityPositionScale,
	} {
		_, valid = protocolPositionDelta(0, position)
		if !valid {
			t.Fatalf("in-range position %v did not fit", position)
		}
	}

	for _, position := range []float64{
		32767.5 / entityPositionScale,
		-32768.5 / entityPositionScale,
	} {
		delta, valid = protocolPositionDelta(0, position)
		if valid || delta != 0 {
			t.Fatalf("out-of-range position %v = (%d, %t), want (0, false)", position, delta, valid)
		}
	}
}

func TestRuntimeEntityPacketsVelocityThresholdAndStop(t *testing.T) {
	configuration := RuntimeEntityTrackingConfig{TrackDeltas: true}

	view := runtimeEntityView{ID: 7}

	tracker := newRuntimeEntityTracker(view)

	tracker.UpdateTick = 1

	view.Velocity.X = 0.0003

	packets := runtimeEntityPackets(view, &tracker, configuration)
	if len(packets) != 0 {
		t.Fatalf("sub-threshold velocity packets = %+v", packets)
	}

	view.Velocity.X = 0.001

	packets = runtimeEntityPackets(view, &tracker, configuration)
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundSetEntityMotionID)

	view.Velocity = game.Velocity{}

	packets = runtimeEntityPackets(view, &tracker, configuration)
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundSetEntityMotionID)
}

func TestRuntimeEntityPacketsRefreshAndAvoidRepeatedFullSync(t *testing.T) {
	view := runtimeEntityView{ID: 7, OnGround: true}

	tracker := newRuntimeEntityTracker(view)

	tracker.UpdateTick = entityPositionRefreshTicks

	packets := runtimeEntityPackets(view, &tracker, RuntimeEntityTrackingConfig{})
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundUpdateEntityPositionID)

	tracker.UpdateTick = 1

	for range 5 {
		packets = runtimeEntityPackets(view, &tracker, RuntimeEntityTrackingConfig{})
		if len(packets) != 0 {
			t.Fatalf("settled entity packets = %+v", packets)
		}
	}
}

func TestSettledItemUsesVanillaUpdateCadence(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.Position = game.Position{X: 8, Y: 1}
	session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, session)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 1, Z: 0.5}, game.Velocity{}, 32767)

	connection.reset()

	for range 80 {
		runtime.Tick()
	}

	fullSynchronizations := 0
	periodicRefreshes := 0

	for _, packetID := range connection.packetIDs(t) {
		switch packetID {
		case protocol.ClientboundSynchronizeEntityPositionID:
			fullSynchronizations++
		case protocol.ClientboundUpdateEntityPositionID:
			periodicRefreshes++
		}
	}

	if fullSynchronizations != 1 || periodicRefreshes != 1 {
		t.Fatalf("settled item movement packets = %d full, %d relative; want initial landing and one 60-tick refresh", fullSynchronizations, periodicRefreshes)
	}
}

func TestItemEntityRestingMovementAndSurfaceFriction(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	resting := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 1, Z: 0.5}, game.Velocity{}, 32767)

	resting.State.mu.Lock()
	resting.OnGround = true
	resting.State.mu.Unlock()

	runtime.Tick()

	if resting.State.Position.Y != 1 || resting.Velocity.Y != -itemEntityGravity {
		t.Fatalf("first resting tick = position %+v, velocity %+v", resting.State.Position, resting.Velocity)
	}

	runtime.Tick()

	if resting.State.Position.Y != 1 || resting.Velocity.Y != -2*itemEntityGravity {
		t.Fatalf("second resting tick = position %+v, velocity %+v", resting.State.Position, resting.Velocity)
	}

	runtime.Tick()

	if resting.State.Position.Y != 1 || resting.Velocity.Y != 0 {
		t.Fatalf("cadence movement tick = position %+v, velocity %+v", resting.State.Position, resting.Velocity)
	}

	stoneVelocity := itemVelocityAfterSurfaceTick(t, game.Stone)
	iceVelocity := itemVelocityAfterSurfaceTick(t, game.Ice)
	blueIceVelocity := itemVelocityAfterSurfaceTick(t, game.BlueIce)

	if math.Abs(stoneVelocity-0.1*0.6*itemEntityDrag) > 1e-15 || math.Abs(iceVelocity-0.1*0.98*itemEntityDrag) > 1e-15 || math.Abs(blueIceVelocity-0.1*0.989*itemEntityDrag) > 1e-15 {
		t.Fatalf("surface velocities = stone %v, ice %v, blue ice %v", stoneVelocity, iceVelocity, blueIceVelocity)
	}
}

func TestItemCollisionUsesResolvedBlockShapes(t *testing.T) {
	bottomSlab, valid := game.OakSlab.WithProperties(game.BlockPropertyValue{Name: "type", Value: "bottom"})
	if !valid {
		t.Fatal("resolve bottom slab")
	}

	eightLayerSnow, valid := game.Snow.WithProperties(game.BlockPropertyValue{Name: "layers", Value: "8"})
	if !valid {
		t.Fatal("resolve eight-layer snow")
	}

	tests := []struct {
		name     string
		block    game.Block
		expected float64
	}{
		{name: "full block", block: game.Stone, expected: 1},
		{name: "bottom slab", block: bottomSlab, expected: 0.5},
		{name: "snow layers", block: eightLayerSnow, expected: 14.0 / 16.0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{}

			world.SetBlock(game.BlockPosition{}, test.block)

			runtime := NewRuntime(world)

			movement := runtime.moveItemEntity(game.Position{X: 0.5, Y: 2, Z: 0.5}, game.Velocity{Y: -2})
			if !movement.OnGround || movement.Position.Y != test.expected {
				t.Fatalf("landing = position %+v, onGround %t", movement.Position, movement.OnGround)
			}
		})
	}
}

func TestItemCollisionBounceWallAndEmbeddedEscape(t *testing.T) {
	t.Run("slime bounce", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{}, game.SlimeBlock)

		runtime := NewRuntime(world)

		viewer := &Session{}

		runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

		item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 2, Z: 0.5}, game.Velocity{Y: -1}, 32767)

		runtime.Tick()

		if !item.OnGround || item.State.Position.Y != 1 || item.Velocity.Y <= 0 {
			t.Fatalf("slime landing = position %+v, velocity %+v, onGround %t", item.State.Position, item.Velocity, item.OnGround)
		}
	})

	t.Run("horizontal collision", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{}, game.Stone)
		world.SetBlock(game.BlockPosition{X: 1, Y: 1}, game.Stone)

		runtime := NewRuntime(world)

		viewer := &Session{}

		runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

		item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 1, Z: 0.5}, game.Velocity{X: 1}, 32767)

		runtime.Tick()

		if item.State.Position.X != 0.875 || item.Velocity.X != 0 {
			t.Fatalf("wall collision = position %+v, velocity %+v", item.State.Position, item.Velocity)
		}
	})

	t.Run("embedded escape", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{}, game.Stone)

		runtime := NewRuntime(world)

		viewer := &Session{}

		runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

		item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 0.1, Z: 0.5}, game.Velocity{}, 32767)

		runtime.Tick()

		if item.State.Position.Z >= 0.5 || item.Velocity.Z >= -0.1 || item.Velocity.Z < -0.3 {
			t.Fatalf("embedded escape = position %+v, velocity %+v", item.State.Position, item.Velocity)
		}
	})
}

func TestPlayerMovementReconcilesItemTracking(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.loadedChunks = map[LoadedChunk]struct{}{{X: 5}: {}}

	joinTestSession(t, runtime, session)

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 95, Y: 64}, game.Velocity{}, 32767)
	if !session.tracksRuntimeEntity(item.State.ID) {
		t.Fatal("item inside tracking range was not tracked")
	}

	connection.reset()

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.Position.X = -2
	})

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundRemoveEntitiesID})

	connection.reset()

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.Position.X = 0
	})

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID})
}

func TestItemEntityMergeSynchronizesMetadataAndRemoval(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, session)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	first := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 20}, game.Position{Y: 64}, game.Velocity{}, 40)
	second := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 6}, game.Position{Y: 64}, game.Velocity{}, 40)

	second.TickCount = 39

	connection.reset()

	second.Tick(runtime, nil)

	if first.Stack.Count != 26 || !second.State.Removed {
		t.Fatalf("merged items = first %+v, second removed %t", first.Stack, second.State.Removed)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundEntityMetadataID, protocol.ClientboundRemoveEntitiesID})
}

func TestItemEntityMergeRejectsFullOrIncompatibleStacks(t *testing.T) {
	full := &runtimeItemEntity{State: RuntimeEntityState{ID: 1}, Stack: game.ItemStack{Item: game.ItemStone, Count: 64}}
	additional := &runtimeItemEntity{State: RuntimeEntityState{ID: 2}, Stack: game.ItemStack{Item: game.ItemStone, Count: 1}}

	removed, receiver, consumed := mergeItemEntities(additional, full)
	if removed || receiver != nil || consumed != nil || full.Stack.Count != 64 || additional.Stack.Count != 1 {
		t.Fatalf("full stack merge = removed %t receiver %v consumed %v", removed, receiver, consumed)
	}

	componented := &runtimeItemEntity{
		State: RuntimeEntityState{ID: 3},
		Stack: game.ItemStack{Item: game.ItemStone, Count: 1, Components: []game.ItemComponent{{Type: 1, Data: []byte{1}}}},
	}

	incompatibleComponent := &runtimeItemEntity{
		State: RuntimeEntityState{ID: 4},
		Stack: game.ItemStack{Item: game.ItemStone, Count: 1, Components: []game.ItemComponent{{Type: 1, Data: []byte{2}}}},
	}

	removed, receiver, consumed = mergeItemEntities(componented, incompatibleComponent)
	if removed || receiver != nil || consumed != nil {
		t.Fatalf("component-incompatible merge = removed %t receiver %v consumed %v", removed, receiver, consumed)
	}

	differentItem := &runtimeItemEntity{State: RuntimeEntityState{ID: 5}, Stack: game.ItemStack{Item: game.ItemDirt, Count: 1}}

	removed, receiver, consumed = mergeItemEntities(componented, differentItem)
	if removed || receiver != nil || consumed != nil {
		t.Fatalf("item-incompatible merge = removed %t receiver %v consumed %v", removed, receiver, consumed)
	}
}

func assertRuntimeEntityPacketID(t *testing.T, packets []runtimeEntityPacket, expected int32) {
	t.Helper()

	if len(packets) != 1 || packets[0].ID != expected {
		t.Fatalf("packet ids = %+v, want [%d]", packets, expected)
	}
}

func itemVelocityAfterSurfaceTick(t *testing.T, surface game.Block) float64 {
	t.Helper()

	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, surface)

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 1, Z: 0.5}, game.Velocity{X: 0.1}, 32767)

	item.State.mu.Lock()
	item.OnGround = true
	item.State.mu.Unlock()

	runtime.Tick()

	return item.Velocity.X
}
