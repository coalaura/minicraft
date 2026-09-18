package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type sourceWaterClipTestCase struct {
	name      string
	blocks    []game.BlockChange
	want      game.BlockPosition
	wantWater bool
}

type naturalAnvilFallTestCase struct {
	name      string
	startY    int32
	wantTicks int32
}

func TestFallingBlockSchedulesAfterTwoActiveTicksAndCarriesProtocolState(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(position)})

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.RedSand}})
	if err != nil || !result.Changed {
		t.Fatalf("place red sand = %+v, %v", result, err)
	}

	runtime.Tick()

	if runtime.World.BlockAt(position) != game.RedSand || findRuntimeFallingBlock(runtime) != nil {
		t.Fatal("red sand fell before its two-tick deadline")
	}

	runtime.Tick()

	entity := findRuntimeFallingBlock(runtime)
	if entity == nil {
		t.Fatal("red sand did not become a falling entity on tick two")
	}

	if runtime.World.BlockAt(position) != game.Air || entity.Block != game.RedSand {
		t.Fatalf("fall start left block %d and carried %d", runtime.World.BlockAt(position), entity.Block)
	}

	packet := entity.AddEntityPacket(runtimeEntitySpawnSnapshot{ID: entity.State.ID, UUID: entity.State.UUID, Position: entity.State.Position})
	if packet.Type != int32(game.EntityFallingBlock) || packet.Data != int32(game.RedSand) {
		t.Fatalf("falling block spawn packet = %+v", packet)
	}

	metadata := entity.EntityMetadata()

	wantMetadata := protocol.EntityMetadataEntry{
		Index: protocol.FallingBlockStartMetadataIndex,
		Type:  protocol.MetadataTypeBlockPosition,
		Value: protocol.MetadataBlockPosition(position),
	}

	if len(metadata) != 1 || metadata[0] != wantMetadata {
		t.Fatalf("falling block metadata = %+v, want %+v", metadata, wantMetadata)
	}

	runtime.Tick()

	assertPositionClose(t, entity.State.Position, game.Position{X: .5, Y: 69.96, Z: .5}, 1e-12)
	assertVelocityClose(t, entity.Velocity, game.Velocity{Y: -.0392}, 1e-12)
}

func TestFallingBlockPacketsFollowAuthoritativeBlockChanges(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	viewer, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Viewer", game.GameModeSpectator)

	viewer.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, viewer)

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	position := game.BlockPosition{Y: 3}

	_, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.Sand}})
	if err != nil {
		t.Fatalf("place sand: %v", err)
	}

	connection.reset()

	runtime.Tick()
	runtime.Tick()

	assertPacketIDs(t, connection.packetIDs(t), []int32{
		protocol.ClientboundBlockUpdateID,
		protocol.ClientboundAddEntityID,
		protocol.ClientboundEntityMetadataID,
	})

	entity := findRuntimeFallingBlock(runtime)
	if entity == nil {
		t.Fatal("scheduled sand did not spawn")
	}

	connection.reset()

	for !entity.State.Removed {
		runtime.Tick()
	}

	packetIDs := connection.packetIDs(t)
	assertPacketSuffix(t, packetIDs, []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundRemoveEntitiesID})
}

func TestFallingBlockScheduledTickPausesWithInactiveChunk(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(position)})

	_, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.Sand}})
	if err != nil {
		t.Fatalf("place sand: %v", err)
	}

	runtime.setSessionActiveChunks(viewer, nil)

	for range 20 {
		runtime.Tick()
	}

	if runtime.World.BlockAt(position) != game.Sand || findRuntimeFallingBlock(runtime) != nil {
		t.Fatal("inactive scheduled falling check advanced")
	}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(position)})

	runtime.Tick()

	if runtime.World.BlockAt(position) != game.Sand {
		t.Fatal("sand fell after only one resumed active tick")
	}

	runtime.Tick()

	if runtime.World.BlockAt(position) != game.Air || findRuntimeFallingBlock(runtime) == nil {
		t.Fatal("sand did not fall after its second resumed active tick")
	}
}

