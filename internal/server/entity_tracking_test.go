package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type itemTrajectorySnapshot struct {
	tick     int
	position game.Position
	velocity game.Velocity
}

type collisionAxisOrderTestCase struct {
	name     string
	movement game.Velocity
	expected game.Velocity
}

type itemCollisionShapeTestCase struct {
	name     string
	block    game.Block
	expected float64
}

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

	packets := runtimeEntityPackets(view, &tracker, configuration, false)
	if len(packets) != 1 || packets[0].ID != protocol.ClientboundUpdateEntityPositionID {
		t.Fatalf("relative position packets = %+v", packets)
	}

	position, valid := packets[0].Encoder.(protocol.UpdateEntityPosition)
	if !valid || position.DeltaX != 1024 || position.DeltaY != 0 || position.DeltaZ != 0 {
		t.Fatalf("relative position packet = %+v", packets[0].Encoder)
	}

	view.Position.X = 8.25

	packets = runtimeEntityPackets(view, &tracker, configuration, false)
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundSynchronizeEntityPositionID)

	view.Position = tracker.PositionBase
	view.OnGround = true

	packets = runtimeEntityPackets(view, &tracker, configuration, false)
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

	packets := runtimeEntityPackets(view, &tracker, configuration, false)
	if len(packets) != 0 {
		t.Fatalf("sub-threshold velocity packets = %+v", packets)
	}

	view.Velocity.X = 0.001

	packets = runtimeEntityPackets(view, &tracker, configuration, false)
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundSetEntityMotionID)

	view.Velocity = game.Velocity{}

	packets = runtimeEntityPackets(view, &tracker, configuration, false)
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundSetEntityMotionID)

	tracker = newRuntimeEntityTracker(runtimeEntityView{ID: view.ID})
	tracker.UpdateTick = 1
	view.Velocity.X = 0.001

	packets = runtimeEntityPackets(view, &tracker, RuntimeEntityTrackingConfig{}, true)
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundSetEntityMotionID)
}

func TestRuntimeEntityPacketsRefreshAndAvoidRepeatedFullSync(t *testing.T) {
	view := runtimeEntityView{ID: 7, OnGround: true}

	tracker := newRuntimeEntityTracker(view)

	tracker.UpdateTick = entityPositionRefreshTicks

	packets := runtimeEntityPackets(view, &tracker, RuntimeEntityTrackingConfig{}, false)
	assertRuntimeEntityPacketID(t, packets, protocol.ClientboundUpdateEntityPositionID)

	tracker.UpdateTick = 1

	for range 5 {
		packets = runtimeEntityPackets(view, &tracker, RuntimeEntityTrackingConfig{}, false)
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

	stoneDrag := float64(float32(0.6) * float32(0.98))
	iceDrag := float64(float32(0.98) * float32(0.98))
	blueIceDrag := float64(float32(0.989) * float32(0.98))

	if math.Abs(stoneVelocity-0.1*stoneDrag) > 1e-15 || math.Abs(iceVelocity-0.1*iceDrag) > 1e-15 || math.Abs(blueIceVelocity-0.1*blueIceDrag) > 1e-15 {
		t.Fatalf("surface velocities = stone %v, ice %v, blue ice %v", stoneVelocity, iceVelocity, blueIceVelocity)
	}
}

func TestItemEntityVanillaAirTrajectory(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	item := runtime.SpawnItemEntity(
		game.ItemStack{Item: game.ItemStone, Count: 1},
		game.Position{Y: 10},
		game.Velocity{X: 0.3, Y: 0.1},
		32767,
	)

	snapshots := []itemTrajectorySnapshot{
		{tick: 1, position: game.Position{X: 0.3, Y: 10.06}, velocity: game.Velocity{X: 0.2940000057220459, Y: 0.058800000000000005}},
		{tick: 5, position: game.Position{X: 1.4411881029657905, Y: 9.896157929600003}, velocity: game.Velocity{X: 0.27117626542916573, Y: -0.09792315859200001}},
		{tick: 10, position: game.Position{X: 2.743908128109814, Y: 8.841500890582672}, velocity: game.Velocity{X: 0.24512188977369778, Y: -0.27683001781165345}},
		{tick: 20, position: game.Position{X: 4.985881280535439, Y: 4.23637890922527}, velocity: game.Velocity{X: 0.20028246948742953, Y: -0.5847275781845053}},
	}

	currentTick := 0

	for _, snapshot := range snapshots {
		for currentTick < snapshot.tick {
			runtime.Tick()

			currentTick++
		}

		assertPositionClose(t, item.State.Position, snapshot.position, 1e-12)
		assertVelocityClose(t, item.Velocity, snapshot.velocity, 1e-12)
	}
}

func TestItemLandingForcesImmediateSynchronization(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.Position = game.Position{X: 8, Y: 1}
	session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, session)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	connection.reset()

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 3, Z: 0.5}, game.Velocity{Y: 0.1}, 32767)

	assertPacketIDs(t, connection.packetIDs(t), []int32{
		protocol.ClientboundAddEntityID,
		protocol.ClientboundEntityMetadataID,
	})

	connection.reset()

	runtime.Tick()

	connection.reset()

	for tick := 2; tick <= 12; tick++ {
		runtime.Tick()

		if packets := connection.packetIDs(t); len(packets) != 0 {
			t.Fatalf("ordinary air tick %d packets = %v", tick, packets)
		}
	}

	runtime.Tick()

	if item.State.Position.Y != 1 || item.Velocity.Y != 0 || !item.OnGround {
		t.Fatalf("landing state = position %+v, velocity %+v, onGround %t", item.State.Position, item.Velocity, item.OnGround)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{
		protocol.ClientboundSetEntityMotionID,
		protocol.ClientboundSynchronizeEntityPositionID,
	})

	connection.reset()

	runtime.Tick()

	if packets := connection.packetIDs(t); len(packets) != 0 {
		t.Fatalf("settled post-landing packets = %v", packets)
	}
}

