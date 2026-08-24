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
		if actual := second.BlockAt(position); actual != expected[position] {
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

	if actual := world.BlockAt(position); actual != Air {
		t.Fatalf("overridden block = %d, want air", actual)
	}

	neighbor := BlockPosition{X: -16, Y: 12, Z: 16}
	if actual := world.BlockAt(neighbor); actual != Stone {
		t.Fatalf("neighbor block = %d, want generated stone", actual)
	}

	world.ClearBlockOverride(position)

	if actual := world.BlockAt(position); actual != Stone {
		t.Fatalf("cleared override block = %d, want generated stone", actual)
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
	if block := world.BlockAt(position); block != Air {
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