func TestFallingBlockCrossChunkMovementReindexesAndPauses(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	entity := runtime.SpawnFallingBlock(game.Position{X: 15.9, Y: 100, Z: .5}, game.Sand, game.BlockPosition{X: 15, Y: 100})

	entity.Velocity.X = .3

	runtime.Tick()

	if entity.State.Chunk != (LoadedChunk{X: 1}) {
		t.Fatalf("falling block chunk = %+v, want x=1", entity.State.Chunk)
	}

	pausedPosition := entity.State.Position
	pausedTime := entity.Time

	for range 10 {
		runtime.Tick()
	}

	if entity.State.Position != pausedPosition || entity.Time != pausedTime {
		t.Fatalf("inactive falling block advanced to %+v at time %d", entity.State.Position, entity.Time)
	}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{X: 1}})

	runtime.Tick()

	if entity.State.Position == pausedPosition || entity.Time != pausedTime+1 {
		t.Fatal("reactivated falling block did not resume")
	}
}

func TestFallingBlockLandingPreservesStateAndDoesNotDuplicate(t *testing.T) {
	support := game.BlockPosition{Y: 68}
	landing := game.BlockPosition{Y: 69}
	start := game.BlockPosition{Y: 72}

	world := &game.World{}

	world.SetBlock(support, game.Stone)

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	facingAnvil := mustBlockState(t, game.Anvil, game.BlockPropertyValue{Name: "facing", Value: "east"})

	entity := runtime.SpawnFallingBlock(game.Position{X: .5, Y: float64(start.Y), Z: .5}, facingAnvil, start)

	tickUntilFallingBlockRemoved(t, runtime, entity, 100)

	placed := runtime.World.BlockAt(landing)

	facing, valid := placed.Property("facing")
	if !valid || facing != "east" {
		t.Fatalf("landed anvil state %d facing %q", placed, facing)
	}

	fallingCount, itemCount := countFallingAndItemEntities(runtime)
	if fallingCount != 0 || itemCount != 0 {
		t.Fatalf("successful landing left %d falling and %d item entities", fallingCount, itemCount)
	}
}

func TestFallingBlockBlockedPlacementDropsExactlyOneItem(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.StoneSlab)

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	entity := runtime.SpawnFallingBlock(game.Position{X: .5, Y: 1, Z: .5}, game.Gravel, game.BlockPosition{Y: 1})

	tickUntilFallingBlockRemoved(t, runtime, entity, 20)

	fallingCount, itemCount := countFallingAndItemEntities(runtime)
	if fallingCount != 0 || itemCount != 1 || runtime.World.BlockAt(game.BlockPosition{}) != game.StoneSlab {
		t.Fatalf("blocked landing left block %d, %d falling and %d item entities", runtime.World.BlockAt(game.BlockPosition{}), fallingCount, itemCount)
	}
}

func TestFallingSandColumnCascadesWithoutDuplication(t *testing.T) {
	world := &game.World{}

	support := game.BlockPosition{Y: 69}

	world.SetBlock(game.BlockPosition{}, game.Stone)
	world.SetBlock(support, game.Stone)

	runtime := NewRuntime(world)
	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	changes := []game.BlockChange{
		{Position: game.BlockPosition{Y: 70}, Replacement: game.Sand},
		{Position: game.BlockPosition{Y: 71}, Replacement: game.Gravel},
		{Position: game.BlockPosition{Y: 72}, Replacement: game.RedSand},
	}

	result, err := runtime.MutateWorldBlocks(changes)
	if err != nil || !result.Changed {
		t.Fatalf("place falling column = %+v, %v", result, err)
	}

	_, err = runtime.MutateWorldBlocks([]game.BlockChange{{Position: support, Replacement: game.Air}})
	if err != nil {
		t.Fatalf("remove column support: %v", err)
	}

	for range 120 {
		runtime.Tick()
	}

	fallingCount, itemCount := countFallingAndItemEntities(runtime)
	if fallingCount != 0 || itemCount != 0 {
		t.Fatalf("settled column left %d falling and %d item entities", fallingCount, itemCount)
	}

	want := []game.Block{game.Sand, game.Gravel, game.RedSand}

	for index, expected := range want {
		position := game.BlockPosition{Y: int32(index + 1)}
		if runtime.World.BlockAt(position) != expected {
			t.Fatalf("settled column y=%d = %d, want %d", position.Y, runtime.World.BlockAt(position), expected)
		}
	}
}

