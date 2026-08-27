package maze

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

func TestGeneratorIsRegistered(t *testing.T) {
	registered, err := generator.New(Name)
	if err != nil {
		t.Fatalf("create registered generator: %v", err)
	}

	if _, ok := registered.(Generator); !ok {
		t.Fatalf("registered generator type = %T", registered)
	}
}

func TestSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}

	for _, seed := range []int64{-123456789, -1, 0, 1, 123456789} {
		spawn := generated.Spawn(seed)

		position := game.BlockPosition{X: int32(spawn.X), Y: int32(spawn.Y), Z: int32(spawn.Z)}

		if block := generated.BlockAt(seed, position); block != game.Air {
			t.Fatalf("seed %d spawn block = %d, want air", seed, block)
		}

		position.Y--
		if block := generated.BlockAt(seed, position); block != game.SmoothStone {
			t.Fatalf("seed %d block below spawn = %d, want smooth stone", seed, block)
		}
	}
}

func TestEveryCellHasExactlyOnePreferredExit(t *testing.T) {
	generated := Generator{}

	for _, seed := range []int64{-17, 0, 1, 42, 999999} {
		mazeOrientation := orientationForSeed(seed)

		for cellZ := int32(-8); cellZ <= 8; cellZ++ {
			for cellX := int32(-8); cellX <= 8; cellX++ {
				horizontal := passagePositions(cellX, cellZ, mazeOrientation.horizontal)
				vertical := passagePositions(cellX, cellZ, mazeOrientation.vertical)

				horizontalOpen := passageIsOpen(generated, seed, horizontal)
				verticalOpen := passageIsOpen(generated, seed, vertical)

				if horizontalOpen == verticalOpen {
					t.Fatalf("seed %d cell (%d,%d) horizontal open=%t vertical open=%t", seed, cellX, cellZ, horizontalOpen, verticalOpen)
				}
			}
		}
	}
}

func TestWalkwaysAreTwoBlocksWide(t *testing.T) {
	generated := Generator{}

	if walkwayWidth != 2 {
		t.Fatalf("walkway width = %d, want 2", walkwayWidth)
	}

	for _, seed := range []int64{-17, 0, 1, 42, 999999} {
		for cellZ := int32(-8); cellZ <= 8; cellZ++ {
			for cellX := int32(-8); cellX <= 8; cellX++ {
				cellMinX := cellX * cellSize
				cellMinZ := cellZ * cellSize

				for offsetZ := int32(1); offsetZ <= walkwayWidth; offsetZ++ {
					for offsetX := int32(1); offsetX <= walkwayWidth; offsetX++ {
						position := game.BlockPosition{
							X: cellMinX + offsetX,
							Y: wallMinY,
							Z: cellMinZ + offsetZ,
						}

						if block := generated.BlockAt(seed, position); block != game.Air {
							t.Fatalf("seed %d cell (%d,%d) walkway block %+v = %d, want air", seed, cellX, cellZ, position, block)
						}
					}
				}
			}
		}
	}
}

func TestMazeWallsAreFourBlocksHigh(t *testing.T) {
	generated := Generator{}
	seed := int64(0)

	if wallHeight != 4 {
		t.Fatalf("wall height = %d, want 4", wallHeight)
	}

	wall := findClosedWall(seed)

	for offset := int32(0); offset < wallHeight; offset++ {
		wall.Y = wallMinY + offset
		if block := generated.BlockAt(seed, wall); block != game.StoneBricks {
			t.Fatalf("wall at y=%d = %d, want stone bricks", wall.Y, block)
		}
	}

	wall.Y = wallMinY + wallHeight
	if block := generated.BlockAt(seed, wall); block != game.Air {
		t.Fatalf("block above wall = %d, want air", block)
	}
}

func TestDifferentSeedsChangeLayout(t *testing.T) {
	changed := false

	for worldZ := int32(-32); worldZ <= 32 && !changed; worldZ++ {
		for worldX := int32(-32); worldX <= 32; worldX++ {
			if isWall(0, worldX, worldZ) != isWall(1, worldX, worldZ) {
				changed = true
				break
			}
		}
	}

	if !changed {
		t.Fatal("different seeds produced identical sampled maze layouts")
	}
}

func TestGenerateSectionMatchesBlockAt(t *testing.T) {
	generated := Generator{}
	seeds := []int64{-1, 0, 1234567}
	chunks := []game.ChunkPosition{{X: -2, Z: -1}, {X: 0, Z: 0}, {X: 3, Z: 2}}
	sectionYValues := []int32{32, 48, 64, 80}

	for _, seed := range seeds {
		for _, chunk := range chunks {
			for _, sectionMinY := range sectionYValues {
				var blocks [game.SectionVolume]game.Block
				uniformBlock, uniform := generated.GenerateSection(seed, chunk, sectionMinY, &blocks)

				for localY := range int32(game.ChunkWidth) {
					for localZ := range int32(game.ChunkWidth) {
						for localX := range int32(game.ChunkWidth) {
							position := game.BlockPosition{
								X: chunk.X*game.ChunkWidth + localX,
								Y: sectionMinY + localY,
								Z: chunk.Z*game.ChunkWidth + localZ,
							}

							want := generated.BlockAt(seed, position)
							got := uniformBlock
							if !uniform {
								got = blocks[localY*256+localZ*16+localX]
							}

							if got != want {
								t.Fatalf("seed %d chunk %+v section %d block %+v = %d, want %d", seed, chunk, sectionMinY, position, got, want)
							}
						}
					}
				}
			}
		}
	}
}

func TestGenerationBounds(t *testing.T) {
	generated := Generator{}
	minY, maxY, ok := generated.GenerationBounds(0, game.ChunkPosition{})

	if !ok {
		t.Fatal("generation bounds reported empty chunk")
	}

	if minY != floorY || maxY != wallMaxY {
		t.Fatalf("generation bounds = %d..%d, want %d..%d", minY, maxY, floorY, wallMaxY)
	}
}

func passagePositions(cellX, cellZ int32, passageDirection direction) [2]game.BlockPosition {
	cellMinX := cellX * cellSize
	cellMinZ := cellZ * cellSize

	positions := [2]game.BlockPosition{
		{X: cellMinX + passageStart, Y: wallMinY, Z: cellMinZ + passageStart},
		{X: cellMinX + passageEnd, Y: wallMinY, Z: cellMinZ + passageEnd},
	}

	switch passageDirection {
	case directionNorth:
		positions[0].Z = cellMinZ
		positions[1].Z = cellMinZ
	case directionEast:
		positions[0].X = cellMinX + cellSize
		positions[1].X = cellMinX + cellSize
	case directionSouth:
		positions[0].Z = cellMinZ + cellSize
		positions[1].Z = cellMinZ + cellSize
	case directionWest:
		positions[0].X = cellMinX
		positions[1].X = cellMinX
	}

	return positions
}

func passageIsOpen(generated Generator, seed int64, positions [2]game.BlockPosition) bool {
	for _, position := range positions {
		if generated.BlockAt(seed, position) != game.Air {
			return false
		}
	}

	return true
}

func findClosedWall(seed int64) game.BlockPosition {
	for worldZ := int32(-16); worldZ <= 16; worldZ++ {
		for worldX := int32(-16); worldX <= 16; worldX++ {
			if isWall(seed, worldX, worldZ) {
				return game.BlockPosition{X: worldX, Y: wallMinY, Z: worldZ}
			}
		}
	}

	panic("maze sample contains no wall")
}
