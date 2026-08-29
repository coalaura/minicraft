package server

import (
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type fluidAcceptanceGenerator struct {
	blocks map[game.BlockPosition]game.Block
}

type fluidAcceptanceReplacementTestCase struct {
	name  string
	fluid game.Block
	want  blockLootContext
}

func (g fluidAcceptanceGenerator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	block, present := g.blocks[position]
	if present {
		return block
	}

	return game.Air
}

func tickFluidAcceptanceSchedule(runtime *Runtime) {
	runtime.worldMutationMu.Lock()
	runtime.tickScheduledBlocksLocked()
	runtime.worldMutationMu.Unlock()
}

func TestFluidAcceptanceRuntimeTickDoesNotDeadlock(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{Y: 70}

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	runtime.World.SetBlock(position, game.Water)

	runtime.scheduleBlockTickLocked(position, scheduledBlockTickFluid, 1)

	complete := make(chan struct{})

	go func() {
		runtime.Tick()

		close(complete)
	}()

	select {
	case <-complete:
	case <-time.After(time.Second):
		t.Fatal("runtime tick deadlocked while executing a fluid mutation")
	}
}

func TestFluidAcceptanceBucketMutationSchedulesAndFlowsAfterDelay(t *testing.T) {
	world := &game.World{Generator: fluidAcceptanceGenerator{}}

	runtime := NewRuntime(world)

	position := game.BlockPosition{Y: 70}
	below := game.BlockPosition{Y: 69}

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	actor.Player.Position = blockMutationTestPlayerPosition(position)
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemWaterBucket, Count: 1}

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	runtime.setSessionActiveChunks(actor, []LoadedChunk{blockLoadedChunk(position)})

	world.SetBlock(position, game.WarpedRoots)

	used, err := runtime.useBucketOn(actor, testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 1), actor.Player.Inventory.Hotbar[0])
	if err != nil || !used {
		t.Fatalf("empty water bucket: used %t, err %v", used, err)
	}

	key := scheduledBlockTickKey{position: position, typeID: scheduledBlockTickFluid}
	if _, scheduled := runtime.scheduledBlockTicks.pending[key]; !scheduled {
		t.Fatal("bucket mutation did not schedule its water source")
	}

	for range waterFluidDelay - 1 {
		tickFluidAcceptanceSchedule(runtime)
	}

	if world.BlockAt(below) != game.Air {
		t.Fatalf("water below source before delay = %d, want air", world.BlockAt(below))
	}

	tickFluidAcceptanceSchedule(runtime)

	state := world.BlockAt(below).FluidState()

	if state.Type() != game.FluidTypeWater || !state.IsFalling() {
		t.Fatalf("water below source after delay = %+v, want falling water", state)
	}
}

func TestFluidAcceptanceCrossChunkRetryWaitsForDestinationActivation(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	source := game.BlockPosition{X: 15, Y: 70}
	destination := game.BlockPosition{X: 16, Y: 70}

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(source)})

	runtime.World.SetBlocks([]game.BlockChange{
		{Position: source, Replacement: game.Water},
		{Position: game.BlockPosition{X: 14, Y: 70}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: 15, Y: 69}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: 15, Y: 70, Z: -1}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: 15, Y: 70, Z: 1}, Replacement: game.Stone},
	})

	runtime.worldMutationMu.Lock()
	runtime.scheduleFluidNeighborsLocked([]game.BlockChange{{Position: source, Replacement: game.Water}})
	runtime.worldMutationMu.Unlock()

	for range waterFluidDelay {
		tickFluidAcceptanceSchedule(runtime)
	}

	if runtime.World.BlockAt(destination) != game.Air {
		t.Fatalf("inactive cross-chunk destination = %d, want air", runtime.World.BlockAt(destination))
	}

	if len(runtime.scheduledBlockTicks.pending) != 0 {
		t.Fatalf("inactive border left %d polling ticks, want none", len(runtime.scheduledBlockTicks.pending))
	}

	deferred := runtime.deferredFluidSources[blockLoadedChunk(destination)]
	if _, present := deferred[source]; !present {
		t.Fatal("source was not deferred until destination activation")
	}

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(source), blockLoadedChunk(destination)})

	for range waterFluidDelay {
		tickFluidAcceptanceSchedule(runtime)
	}

	if runtime.World.BlockAt(destination).FluidState().Type() != game.FluidTypeWater {
		t.Fatal("source did not flow after its destination chunk activated")
	}
}