func TestFallingBlockLargeCollapseSettlesWithoutDuplication(t *testing.T) {
	world := &game.World{}

	changes := make([]game.BlockChange, 0, 6*6*8)
	supportChanges := make([]game.BlockChange, 0, 6*6)

	for blockX := range 6 {
		for blockZ := range 6 {
			world.SetBlock(game.BlockPosition{X: int32(blockX), Z: int32(blockZ)}, game.Stone)

			support := game.BlockPosition{X: int32(blockX), Y: 16, Z: int32(blockZ)}

			world.SetBlock(support, game.Stone)

			supportChanges = append(supportChanges, game.BlockChange{Position: support, Replacement: game.Air})

			for layer := range 8 {
				blockY := int32(layer + 17)
				block := game.Sand

				if blockY%2 == 0 {
					block = game.Gravel
				}

				position := game.BlockPosition{X: int32(blockX), Y: blockY, Z: int32(blockZ)}

				changes = append(changes, game.BlockChange{Position: position, Replacement: block})
			}
		}
	}

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	result, err := runtime.MutateWorldBlocks(changes)
	if err != nil || !result.Changed {
		t.Fatalf("place collapse blocks = %+v, %v", result, err)
	}

	result, err = runtime.MutateWorldBlocks(supportChanges)
	if err != nil || !result.Changed {
		t.Fatalf("remove collapse supports = %+v, %v", result, err)
	}

	for range 240 {
		runtime.Tick()
	}

	fallingCount, itemCount := countFallingAndItemEntities(runtime)
	if fallingCount != 0 || itemCount != 0 {
		t.Fatalf("large collapse left %d falling and %d item entities", fallingCount, itemCount)
	}

	for blockX := range 6 {
		for blockZ := range 6 {
			for layer := range 8 {
				blockY := int32(layer + 1)
				expected := game.Sand

				if blockY%2 == 0 {
					expected = game.Gravel
				}

				position := game.BlockPosition{X: int32(blockX), Y: blockY, Z: int32(blockZ)}
				if runtime.World.BlockAt(position) != expected {
					t.Fatalf("settled collapse block at %+v = %d, want %d", position, runtime.World.BlockAt(position), expected)
				}
			}
		}
	}
}

func TestConcretePowderHardensOnPlacementAndHighSpeedSourceWaterIntersection(t *testing.T) {
	waterPosition := game.BlockPosition{Y: 3}

	world := &game.World{}

	world.SetBlock(waterPosition, game.Water)

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	adjacent := game.BlockPosition{X: 1, Y: 3}

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: adjacent, Replacement: game.WhiteConcretePowder}})

	if err != nil || !result.Changed || runtime.World.BlockAt(adjacent) != game.WhiteConcrete {
		t.Fatalf("place powder beside water = %+v, %v, block %d", result, err, runtime.World.BlockAt(adjacent))
	}

	entity := runtime.SpawnFallingBlock(game.Position{X: .5, Y: 5, Z: .5}, game.RedConcretePowder, game.BlockPosition{Y: 5})

	entity.Velocity.Y = -2

	runtime.Tick()

	if !entity.State.Removed || runtime.World.BlockAt(waterPosition) != game.RedConcrete {
		t.Fatalf("fast powder removed %t, water position block %d", entity.State.Removed, runtime.World.BlockAt(waterPosition))
	}
}

