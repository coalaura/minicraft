package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type fluidMixingTestCase struct {
	name    string
	changes []game.BlockChange
	want    game.Block
	result  game.BlockPosition
}

func tickFluid(t *testing.T, runtime *Runtime, position game.BlockPosition) {
	t.Helper()

	runtime.worldMutationMu.Lock()
	runtime.tickFluidLocked(position)
	runtime.worldMutationMu.Unlock()
}

func fluidWithLevel(t *testing.T, fluid FlowingFluid, level int) game.Block {
	t.Helper()

	return fluidBlock(fluid, level)
}

func TestWaterFlowsDownBeforeSides(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{Y: 70}

	runtime.World.SetBlocks([]game.BlockChange{
		{Position: position, Replacement: game.Water},
		{Position: game.BlockPosition{X: -1, Y: 69}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: 1, Y: 69}, Replacement: game.Stone},
		{Position: game.BlockPosition{Y: 68}, Replacement: game.Stone},
	})

	tickFluid(t, runtime, position)

	below := runtime.World.BlockAt(game.BlockPosition{Y: 69}).FluidState()
	if below.Type() != game.FluidTypeWater || !below.IsFalling() || below.Amount() != 8 {
		t.Fatalf("block below = %+v, want falling water", below)
	}

	if runtime.World.BlockAt(game.BlockPosition{X: -1, Y: 70}) != game.Air {
		t.Fatal("water spread sideways despite an open downward path")
	}
}

func TestWaterSourceSpreadsAfterDownWithThreeSources(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{Y: 70}

	runtime.World.SetBlocks([]game.BlockChange{
		{Position: position, Replacement: fluidWithLevel(t, waterFluid, 1)},
		{Position: game.BlockPosition{X: -1, Y: 70}, Replacement: game.Water},
		{Position: game.BlockPosition{X: 1, Y: 70}, Replacement: game.Water},
		{Position: game.BlockPosition{Y: 70, Z: -1}, Replacement: game.Water},
		{Position: game.BlockPosition{Y: 69, Z: 1}, Replacement: game.Stone},
	})

	tickFluid(t, runtime, position)

	if runtime.World.BlockAt(game.BlockPosition{Y: 69}).FluidState().Type() != game.FluidTypeWater {
		t.Fatal("three-source water did not flow downward")
	}

	if runtime.World.BlockAt(game.BlockPosition{Y: 70, Z: 1}).FluidState().Type() != game.FluidTypeWater {
		t.Fatal("three-source water did not spread sideways after flowing down")
	}
}

func TestWaterWaterlogsWithoutReplacementLoot(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{Y: 70}

	stairs, valid := game.OakStairs.WithProperties(game.BlockPropertyValue{Name: "waterlogged", Value: "false"})

	if !valid {
		t.Fatal("resolve dry stairs")
	}

	runtime.World.SetBlocks([]game.BlockChange{
		{Position: position, Replacement: game.Water},
		{Position: game.BlockPosition{Y: 69}, Replacement: stairs},
	})

	tickFluid(t, runtime, position)

	waterlogged := runtime.World.BlockAt(game.BlockPosition{Y: 69})
	if waterlogged.FluidState().Type() != game.FluidTypeWater || !waterlogged.Waterloggable() {
		t.Fatalf("waterlogged block = %v, want waterlogged stairs", waterlogged)
	}

	if len(runtime.runtimeBlockMutations) != 1 {
		t.Fatalf("queued mutations = %d, want 1", len(runtime.runtimeBlockMutations))
	}

	record := runtime.runtimeBlockMutations[0].delivery.records[0]

	if record.lootContext != blockLootNone {
		t.Fatalf("waterlogging loot = %v, want no loot handling", record.lootContext)
	}
}