func TestCollisionAxisOrderMatchesVanilla(t *testing.T) {
	box := game.AABB{MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1}
	zBarrier := game.AABB{MinX: 0.25, MinY: 0, MinZ: 1, MaxX: 0.5, MaxY: 1, MaxZ: 1.25}
	xBarrier := game.AABB{MinX: 1, MinY: 0, MinZ: 0.25, MaxX: 1.25, MaxY: 1, MaxZ: 0.5}

	tests := []collisionAxisOrderTestCase{
		{name: "Z dominant resolves Z before X", movement: game.Velocity{X: 0.8, Z: 1}, expected: game.Velocity{X: 0.8}},
		{name: "X dominant resolves X before Z", movement: game.Velocity{X: 1, Z: 0.8}, expected: game.Velocity{Z: 0.8}},
		{name: "equal diagonal resolves X before Z", movement: game.Velocity{X: 1, Z: 1}, expected: game.Velocity{Z: 1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			barrier := zBarrier
			if math.Abs(test.movement.X) >= math.Abs(test.movement.Z) {
				barrier = xBarrier
			}

			resolved := collideAABBWithBlocks(box, []game.AABB{barrier}, test.movement)
			if resolved != test.expected {
				t.Fatalf("resolved movement = %+v, want %+v", resolved, test.expected)
			}
		})
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

	tests := []itemCollisionShapeTestCase{
		{name: "full block", block: game.Stone, expected: 1},
		{name: "bottom slab", block: bottomSlab, expected: 0.5},
		{name: "stairs", block: game.OakStairs, expected: 1},
		{name: "fence", block: game.OakFence, expected: 1.5},
		{name: "wall", block: game.CobblestoneWall, expected: 1.5},
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

func assertPositionClose(t *testing.T, actual, expected game.Position, tolerance float64) {
	t.Helper()

	if math.Abs(actual.X-expected.X) > tolerance || math.Abs(actual.Y-expected.Y) > tolerance || math.Abs(actual.Z-expected.Z) > tolerance {
		t.Fatalf("position = %+v, want %+v", actual, expected)
	}
}

func assertVelocityClose(t *testing.T, actual, expected game.Velocity, tolerance float64) {
	t.Helper()

	if math.Abs(actual.X-expected.X) > tolerance || math.Abs(actual.Y-expected.Y) > tolerance || math.Abs(actual.Z-expected.Z) > tolerance {
		t.Fatalf("velocity = %+v, want %+v", actual, expected)
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