func TestConcretePowderSourceWaterClipUsesColliderShapes(t *testing.T) {
	tests := []sourceWaterClipTestCase{
		{
			name:   "source water",
			blocks: []game.BlockChange{{Position: game.BlockPosition{Y: 2}, Replacement: game.Water}},
			want:   game.BlockPosition{Y: 2}, wantWater: true,
		},
		{
			name:   "flowing water",
			blocks: []game.BlockChange{{Position: game.BlockPosition{Y: 2}, Replacement: mustBlockState(t, game.Water, game.BlockPropertyValue{Name: "level", Value: "1"})}},
			want:   game.BlockPosition{}, wantWater: false,
		},
		{
			name: "source water behind partial collider",
			blocks: []game.BlockChange{
				{Position: game.BlockPosition{Y: 3}, Replacement: game.StoneSlab},
				{Position: game.BlockPosition{Y: 2}, Replacement: game.Water},
			},
			want: game.BlockPosition{}, wantWater: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{}

			world.SetBlocks(test.blocks)

			runtime := NewRuntime(world)

			position, found := runtime.clipSourceWaterSegment(game.Position{X: .5, Y: 5, Z: .5}, game.Position{X: .5, Y: 1, Z: .5})
			if found != test.wantWater || found && position != test.want {
				t.Fatalf("source water clip = %+v, %t, want %+v, %t", position, found, test.want, test.wantWater)
			}
		})
	}
}

func TestFallingBlockHorizontalCollisionClearsVelocityBeforeDrag(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{X: 1, Y: 5}, game.Stone)

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	entity := runtime.SpawnFallingBlock(game.Position{X: .5, Y: 5, Z: .5}, game.Sand, game.BlockPosition{Y: 5})

	entity.Velocity.X = 1

	runtime.Tick()

	if entity.Velocity.X != 0 {
		t.Fatalf("horizontal collision velocity = %v, want 0", entity.Velocity.X)
	}
}

func TestFallingBlockPlacementUsesDirectionalReplacementContext(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	oneLayerSnow := mustBlockState(t, game.Snow, game.BlockPropertyValue{Name: "layers", Value: "1"})

	world.SetBlock(game.BlockPosition{Y: 1}, oneLayerSnow)

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	entity := runtime.SpawnFallingBlock(game.Position{X: .5, Y: 2, Z: .5}, game.Sand, game.BlockPosition{Y: 2})
	tickUntilFallingBlockRemoved(t, runtime, entity, 20)

	if runtime.World.BlockAt(game.BlockPosition{Y: 1}) != game.Sand {
		t.Fatalf("falling sand did not replace one-layer snow: %d", runtime.World.BlockAt(game.BlockPosition{Y: 1}))
	}
}

func TestNaturalAnvilFallsDeliverLandEventAfterRemoval(t *testing.T) {
	tests := []naturalAnvilFallTestCase{
		{name: "one block", startY: 2, wantTicks: 7},
		{name: "four blocks", startY: 5, wantTicks: 15},
		{name: "fifteen blocks", startY: 16, wantTicks: 30},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{}

			world.SetBlock(game.BlockPosition{}, game.Stone)

			runtime := NewRuntime(world)

			runtime.entityRandom = fallingBlockNoDamageRandom

			viewer, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Viewer", game.GameModeSpectator)

			viewer.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

			joinTestSession(t, runtime, viewer)

			runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

			entity := runtime.SpawnFallingBlock(game.Position{X: .5, Y: float64(test.startY), Z: .5}, game.Anvil, game.BlockPosition{Y: test.startY})

			connection.reset()

			tickUntilFallingBlockRemoved(t, runtime, entity, 200)

			if runtime.World.BlockAt(game.BlockPosition{Y: 1}) != game.Anvil {
				t.Fatalf("anvil from y=%d landed as %d", test.startY, runtime.World.BlockAt(game.BlockPosition{Y: 1}))
			}

			if entity.Time != test.wantTicks {
				t.Fatalf("anvil from y=%d landed on tick %d, want %d", test.startY, entity.Time, test.wantTicks)
			}

			assertPacketSuffix(t, connection.packetIDs(t), []int32{
				protocol.ClientboundBlockUpdateID,
				protocol.ClientboundRemoveEntitiesID,
				protocol.ClientboundLevelEventID,
			})
		})
	}
}