func TestLavaMixingOutcomes(t *testing.T) {
	tests := []fluidMixingTestCase{
		{
			name: "source lava horizontally makes obsidian",
			changes: []game.BlockChange{
				{Position: game.BlockPosition{Y: 70}, Replacement: game.Lava},
				{Position: game.BlockPosition{Y: 69}, Replacement: game.Stone},
				{Position: game.BlockPosition{X: 1, Y: 70}, Replacement: game.Water},
			},
			want: game.Obsidian, result: game.BlockPosition{Y: 70},
		},
		{
			name: "flowing lava horizontally makes cobblestone",
			changes: []game.BlockChange{
				{Position: game.BlockPosition{Y: 70}, Replacement: fluidWithLevel(t, lavaFluid, 1)},
				{Position: game.BlockPosition{X: -1, Y: 70}, Replacement: game.Lava},
				{Position: game.BlockPosition{Y: 69}, Replacement: game.Stone},
				{Position: game.BlockPosition{X: 1, Y: 70}, Replacement: game.Water},
			},
			want: game.Cobblestone, result: game.BlockPosition{Y: 70},
		},
		{
			name: "downward lava into water makes stone",
			changes: []game.BlockChange{
				{Position: game.BlockPosition{Y: 70}, Replacement: game.Lava},
				{Position: game.BlockPosition{Y: 69}, Replacement: game.Water},
			},
			want: game.Stone, result: game.BlockPosition{Y: 69},
		},
		{
			name: "soul soil and blue ice make basalt",
			changes: []game.BlockChange{
				{Position: game.BlockPosition{Y: 70}, Replacement: game.Lava},
				{Position: game.BlockPosition{Y: 68}, Replacement: game.SoulSoil},
				{Position: game.BlockPosition{X: 1, Y: 69}, Replacement: game.BlueIce},
			},
			want: game.Basalt, result: game.BlockPosition{Y: 69},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(game.NewOverworld(nil))

			runtime.World.SetBlocks(test.changes)

			tickFluid(t, runtime, game.BlockPosition{Y: 70})

			result := runtime.World.BlockAt(test.result)

			if result != test.want {
				t.Fatalf("mixed block = %v, want %v", result, test.want)
			}
		})
	}
}

func TestLavaFlowMixesOnInitialContact(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	source := game.BlockPosition{X: 8, Y: 70, Z: 8}
	contact := game.BlockPosition{X: 9, Y: 70, Z: 8}
	water := game.BlockPosition{X: 10, Y: 70, Z: 8}

	changes := []game.BlockChange{
		{Position: source, Replacement: game.Lava},
		{Position: water, Replacement: game.Water},
	}

	for xOffset := int32(-2); xOffset <= 2; xOffset++ {
		for zOffset := int32(-2); zOffset <= 2; zOffset++ {
			support := game.BlockPosition{X: source.X + xOffset, Y: 69, Z: source.Z + zOffset}
			changes = append(changes, game.BlockChange{Position: support, Replacement: game.Stone})
		}
	}

	runtime.World.SetBlocks(changes)

	tickFluid(t, runtime, source)

	if runtime.World.BlockAt(contact) != game.Cobblestone {
		t.Fatalf("initial lava contact = %v, want cobblestone", runtime.World.BlockAt(contact))
	}

	if len(runtime.runtimeBlockMutations) != 1 {
		t.Fatalf("mixing mutations = %d, want 1", len(runtime.runtimeBlockMutations))
	}

	events := runtime.runtimeBlockMutations[0].delivery.runtimeEvents
	if len(events) != 1 || events[0].Event != protocol.LevelEventLavaFizz || events[0].Position != contact {
		t.Fatalf("mixing events = %+v, want lava fizz at %v", events, contact)
	}
}

func TestWaterFlowMixesExistingLavaOnInitialContact(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	source := game.BlockPosition{X: 8, Y: 70, Z: 8}
	contact := game.BlockPosition{X: 9, Y: 70, Z: 8}
	lava := game.BlockPosition{X: 10, Y: 70, Z: 8}

	changes := []game.BlockChange{
		{Position: source, Replacement: game.Water},
		{Position: lava, Replacement: game.Lava},
	}

	for xOffset := int32(-2); xOffset <= 2; xOffset++ {
		for zOffset := int32(-2); zOffset <= 2; zOffset++ {
			support := game.BlockPosition{X: source.X + xOffset, Y: 69, Z: source.Z + zOffset}
			changes = append(changes, game.BlockChange{Position: support, Replacement: game.Stone})
		}
	}

	runtime.World.SetBlocks(changes)

	tickFluid(t, runtime, source)

	if runtime.World.FluidAt(contact).Type() != game.FluidTypeWater {
		t.Fatalf("contact block = %v, want flowing water", runtime.World.BlockAt(contact))
	}

	if runtime.World.BlockAt(lava) != game.Obsidian {
		t.Fatalf("lava on initial water contact = %v, want obsidian", runtime.World.BlockAt(lava))
	}

	events := runtime.runtimeBlockMutations[0].delivery.runtimeEvents
	if len(events) != 1 || events[0].Event != protocol.LevelEventLavaFizz || events[0].Position != lava {
		t.Fatalf("mixing events = %+v, want lava fizz at %v", events, lava)
	}
}

