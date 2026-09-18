package server

import (
	"math"
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type boatStatusTestCase struct {
	name   string
	block  game.Block
	status boatStatus
}

type boatGameModeTestCase struct {
	name      string
	gameMode  game.GameMode
	wantCount int32
}

var boatBenchmarkPassengerCounts = [...]int{0, 1, 2}

func TestSpawnBoatVariants(t *testing.T) {
	variants := [...]game.EntityType{game.EntityAcaciaBoat, game.EntityBirchBoat, game.EntityCherryBoat, game.EntityDarkOakBoat, game.EntityJungleBoat, game.EntityMangroveBoat, game.EntityOakBoat, game.EntityPaleOakBoat, game.EntitySpruceBoat, game.EntityBambooRaft}

	runtime := NewRuntime(&game.World{})

	for _, entityType := range variants {
		boat := runtime.SpawnBoat(entityType, game.Position{})
		if boat == nil || boat.State.Type != entityType {
			t.Fatalf("SpawnBoat(%v) did not register its variant", entityType)
		}

		metadata := boat.EntityMetadata()
		if len(metadata) != 6 || metadata[0].Index != 8 || metadata[5].Index != 13 {
			t.Fatalf("boat metadata = %#v, want indices 8 through 13", metadata)
		}
	}

	if runtime.SpawnBoat(game.EntityOakChestBoat, game.Position{}) != nil {
		t.Fatal("chest boat was registered")
	}
}

func TestBoatStatusAndFloatTrace(t *testing.T) {
	world := &game.World{}

	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			world.SetBlock(game.BlockPosition{X: x, Y: 0, Z: z}, game.Water)
		}
	}

	runtime := NewRuntime(world)

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 0.4})

	boat.Tick(runtime, nil)

	wantSurface := 8.0 / 9.0

	wantLevel := 0.4 + boatHeight
	if boat.Status != boatStatusWater || boat.WaterLevel != wantLevel {
		t.Fatalf("water status = %v at level %v, want water at %v", boat.Status, boat.WaterLevel, wantLevel)
	}

	wantSnap := wantSurface - boatHeight + 0.101
	if math.Abs(boat.State.Position.Y-wantSnap) > 1e-9 {
		t.Fatalf("water entry Y = %v, want vanilla surface snap", boat.State.Position.Y)
	}

	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			world.SetBlock(game.BlockPosition{X: x, Y: 0, Z: z}, game.Stone)
		}
	}

	boat.State.mu.Lock()
	boat.State.Position = game.Position{Y: 1}
	boat.Velocity = game.Velocity{}
	boat.State.mu.Unlock()

	boat.Tick(runtime, nil)

	if boat.Status != boatStatusLand {
		t.Fatalf("land status = %v, want land", boat.Status)
	}

	boat.State.mu.Lock()
	boat.State.Position = game.Position{Y: 5}
	boat.Velocity = game.Velocity{}
	boat.State.mu.Unlock()

	boat.Tick(runtime, nil)

	if boat.Status != boatStatusAir || boat.Velocity.Y >= 0 {
		t.Fatalf("air status/velocity = %v/%v, want falling airboat", boat.Status, boat.Velocity.Y)
	}
}

