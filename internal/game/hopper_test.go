package game

import "testing"

type hopperEntityGenerator struct {
	position BlockPosition
}

func (g hopperEntityGenerator) BlockAt(_ int64, position BlockPosition) Block {
	if position == g.position {
		return Hopper
	}

	return Air
}

func (g hopperEntityGenerator) GenerateBlockEntities(_ int64, chunk ChunkPosition) ChunkBlockEntities {
	positionChunk, local := blockIndex(g.position)
	if chunk != positionChunk {
		return nil
	}

	entity := NewBlockEntity(BlockEntityTypeHopper)
	hopper := entity.Data.(*HopperBlockEntityData)

	hopper.Items[0] = ItemStack{
		Item:              ItemStone,
		Count:             3,
		Components:        []ItemComponent{{Type: 8, Data: []byte{1, 2}}},
		RemovedComponents: []int32{5},
	}

	hopper.TransferCooldown = 6

	return ChunkBlockEntities{local: entity}
}

func TestHopperBlockEntityMetadataAndDefaultData(t *testing.T) {
	blockDefinition, blockValid := Hopper.Definition()
	if !blockValid || blockDefinition.BlockEntityType != BlockEntityTypeHopper {
		t.Fatalf("hopper block entity type = %d, %v", blockDefinition.BlockEntityType, blockValid)
	}

	definition, valid := BlockEntityTypeHopper.Definition()
	if !valid || definition.Name != "hopper" || definition.ProtocolRegistryID12111 != 18 || definition.InventorySlots != HopperSlotCount || HopperSlotCount != 5 {
		t.Fatalf("hopper block entity definition = %+v, %v", definition, valid)
	}

	entity := NewBlockEntity(BlockEntityTypeHopper)

	hopper, valid := entity.Data.(*HopperBlockEntityData)
	if !valid || len(hopper.Items) != 5 || hopper.TransferCooldown != -1 {
		t.Fatalf("new hopper data = %+v, %v", entity.Data, valid)
	}

	items, valid := entity.Inventory()
	if !valid || len(items) != 5 {
		t.Fatalf("hopper inventory = %d, %v; want 5, true", len(items), valid)
	}
}

func TestHopperBlockEntityCloneAndEqualityIncludeCooldownAndItemComponents(t *testing.T) {
	original := NewBlockEntity(BlockEntityTypeHopper)
	hopper := original.Data.(*HopperBlockEntityData)

	hopper.TransferCooldown = 4
	hopper.Items[0] = ItemStack{
		Item:              ItemStone,
		Count:             2,
		Components:        []ItemComponent{{Type: 8, Data: []byte{1, 2}}},
		RemovedComponents: []int32{5},
	}

	clone := original.Clone()
	if !original.Equal(clone) {
		t.Fatal("cloned hopper is not equal to its original")
	}

	clonedHopper := clone.Data.(*HopperBlockEntityData)

	clonedHopper.Items[0].Components[0].Data[0] = 9
	clonedHopper.Items[0].RemovedComponents[0] = 6

	if original.Equal(clone) || hopper.Items[0].Components[0].Data[0] != 1 || hopper.Items[0].RemovedComponents[0] != 5 {
		t.Fatal("hopper clone did not copy item components independently")
	}

	clone = original.Clone()
	clone.Data.(*HopperBlockEntityData).TransferCooldown++

	if original.Equal(clone) {
		t.Fatal("hoppers with different transfer cooldowns are equal")
	}
}

func TestGeneratedHopperMutationUsesCopyOnWriteAndCollapsesWhenRestored(t *testing.T) {
	position := BlockPosition{X: 3, Y: 70, Z: -17}
	world := &World{Generator: hopperEntityGenerator{position: position}}

	original, present := world.BlockEntityAt(position)
	if !present {
		t.Fatal("generated hopper entity is absent")
	}

	mutated := original.Clone()
	hopper := mutated.Data.(*HopperBlockEntityData)

	hopper.TransferCooldown++
	hopper.Items[0].Components[0].Data[0]++

	if !world.SetBlockEntity(position, mutated) {
		t.Fatal("mutating generated hopper entity failed")
	}

	count := world.BlockEntityOverrideCount()
	if count != 1 {
		t.Fatalf("block entity overrides after mutation = %d, want 1", count)
	}

	entity, present := world.BlockEntityAt(position)
	if !present || !entity.Equal(mutated) {
		t.Fatalf("mutated hopper entity = %+v, %v; want %+v, true", entity, present, mutated)
	}

	if !world.SetBlockEntity(position, original) {
		t.Fatal("restoring generated hopper entity failed")
	}

	count = world.BlockEntityOverrideCount()
	if count != 0 {
		t.Fatalf("block entity overrides after restoration = %d, want 0", count)
	}
}
