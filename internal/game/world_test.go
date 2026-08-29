package game

import (
	"math"
	"slices"
	"testing"
)

type blockIndexTest struct {
	position BlockPosition
	chunk    ChunkPosition
	local    LocalBlockPosition
}

type coordinateGenerator struct{}

func (coordinateGenerator) BlockAt(seed int64, position BlockPosition) Block {
	value := seed + int64(position.X)*3 + int64(position.Y)*5 + int64(position.Z)*7
	if value&1 == 0 {
		return Stone
	}

	return Air
}

type solidGenerator struct {
	block Block
}

type spawningGenerator struct {
	spawnSeed int64
}

type metadataGenerator struct {
	metadataSeed int64
}

type countingOrdinaryBlockEntityGenerator struct {
	entityCalls int
}

type countingRemovalBlockEntityGenerator struct {
	entityCalls int
}

func (g *spawningGenerator) BlockAt(_ int64, _ BlockPosition) Block {
	return Air
}

func (g *spawningGenerator) Spawn(seed int64) Position {
	g.spawnSeed = seed

	return Position{X: 4.5, Y: 90, Z: -2.5}
}

func (g solidGenerator) BlockAt(_ int64, _ BlockPosition) Block {
	return g.block
}

func (g *metadataGenerator) BlockAt(_ int64, _ BlockPosition) Block {
	return Air
}

func (g *metadataGenerator) WorldMetadata(seed int64) WorldMetadata {
	g.metadataSeed = seed

	return WorldMetadata{SeaLevel: 27}
}

func (g *countingOrdinaryBlockEntityGenerator) BlockAt(_ int64, _ BlockPosition) Block {
	return Stone
}

func (g *countingOrdinaryBlockEntityGenerator) GenerateBlockEntities(_ int64, _ ChunkPosition) ChunkBlockEntities {
	g.entityCalls++

	return nil
}

func (g *countingRemovalBlockEntityGenerator) BlockAt(_ int64, _ BlockPosition) Block {
	return Barrel
}

func (g *countingRemovalBlockEntityGenerator) GenerateBlockEntities(_ int64, _ ChunkPosition) ChunkBlockEntities {
	g.entityCalls++

	return ChunkBlockEntities{
		{X: 1, Y: 70, Z: 1}: NewBlockEntity(BlockEntityTypeBarrel),
		{X: 2, Y: 70, Z: 1}: NewBlockEntity(BlockEntityTypeBarrel),
	}
}

func TestNewOverworldUsesSeedAndGeneratorSpawn(t *testing.T) {
	generator := &spawningGenerator{}
	world := NewOverworld(generator, 42)

	if world.Seed != 42 || generator.spawnSeed != 42 {
		t.Fatalf("world seed = %d, generator seed = %d, want 42", world.Seed, generator.spawnSeed)
	}

	expected := Position{X: 4.5, Y: 90, Z: -2.5}
	if world.Spawn != expected {
		t.Fatalf("world spawn = %+v, want %+v", world.Spawn, expected)
	}
}

func TestNewOverworldUsesGeneratorMetadataAndDefaultSeaLevel(t *testing.T) {
	seaLevel := NewOverworld(nil).SeaLevel
	if seaLevel != 63 {
		t.Fatalf("default sea level = %d, want 63", seaLevel)
	}

	generator := &metadataGenerator{}

	world := NewOverworld(generator, 42)

	if world.SeaLevel != 27 {
		t.Fatalf("generator sea level = %d, want 27", world.SeaLevel)
	}

	if generator.metadataSeed != 42 {
		t.Fatalf("metadata seed = %d, want 42", generator.metadataSeed)
	}
}

func TestWorldTimeAdvancesAndFreezesDayTime(t *testing.T) {
	world := NewOverworld(nil)

	world.SetTime(18000, false)

	state := world.AdvanceTime()
	if state.Age != 1 || state.DayTime != 18000 || state.DayCycle {
		t.Fatalf("fixed time after tick = %+v", state)
	}

	world.SetTime(6000, true)

	state = world.AdvanceTime()
	if state.Age != 1 || state.DayTime != 6001 || !state.DayCycle {
		t.Fatalf("cycling time after tick = %+v", state)
	}
}

func TestWorldGenerationIsDeterministic(t *testing.T) {
	positions := []BlockPosition{
		{X: -17, Y: -64, Z: 31},
		{X: -16, Y: 69, Z: -1},
		{X: 0, Y: 70, Z: 0},
		{X: 16, Y: 255, Z: 16},
	}

	first := &World{Seed: 42, Generator: coordinateGenerator{}}
	second := &World{Seed: 42, Generator: coordinateGenerator{}}

	expected := make(map[BlockPosition]Block, len(positions))

	for _, position := range positions {
		expected[position] = first.BlockAt(position)
	}

	for _, position := range slices.Backward(positions) {
		actual := second.BlockAt(position)
		if actual != expected[position] {
			t.Fatalf("block at %+v = %d, want %d", position, actual, expected[position])
		}
	}

	differentSeed := &World{Seed: 43, Generator: coordinateGenerator{}}
	if differentSeed.BlockAt(positions[0]) == expected[positions[0]] {
		t.Fatal("different world seed produced the same test block")
	}
}