func TestRecomputeUsesFallingFluidAbove(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{Y: 70}

	runtime.World.SetBlocks([]game.BlockChange{
		{Position: position, Replacement: fluidWithLevel(t, waterFluid, 4)},
		{Position: game.BlockPosition{Y: 71}, Replacement: fluidWithLevel(t, waterFluid, 8)},
	})

	amount, falling, _ := runtime.recomputeFluidLevel(position, waterFluid, runtime.World.BlockAt(position).FluidState())

	if amount != 8 || !falling {
		t.Fatalf("fluid below falling water = amount %d falling %v, want amount 8 falling", amount, falling)
	}
}

func TestWaterSourceSpreadsSidewaysWithoutNearbyHole(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{X: 8, Y: 70, Z: 8}

	changes := []game.BlockChange{{Position: position, Replacement: game.Water}}

	for xOffset := int32(-5); xOffset <= 5; xOffset++ {
		for zOffset := int32(-5); zOffset <= 5; zOffset++ {
			floor := game.BlockPosition{X: position.X + xOffset, Y: 69, Z: position.Z + zOffset}
			changes = append(changes, game.BlockChange{Position: floor, Replacement: game.Stone})
		}
	}

	runtime.World.SetBlocks(changes)

	tickFluid(t, runtime, position)

	for _, offset := range fluidSides {
		neighbor := game.BlockPosition{X: position.X + offset.X, Y: position.Y, Z: position.Z + offset.Z}

		state := runtime.World.FluidAt(neighbor)

		if state.Type() != game.FluidTypeWater || state.Amount() != 7 || state.IsSource() || state.IsFalling() {
			t.Fatalf("side fluid at %v = amount %d source %v falling %v, want flowing amount 7", neighbor, state.Amount(), state.IsSource(), state.IsFalling())
		}
	}
}

func TestFallingWaterColumnRemainsFalling(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	source := game.BlockPosition{Y: 72}
	firstBelow := game.BlockPosition{Y: 71}
	secondBelow := game.BlockPosition{Y: 70}

	runtime.World.SetBlock(source, game.Water)

	tickFluid(t, runtime, source)
	tickFluid(t, runtime, firstBelow)

	positions := []game.BlockPosition{firstBelow, secondBelow}

	for _, position := range positions {
		state := runtime.World.FluidAt(position)

		if state.Type() != game.FluidTypeWater || state.Amount() != 8 || state.IsSource() || !state.IsFalling() {
			t.Fatalf("column fluid at %v = amount %d source %v falling %v, want falling amount 8", position, state.Amount(), state.IsSource(), state.IsFalling())
		}
	}
}

func TestSideFlowDropsWithoutSpreadingAcrossSecondLevel(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	source := game.BlockPosition{X: 8, Y: 71, Z: 8}
	falling := game.BlockPosition{X: 8, Y: 70, Z: 8}
	side := game.BlockPosition{X: 9, Y: 70, Z: 8}
	sideBelow := game.BlockPosition{X: 9, Y: 69, Z: 8}
	outer := game.BlockPosition{X: 10, Y: 70, Z: 8}
	support := game.BlockPosition{X: 8, Y: 69, Z: 8}

	runtime.World.SetBlocks([]game.BlockChange{
		{Position: source, Replacement: game.Water},
		{Position: support, Replacement: game.Stone},
	})

	tickFluid(t, runtime, source)
	tickFluid(t, runtime, falling)
	tickFluid(t, runtime, side)
	tickFluid(t, runtime, side)

	state := runtime.World.FluidAt(sideBelow)
	if state.Type() != game.FluidTypeWater || !state.IsFalling() {
		t.Fatalf("fluid below side stream = type %d falling %v, want falling water", state.Type(), state.IsFalling())
	}

	state = runtime.World.FluidAt(outer)
	if !state.Empty() {
		t.Fatalf("side stream spread across second level with state %+v", state)
	}
}