func TestBoatLongQuantitativeTraces(t *testing.T) {
	t.Run("air", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 20})

		boat.Velocity.X = 1

		for range 20 {
			boat.Tick(runtime, nil)
		}

		assertBoatFloatClose(t, boat.State.Position.X, 7.905810108684875)
		assertBoatFloatClose(t, boat.State.Position.Y, 11.6)
		assertBoatFloatClose(t, boat.Velocity.X, 0.1215766545905694)
		assertBoatFloatClose(t, boat.Velocity.Y, -0.8)
	})

	t.Run("land", func(t *testing.T) {
		world := &game.World{}

		for x := int32(-2); x <= 3; x++ {
			for z := int32(-1); z <= 1; z++ {
				world.SetBlock(game.BlockPosition{X: x, Z: z}, game.Stone)
			}
		}

		runtime := NewRuntime(world)

		boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 1})

		boat.Velocity.X = 1

		for range 20 {
			boat.Tick(runtime, nil)
		}

		assertBoatFloatClose(t, boat.State.Position.X, 1.4999451570198654)
		assertBoatFloatClose(t, boat.Velocity.X, math.Pow(0.6, 20))

		if boat.Status != boatStatusLand || boat.Velocity.Y != 0 {
			t.Fatalf("land trace status/vertical velocity = %v/%v", boat.Status, boat.Velocity.Y)
		}
	})

	t.Run("water", func(t *testing.T) {
		world := &game.World{}

		for x := int32(-2); x <= 2; x++ {
			for z := int32(-2); z <= 2; z++ {
				world.SetBlock(game.BlockPosition{X: x, Z: z}, game.Water)
			}
		}

		runtime := NewRuntime(world)

		boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 0.4})

		for range 20 {
			boat.Tick(runtime, nil)
		}

		assertBoatFloatClose(t, boat.State.Position.Y, 0.5224210002651635)
		assertBoatFloatClose(t, boat.Velocity.Y, -0.0019509213188204158)

		if boat.Status != boatStatusWater {
			t.Fatalf("water trace status = %v", boat.Status)
		}
	})
}

func TestBoatUnderwaterStatuses(t *testing.T) {
	tests := [...]boatStatusTestCase{
		{name: "source", block: game.Water, status: boatStatusUnderWater},
		{name: "flowing", block: mustFluidBlock(t, game.Water, "1"), status: boatStatusUnderFlowingWater},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{}

			for x := int32(-1); x <= 1; x++ {
				for z := int32(-1); z <= 1; z++ {
					world.SetBlock(game.BlockPosition{X: x, Z: z}, test.block)
				}
			}

			runtime := NewRuntime(world)

			boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{})

			boat.Status = boatStatusWater

			boat.Tick(runtime, nil)

			if boat.Status != test.status {
				t.Fatalf("status = %v, want %v", boat.Status, test.status)
			}
		})
	}
}

func TestBoatMoveVehicleAcceptsClearMovementAndCorrectsRejection(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	controller, connection := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000006", "controller")

	runtime.AssignEntityID(controller)
	runtime.addSession(controller)

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 5})

	if !runtime.MountPassenger(boat, controller) {
		t.Fatal("controller mount failed")
	}

	connection.reset()

	controller.handleMoveVehicle(protocol.MoveVehicle{X: 0.25, Y: 5, Z: 0.5, Yaw: 30, Pitch: 5, OnGround: false})

	assertBoatPositionClose(t, boat.State.Position, game.Position{X: 0.25, Y: 5, Z: 0.5})
	assertBoatPositionClose(t, controller.playerView().Position, boatPassengerPosition(boat.State.Position, 30, 0, 1, false))

	if len(packetsByID(t, connection, protocol.ClientboundMoveVehicleID)) != 0 {
		t.Fatal("clear vehicle movement was corrected")
	}

	controller.handleMoveVehicle(protocol.MoveVehicle{X: 20, Y: 5, Z: 0.5, Yaw: 45, Pitch: 10})

	assertBoatPositionClose(t, boat.State.Position, game.Position{X: 0.25, Y: 5, Z: 0.5})

	packets := packetsByID(t, connection, protocol.ClientboundMoveVehicleID)
	if len(packets) != 1 {
		t.Fatalf("vehicle corrections = %d, want 1", len(packets))
	}

	reader := protocol.NewPacketReader(packets[0].Data)

	correction := game.Position{X: reader.Double(), Y: reader.Double(), Z: reader.Double()}

	yaw := reader.Float()
	pitch := reader.Float()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode vehicle correction: %v", err)
	}

	assertBoatPositionClose(t, correction, boat.State.Position)

	if yaw != 30 || pitch != 5 {
		t.Fatalf("correction rotation = %v/%v, want 30/5", yaw, pitch)
	}
}

