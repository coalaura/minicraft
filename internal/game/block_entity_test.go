package game

import "testing"

type testBlockEntityData struct {
	value int
}

type pointBlockEntityGenerator struct {
	position   BlockPosition
	pointCalls int
	chunkCalls int
}

type barrelEntityGenerator struct {
	position BlockPosition
}

type chestEntityGenerator struct {
	first  BlockPosition
	second BlockPosition
}

type staleBlockEntityGenerator struct {
	chunkCalls int
}

type blockEntityMetadataTestCase struct {
	block      Block
	entityType BlockEntityType
	name       string
	registryID int32
}

func (g barrelEntityGenerator) BlockAt(_ int64, position BlockPosition) Block {
	if position == g.position {
		return Barrel
	}

	return Air
}

func (g barrelEntityGenerator) GenerateBlockEntities(seed int64, chunk ChunkPosition) ChunkBlockEntities {
	positionChunk, local := blockIndex(g.position)
	if chunk != positionChunk {
		return nil
	}

	entity := NewBlockEntity(BlockEntityTypeBarrel)

	items, _ := entity.Inventory()

	items[0] = ItemStack{Item: ItemStone, Count: int32(seed%64 + 1)}

	return ChunkBlockEntities{local: entity}
}

func (g chestEntityGenerator) BlockAt(_ int64, position BlockPosition) Block {
	if position == g.first || position == g.second {
		return Chest
	}

	return Stone
}

func (g chestEntityGenerator) GenerateBlockEntities(_ int64, chunk ChunkPosition) ChunkBlockEntities {
	entities := make(ChunkBlockEntities)

	for index, position := range []BlockPosition{g.first, g.second} {
		positionChunk, local := blockIndex(position)
		if positionChunk != chunk {
			continue
		}

		entity := NewBlockEntity(BlockEntityTypeChest)

		items, _ := entity.Inventory()

		items[0] = ItemStack{Item: ItemStone, Count: int32(index + 1)}

		entities[local] = entity
	}

	return entities
}

func (g *staleBlockEntityGenerator) BlockAt(_ int64, _ BlockPosition) Block {
	return Stone
}

func (g *staleBlockEntityGenerator) GenerateBlockEntities(_ int64, _ ChunkPosition) ChunkBlockEntities {
	g.chunkCalls++

	return ChunkBlockEntities{
		{X: 1, Y: 70, Z: 1}:  NewBlockEntity(BlockEntityTypeChest),
		{X: -1, Y: 70, Z: 1}: NewBlockEntity(BlockEntityTypeChest),
	}
}

func (data *testBlockEntityData) CloneBlockEntityData() BlockEntityData {
	clone := *data
	return &clone
}

func (data *testBlockEntityData) EqualBlockEntityData(other BlockEntityData) bool {
	otherData, valid := other.(*testBlockEntityData)
	return valid && data.value == otherData.value
}

func (g *pointBlockEntityGenerator) BlockAt(_ int64, position BlockPosition) Block {
	if position == g.position {
		return Barrel
	}

	return Air
}

func (g *pointBlockEntityGenerator) GenerateBlockEntity(_ int64, position BlockPosition) (BlockEntity, bool) {
	g.pointCalls++
	if position != g.position {
		return BlockEntity{}, false
	}

	return NewBlockEntity(BlockEntityTypeBarrel), true
}

func (g *pointBlockEntityGenerator) GenerateBlockEntities(_ int64, chunk ChunkPosition) ChunkBlockEntities {
	g.chunkCalls++

	positionChunk, local := blockIndex(g.position)
	if chunk != positionChunk {
		return nil
	}

	return ChunkBlockEntities{local: NewBlockEntity(BlockEntityTypeBarrel)}
}

func TestBlockEntityDataSupportsDifferentShapes(t *testing.T) {
	inventory := NewInventoryBlockEntity(BlockEntityTypeBarrel, 5)

	items, valid := inventory.Inventory()
	if !valid || len(items) != 5 {
		t.Fatalf("arbitrary inventory shape = %d, %v; want 5, true", len(items), valid)
	}

	withoutInventory := BlockEntity{Type: BlockEntityType(2)}
	if _, valid = withoutInventory.Inventory(); valid {
		t.Fatal("data-less block entity unexpectedly exposes inventory")
	}

	typed := BlockEntity{Type: BlockEntityType(3), Data: &testBlockEntityData{value: 7}}

	clone := typed.Clone()

	clone.Data.(*testBlockEntityData).value = 8

	if typed.Equal(clone) || typed.Data.(*testBlockEntityData).value != 7 {
		t.Fatal("type-specific block entity data did not clone independently")
	}
}