func TestBrokenAnvilDeliversEventBeforeItemSpawn(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.StoneSlab)

	runtime := NewRuntime(world)

	viewer, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Viewer", game.GameModeSpectator)
	viewer.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, viewer)

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	entity := runtime.SpawnFallingBlock(game.Position{X: .5, Y: 1, Z: .5}, game.Anvil, game.BlockPosition{Y: 1})

	connection.reset()

	tickUntilFallingBlockRemoved(t, runtime, entity, 20)

	assertPacketSuffix(t, connection.packetIDs(t), []int32{
		protocol.ClientboundRemoveEntitiesID,
		protocol.ClientboundLevelEventID,
		protocol.ClientboundAddEntityID,
		protocol.ClientboundEntityMetadataID,
	})
}

func TestAnvilLandingDamagesLivingAndDegradesWithPreservedFacing(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 0
	}

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	victim := spawnTestRuntimeLivingEntity(runtime, game.Position{X: .5, Y: 1, Z: .5}, 20)

	facingAnvil := mustBlockState(t, game.Anvil, game.BlockPropertyValue{Name: "facing", Value: "west"})

	entity := runtime.SpawnFallingBlock(game.Position{X: .5, Y: 1, Z: .5}, facingAnvil, game.BlockPosition{Y: 4})

	runtime.landFallingBlock(entity, entity.State.ID, facingAnvil, game.BlockPosition{Y: 1}, entity.State.Position, 3, facingAnvil.FallingDefinition(), false, true, false)

	if victim.Living.Health != 16 {
		t.Fatalf("anvil victim health = %v, want 16", victim.Living.Health)
	}

	placed := runtime.World.BlockAt(game.BlockPosition{Y: 1})

	definition, valid := placed.Definition()
	if !valid || definition.ID != game.ChippedAnvilID {
		t.Fatalf("degraded anvil definition = %d, want %d", definition.ID, game.ChippedAnvilID)
	}

	facing, valid := placed.Property("facing")
	if !valid || facing != "west" {
		t.Fatalf("degraded anvil facing = %q", facing)
	}
}

func TestFallingBlockTickDoesNotAllocateAfterWarmup(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	entity := runtime.SpawnFallingBlock(game.Position{X: .5, Y: 1_000_000, Z: .5}, game.Sand, game.BlockPosition{Y: 1_000_000})

	entity.Time = -1_000

	runtime.tickActiveChunks()

	allocations := testing.AllocsPerRun(100, runtime.tickActiveChunks)
	if allocations != 0 {
		t.Fatalf("warmed falling block tick allocations = %f, want 0", allocations)
	}
}

func tickUntilFallingBlockRemoved(t *testing.T, runtime *Runtime, entity *runtimeFallingBlockEntity, maximumTicks int) {
	t.Helper()

	for range maximumTicks {
		if entity.State.Removed {
			return
		}

		runtime.Tick()
	}

	t.Fatalf("falling block remained after %d ticks at %+v", maximumTicks, entity.State.Position)
}

func assertPacketSuffix(t *testing.T, actual, suffix []int32) {
	t.Helper()

	if len(actual) < len(suffix) {
		t.Fatalf("packet ids = %v, want suffix %v", actual, suffix)
	}

	start := len(actual) - len(suffix)

	for index, packetID := range suffix {
		if actual[start+index] != packetID {
			t.Fatalf("packet ids = %v, want suffix %v", actual, suffix)
		}
	}
}

func fallingBlockNoDamageRandom() float32 {
	return 1
}

func findRuntimeFallingBlock(runtime *Runtime) *runtimeFallingBlockEntity {
	for _, candidate := range runtime.snapshotRuntimeEntities() {
		entity, valid := candidate.(*runtimeFallingBlockEntity)
		if valid {
			return entity
		}
	}

	return nil
}

func countFallingAndItemEntities(runtime *Runtime) (int, int) {
	var (
		fallingCount int
		itemCount    int
	)

	for _, candidate := range runtime.snapshotRuntimeEntities() {
		switch candidate.(type) {
		case *runtimeFallingBlockEntity:
			fallingCount++
		case *runtimeItemEntity:
			itemCount++
		}
	}

	return fallingCount, itemCount
}