func TestBoatMoveVehicleRejectsVerticalCollision(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	controller, connection := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000016", "controller")

	runtime.AssignEntityID(controller)
	runtime.addSession(controller)

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 1})

	boat.Rotation = game.Rotation{Yaw: 20, Pitch: 3}

	if !runtime.MountPassenger(boat, controller) {
		t.Fatal("controller mount failed")
	}

	connection.reset()

	controller.handleMoveVehicle(protocol.MoveVehicle{Y: -1, Yaw: 80, Pitch: 10})

	assertBoatPositionClose(t, boat.State.Position, game.Position{Y: 1})

	packets := packetsByID(t, connection, protocol.ClientboundMoveVehicleID)
	if len(packets) != 1 {
		t.Fatalf("vehicle corrections = %d, want 1", len(packets))
	}

	reader := protocol.NewPacketReader(packets[0].Data)

	correction := game.Position{X: reader.Double(), Y: reader.Double(), Z: reader.Double()}

	yaw := reader.Float()
	pitch := reader.Float()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode vehicle correction: %v", err)
	}

	assertBoatPositionClose(t, correction, game.Position{Y: 1})

	if yaw != 20 || pitch != 3 {
		t.Fatalf("correction rotation = %v/%v, want 20/3", yaw, pitch)
	}
}

func TestBoatMountSeatsAndInput(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	first, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000001", "first")
	second, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000002", "second")
	third, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000004", "third")

	runtime.AssignEntityID(first)
	runtime.AssignEntityID(second)
	runtime.AssignEntityID(third)

	runtime.addSession(first)
	runtime.addSession(second)
	runtime.addSession(third)

	boat := runtime.SpawnBoat(game.EntityBambooRaft, game.Position{Y: 5})

	if !runtime.MountPassenger(boat, first) || !runtime.MountPassenger(boat, second) {
		t.Fatal("boat did not accept two ordered passengers")
	}

	if runtime.MountPassenger(boat, third) {
		t.Fatal("boat accepted a third passenger")
	}

	first.Player.Rotation.Yaw = 180

	first.handlePaddleBoat(protocol.PaddleBoat{LeftPaddle: true, RightPaddle: true})

	boat.Tick(runtime, nil)

	firstPosition := first.playerView().Position
	secondPosition := second.playerView().Position

	wantY := boat.State.Position.Y - 0.1

	assertBoatPositionClose(t, firstPosition, game.Position{Y: wantY, Z: boat.State.Position.Z + 0.2})
	assertBoatPositionClose(t, secondPosition, game.Position{Y: wantY, Z: boat.State.Position.Z - 0.6})

	if boat.Velocity.Z != 0 || !boat.LeftPaddle || !boat.RightPaddle {
		t.Fatalf("client paddle state changed server velocity: %#v/%t/%t", boat.Velocity, boat.LeftPaddle, boat.RightPaddle)
	}

	if first.playerView().Rotation.Yaw != -105 {
		t.Fatalf("mounted player yaw = %v, want -105", first.playerView().Rotation.Yaw)
	}

	first.handleMoveVehicle(protocol.MoveVehicle{X: boat.State.Position.X, Y: boat.State.Position.Y, Z: boat.State.Position.Z, Yaw: 90})

	boat.State.mu.RLock()
	yaw := boat.Rotation.Yaw
	boat.State.mu.RUnlock()

	if yaw != 90 {
		t.Fatalf("controlling vehicle rotation = %v, want 90", yaw)
	}
}

func TestBoatPassengerGeometry(t *testing.T) {
	position := game.Position{X: 4, Y: 8, Z: 12}

	ordinary := boatPassengerPosition(position, 90, 0, 2, false)
	assertBoatPositionClose(t, ordinary, game.Position{X: 3.8, Y: 7.5875, Z: 12})

	second := boatPassengerPosition(position, 90, 1, 2, false)
	assertBoatPositionClose(t, second, game.Position{X: 4.6, Y: 7.5875, Z: 12})

	raft := boatPassengerPosition(position, 0, 0, 1, true)
	assertBoatPositionClose(t, raft, game.Position{X: 4, Y: 7.9, Z: 12})
}