func TestBlockEntityMetadataDefinesIdentityAndProtocolValues(t *testing.T) {
	barrelDefinition, valid := Barrel.Definition()
	if !valid || barrelDefinition.BlockEntityType != BlockEntityTypeBarrel {
		t.Fatalf("barrel block entity type = %d, %v", barrelDefinition.BlockEntityType, valid)
	}

	if BlockEntityTypeForBlock(Stone) != BlockEntityTypeNone {
		t.Fatal("ordinary block unexpectedly hosts a block entity")
	}

	entityDefinition, valid := BlockEntityTypeBarrel.Definition()
	if !valid || entityDefinition.Name != "barrel" || entityDefinition.ProtocolRegistryID12111 != 27 || entityDefinition.InventorySlots != BarrelSlotCount {
		t.Fatalf("barrel block entity definition = %+v, %v", entityDefinition, valid)
	}

	for _, test := range []blockEntityMetadataTestCase{
		{block: Chest, entityType: BlockEntityTypeChest, name: "chest", registryID: 1},
		{block: TrappedChest, entityType: BlockEntityTypeTrappedChest, name: "trapped_chest", registryID: 2},
	} {
		blockDefinition, blockValid := test.block.Definition()
		entityDefinition, entityValid := test.entityType.Definition()

		if !blockValid || blockDefinition.BlockEntityType != test.entityType || !entityValid || entityDefinition.Name != test.name || entityDefinition.ProtocolRegistryID12111 != test.registryID || entityDefinition.InventorySlots != ChestSlotCount {
			t.Fatalf("%s block/entity definitions = %+v, %+v", test.name, blockDefinition, entityDefinition)
		}
	}

	for _, test := range copperChestBlocksForTest() {
		blockDefinition, blockValid := test.block.Definition()
		entityDefinition, entityValid := BlockEntityTypeChest.Definition()

		if !blockValid || blockDefinition.Behavior != BlockBehaviorChest || blockDefinition.Collision != BlockCollisionChest || blockDefinition.BlockEntityType != BlockEntityTypeChest || !entityValid || entityDefinition.ProtocolRegistryID12111 != 1 || entityDefinition.InventorySlots != ChestSlotCount {
			t.Errorf("%s block/entity definitions = %+v, %+v", test.name, blockDefinition, entityDefinition)
		}
	}
}

func TestGeneratedChestHalvesUseIndependentCopyOnWriteAndTombstones(t *testing.T) {
	first := BlockPosition{X: 3, Y: 70, Z: 4}
	second := BlockPosition{X: 4, Y: 70, Z: 4}

	world := &World{Generator: chestEntityGenerator{first: first, second: second}}

	firstEntity, firstPresent := world.BlockEntityAt(first)
	secondEntity, secondPresent := world.BlockEntityAt(second)

	if !firstPresent || !secondPresent {
		t.Fatal("generated chest halves are absent")
	}

	mutated := firstEntity.Clone()
	items, _ := mutated.Inventory()

	items[0].Count++

	if !world.SetBlockEntity(first, mutated) {
		t.Fatal("mutating first generated chest half failed")
	}

	if count := world.BlockEntityOverrideCount(); count != 1 {
		t.Fatalf("block entity overrides after one-half mutation = %d, want 1", count)
	}

	unchanged, present := world.BlockEntityAt(second)
	if !present || !unchanged.Equal(secondEntity) {
		t.Fatalf("untouched generated chest half = %+v, %v; want %+v, true", unchanged, present, secondEntity)
	}

	world.SetBlock(first, Air)
	world.SetBlock(first, Chest)

	replacement, present := world.BlockEntityAt(first)
	if !present || !replacement.Equal(NewBlockEntity(BlockEntityTypeChest)) {
		t.Fatalf("replacement chest half = %+v, %v; want empty chest", replacement, present)
	}

	if replacement.Equal(firstEntity) {
		t.Fatal("replacement chest resurrected generated contents")
	}
}

func TestGeneratedBlockEntitiesAreFilteredAgainstHostAndChunkLocality(t *testing.T) {
	generator := &staleBlockEntityGenerator{}
	world := &World{Generator: generator}

	if entity, present := world.BlockEntityAt(BlockPosition{X: 1, Y: 70, Z: 1}); present {
		t.Fatalf("stale point block entity = %+v, true; want absent", entity)
	}

	if entities := world.SnapshotChunkBlockEntities(ChunkPosition{}); len(entities) != 0 {
		t.Fatalf("filtered chunk block entities = %+v, want none", entities)
	}
}

func TestBlockEntityPointLookupAvoidsChunkEnumeration(t *testing.T) {
	position := BlockPosition{X: 3, Y: 70, Z: 4}

	generator := &pointBlockEntityGenerator{position: position}

	world := &World{Generator: generator}

	entity, present := world.BlockEntityAt(position)
	if !present || entity.Type != BlockEntityTypeBarrel {
		t.Fatalf("point block entity = %+v, %v", entity, present)
	}

	if generator.pointCalls != 1 || generator.chunkCalls != 0 {
		t.Fatalf("generator calls = point %d, chunk %d; want 1, 0", generator.pointCalls, generator.chunkCalls)
	}

	world.SnapshotChunkBlockEntities(ChunkPosition{})
	if generator.chunkCalls != 1 {
		t.Fatalf("chunk snapshot calls = %d, want 1", generator.chunkCalls)
	}
}

