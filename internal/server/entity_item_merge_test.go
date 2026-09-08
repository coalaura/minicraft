package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestItemMergeUsesGlobalEntityIDOrderAcrossChunks(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	first := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 16.1, Y: 64, Z: 16.1}, game.Velocity{}, 40)

	runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemDirt, Count: 1}, game.Position{X: 15.9, Y: 64, Z: 16.1}, game.Velocity{}, 40)

	source := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 15.9, Y: 64, Z: 15.9}, game.Velocity{}, 40)
	later := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 16.1, Y: 64, Z: 15.9}, game.Velocity{}, 40)

	removed := runtime.mergeItemEntity(source)
	if !removed || !source.State.Removed {
		t.Fatal("source was not consumed by the first ID-ordered candidate")
	}

	if first.Stack.Count != 2 || later.Stack.Count != 1 {
		t.Fatalf("merged counts = first %d, later %d", first.Stack.Count, later.Stack.Count)
	}

	runtime.entityMu.RLock()
	_, sourceRegistered := runtime.entities[source.State.ID]
	runtime.entityMu.RUnlock()

	if sourceRegistered {
		t.Fatal("consumed source remains registered")
	}
}

func TestItemMergeRemovesMultipleConsumedCandidatesWithoutSkipping(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	candidates := []*runtimeItemEntity{
		runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 16.1, Y: 64, Z: 16.1}, game.Velocity{}, 40),
		runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 15.9, Y: 64, Z: 16.1}, game.Velocity{}, 40),
		runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 16.1, Y: 64, Z: 15.9}, game.Velocity{}, 40),
		runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 15.8, Y: 64, Z: 15.8}, game.Velocity{}, 40),
	}

	source := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 60}, game.Position{X: 15.9, Y: 64, Z: 15.9}, game.Velocity{}, 40)

	removed := runtime.mergeItemEntity(source)
	if removed || source.State.Removed || source.Stack.Count != 64 {
		t.Fatalf("source after merge = count %d, removed %t", source.Stack.Count, source.State.Removed)
	}

	runtime.entityMu.RLock()
	registeredCount := len(runtime.runtimeEntities)
	indexedCount := 0

	for _, entities := range runtime.entitiesByChunk {
		indexedCount += len(entities)
	}

	for _, candidate := range candidates {
		if _, registered := runtime.entities[candidate.State.ID]; registered {
			t.Errorf("consumed candidate %d remains registered", candidate.State.ID)
		}
	}

	runtime.entityMu.RUnlock()

	if registeredCount != 1 || indexedCount != 1 {
		t.Fatalf("registry counts = runtime %d, indexed %d", registeredCount, indexedCount)
	}
}

func TestItemMergeCandidateScanDoesNotAllocate(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	positions := [...]game.Position{
		{X: 15.9, Y: 64, Z: 15.9},
		{X: 15.9, Y: 64, Z: 16.1},
		{X: 16.1, Y: 64, Z: 15.9},
		{X: 16.1, Y: 64, Z: 16.1},
	}

	source := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, positions[0], game.Velocity{}, 40)

	for _, position := range positions {
		runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemDirt, Count: 1}, position, game.Velocity{}, 40)
	}

	allocations := testing.AllocsPerRun(100, func() {
		runtime.mergeItemEntity(source)
	})

	if allocations != 0 {
		t.Fatalf("merge candidate scan allocations = %g, want 0", allocations)
	}
}