func TestFluidAcceptanceInactiveQueuePausesWithoutBacklog(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{Y: 70}
	below := game.BlockPosition{Y: 69}

	session := &Session{}

	runtime.World.SetBlock(position, game.Water)

	runtime.worldMutationMu.Lock()
	runtime.scheduleFluidNeighborsLocked([]game.BlockChange{{Position: position, Replacement: game.Water}})
	runtime.worldMutationMu.Unlock()

	for range 20 {
		tickFluidAcceptanceSchedule(runtime)
	}

	if runtime.World.BlockAt(below) != game.Air || len(runtime.scheduledBlockTicks.pending) != 1 {
		t.Fatalf("inactive fluid queue = below %d, pending %d; want air and one paused tick", runtime.World.BlockAt(below), len(runtime.scheduledBlockTicks.pending))
	}

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	for range waterFluidDelay - 1 {
		tickFluidAcceptanceSchedule(runtime)
	}

	if runtime.World.BlockAt(below) != game.Air {
		t.Fatalf("fluid flowed before its active-chunk delay elapsed: %d", runtime.World.BlockAt(below))
	}

	tickFluidAcceptanceSchedule(runtime)

	if runtime.World.BlockAt(below).FluidState().Type() != game.FluidTypeWater {
		t.Fatal("fluid did not flow after exactly one resumed delay")
	}
}

func TestFluidAcceptanceEnclosedSourceStopsScheduling(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{Y: 70}

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	runtime.World.SetBlocks([]game.BlockChange{
		{Position: position, Replacement: game.Water},
		{Position: game.BlockPosition{X: -1, Y: 70}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: 1, Y: 70}, Replacement: game.Stone},
		{Position: game.BlockPosition{Y: 69}, Replacement: game.Stone},
		{Position: game.BlockPosition{Y: 71}, Replacement: game.Stone},
		{Position: game.BlockPosition{Y: 70, Z: -1}, Replacement: game.Stone},
		{Position: game.BlockPosition{Y: 70, Z: 1}, Replacement: game.Stone},
	})

	runtime.worldMutationMu.Lock()
	runtime.scheduleFluidNeighborsLocked([]game.BlockChange{{Position: position, Replacement: game.Water}})
	runtime.worldMutationMu.Unlock()

	for range waterFluidDelay {
		tickFluidAcceptanceSchedule(runtime)
	}

	if len(runtime.scheduledBlockTicks.pending) != 0 {
		t.Fatalf("enclosed stable source left %d scheduled ticks", len(runtime.scheduledBlockTicks.pending))
	}
}