func TestBoatDismountUsesSafeSidePosition(t *testing.T) {
	world := &game.World{}

	for x := int32(-3); x <= 3; x++ {
		for z := int32(-3); z <= 3; z++ {
			world.SetBlock(game.BlockPosition{X: x, Z: z}, game.Stone)
		}
	}

	runtime := NewRuntime(world)

	passenger, connection := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000005", "passenger")

	runtime.AssignEntityID(passenger)
	runtime.addSession(passenger)
	runtime.setSessionActiveChunks(passenger, []LoadedChunk{{}})

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 1})

	passenger.trackRuntimeEntity(boat)

	if !runtime.MountPassenger(boat, passenger) {
		t.Fatal("mount failed")
	}

	connection.reset()

	if !runtime.DismountPassenger(passenger) {
		t.Fatal("dismount failed")
	}

	position := passenger.playerView().Position
	wantDistance := (boatWidth*math.Sqrt2 + 0.6 + 0.00001) / 2

	assertBoatPositionClose(t, position, game.Position{Y: 1, Z: wantDistance})

	if passenger.VehicleID() != 0 {
		t.Fatal("dismounted passenger retained vehicle")
	}

	packets := connection.packets(t)
	if len(packets) < 2 || packets[0].ID != protocol.ClientboundSetPassengersID || packets[1].ID != protocol.ClientboundPlayerPositionID {
		t.Fatalf("dismount packet order = %#v, want passengers then player position", packets)
	}
}

func TestBoatItemPlacesVariantAndConsumesSurvivalStack(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	world.SetBlock(game.BlockPosition{X: 0, Y: 70, Z: 2}, game.Water)

	runtime := NewRuntime(world)

	player, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000026", "player")

	player.Player.Position = game.Position{X: 0.5, Y: 69.13, Z: 0.5}
	player.Player.GameMode = game.GameModeSurvival
	player.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemCherryBoat, Count: 1}

	handled, err := useBoatItem(player, player.Player.Inventory.Hotbar[0], protocol.MainHand)
	if err != nil {
		t.Fatalf("place boat: %v", err)
	}

	if !handled || !player.Player.Inventory.Hotbar[0].Empty() {
		t.Fatalf("boat placement handled/inventory = %t/%#v", handled, player.Player.Inventory.Hotbar[0])
	}

	for _, entity := range runtime.snapshotRuntimeEntities() {
		boat, boatEntity := entity.(*runtimeBoatEntity)
		if boatEntity && boat.State.Type == game.EntityCherryBoat {
			assertBoatPositionClose(t, boat.State.Position, game.Position{X: 0.5, Y: 70.75, Z: 2})

			return
		}
	}

	t.Fatal("cherry boat item did not spawn a cherry boat")
}

func TestBoatItemConsumptionByGameMode(t *testing.T) {
	tests := [...]boatGameModeTestCase{
		{name: "adventure", gameMode: game.GameModeAdventure},
		{name: "creative", gameMode: game.GameModeCreative, wantCount: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

			world.SetBlock(game.BlockPosition{X: 0, Y: 70, Z: 2}, game.Water)

			runtime := NewRuntime(world)

			player, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000027", "player")

			player.Player.Position = game.Position{X: 0.5, Y: 69.13, Z: 0.5}
			player.Player.GameMode = test.gameMode
			player.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemOakBoat, Count: 1}

			handled, err := useBoatItem(player, player.Player.Inventory.Hotbar[0], protocol.MainHand)
			if err != nil {
				t.Fatalf("place boat: %v", err)
			}

			if !handled || player.Player.Inventory.Hotbar[0].Count != test.wantCount {
				t.Fatalf("boat placement handled/count = %t/%d, want true/%d", handled, player.Player.Inventory.Hotbar[0].Count, test.wantCount)
			}
		})
	}
}

func TestMountedRuntimeEntityUsesVehicleTickAuthority(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}, {X: 1}})

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{X: 15.9, Y: 20})

	passenger := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 15.9, Y: 20}, 20)

	boat.Velocity.X = 0.2

	if !runtime.MountPassenger(boat, passenger) {
		t.Fatal("runtime passenger mount failed")
	}

	runtime.Tick()

	if passenger.TickCount != 0 {
		t.Fatalf("mounted passenger ticked independently %d times", passenger.TickCount)
	}

	if boat.State.Chunk != (LoadedChunk{X: 1}) || passenger.State.Chunk != (LoadedChunk{X: 1}) {
		t.Fatalf("vehicle/passenger chunks = %+v/%+v", boat.State.Chunk, passenger.State.Chunk)
	}

	want := boatPassengerPosition(boat.State.Position, boat.Rotation.Yaw, 0, 1, false)
	assertBoatPositionClose(t, passenger.State.Position, want)

	if !runtime.DismountPassenger(passenger) {
		t.Fatal("runtime passenger dismount failed")
	}

	runtime.Tick()

	if passenger.TickCount != 1 {
		t.Fatalf("dismounted passenger tick count = %d, want 1", passenger.TickCount)
	}
}

