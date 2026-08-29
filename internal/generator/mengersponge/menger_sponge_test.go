package mengersponge

import (
	"reflect"
	"sync"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

type generatedChunkSnapshot struct {
	sections [7][game.SectionVolume]game.Block
}

type mengerPatternTestCase struct {
	name  string
	x     int32
	y     int32
	z     int32
	block game.Block
}

func TestGeneratorIsRegistered(t *testing.T) {
	registered, err := generator.New(Name)
	if err != nil {
		t.Fatalf("create registered generator: %v", err)
	}

	if _, ok := registered.(Generator); !ok {
		t.Fatalf("registered generator type = %T", registered)
	}
}

func TestGeneratorSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}

	spawn := generated.Spawn(0)

	position := game.BlockPosition{
		X: int32(spawn.X),
		Y: int32(spawn.Y),
		Z: int32(spawn.Z),
	}

	spawnBlock := generated.BlockAt(0, position)
	if spawnBlock != game.Air {
		t.Fatalf("spawn block = %d, want air", spawnBlock)
	}

	position.Y++

	blockAboveSpawn := generated.BlockAt(0, position)
	if blockAboveSpawn != game.Air {
		t.Fatalf("block above spawn = %d, want air", blockAboveSpawn)
	}

	position.Y -= 2

	blockBelowSpawn := generated.BlockAt(0, position)
	if blockBelowSpawn != game.Stone {
		t.Fatalf("block below spawn = %d, want stone", blockBelowSpawn)
	}
}

func TestGeneratorBuildsMengerPattern(t *testing.T) {
	generated := Generator{}

	tests := []mengerPatternTestCase{
		{
			name:  "origin",
			x:     0,
			y:     minBuildY,
			z:     0,
			block: game.Stone,
		},
		{
			name:  "small opening",
			x:     1,
			y:     minBuildY,
			z:     1,
			block: game.Air,
		},
		{
			name:  "small strut",
			x:     1,
			y:     minBuildY,
			z:     0,
			block: game.Stone,
		},
		{
			name:  "next scale opening",
			x:     3,
			y:     minBuildY,
			z:     3,
			block: game.Air,
		},
		{
			name:  "next scale strut",
			x:     3,
			y:     minBuildY,
			z:     0,
			block: game.Stone,
		},
		{
			name:  "vertical opening",
			x:     1,
			y:     minBuildY + 1,
			z:     0,
			block: game.Air,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position := game.BlockPosition{
				X: test.x,
				Y: test.y,
				Z: test.z,
			}

			block := generated.BlockAt(0, position)
			if block != test.block {
				t.Fatalf("block at %+v = %d, want %d", position, block, test.block)
			}
		})
	}
}

const farScale = int32(387420489) // 3^18

func TestGeneratorExtendsToArbitrarilyLargeCoordinates(t *testing.T) {
	generated := Generator{}

	strut := game.BlockPosition{
		X: farScale,
		Y: minBuildY,
		Z: 0,
	}

	farStrutBlock := generated.BlockAt(0, strut)
	if farStrutBlock != game.Stone {
		t.Fatalf("far strut = %d, want stone", farStrutBlock)
	}

	opening := game.BlockPosition{
		X: farScale,
		Y: minBuildY,
		Z: farScale,
	}

	farOpeningBlock := generated.BlockAt(0, opening)
	if farOpeningBlock != game.Air {
		t.Fatalf("far opening = %d, want air", farOpeningBlock)
	}
}

func TestGeneratorIsSymmetricAcrossNegativeCoordinates(t *testing.T) {
	generated := Generator{}

	positions := []game.BlockPosition{
		{X: 1, Y: minBuildY, Z: 1},
		{X: 27, Y: 72, Z: 9},
		{X: 729, Y: 200, Z: 243},
	}

	for _, positive := range positions {
		expected := generated.BlockAt(0, positive)

		variants := []game.BlockPosition{
			{X: -positive.X, Y: positive.Y, Z: positive.Z},
			{X: positive.X, Y: positive.Y, Z: -positive.Z},
			{X: -positive.X, Y: positive.Y, Z: -positive.Z},
		}

		for _, variant := range variants {
			variantBlock := generated.BlockAt(0, variant)
			if variantBlock != expected {
				t.Errorf(
					"block at %+v = %d, want %d to match %+v",
					variant,
					variantBlock,
					expected,
					positive,
				)
			}
		}
	}
}

