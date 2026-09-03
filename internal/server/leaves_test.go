package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestLeafRandomTickEligibility(t *testing.T) {
	terminal := mustBlockState(t, game.OakLeaves,
		game.BlockPropertyValue{Name: "distance", Value: "7"},
		game.BlockPropertyValue{Name: "persistent", Value: "false"},
	)

	persistent := mustBlockState(t, terminal, game.BlockPropertyValue{Name: "persistent", Value: "true"})
	supported := mustBlockState(t, terminal, game.BlockPropertyValue{Name: "distance", Value: "6"})

	if !terminal.RandomlyTicks() {
		t.Fatal("terminal nonpersistent leaves do not randomly tick")
	}

	if persistent.RandomlyTicks() {
		t.Fatal("persistent leaves randomly tick")
	}

	if supported.RandomlyTicks() {
		t.Fatal("supported leaves randomly tick")
	}
}

func TestPersistentLeavesNeverDecay(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	leaves := mustBlockState(t, game.OakLeaves,
		game.BlockPropertyValue{Name: "distance", Value: "7"},
		game.BlockPropertyValue{Name: "persistent", Value: "true"},
	)

	runtime := NewRuntime(game.NewOverworld(nil))

	runtime.World.SetBlock(position, leaves)

	runtime.randomTickLeafLocked(position, leaves)

	if runtime.World.BlockAt(position) != leaves {
		t.Fatal("persistent leaves decayed")
	}
}

func TestLeafDistancePropagatesAfterSupportChanges(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{}

	positions := []game.BlockPosition{{X: 1, Y: 70}, {X: 2, Y: 70}, {X: 3, Y: 70}}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	runtime.World.SetBlock(game.BlockPosition{Y: 70}, game.OakLog)

	for index, position := range positions {
		leaves := leafTestState(t, index+1)

		runtime.World.SetBlock(position, leaves)
	}

	mutateLeafTestBlock(t, runtime, game.BlockPosition{Y: 70}, game.Air)

	for range 8 {
		runtime.tickScheduledBlocksLocked()
	}

	for _, position := range positions {
		distance := blockPropertyInt(runtime.World.BlockAt(position), "distance")
		if distance != 7 {
			t.Fatalf("distance after support removal at %+v = %d, want 7", position, distance)
		}
	}

	mutateLeafTestBlock(t, runtime, game.BlockPosition{Y: 70}, game.OakLog)

	for range 8 {
		runtime.tickScheduledBlocksLocked()
	}

	for index, position := range positions {
		distance := blockPropertyInt(runtime.World.BlockAt(position), "distance")
		if distance != index+1 {
			t.Fatalf("restored distance at %+v = %d, want %d", position, distance, index+1)
		}
	}
}

func TestLeafDistancePropagationCrossesActiveChunkBoundary(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{}

	logPosition := game.BlockPosition{X: 14, Y: 70}
	firstLeaf := game.BlockPosition{X: 15, Y: 70}
	secondLeaf := game.BlockPosition{X: 16, Y: 70}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}, {X: 1}})

	runtime.World.SetBlock(logPosition, game.OakLog)
	runtime.World.SetBlock(firstLeaf, leafTestState(t, 1))
	runtime.World.SetBlock(secondLeaf, leafTestState(t, 2))

	mutateLeafTestBlock(t, runtime, logPosition, game.Air)

	for range 8 {
		runtime.tickScheduledBlocksLocked()
	}

	distance := blockPropertyInt(runtime.World.BlockAt(secondLeaf), "distance")
	if distance != 7 {
		t.Fatalf("cross-chunk distance = %d, want 7", distance)
	}
}

func TestLeafScheduledWorkPausesWhileChunkInactive(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{}

	logPosition := game.BlockPosition{X: 15, Y: 70}
	leafPosition := game.BlockPosition{X: 16, Y: 70}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	runtime.World.SetBlock(logPosition, game.OakLog)
	runtime.World.SetBlock(leafPosition, leafTestState(t, 1))

	mutateLeafTestBlock(t, runtime, logPosition, game.Air)

	runtime.tickScheduledBlocksLocked()

	distance := blockPropertyInt(runtime.World.BlockAt(leafPosition), "distance")
	if distance != 1 {
		t.Fatalf("inactive distance = %d, want 1", distance)
	}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}, {X: 1}})

	runtime.tickScheduledBlocksLocked()

	distance = blockPropertyInt(runtime.World.BlockAt(leafPosition), "distance")
	if distance != 7 {
		t.Fatalf("reactivated distance = %d, want 7", distance)
	}
}

func TestLeafDecayUsesCanonicalLoot(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	leaves := leafTestState(t, 7)

	runtime := NewRuntime(game.NewOverworld(nil))

	runtime.World.SetBlock(position, leaves)

	runtime.lootRandomFloat = func() float32 {
		return 0
	}

	runtime.lootRandomInt = func(int) int {
		return 0
	}

	runtime.randomTickLeafLocked(position, leaves)

	mutations := runtime.takeRuntimeBlockMutationsLocked()

	runtime.completeRuntimeBlockMutations(mutations)

	if runtime.World.BlockAt(position) != game.Air {
		t.Fatal("terminal leaves were not removed")
	}

	drops := make(map[game.Item]bool)

	for _, entity := range runtime.snapshotRuntimeEntities() {
		drop := entity.(*runtimeItemEntity)
		drops[drop.Stack.Item] = true
	}

	expectedDrops := []game.Item{game.ItemOakSapling, game.ItemStick, game.ItemApple}

	for _, item := range expectedDrops {
		if !drops[item] {
			t.Fatalf("canonical leaf loot missing item %d", item)
		}
	}
}

func TestGeneratedBaselineLeavesParticipateWithoutOverride(t *testing.T) {
	leaves := leafTestState(t, 7)

	generator := randomTickBoundedGenerator{
		base:    randomTickTestGenerator{block: leaves},
		minY:    64,
		maxY:    79,
		present: true,
	}

	world := game.NewOverworld(generator)

	runtime := NewRuntime(world)

	position := game.BlockPosition{Y: 70}

	overrides := world.SnapshotChunkOverrides(game.ChunkPosition{})
	if len(overrides) != 0 {
		t.Fatalf("initial overrides = %d, want 0", len(overrides))
	}

	chunk := runtime.newActiveChunk(LoadedChunk{})

	assertRandomTickSections(t, chunk.snapshotRandomTickSections(), []int32{64})

	runtime.randomTickLeafLocked(position, world.BlockAt(position))

	if world.BlockAt(position) != game.Air {
		t.Fatal("generated baseline leaves did not decay")
	}
}

func leafTestState(t *testing.T, distance int) game.Block {
	t.Helper()

	return mustBlockState(t, game.OakLeaves,
		game.BlockPropertyValue{Name: "distance", Value: decimalBlockPropertyValue(distance)},
		game.BlockPropertyValue{Name: "persistent", Value: "false"},
	)
}

func mutateLeafTestBlock(t *testing.T, runtime *Runtime, position game.BlockPosition, replacement game.Block) {
	t.Helper()

	change := game.BlockChange{Position: position, Replacement: replacement}

	result, _, err := runtime.mutateBlocksLocked(nil, BlockMutationPlace, []game.BlockChange{change}, 1, true, false, true, false)

	if err != nil || !result.Changed {
		t.Fatalf("mutate leaf support: %+v, %v", result, err)
	}
}
