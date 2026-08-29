package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type generatedRemovalChest struct {
	position game.BlockPosition
	count    int32
}

type invalidLightingRemovalChest struct {
	generatedRemovalChest
}

type containerRemovalTestCase struct {
	name       string
	block      game.Block
	entityType game.BlockEntityType
}

func (generator generatedRemovalChest) BlockAt(_ int64, position game.BlockPosition) game.Block {
	if position == generator.position {
		return game.Chest
	}

	return game.Air
}

func (generator generatedRemovalChest) GenerateBlockEntity(_ int64, position game.BlockPosition) (game.BlockEntity, bool) {
	if position != generator.position {
		return game.BlockEntity{}, false
	}

	entity := game.NewBlockEntity(game.BlockEntityTypeChest)

	items, _ := entity.Inventory()

	items[0] = game.ItemStack{Item: game.ItemDiamond, Count: generator.count}

	return entity, true
}

func (generator invalidLightingRemovalChest) BlockAt(seed int64, position game.BlockPosition) game.Block {
	if position == generator.position {
		return generator.generatedRemovalChest.BlockAt(seed, position)
	}

	if position == (game.BlockPosition{X: generator.position.X + 1, Y: generator.position.Y, Z: generator.position.Z}) {
		return game.Air
	}

	return game.MaxBlockState + 1
}

func TestContainerRemovalDropsContentsAndPropertyChangesDoNot(t *testing.T) {
	tests := []containerRemovalTestCase{
		{name: "chest", block: game.Chest, entityType: game.BlockEntityTypeChest},
		{name: "trapped chest", block: game.TrappedChest, entityType: game.BlockEntityTypeTrappedChest},
		{name: "barrel", block: game.Barrel, entityType: game.BlockEntityTypeBarrel},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position := game.BlockPosition{Y: 70}

			world := &game.World{}

			world.SetBlock(position, test.block)

			entity := game.NewBlockEntity(test.entityType)
			items, _ := entity.Inventory()

			items[0] = game.ItemStack{Item: game.ItemDiamond, Count: 37}

			world.SetBlockEntity(position, entity)

			runtime := NewRuntime(world)

			preservedState := test.block

			if test.entityType == game.BlockEntityTypeBarrel {
				preservedState = mustBlockState(t, game.Barrel,
					game.BlockPropertyValue{Name: "facing", Value: "north"},
					game.BlockPropertyValue{Name: "open", Value: "true"},
				)
			}

			result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: preservedState}})
			if err != nil {
				t.Fatalf("property mutation: %v", err)
			}

			if result.Changed && len(runtime.snapshotRuntimeEntities()) != 0 {
				t.Fatal("same block entity type dropped contents during a property change")
			}

			result, err = runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.Stone}})
			if err != nil || !result.Changed {
				t.Fatalf("replacement = %+v, %v", result, err)
			}

			if countDroppedItem(runtime, game.ItemDiamond) != 37 {
				t.Fatalf("dropped diamonds = %d, want 37", countDroppedItem(runtime, game.ItemDiamond))
			}
		})
	}
}

func TestDoubleChestRemovalDropsOnlyRemovedPhysicalHalf(t *testing.T) {
	left := game.BlockPosition{X: 0, Y: 70}
	right := game.BlockPosition{X: 1, Y: 70}

	world := &game.World{}

	world.SetBlock(left, mustBlockState(t, game.Chest,
		game.BlockPropertyValue{Name: "facing", Value: "south"},
		game.BlockPropertyValue{Name: "type", Value: "left"},
	))

	world.SetBlock(right, mustBlockState(t, game.Chest,
		game.BlockPropertyValue{Name: "facing", Value: "south"},
		game.BlockPropertyValue{Name: "type", Value: "right"},
	))

	leftEntity, _ := world.BlockEntityAt(left)
	leftItems, _ := leftEntity.Inventory()

	leftItems[0] = game.ItemStack{Item: game.ItemDiamond, Count: 11}

	world.SetBlockEntity(left, leftEntity)

	rightEntity, _ := world.BlockEntityAt(right)
	rightItems, _ := rightEntity.Inventory()

	rightItems[0] = game.ItemStack{Item: game.ItemGoldIngot, Count: 13}

	world.SetBlockEntity(right, rightEntity)

	runtime := NewRuntime(world)

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: left, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("break left chest = %+v, %v", result, err)
	}

	if countDroppedItem(runtime, game.ItemDiamond) != 11 || countDroppedItem(runtime, game.ItemGoldIngot) != 0 {
		t.Fatalf("double chest drops = diamonds %d, gold %d", countDroppedItem(runtime, game.ItemDiamond), countDroppedItem(runtime, game.ItemGoldIngot))
	}

	surviving, present := world.BlockEntityAt(right)

	survivingItems, inventory := surviving.Inventory()
	if !present || !inventory || survivingItems[0].Count != 13 {
		t.Fatalf("surviving chest entity = %+v, %v", surviving, present)
	}
}

func TestGeneratedChestContentsDropOnceWithoutResurrection(t *testing.T) {
	position := game.BlockPosition{X: 3, Y: 70, Z: 4}

	world := &game.World{Generator: generatedRemovalChest{position: position, count: 23}}

	runtime := NewRuntime(world)

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("break generated chest = %+v, %v", result, err)
	}

	if countDroppedItem(runtime, game.ItemDiamond) != 23 {
		t.Fatalf("first break dropped %d diamonds, want 23", countDroppedItem(runtime, game.ItemDiamond))
	}

	result, err = runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.Chest}})
	if err != nil || !result.Changed {
		t.Fatalf("replace generated chest = %+v, %v", result, err)
	}

	replacement, present := world.BlockEntityAt(position)

	items, inventory := replacement.Inventory()
	if !present || !inventory || !items[0].Empty() {
		t.Fatalf("replacement generated chest = %+v, %v", replacement, present)
	}

	result, err = runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("break replacement chest = %+v, %v", result, err)
	}

	if countDroppedItem(runtime, game.ItemDiamond) != 23 {
		t.Fatalf("generated contents resurrected; total drops = %d", countDroppedItem(runtime, game.ItemDiamond))
	}
}