func TestMountedPlayerCrossesChunkDuringRuntimeTick(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	player, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000028", "player")

	player.Player.Position = game.Position{X: 15.9, Y: 20}

	runtime.AssignEntityID(player)
	runtime.addSession(player)

	player.chunkMx.Lock()
	player.hasChunkCenter = true
	player.centerChunk = LoadedChunk{}
	player.loadedChunks = make(map[LoadedChunk]struct{})
	player.chunkStreamStarted = true
	player.chunkStreamNotify = make(chan struct{}, 1)
	player.chunkMx.Unlock()

	runtime.setSessionActiveChunks(player, []LoadedChunk{{}, {X: 1}})
	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{X: 16.1, Y: 20})

	if !runtime.MountPassenger(boat, player) {
		t.Fatal("player mount failed")
	}

	finished := make(chan struct{})

	go func() {
		runtime.Tick()
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("runtime tick deadlocked while mounted player crossed chunks")
	}

	player.chunkMx.Lock()
	center := player.centerChunk
	player.chunkMx.Unlock()

	if center != (LoadedChunk{X: 1}) {
		t.Fatalf("mounted player chunk center = %+v, want {X:1}", center)
	}
}

func TestBoatInactiveChunkPauses(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 20})

	initial := boat.State.Position

	runtime.Tick()

	if boat.State.Position != initial {
		t.Fatal("boat moved in inactive chunk")
	}

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	runtime.Tick()

	if boat.State.Position == initial {
		t.Fatal("boat did not resume in active chunk")
	}

	paused := boat.State.Position

	runtime.setSessionActiveChunks(viewer, nil)

	runtime.Tick()

	if boat.State.Position != paused {
		t.Fatal("boat moved after chunk became inactive")
	}
}

func TestBoatPushesLivingEntitiesButNotPassengers(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{})

	controller, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000029", "controller")

	runtime.AssignEntityID(controller)
	runtime.addSession(controller)

	if !runtime.MountPassenger(boat, controller) {
		t.Fatal("controller mount failed")
	}

	target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 0.5}, 20)

	runtime.pushBoatEntities(boat.State.ID, boat.State.Position)

	want := 0.05 / math.Sqrt2
	assertBoatFloatClose(t, target.Living.Velocity.X, want)

	target.Living.Velocity = game.Velocity{}

	if !runtime.MountPassenger(boat, target) {
		t.Fatal("target mount failed")
	}

	runtime.pushBoatEntities(boat.State.ID, boat.State.Position)

	if target.Living.Velocity != (game.Velocity{}) {
		t.Fatalf("passenger was pushed: %#v", target.Living.Velocity)
	}
}

func TestBoatAutomaticallyBoardsNearbyLivingEntity(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{})

	passenger := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 0.5}, 20)

	runtime.pushBoatEntities(boat.State.ID, boat.State.Position)

	if runtime.passengerVehicleID(passenger.State.ID) != boat.State.ID {
		t.Fatal("eligible nearby living entity did not board boat")
	}

	if passenger.Living.Velocity != (game.Velocity{}) {
		t.Fatalf("new passenger was pushed: %#v", passenger.Living.Velocity)
	}
}