func TestWorldOverridesTakePrecedence(t *testing.T) {
	world := &World{Generator: solidGenerator{block: Stone}}

	position := BlockPosition{X: -17, Y: 12, Z: 16}

	world.SetBlock(position, Air)

	actual := world.BlockAt(position)
	if actual != Air {
		t.Fatalf("overridden block = %d, want air", actual)
	}

	neighbor := BlockPosition{X: -16, Y: 12, Z: 16}

	neighborBlock := world.BlockAt(neighbor)
	if neighborBlock != Stone {
		t.Fatalf("neighbor block = %d, want generated stone", neighborBlock)
	}

	world.ClearBlockOverride(position)

	clearedBlock := world.BlockAt(position)
	if clearedBlock != Stone {
		t.Fatalf("cleared override block = %d, want generated stone", clearedBlock)
	}
}

func TestSetBlockRemovesGeneratorEquivalentOverride(t *testing.T) {
	world := &World{Generator: solidGenerator{block: Stone}}

	position := BlockPosition{X: -17, Y: 12, Z: 16}

	world.SetBlock(position, Air)

	chunk, _ := blockIndex(position)
	if len(world.overrides[chunk]) != 1 {
		t.Fatalf("chunk overrides = %d, want 1", len(world.overrides[chunk]))
	}

	world.SetBlock(position, Stone)

	if len(world.overrides) != 0 {
		t.Fatalf("world override chunks = %d, want 0", len(world.overrides))
	}
}

func TestSetBlockDoesNotStoreGeneratedAir(t *testing.T) {
	world := &World{}

	world.SetBlock(BlockPosition{X: 1, Y: 2, Z: 3}, Air)

	if world.overrides != nil {
		t.Fatalf("world overrides = %#v, want nil", world.overrides)
	}
}

func TestSetBlocksAppliesSparseBatch(t *testing.T) {
	world := &World{Generator: solidGenerator{block: Stone}}

	first := BlockPosition{X: 1, Y: 2, Z: 3}
	second := BlockPosition{X: 17, Y: 4, Z: 5}

	world.SetBlocks([]BlockChange{
		{Position: first, Replacement: Air},
		{Position: second, Replacement: Dirt},
	})

	if world.BlockAt(first) != Air || world.BlockAt(second) != Dirt {
		t.Fatalf("batch blocks = %d, %d; want air, dirt", world.BlockAt(first), world.BlockAt(second))
	}

	world.SetBlocks([]BlockChange{
		{Position: first, Replacement: Stone},
		{Position: second, Replacement: Stone},
	})

	if len(world.overrides) != 0 {
		t.Fatalf("generator-equivalent batch left overrides: %#v", world.overrides)
	}
}

func TestSetBlocksSkipsBlockEntityGenerationForOrdinaryBulkChanges(t *testing.T) {
	generator := &countingOrdinaryBlockEntityGenerator{}
	world := &World{Generator: generator}

	changes := make([]BlockChange, 0, 512)

	for index := range 512 {
		coordinate := int32(index)
		changes = append(changes, BlockChange{
			Position:    BlockPosition{X: coordinate, Y: 70, Z: coordinate % 16},
			Replacement: Dirt,
		})
	}

	world.SetBlocks(changes)

	if generator.entityCalls != 0 {
		t.Fatalf("ordinary bulk mutation generated block entities %d times, want 0", generator.entityCalls)
	}
}

func TestSetBlocksCachesChunkEntityGenerationForMultipleRemovals(t *testing.T) {
	generator := &countingRemovalBlockEntityGenerator{}
	world := &World{Generator: generator}

	world.SetBlocks([]BlockChange{
		{Position: BlockPosition{X: 1, Y: 70, Z: 1}, Replacement: Air},
		{Position: BlockPosition{X: 2, Y: 70, Z: 1}, Replacement: Air},
	})

	if generator.entityCalls != 1 {
		t.Fatalf("same-chunk removals generated block entities %d times, want 1", generator.entityCalls)
	}

	count := world.BlockEntityOverrideCount()
	if count != 2 {
		t.Fatalf("same-chunk removal tombstones = %d, want 2", count)
	}
}

func TestSnapshotChunkOverridesIsIndependent(t *testing.T) {
	world := &World{Generator: solidGenerator{block: Stone}}

	position := BlockPosition{X: 2, Y: 40, Z: 3}
	world.SetBlock(position, Air)

	snapshot := world.SnapshotChunkOverrides(ChunkPosition{})

	local := LocalBlockPosition{X: 2, Y: 40, Z: 3}
	if snapshot[local] != Air {
		t.Fatalf("snapshot override = %d, want air", snapshot[local])
	}

	snapshot[local] = Stone

	block := world.BlockAt(position)
	if block != Air {
		t.Fatalf("snapshot mutation changed world block to %d", block)
	}
}

func TestBlockIndexHandlesNegativeBoundaries(t *testing.T) {
	tests := []blockIndexTest{
		{
			position: BlockPosition{X: -1, Z: -1},
			chunk:    ChunkPosition{X: -1, Z: -1},
			local:    LocalBlockPosition{X: 15, Z: 15},
		},
		{
			position: BlockPosition{X: -16, Z: -16},
			chunk:    ChunkPosition{X: -1, Z: -1},
			local:    LocalBlockPosition{},
		},
		{
			position: BlockPosition{X: -17, Z: -17},
			chunk:    ChunkPosition{X: -2, Z: -2},
			local:    LocalBlockPosition{X: 15, Z: 15},
		},
		{
			position: BlockPosition{X: math.MinInt32, Z: math.MinInt32},
			chunk:    ChunkPosition{X: -134217728, Z: -134217728},
			local:    LocalBlockPosition{},
		},
	}

	for _, test := range tests {
		chunk, local := blockIndex(test.position)
		if chunk != test.chunk || local != test.local {
			t.Errorf(
				"blockIndex(%+v) = (%+v, %+v), want (%+v, %+v)",
				test.position,
				chunk,
				local,
				test.chunk,
				test.local,
			)
		}
	}
}