func TestGeneratorStopsAtBuildHeight(t *testing.T) {
	generated := Generator{}

	positions := []game.BlockPosition{
		{X: 0, Y: minBuildY - 1, Z: 0},
		{X: 0, Y: maxBuildY + 1, Z: 0},
	}

	for _, position := range positions {
		block := generated.BlockAt(0, position)
		if block != game.Air {
			t.Errorf("block outside build height at %+v = %d, want air", position, block)
		}
	}
}

func TestPreparedChunkAndSectionGeneratorMatchBlockAt(t *testing.T) {
	generated := Generator{}

	chunks := []game.ChunkPosition{
		{X: 0, Z: 0},
		{X: -7, Z: 11},
		{X: 29, Z: -18},
		{X: 134217727, Z: -134217728},
	}

	sections := []int32{-80, -72, -64, 64, 304, 312, 320}

	for _, chunk := range chunks {
		prepared := generated.GenerateChunk(42, chunk)

		for _, sectionMinY := range sections {
			var preparedBlocks [game.SectionVolume]game.Block

			preparedBlock, preparedUniform := prepared.GenerateSection(sectionMinY, &preparedBlocks)

			var sectionBlocks [game.SectionVolume]game.Block

			sectionBlock, sectionUniform := generated.GenerateSection(42, chunk, sectionMinY, &sectionBlocks)

			if preparedBlock != sectionBlock || preparedUniform != sectionUniform || preparedBlocks != sectionBlocks {
				t.Fatalf("chunk %+v section %d prepared result differs from SectionGenerator", chunk, sectionMinY)
			}

			for localY := range int32(game.ChunkWidth) {
				for localZ := range int32(game.ChunkWidth) {
					for localX := range int32(game.ChunkWidth) {
						position := game.BlockPosition{
							X: chunk.X*game.ChunkWidth + localX,
							Y: sectionMinY + localY,
							Z: chunk.Z*game.ChunkWidth + localZ,
						}

						index := localY*256 + localZ*16 + localX
						block := preparedBlocks[index]

						if preparedUniform {
							block = preparedBlock
						}

						want := generated.BlockAt(42, position)
						if block != want {
							t.Fatalf("chunk %+v section %d position %+v = %d, want %d", chunk, sectionMinY, position, block, want)
						}
					}
				}
			}
		}
	}
}

func TestPreparedChunkGenerationIsDeterministicAndConcurrent(t *testing.T) {
	generated := Generator{}

	chunk := game.ChunkPosition{X: -7, Z: 11}

	sections := [...]int32{-80, -72, -64, 64, 304, 312, 320}

	want := snapshotGeneratedChunk(generated.GenerateChunk(42, chunk), sections)

	for range 10 {
		got := snapshotGeneratedChunk(generated.GenerateChunk(42, chunk), sections)
		if !reflect.DeepEqual(got, want) {
			t.Fatal("repeated chunk preparation produced different sections")
		}
	}

	prepared := generated.GenerateChunk(42, chunk)

	results := make(chan generatedChunkSnapshot, 16)

	var group sync.WaitGroup

	for index := range cap(results) {
		group.Add(1)

		go func(index int) {
			defer group.Done()

			if index%2 == 0 {
				results <- snapshotGeneratedChunk(prepared, sections)

				return
			}

			results <- snapshotGeneratedChunk(generated.GenerateChunk(42, chunk), sections)
		}(index)
	}

	group.Wait()
	close(results)

	for got := range results {
		if !reflect.DeepEqual(got, want) {
			t.Fatal("concurrent chunk generation produced different sections")
		}
	}
}

func snapshotGeneratedChunk(generated game.GeneratedChunk, sections [7]int32) generatedChunkSnapshot {
	var snapshot generatedChunkSnapshot

	for index, sectionMinY := range sections {
		block, uniform := generated.GenerateSection(sectionMinY, &snapshot.sections[index])
		if uniform {
			for blockIndex := range snapshot.sections[index] {
				snapshot.sections[index][blockIndex] = block
			}
		}
	}

	return snapshot
}