func TestBoatTickDoesNotAllocateAfterWarmup(t *testing.T) {
	for _, passengerCount := range boatBenchmarkPassengerCounts {
		t.Run(string(rune('0'+passengerCount)), func(t *testing.T) {
			runtime := NewRuntime(newBoatBenchmarkWaterWorld())

			boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 0.4})

			for range passengerCount {
				session, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000020", "passenger")

				runtime.AssignEntityID(session)
				runtime.addSession(session)
				runtime.MountPassenger(boat, session)
			}

			boat.Tick(runtime, nil)

			allocations := testing.AllocsPerRun(100, func() {
				boat.State.Position = game.Position{Y: 0.4}
				boat.Velocity = game.Velocity{}

				boat.Tick(runtime, nil)
			})

			if allocations != 0 {
				t.Fatalf("boat tick allocations = %v, want 0", allocations)
			}
		})
	}
}

func TestBoatAttackDropsVariantAndCleansPassengers(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000003", "player")

	runtime.AssignEntityID(session)
	runtime.addSession(session)

	boat := runtime.SpawnBoat(game.EntityCherryBoat, game.Position{})
	if !runtime.MountPassenger(boat, session) {
		t.Fatal("mount failed")
	}

	for range 5 {
		boat.RuntimeEntityAttack(runtime, session)
	}

	if !boat.State.Removed || session.VehicleID() != 0 {
		t.Fatal("destroyed boat did not remove passengers")
	}

	entities := runtime.snapshotRuntimeEntities()

	for _, entity := range entities {
		item, ok := entity.(*runtimeItemEntity)
		if ok && item.Stack.Item == game.ItemCherryBoat {
			return
		}
	}

	t.Fatal("destroyed cherry boat did not drop its variant item")
}

func BenchmarkBoatTick(b *testing.B) {
	for _, passengers := range boatBenchmarkPassengerCounts {
		b.Run("passengers="+string(rune('0'+passengers)), func(b *testing.B) {
			runtime := NewRuntime(newBoatBenchmarkWaterWorld())

			boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 0.4})

			boat.LeftPaddle = true
			boat.RightPaddle = true

			for range passengers {
				session, _ := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000010", "passenger")

				runtime.AssignEntityID(session)
				runtime.addSession(session)
				runtime.MountPassenger(boat, session)
			}

			boat.Tick(runtime, nil)

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				boat.Tick(runtime, nil)
			}
		})
	}
}

func BenchmarkBoatTickHundredNoViewer(b *testing.B) {
	benchmarkBoatTickHundred(b, false)
}

func BenchmarkBoatTickHundredOneViewer(b *testing.B) {
	benchmarkBoatTickHundred(b, true)
}

func benchmarkBoatTickHundred(b *testing.B, viewer bool) {
	runtime := NewRuntime(newBoatBenchmarkWaterWorld())

	if viewer {
		session := benchmarkSession(b, runtime, "boat-viewer", game.Position{X: 50, Y: 0.4})

		chunks := make([]LoadedChunk, 0, 7)

		session.chunkMx.Lock()

		for chunkX := int32(0); chunkX <= 6; chunkX++ {
			chunk := LoadedChunk{X: chunkX}

			session.loadedChunks[chunk] = struct{}{}
			chunks = append(chunks, chunk)
		}

		session.chunkMx.Unlock()

		runtime.setSessionActiveChunks(session, chunks)
	}

	boats := make([]*runtimeBoatEntity, 100)

	for index := range boats {
		boats[index] = runtime.SpawnBoat(game.EntityOakBoat, game.Position{X: float64(index), Y: 0.4})
	}

	for _, boat := range boats {
		boat.Tick(runtime, nil)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, boat := range boats {
			boat.Tick(runtime, nil)
		}
	}
}

func newBoatBenchmarkWaterWorld() *game.World {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	for x := int32(-1); x <= 100; x++ {
		for z := int32(-1); z <= 1; z++ {
			world.SetBlock(game.BlockPosition{X: x, Z: z}, game.Water)
		}
	}

	return world
}

func assertBoatPositionClose(t *testing.T, actual, expected game.Position) {
	t.Helper()

	assertBoatFloatClose(t, actual.X, expected.X)
	assertBoatFloatClose(t, actual.Y, expected.Y)
	assertBoatFloatClose(t, actual.Z, expected.Z)
}

func assertBoatFloatClose(t *testing.T, actual, expected float64) {
	t.Helper()

	if math.Abs(actual-expected) > 1e-8 {
		t.Fatalf("value = %.12f, want %.12f", actual, expected)
	}
}