func TestGeneratedBarrelContentsAreDeterministicAndDoNotCreateOverrides(t *testing.T) {
	position := BlockPosition{X: -17, Y: 70, Z: 31}
	generator := barrelEntityGenerator{position: position}

	first := &World{Seed: 42, Generator: generator}
	second := &World{Seed: 42, Generator: generator}
	differentSeed := &World{Seed: 43, Generator: generator}

	expected, present := first.BlockEntityAt(position)
	if !present {
		t.Fatal("generated barrel entity is absent")
	}

	actual, present := second.BlockEntityAt(position)
	if !present || !actual.Equal(expected) {
		t.Fatalf("same-seed barrel entity = %+v, %v; want %+v, true", actual, present, expected)
	}

	different, present := differentSeed.BlockEntityAt(position)
	if !present || different.Equal(expected) {
		t.Fatalf("different-seed barrel entity = %+v, %v; want contents different from %+v", different, present, expected)
	}

	chunk, local := blockIndex(position)
	snapshot := first.SnapshotChunkBlockEntities(chunk)

	if entity := snapshot[local]; !entity.Equal(expected) {
		t.Fatalf("snapshot barrel entity = %+v, want %+v", entity, expected)
	}

	if count := first.BlockEntityOverrideCount(); count != 0 {
		t.Fatalf("block entity overrides after reads = %d, want 0", count)
	}
}

func TestGeneratedBarrelMutationUsesCopyOnWriteAndCollapsesWhenRestored(t *testing.T) {
	position := BlockPosition{X: 3, Y: 70, Z: -17}
	world := &World{Seed: 42, Generator: barrelEntityGenerator{position: position}}

	original, present := world.BlockEntityAt(position)
	if !present {
		t.Fatal("generated barrel entity is absent")
	}

	mutated := original.Clone()

	items, _ := mutated.Inventory()

	items[0].Count++

	if !world.SetBlockEntity(position, mutated) {
		t.Fatal("mutating generated barrel entity failed")
	}

	if count := world.BlockEntityOverrideCount(); count != 1 {
		t.Fatalf("block entity overrides after mutation = %d, want 1", count)
	}

	entity, present := world.BlockEntityAt(position)
	if !present || !entity.Equal(mutated) {
		t.Fatalf("mutated barrel entity = %+v, %v; want %+v, true", entity, present, mutated)
	}

	if !world.SetBlockEntity(position, original) {
		t.Fatal("restoring generated barrel entity failed")
	}

	if count := world.BlockEntityOverrideCount(); count != 0 {
		t.Fatalf("block entity overrides after restoration = %d, want 0", count)
	}
}

func TestGeneratedBarrelBreakCreatesTombstoneAndReplacementIsEmpty(t *testing.T) {
	position := BlockPosition{X: 1, Y: 70, Z: 2}
	world := &World{Seed: 42, Generator: barrelEntityGenerator{position: position}}

	original, present := world.BlockEntityAt(position)
	if !present {
		t.Fatal("generated barrel entity is absent")
	}

	world.SetBlock(position, Air)

	if entity, present := world.BlockEntityAt(position); present {
		t.Fatalf("broken barrel entity = %+v, true; want absent", entity)
	}

	chunk, local := blockIndex(position)
	if _, present := world.SnapshotChunkBlockEntities(chunk)[local]; present {
		t.Fatal("broken barrel reappeared in snapshot")
	}

	if count := world.BlockEntityOverrideCount(); count != 1 {
		t.Fatalf("block entity overrides after break = %d, want 1", count)
	}

	world.SetBlock(position, Barrel)

	entity, present := world.BlockEntityAt(position)
	if !present {
		t.Fatal("replacement barrel entity is absent")
	}

	if !entity.Equal(NewBlockEntity(BlockEntityTypeBarrel)) {
		t.Fatalf("replacement barrel entity = %+v, want empty barrel", entity)
	}

	fresh := &World{Seed: 42, Generator: barrelEntityGenerator{position: position}}

	reconstructed, present := fresh.BlockEntityAt(position)
	if !present {
		t.Fatal("fresh generated barrel entity is absent")
	}

	if !reconstructed.Equal(original) {
		t.Fatalf("fresh barrel entity = %+v, want %+v", reconstructed, original)
	}

	if entity.Equal(reconstructed) {
		t.Fatalf("replacement barrel entity = %+v, want contents distinct from generated %+v", entity, reconstructed)
	}
}

func TestClearBlockOverrideRestoresGeneratedBarrel(t *testing.T) {
	position := BlockPosition{X: 1, Y: 70, Z: 2}
	world := &World{Seed: 42, Generator: barrelEntityGenerator{position: position}}

	original, present := world.BlockEntityAt(position)
	if !present {
		t.Fatal("generated barrel entity is absent")
	}

	world.SetBlock(position, Air)
	world.ClearBlockOverride(position)

	if block := world.BlockAt(position); block != Barrel {
		t.Fatalf("cleared barrel block = %d, want barrel", block)
	}

	entity, present := world.BlockEntityAt(position)
	if !present || !entity.Equal(original) {
		t.Fatalf("cleared barrel entity = %+v, %v; want %+v, true", entity, present, original)
	}

	if count := world.BlockEntityOverrideCount(); count != 0 {
		t.Fatalf("block entity overrides after clear = %d, want 0", count)
	}
}