func TestFluidAcceptanceFiniteBasinSettlesWithoutFurtherMutations(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	runtime := NewRuntime(world)

	session := &Session{}

	source := game.BlockPosition{X: 5, Y: 70, Z: 5}

	changes := make([]game.BlockChange, 0, 161)

	for blockX := int32(0); blockX <= 10; blockX++ {
		for blockZ := int32(0); blockZ <= 10; blockZ++ {
			floor := game.BlockPosition{X: blockX, Y: 69, Z: blockZ}

			changes = append(changes, game.BlockChange{Position: floor, Replacement: game.Stone})

			if blockX == 0 || blockX == 10 || blockZ == 0 || blockZ == 10 {
				wall := game.BlockPosition{X: blockX, Y: 70, Z: blockZ}

				changes = append(changes, game.BlockChange{Position: wall, Replacement: game.Stone})
			}
		}
	}

	changes = append(changes, game.BlockChange{Position: source, Replacement: game.Water})

	world.SetBlocks(changes)

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(source)})

	runtime.worldMutationMu.Lock()
	runtime.scheduleFluidNeighborsLocked([]game.BlockChange{{Position: source, Replacement: game.Water}})
	runtime.worldMutationMu.Unlock()

	for range 500 {
		tickFluidAcceptanceSchedule(runtime)
	}

	mutations := len(runtime.runtimeBlockMutations)

	for range 100 {
		tickFluidAcceptanceSchedule(runtime)
	}

	if len(runtime.runtimeBlockMutations) != mutations {
		t.Fatalf("settled basin mutations grew from %d to %d", mutations, len(runtime.runtimeBlockMutations))
	}

	if len(runtime.scheduledBlockTicks.pending) != 0 {
		t.Fatalf("settled basin left %d scheduled ticks", len(runtime.scheduledBlockTicks.pending))
	}
}

func TestFluidAcceptanceReplacementLootContexts(t *testing.T) {
	tests := []fluidAcceptanceReplacementTestCase{
		{name: "water replacement has no-breaker loot", fluid: game.Water, want: blockLootNoBreaker},
		{name: "lava replacement has no loot", fluid: game.Lava, want: blockLootNone},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(game.NewOverworld(nil))

			position := game.BlockPosition{Y: 70}

			runtime.World.SetBlocks([]game.BlockChange{{Position: position, Replacement: test.fluid}, {Position: game.BlockPosition{Y: 69}, Replacement: game.WarpedRoots}})

			tickFluid(t, runtime, position)

			if len(runtime.runtimeBlockMutations) != 1 {
				t.Fatalf("queued mutations = %d, want 1", len(runtime.runtimeBlockMutations))
			}

			record := runtime.runtimeBlockMutations[0].delivery.records[0]
			if record.lootContext != test.want {
				t.Fatalf("replacement loot context = %d, want %d", record.lootContext, test.want)
			}
		})
	}
}

func TestFluidAcceptanceProceduralSourceStartsAndCopyOnWriteCollapses(t *testing.T) {
	position := game.BlockPosition{Y: 70}
	east := game.BlockPosition{X: 1, Y: 70}

	generator := fluidAcceptanceGenerator{blocks: map[game.BlockPosition]game.Block{
		position:             game.Water,
		{X: -1, Y: 70}:       game.Stone,
		{Y: 69}:              game.Stone,
		{Y: 70, Z: -1}:       game.Stone,
		{Y: 70, Z: 1}:        game.Stone,
		{X: 1, Y: 70, Z: -1}: game.Stone,
		{X: 1, Y: 69}:        game.Stone,
	}}

	world := &game.World{Generator: generator}

	runtime := NewRuntime(world)

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: game.BlockPosition{X: -1, Y: 70}, Replacement: game.Dirt}})
	if err != nil || !result.Changed {
		t.Fatalf("authoritative neighbor mutation: result=%+v err=%v", result, err)
	}

	key := scheduledBlockTickKey{position: position, typeID: scheduledBlockTickFluid}
	if _, scheduled := runtime.scheduledBlockTicks.pending[key]; !scheduled {
		t.Fatal("generated source was not scheduled after neighboring authoritative mutation")
	}

	for range waterFluidDelay {
		tickFluidAcceptanceSchedule(runtime)
	}

	if world.BlockAt(east).FluidState().Type() != game.FluidTypeWater {
		t.Fatal("generated source did not start flowing")
	}

	world.SetBlock(position, game.Air)

	overrides := world.SnapshotChunkOverrides(game.ChunkPosition{})

	local := game.LocalBlockPosition{Y: position.Y}

	if _, present := overrides[local]; !present {
		t.Fatal("changing generated source did not create an override")
	}

	world.SetBlock(position, game.Water)

	overrides = world.SnapshotChunkOverrides(game.ChunkPosition{})

	if _, present := overrides[local]; present {
		t.Fatal("restoring generated source fluid left a copy-on-write override")
	}
}