func TestFluidFlowUsesCombinedFaceOcclusion(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	if runtime.canFlowBetween(game.Stone, game.Stone, game.BlockFaceEast) {
		t.Fatal("fluid can pass through a fully occluded joint")
	}

	if !runtime.canFlowBetween(game.Water, game.Air, game.BlockFaceEast) {
		t.Fatal("fluid cannot pass through an open joint")
	}
}

func TestFlowFromWaterloggedStairsRemainsAcrossOpenFace(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{X: 8, Y: 70, Z: 8}

	open := game.BlockPosition{X: position.X, Y: position.Y, Z: position.Z + 1}

	stairs, valid := game.OakStairs.WithProperties(game.BlockPropertyValue{Name: "waterlogged", Value: "true"})

	if !valid {
		t.Fatal("resolve waterlogged stairs")
	}

	runtime.World.SetBlocks([]game.BlockChange{
		{Position: position, Replacement: stairs},
		{Position: game.BlockPosition{X: position.X - 1, Y: position.Y, Z: position.Z}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: position.X + 1, Y: position.Y, Z: position.Z}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: position.X, Y: position.Y, Z: position.Z - 1}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: position.X, Y: position.Y - 1, Z: position.Z}, Replacement: game.Stone},
	})

	tickFluid(t, runtime, position)

	initial := runtime.World.FluidAt(open)
	if initial.Type() != game.FluidTypeWater || initial.Amount() != 7 {
		t.Fatalf("initial open-face flow = %+v, want water amount 7", initial)
	}

	tickFluid(t, runtime, open)

	settled := runtime.World.FluidAt(open)
	if settled.Type() != game.FluidTypeWater || settled.Amount() != 7 {
		t.Fatalf("recomputed open-face flow = %+v, want stable water amount 7", settled)
	}
}

func TestFluidMutationSchedulesWaterloggedNeighbors(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	position := game.BlockPosition{Y: 70}

	stairs, valid := game.OakStairs.WithProperties(game.BlockPropertyValue{Name: "waterlogged", Value: "false"})

	if !valid {
		t.Fatal("resolve dry stairs")
	}

	runtime.World.SetBlocks([]game.BlockChange{
		{Position: position, Replacement: game.Water},
		{Position: game.BlockPosition{Y: 69}, Replacement: stairs},
	})

	tickFluid(t, runtime, position)

	waterlogged := game.BlockPosition{Y: 69}

	key := scheduledFluidTickKey{position: waterlogged, typeID: game.FluidStateTypeWater}

	_, scheduled := runtime.scheduledFluidTicks.pending[key]
	if !scheduled {
		t.Fatal("waterlogged fluid was not scheduled after an authoritative mutation")
	}
}

func TestLavaDelayUsesInjectedRandomOnlyForRisingNonfallingState(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	oldState := fluidWithLevel(t, lavaFluid, 3).FluidState()
	newState := fluidWithLevel(t, lavaFluid, 2).FluidState()

	position := game.BlockPosition{}

	runtime.fluidRandom = func(game.BlockPosition, int) int {
		return 1
	}

	delay := runtime.fluidDelay(lavaFluid, position, oldState, newState)

	if delay != lavaFluidDelay*4 {
		t.Fatalf("rising lava delay = %d, want %d", delay, lavaFluidDelay*4)
	}

	runtime.fluidRandom = func(game.BlockPosition, int) int {
		return 0
	}

	delay = runtime.fluidDelay(lavaFluid, position, oldState, newState)

	if delay != lavaFluidDelay {
		t.Fatalf("selected lava delay = %d, want %d", delay, lavaFluidDelay)
	}

	falling := fluidWithLevel(t, lavaFluid, 8).FluidState()

	delay = runtime.fluidDelay(lavaFluid, position, falling, newState)

	if delay != lavaFluidDelay {
		t.Fatalf("falling lava delay = %d, want %d", delay, lavaFluidDelay)
	}
}