func TestContainerRemovalDeliversBlockUpdateBeforeDroppedItems(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	world.SetBlock(position, game.Barrel)

	entity, _ := world.BlockEntityAt(position)
	items, _ := entity.Inventory()

	items[0] = game.ItemStack{Item: game.ItemDiamond, Count: 1}

	world.SetBlockEntity(position, entity)

	runtime := NewRuntime(world)

	viewer, connection := newPlacementTestSession(runtime, position)

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{blockLoadedChunk(position)})

	joinTestSession(t, runtime, viewer)

	connection.reset()

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("remove barrel = %+v, %v", result, err)
	}

	blockUpdateIndex := -1
	addEntityIndex := -1

	for index, packetID := range connection.packetIDs(t) {
		if packetID == protocol.ClientboundBlockUpdateID && blockUpdateIndex < 0 {
			blockUpdateIndex = index
		}

		if packetID == protocol.ClientboundAddEntityID && addEntityIndex < 0 {
			addEntityIndex = index
		}
	}

	if blockUpdateIndex < 0 || addEntityIndex < 0 || blockUpdateIndex >= addEntityIndex {
		t.Fatalf("block/drop packet order = %v", connection.packetIDs(t))
	}
}

func TestContainerRemovalMatchesVanillaSplitRandomness(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	randomValues := []float32{
		0.1, 0.2, 0.3,
		0, 0.4, 0.5, 0.6, 0.1, 0.7, 0.2, 0.8, 0.3,
		0.6, 0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.3, 0.2,
	}

	randomIndex := 0

	runtime.entityRandom = func() float32 {
		value := randomValues[randomIndex]
		randomIndex++

		return value
	}

	runtime.dropContainerContents(game.BlockPosition{X: 2, Y: 70, Z: -3}, []game.ItemStack{{Item: game.ItemDiamond, Count: 31}})

	if randomIndex != len(randomValues) {
		t.Fatalf("random calls = %d, want %d", randomIndex, len(randomValues))
	}

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 2 {
		t.Fatalf("split entity count = %d, want 2", len(entities))
	}

	expectedPosition := game.Position{
		X: 2.125 + float64(float32(0.1))*0.75,
		Y: 70 + float64(float32(0.2))*0.75,
		Z: -2.875 + float64(float32(0.3))*0.75,
	}

	var total int32

	for _, entity := range entities {
		dropped, valid := entity.(*runtimeItemEntity)
		if !valid {
			t.Fatalf("dropped entity = %T", entity)
		}

		total += dropped.Stack.Count

		if dropped.State.Position != expectedPosition || dropped.PickupDelay != 0 {
			t.Fatalf("container drop = position %+v, delay %d", dropped.State.Position, dropped.PickupDelay)
		}
	}

	if total != 31 {
		t.Fatalf("dropped count = %d, want 31", total)
	}

	expectedVelocity := game.Velocity{
		X: float64(float32(0.6)-float32(0.1)) * motionDeviation,
		Y: 0.2 + float64(float32(0.7)-float32(0.2))*motionDeviation,
		Z: float64(float32(0.8)-float32(0.3)) * motionDeviation,
	}

	found := false

	for _, entity := range entities {
		dropped := entity.(*runtimeItemEntity)
		if dropped.Stack.Count == 10 {
			found = true

			if math.Abs(dropped.Velocity.X-expectedVelocity.X) > 1e-15 || math.Abs(dropped.Velocity.Y-expectedVelocity.Y) > 1e-15 || math.Abs(dropped.Velocity.Z-expectedVelocity.Z) > 1e-15 {
				t.Fatalf("first split velocity = %+v, want %+v", dropped.Velocity, expectedVelocity)
			}
		}
	}

	if !found {
		t.Fatal("missing first 10-item split")
	}
}

func TestCommittedContainerRemovalDropsContentsAfterLightingFailure(t *testing.T) {
	position := game.BlockPosition{X: 3, Y: 70, Z: 4}

	generator := invalidLightingRemovalChest{generatedRemovalChest{position: position, count: 23}}

	world := &game.World{Generator: generator, Lighting: game.LightingNormal}

	runtime := NewRuntime(world)

	lightChangePosition := game.BlockPosition{X: position.X + 1, Y: position.Y, Z: position.Z}

	result, err := runtime.MutateWorldBlocksStrict([]game.BlockChange{
		{Position: position, Replacement: game.Air},
		{Position: lightChangePosition, Replacement: game.Stone},
	})

	if err == nil || !result.Changed {
		t.Fatalf("lighting-failed removal = %+v, %v", result, err)
	}

	if world.BlockAt(position) != game.Air {
		t.Fatalf("committed block = %v, want air", world.BlockAt(position))
	}

	if countDroppedItem(runtime, game.ItemDiamond) != 23 {
		t.Fatalf("committed removal dropped %d diamonds, want 23", countDroppedItem(runtime, game.ItemDiamond))
	}
}

func countDroppedItem(runtime *Runtime, item game.Item) int32 {
	var count int32

	for _, entity := range runtime.snapshotRuntimeEntities() {
		dropped, valid := entity.(*runtimeItemEntity)
		if valid && dropped.Stack.Item == item {
			count += dropped.Stack.Count
		}
	}

	return count
}