func TestFluidAcceptanceRulesAndFastLavaEnvironment(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{}

	oldState := fluidWithLevel(t, lavaFluid, 3).FluidState()
	newState := fluidWithLevel(t, lavaFluid, 2).FluidState()

	runtime.fluidRandom = func(game.BlockPosition, int) int {
		return 0
	}

	delay := runtime.fluidDelay(lavaFluid, position, oldState, newState)

	if delay != lavaFluidDelay {
		t.Fatalf("overworld lava delay = %d, want %d", delay, lavaFluidDelay)
	}

	dropOff := runtime.fluidDropOff(lavaFluid)
	if dropOff != lavaDropOff {
		t.Fatalf("overworld lava dropoff = %d, want %d", dropOff, lavaDropOff)
	}

	slope := runtime.fluidSlope(lavaFluid)
	if slope != lavaSlope {
		t.Fatalf("overworld lava slope = %d, want %d", slope, lavaSlope)
	}

	runtime.FluidEnvironment.FastLava = true

	delay = runtime.fluidDelay(lavaFluid, position, oldState, newState)

	if delay != lavaNetherDelay {
		t.Fatalf("fast lava delay = %d, want %d", delay, lavaNetherDelay)
	}

	dropOff = runtime.fluidDropOff(lavaFluid)
	if dropOff != lavaFastDropOff {
		t.Fatalf("fast lava dropoff = %d, want %d", dropOff, lavaFastDropOff)
	}

	slope = runtime.fluidSlope(lavaFluid)
	if slope != lavaFastSlope {
		t.Fatalf("fast lava slope = %d, want %d", slope, lavaFastSlope)
	}

	if !runtime.fluidSourceConversion(game.FluidTypeWater) || runtime.fluidSourceConversion(game.FluidTypeLava) {
		t.Fatal("default source conversion rules do not enable water only")
	}

	runtime.FluidRules = FluidRules{LavaSourceConversion: true}

	if runtime.fluidSourceConversion(game.FluidTypeWater) || !runtime.fluidSourceConversion(game.FluidTypeLava) {
		t.Fatal("configured source conversion rules were not applied")
	}
}

func TestFluidAcceptanceWaterConvertsSourcesAndLavaDoesNotByDefault(t *testing.T) {
	tests := []fluidAcceptanceReplacementTestCase{
		{name: "water", fluid: game.Water, want: blockLootNoBreaker},
		{name: "lava", fluid: game.Lava, want: blockLootNone},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(game.NewOverworld(nil))

			position := game.BlockPosition{Y: 70}

			flowing := fluidWithLevel(t, FlowingFluid{block: test.fluid, typeID: test.fluid.FluidState().Type(), dropOff: waterDropOff}, 1)

			if test.fluid == game.Lava {
				flowing = fluidWithLevel(t, lavaFluid, 1)
			}

			runtime.World.SetBlocks([]game.BlockChange{
				{Position: position, Replacement: flowing},
				{Position: game.BlockPosition{X: -1, Y: 70}, Replacement: test.fluid},
				{Position: game.BlockPosition{X: 1, Y: 70}, Replacement: test.fluid},
				{Position: game.BlockPosition{Y: 69}, Replacement: game.Stone},
			})

			tickFluid(t, runtime, position)

			isSource := runtime.World.BlockAt(position).FluidState().IsSource()
			if test.fluid == game.Water && !isSource {
				t.Fatal("water did not convert between two sources")
			}

			if test.fluid == game.Lava && isSource {
				t.Fatal("lava converted to a source with default rules")
			}
		})
	}
}
