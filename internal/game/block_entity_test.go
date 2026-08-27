package game

import "testing"

type barrelEntityGenerator struct {
	position BlockPosition
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

	return ChunkBlockEntities{
		local: {
			Type: BlockEntityTypeBarrel,
			Items: [BarrelSlotCount]ItemStack{
				{Item: ItemStone, Count: int32(seed%64 + 1)},
			},
		},
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
	mutated.Items[0].Count++

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

	if !entity.Equal(BlockEntity{Type: BlockEntityTypeBarrel}) {
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
