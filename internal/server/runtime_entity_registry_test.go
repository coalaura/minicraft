package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestRuntimeEntityRegistryMaintainsIDOrder(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	first := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 16.5, Y: 1}, game.Velocity{}, 0)
	second := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemDirt, Count: 1}, game.Position{X: 0.5, Y: 1}, game.Velocity{}, 0)
	third := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 16.5, Y: 1}, game.Velocity{}, 0)

	entities := runtime.snapshotRuntimeEntities()
	want := []int32{first.State.ID, second.State.ID, third.State.ID}

	assertRuntimeEntityIDs(t, entities, want)

	runtime.removeRuntimeEntity(second.State.ID)

	entities = runtime.snapshotRuntimeEntities()
	want = []int32{first.State.ID, third.State.ID}

	assertRuntimeEntityIDs(t, entities, want)

	chunkEntities := runtime.snapshotEntitiesInChunk(LoadedChunk{X: 1})
	if len(chunkEntities) != 2 || chunkEntities[first.State.ID] != first || chunkEntities[third.State.ID] != third {
		t.Fatalf("chunk registry = %#v", chunkEntities)
	}
}

func TestAppendItemEntitiesInBoxDoesNotAllocate(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	for index := range 3 {
		position := game.Position{X: float64(index) + 0.5, Y: 1, Z: 0.5}

		runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, position, game.Velocity{}, 0)
	}

	box := game.AABB{MinX: 0, MinY: 0, MinZ: 0, MaxX: 16, MaxY: 2, MaxZ: 16}

	items := make([]*runtimeItemEntity, 0, 3)
	items = runtime.appendItemEntitiesInBox(items, box)

	for index, item := range items {
		wantID := int32(index + 1)

		if item.State.ID != wantID {
			t.Fatalf("item %d ID = %d, want %d", index, item.State.ID, wantID)
		}
	}

	allocations := testing.AllocsPerRun(100, func() {
		items = runtime.appendItemEntitiesInBox(items[:0], box)
	})

	if allocations != 0 {
		t.Fatalf("append item entities allocations = %f, want 0", allocations)
	}

	if len(items) != 3 {
		t.Fatalf("item count = %d, want 3", len(items))
	}
}

func assertRuntimeEntityIDs(t *testing.T, entities []RuntimeEntity, want []int32) {
	t.Helper()

	if len(entities) != len(want) {
		t.Fatalf("entity count = %d, want %d", len(entities), len(want))
	}

	for index, entity := range entities {
		id := entity.RuntimeEntityState().ID
		if id != want[index] {
			t.Fatalf("entity %d ID = %d, want %d", index, id, want[index])
		}
	}
}
